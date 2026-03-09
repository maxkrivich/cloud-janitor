package notify

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check.
var _ domain.Notifier = (*DiscordNotifier)(nil)

// discordMode represents the authentication mode for Discord.
type discordMode int

const (
	discordModeWebhook discordMode = iota + 1
	discordModeBot
)

// DiscordNotifier sends notifications to Discord using discordgo.
type DiscordNotifier struct {
	session      *discordgo.Session
	webhookID    string
	webhookToken string
	channelID    string
	mode         discordMode
}

// DiscordOption configures a DiscordNotifier.
type DiscordOption func(*DiscordNotifier)

// WithDiscordHTTPClient sets a custom HTTP client.
func WithDiscordHTTPClient(client *http.Client) DiscordOption {
	return func(n *DiscordNotifier) {
		if n.session != nil {
			n.session.Client = client
		}
	}
}

// webhookURLRegex matches Discord webhook URLs and extracts ID and token.
// Format: https://discord.com/api/webhooks/{id}/{token}
var webhookURLRegex = regexp.MustCompile(`https://(?:discord\.com|discordapp\.com)/api/webhooks/(\d+)/([A-Za-z0-9_-]+)`)

// parseWebhookURL extracts the webhook ID and token from a Discord webhook URL.
func parseWebhookURL(webhookURL string) (id, token string, err error) {
	matches := webhookURLRegex.FindStringSubmatch(webhookURL)
	if matches == nil || len(matches) != 3 {
		return "", "", fmt.Errorf("invalid discord webhook URL format")
	}
	return matches[1], matches[2], nil
}

// NewDiscordNotifierWebhook creates a notifier using a webhook URL.
func NewDiscordNotifierWebhook(webhookURL string, opts ...DiscordOption) (*DiscordNotifier, error) {
	webhookID, webhookToken, err := parseWebhookURL(webhookURL)
	if err != nil {
		return nil, fmt.Errorf("parsing webhook URL: %w", err)
	}

	// Create a session without authentication (webhook-only)
	session, err := discordgo.New("")
	if err != nil {
		return nil, fmt.Errorf("creating discordgo session: %w", err)
	}

	n := &DiscordNotifier{
		session:      session,
		webhookID:    webhookID,
		webhookToken: webhookToken,
		mode:         discordModeWebhook,
	}

	for _, opt := range opts {
		opt(n)
	}

	return n, nil
}

// NewDiscordNotifierBot creates a notifier using a bot token.
func NewDiscordNotifierBot(botToken, channelID string, opts ...DiscordOption) (*DiscordNotifier, error) {
	if botToken == "" {
		return nil, fmt.Errorf("bot token is required")
	}
	if channelID == "" {
		return nil, fmt.Errorf("channel ID is required")
	}

	// Ensure bot token has "Bot " prefix
	if !strings.HasPrefix(botToken, "Bot ") {
		botToken = "Bot " + botToken
	}

	session, err := discordgo.New(botToken)
	if err != nil {
		return nil, fmt.Errorf("creating discordgo session: %w", err)
	}

	n := &DiscordNotifier{
		session:   session,
		channelID: channelID,
		mode:      discordModeBot,
	}

	for _, opt := range opts {
		opt(n)
	}

	return n, nil
}

// NotifyTagged sends a notification about tagged resources.
func (n *DiscordNotifier) NotifyTagged(ctx context.Context, event domain.NotificationEvent) error {
	embed := n.buildTaggedEmbed(event)
	return n.sendEmbed(ctx, embed)
}

// NotifyDeleted sends a notification about deleted resources.
func (n *DiscordNotifier) NotifyDeleted(ctx context.Context, event domain.NotificationEvent) error {
	embed := n.buildDeletedEmbed(event)
	return n.sendEmbed(ctx, embed)
}

func (n *DiscordNotifier) buildTaggedEmbed(event domain.NotificationEvent) *discordgo.MessageEmbed {
	var expDate string
	if event.ExpirationDate != nil {
		expDate = event.ExpirationDate.Format("2006-01-02")
	}

	accountName := event.AccountName
	if accountName == "" {
		accountName = event.AccountID
	}

	return &discordgo.MessageEmbed{
		Title:       "Cloud Janitor: Resources Tagged for Expiration",
		Description: fmt.Sprintf("The following resources have been tagged with expiration date **%s**", expDate),
		Color:       0xFFA500, // Orange
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Account", Value: accountName, Inline: true},
			{Name: "Region", Value: event.Region, Inline: true},
			{Name: "Resources", Value: n.formatResourceList(event.Resources), Inline: false},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("These resources will be automatically deleted on %s. Update the expiration-date tag to keep them.", expDate),
		},
		Timestamp: event.Timestamp.Format(time.RFC3339),
	}
}

func (n *DiscordNotifier) buildDeletedEmbed(event domain.NotificationEvent) *discordgo.MessageEmbed {
	accountName := event.AccountName
	if accountName == "" {
		accountName = event.AccountID
	}

	return &discordgo.MessageEmbed{
		Title:       "Cloud Janitor: Resources Deleted",
		Description: "The following expired resources have been deleted",
		Color:       0xFF0000, // Red
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Account", Value: accountName, Inline: true},
			{Name: "Region", Value: event.Region, Inline: true},
			{Name: "Resources", Value: n.formatResourceList(event.Resources), Inline: false},
		},
		Timestamp: event.Timestamp.Format(time.RFC3339),
	}
}

func (n *DiscordNotifier) formatResourceList(resources []domain.Resource) string {
	var sb strings.Builder
	sb.WriteString("```\n")
	for _, r := range resources {
		name := r.Name
		if name != "" {
			name = fmt.Sprintf(" (%s)", name)
		}
		sb.WriteString(fmt.Sprintf("%s: %s%s\n", r.Type, r.ID, name))
	}
	sb.WriteString("```")
	return sb.String()
}

func (n *DiscordNotifier) sendEmbed(ctx context.Context, embed *discordgo.MessageEmbed) error {
	switch n.mode {
	case discordModeWebhook:
		return n.sendViaWebhook(ctx, embed)
	case discordModeBot:
		return n.sendViaBot(ctx, embed)
	default:
		return fmt.Errorf("unknown discord mode: %d", n.mode)
	}
}

func (n *DiscordNotifier) sendViaWebhook(_ context.Context, embed *discordgo.MessageEmbed) error {
	params := &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{embed},
	}

	_, err := n.session.WebhookExecute(n.webhookID, n.webhookToken, false, params)
	if err != nil {
		return fmt.Errorf("executing discord webhook: %w", err)
	}

	return nil
}

func (n *DiscordNotifier) sendViaBot(_ context.Context, embed *discordgo.MessageEmbed) error {
	_, err := n.session.ChannelMessageSendEmbed(n.channelID, embed)
	if err != nil {
		return fmt.Errorf("sending discord message: %w", err)
	}

	return nil
}
