// Package notify provides notification adapters for various services.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check.
var _ domain.Notifier = (*SlackNotifier)(nil)

// SlackNotifier sends notifications to Slack via webhooks.
type SlackNotifier struct {
	webhookURL string
	channel    string
	client     *http.Client
}

// NewSlackNotifier creates a new SlackNotifier.
func NewSlackNotifier(webhookURL string, opts ...SlackOption) *SlackNotifier {
	n := &SlackNotifier{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(n)
	}
	return n
}

// SlackOption configures a SlackNotifier.
type SlackOption func(*SlackNotifier)

// WithSlackChannel sets the channel override.
func WithSlackChannel(channel string) SlackOption {
	return func(n *SlackNotifier) {
		n.channel = channel
	}
}

// WithSlackHTTPClient sets a custom HTTP client.
func WithSlackHTTPClient(client *http.Client) SlackOption {
	return func(n *SlackNotifier) {
		n.client = client
	}
}

// NotifyTagged sends a notification about tagged resources.
func (n *SlackNotifier) NotifyTagged(ctx context.Context, event domain.NotificationEvent) error {
	msg := n.buildTaggedMessage(event)
	return n.send(ctx, msg)
}

// NotifyDeleted sends a notification about deleted resources.
func (n *SlackNotifier) NotifyDeleted(ctx context.Context, event domain.NotificationEvent) error {
	msg := n.buildDeletedMessage(event)
	return n.send(ctx, msg)
}

func (n *SlackNotifier) buildTaggedMessage(event domain.NotificationEvent) slackMessage {
	var expDate string
	if event.ExpirationDate != nil {
		expDate = event.ExpirationDate.Format("2006-01-02")
	}

	accountName := event.AccountName
	if accountName == "" {
		accountName = event.AccountID
	}

	text := fmt.Sprintf(":label: *Cloud Janitor: Resources Tagged for Expiration*\n\n"+
		"*Account:* %s\n"+
		"*Region:* %s\n\n"+
		"The following resources have been tagged with expiration date *%s*:\n\n",
		accountName, event.Region, expDate)

	text += n.formatResourceTable(event.Resources)

	text += fmt.Sprintf("\n:warning: These resources will be automatically deleted on *%s*.\n\n"+
		"To keep a resource, update its `expiration-date` tag to a future date or `never`.", expDate)

	msg := slackMessage{
		Text: text,
	}
	if n.channel != "" {
		msg.Channel = n.channel
	}

	return msg
}

func (n *SlackNotifier) buildDeletedMessage(event domain.NotificationEvent) slackMessage {
	accountName := event.AccountName
	if accountName == "" {
		accountName = event.AccountID
	}

	text := fmt.Sprintf(":wastebasket: *Cloud Janitor: Resources Deleted*\n\n"+
		"*Account:* %s\n"+
		"*Region:* %s\n\n"+
		"The following expired resources have been deleted:\n\n",
		accountName, event.Region)

	text += n.formatResourceTable(event.Resources)

	msg := slackMessage{
		Text: text,
	}
	if n.channel != "" {
		msg.Channel = n.channel
	}

	return msg
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
		sb.WriteString(fmt.Sprintf("%-12s | %-24s | %s\n", r.Type, r.ID, name))
	}
	sb.WriteString("```\n")
	return sb.String()
}

func (n *SlackNotifier) send(ctx context.Context, msg slackMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling slack message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending slack message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}

	return nil
}

type slackMessage struct {
	Channel string `json:"channel,omitempty"`
	Text    string `json:"text"`
}
