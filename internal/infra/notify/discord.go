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
var _ domain.Notifier = (*DiscordNotifier)(nil)

// DiscordNotifier sends notifications to Discord via webhooks.
type DiscordNotifier struct {
	webhookURL string
	client     *http.Client
}

// NewDiscordNotifier creates a new DiscordNotifier.
func NewDiscordNotifier(webhookURL string, opts ...DiscordOption) *DiscordNotifier {
	n := &DiscordNotifier{
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

// DiscordOption configures a DiscordNotifier.
type DiscordOption func(*DiscordNotifier)

// WithDiscordHTTPClient sets a custom HTTP client.
func WithDiscordHTTPClient(client *http.Client) DiscordOption {
	return func(n *DiscordNotifier) {
		n.client = client
	}
}

// NotifyTagged sends a notification about tagged resources.
func (n *DiscordNotifier) NotifyTagged(ctx context.Context, event domain.NotificationEvent) error {
	msg := n.buildTaggedMessage(event)
	return n.send(ctx, msg)
}

// NotifyDeleted sends a notification about deleted resources.
func (n *DiscordNotifier) NotifyDeleted(ctx context.Context, event domain.NotificationEvent) error {
	msg := n.buildDeletedMessage(event)
	return n.send(ctx, msg)
}

func (n *DiscordNotifier) buildTaggedMessage(event domain.NotificationEvent) discordMessage {
	var expDate string
	if event.ExpirationDate != nil {
		expDate = event.ExpirationDate.Format("2006-01-02")
	}

	accountName := event.AccountName
	if accountName == "" {
		accountName = event.AccountID
	}

	embed := discordEmbed{
		Title:       "Cloud Janitor: Resources Tagged for Expiration",
		Description: fmt.Sprintf("The following resources have been tagged with expiration date **%s**", expDate),
		Color:       0xFFA500, // Orange
		Fields: []discordField{
			{Name: "Account", Value: accountName, Inline: true},
			{Name: "Region", Value: event.Region, Inline: true},
			{Name: "Resources", Value: n.formatResourceList(event.Resources), Inline: false},
		},
		Footer: discordFooter{
			Text: fmt.Sprintf("These resources will be automatically deleted on %s. Update the expiration-date tag to keep them.", expDate),
		},
		Timestamp: event.Timestamp.Format(time.RFC3339),
	}

	return discordMessage{Embeds: []discordEmbed{embed}}
}

func (n *DiscordNotifier) buildDeletedMessage(event domain.NotificationEvent) discordMessage {
	accountName := event.AccountName
	if accountName == "" {
		accountName = event.AccountID
	}

	embed := discordEmbed{
		Title:       "Cloud Janitor: Resources Deleted",
		Description: "The following expired resources have been deleted",
		Color:       0xFF0000, // Red
		Fields: []discordField{
			{Name: "Account", Value: accountName, Inline: true},
			{Name: "Region", Value: event.Region, Inline: true},
			{Name: "Resources", Value: n.formatResourceList(event.Resources), Inline: false},
		},
		Timestamp: event.Timestamp.Format(time.RFC3339),
	}

	return discordMessage{Embeds: []discordEmbed{embed}}
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

func (n *DiscordNotifier) send(ctx context.Context, msg discordMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling discord message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating discord request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending discord message: %w", err)
	}
	defer resp.Body.Close()

	// Discord returns 204 No Content on success
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("discord returned status %d", resp.StatusCode)
	}

	return nil
}

type discordMessage struct {
	Embeds []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Color       int            `json:"color"`
	Fields      []discordField `json:"fields"`
	Footer      discordFooter  `json:"footer,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
}

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type discordFooter struct {
	Text string `json:"text"`
}
