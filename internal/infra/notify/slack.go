// Package notify provides notification adapters for various services.
package notify

import (
	"context"
	"fmt"
	"strings"

	"github.com/slack-go/slack"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check.
var _ domain.Notifier = (*SlackNotifier)(nil)

// slackMode represents the authentication mode for Slack.
type slackMode int

const (
	slackModeWebhook slackMode = iota + 1
	slackModeBot
	slackModeApp
)

// SlackNotifier sends notifications to Slack using slack-go/slack.
type SlackNotifier struct {
	client     *slack.Client
	webhookURL string
	channelID  string
	channel    string // Optional channel override
	mode       slackMode
	debug      bool
}

// SlackOption configures a SlackNotifier.
type SlackOption func(*SlackNotifier)

// WithSlackChannel sets the channel override.
func WithSlackChannel(channel string) SlackOption {
	return func(n *SlackNotifier) {
		n.channel = channel
	}
}

// WithSlackDebug enables debug logging.
func WithSlackDebug(debug bool) SlackOption {
	return func(n *SlackNotifier) {
		n.debug = debug
	}
}

// NewSlackNotifierWebhook creates a notifier using a webhook URL.
func NewSlackNotifierWebhook(webhookURL string, opts ...SlackOption) *SlackNotifier {
	n := &SlackNotifier{
		webhookURL: webhookURL,
		mode:       slackModeWebhook,
	}

	for _, opt := range opts {
		opt(n)
	}

	return n
}

// NewSlackNotifierBot creates a notifier using a bot token.
func NewSlackNotifierBot(botToken, channelID string, opts ...SlackOption) (*SlackNotifier, error) {
	if botToken == "" {
		return nil, fmt.Errorf("bot token is required")
	}
	if channelID == "" {
		return nil, fmt.Errorf("channel ID is required")
	}

	n := &SlackNotifier{
		channelID: channelID,
		mode:      slackModeBot,
	}

	for _, opt := range opts {
		opt(n)
	}

	clientOpts := []slack.Option{}
	if n.debug {
		clientOpts = append(clientOpts, slack.OptionDebug(true))
	}

	n.client = slack.New(botToken, clientOpts...)

	return n, nil
}

// NewSlackNotifierApp creates a notifier using an app token (Socket Mode ready).
func NewSlackNotifierApp(appToken, botToken, channelID string, opts ...SlackOption) (*SlackNotifier, error) {
	if appToken == "" {
		return nil, fmt.Errorf("app token is required")
	}
	if botToken == "" {
		return nil, fmt.Errorf("bot token is required")
	}
	if channelID == "" {
		return nil, fmt.Errorf("channel ID is required")
	}

	n := &SlackNotifier{
		channelID: channelID,
		mode:      slackModeApp,
	}

	for _, opt := range opts {
		opt(n)
	}

	clientOpts := []slack.Option{
		slack.OptionAppLevelToken(appToken),
	}
	if n.debug {
		clientOpts = append(clientOpts, slack.OptionDebug(true))
	}

	n.client = slack.New(botToken, clientOpts...)

	return n, nil
}

// NotifyTagged sends a notification about tagged resources.
func (n *SlackNotifier) NotifyTagged(ctx context.Context, event domain.NotificationEvent) error {
	blocks := n.buildTaggedBlocks(event)
	return n.send(ctx, blocks, n.buildTaggedFallbackText(event))
}

// NotifyDeleted sends a notification about deleted resources.
func (n *SlackNotifier) NotifyDeleted(ctx context.Context, event domain.NotificationEvent) error {
	blocks := n.buildDeletedBlocks(event)
	return n.send(ctx, blocks, n.buildDeletedFallbackText(event))
}

func (n *SlackNotifier) buildTaggedBlocks(event domain.NotificationEvent) []slack.Block {
	var expDate string
	if event.ExpirationDate != nil {
		expDate = event.ExpirationDate.Format("2006-01-02")
	}

	accountName := event.AccountName
	if accountName == "" {
		accountName = event.AccountID
	}

	blocks := []slack.Block{
		// Header
		slack.NewHeaderBlock(
			slack.NewTextBlockObject(slack.PlainTextType, "🏷️ Cloud Janitor: Resources Tagged for Expiration", true, false),
		),

		// Context (Account & Region)
		slack.NewContextBlock(
			"context",
			slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Account:* %s  •  *Region:* %s", accountName, event.Region), false, false),
		),

		// Divider
		slack.NewDividerBlock(),

		// Section with expiration info
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType,
				fmt.Sprintf("The following resources have been tagged with expiration date *%s*:", expDate),
				false, false),
			nil, nil,
		),

		// Resource table
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, n.formatResourceTable(event.Resources), false, false),
			nil, nil,
		),

		// Divider
		slack.NewDividerBlock(),

		// Warning footer
		slack.NewContextBlock(
			"footer",
			slack.NewTextBlockObject(slack.MarkdownType,
				fmt.Sprintf("⚠️ These resources will be automatically deleted on *%s*. Update the `expiration-date` tag to keep them.", expDate),
				false, false),
		),
	}

	return blocks
}

func (n *SlackNotifier) buildDeletedBlocks(event domain.NotificationEvent) []slack.Block {
	accountName := event.AccountName
	if accountName == "" {
		accountName = event.AccountID
	}

	blocks := []slack.Block{
		// Header
		slack.NewHeaderBlock(
			slack.NewTextBlockObject(slack.PlainTextType, "🗑️ Cloud Janitor: Resources Deleted", true, false),
		),

		// Context (Account & Region)
		slack.NewContextBlock(
			"context",
			slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Account:* %s  •  *Region:* %s", accountName, event.Region), false, false),
		),

		// Divider
		slack.NewDividerBlock(),

		// Section with info
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType,
				"The following expired resources have been deleted:",
				false, false),
			nil, nil,
		),

		// Resource table
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, n.formatResourceTable(event.Resources), false, false),
			nil, nil,
		),
	}

	return blocks
}

func (n *SlackNotifier) formatResourceTable(resources []domain.Resource) string {
	var sb strings.Builder
	sb.WriteString("```\n")
	sb.WriteString(fmt.Sprintf("%-12s | %-24s | %s\n", "Type", "Resource ID", "Name"))
	sb.WriteString(fmt.Sprintf("%-12s-+-%-24s-+-%s\n", "------------", "------------------------", "--------------------"))

	for _, r := range resources {
		name := r.Name
		if len(name) > 20 {
			name = name[:17] + "..."
		}
		resourceType := string(r.Type)
		if len(resourceType) > 12 {
			resourceType = resourceType[:12]
		}
		resourceID := r.ID
		if len(resourceID) > 24 {
			resourceID = resourceID[:21] + "..."
		}
		sb.WriteString(fmt.Sprintf("%-12s | %-24s | %s\n", resourceType, resourceID, name))
	}
	sb.WriteString("```")
	return sb.String()
}

func (n *SlackNotifier) buildTaggedFallbackText(event domain.NotificationEvent) string {
	var expDate string
	if event.ExpirationDate != nil {
		expDate = event.ExpirationDate.Format("2006-01-02")
	}
	return fmt.Sprintf("Cloud Janitor: %d resources tagged for expiration on %s", len(event.Resources), expDate)
}

func (n *SlackNotifier) buildDeletedFallbackText(event domain.NotificationEvent) string {
	return fmt.Sprintf("Cloud Janitor: %d expired resources deleted", len(event.Resources))
}

func (n *SlackNotifier) send(ctx context.Context, blocks []slack.Block, fallbackText string) error {
	switch n.mode {
	case slackModeWebhook:
		return n.sendViaWebhook(ctx, blocks, fallbackText)
	case slackModeBot, slackModeApp:
		return n.sendViaClient(ctx, blocks, fallbackText)
	default:
		return fmt.Errorf("unknown slack mode: %d", n.mode)
	}
}

func (n *SlackNotifier) sendViaWebhook(_ context.Context, blocks []slack.Block, fallbackText string) error {
	msg := &slack.WebhookMessage{
		Text:   fallbackText,
		Blocks: &slack.Blocks{BlockSet: blocks},
	}

	if n.channel != "" {
		msg.Channel = n.channel
	}

	err := slack.PostWebhook(n.webhookURL, msg)
	if err != nil {
		return fmt.Errorf("posting slack webhook: %w", err)
	}

	return nil
}

func (n *SlackNotifier) sendViaClient(ctx context.Context, blocks []slack.Block, fallbackText string) error {
	channelID := n.channelID
	if n.channel != "" {
		channelID = n.channel
	}

	opts := []slack.MsgOption{
		slack.MsgOptionText(fallbackText, false),
		slack.MsgOptionBlocks(blocks...),
	}

	_, _, err := n.client.PostMessageContext(ctx, channelID, opts...)
	if err != nil {
		return fmt.Errorf("posting slack message: %w", err)
	}

	return nil
}
