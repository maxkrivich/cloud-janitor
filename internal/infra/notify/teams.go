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
var _ domain.Notifier = (*TeamsNotifier)(nil)

// Theme colors for Microsoft Teams message cards.
const (
	teamsColorOrange = "FFA500" // Tagged resources
	teamsColorRed    = "FF0000" // Deleted resources
)

// TeamsNotifier sends notifications to Microsoft Teams using incoming webhooks.
type TeamsNotifier struct {
	webhookURL string
	client     *http.Client
}

// TeamsOption configures a TeamsNotifier.
type TeamsOption func(*TeamsNotifier)

// WithTeamsHTTPClient sets a custom HTTP client.
func WithTeamsHTTPClient(client *http.Client) TeamsOption {
	return func(n *TeamsNotifier) {
		n.client = client
	}
}

// NewTeamsNotifier creates a new TeamsNotifier with the given webhook URL.
func NewTeamsNotifier(webhookURL string, opts ...TeamsOption) *TeamsNotifier {
	n := &TeamsNotifier{
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

// NotifyTagged sends a notification about tagged resources.
func (n *TeamsNotifier) NotifyTagged(ctx context.Context, event domain.NotificationEvent) error {
	card := n.buildTaggedCard(event)
	return n.send(ctx, card)
}

// NotifyDeleted sends a notification about deleted resources.
func (n *TeamsNotifier) NotifyDeleted(ctx context.Context, event domain.NotificationEvent) error {
	card := n.buildDeletedCard(event)
	return n.send(ctx, card)
}

// messageCard represents a Microsoft Teams MessageCard.
// See: https://learn.microsoft.com/en-us/outlook/actionable-messages/message-card-reference
type messageCard struct {
	Type       string           `json:"@type"`
	Context    string           `json:"@context"`
	ThemeColor string           `json:"themeColor"`
	Summary    string           `json:"summary"`
	Sections   []messageSection `json:"sections"`
}

// messageSection represents a section in a MessageCard.
type messageSection struct {
	ActivityTitle    string        `json:"activityTitle,omitempty"`
	ActivitySubtitle string        `json:"activitySubtitle,omitempty"`
	ActivityImage    string        `json:"activityImage,omitempty"`
	Facts            []messageFact `json:"facts,omitempty"`
	Markdown         bool          `json:"markdown"`
	Text             string        `json:"text,omitempty"`
}

// messageFact represents a key-value fact in a MessageCard section.
type messageFact struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func (n *TeamsNotifier) buildTaggedCard(event domain.NotificationEvent) messageCard {
	var expDate string
	if event.ExpirationDate != nil {
		expDate = event.ExpirationDate.Format("2006-01-02")
	}

	accountName := event.AccountName
	if accountName == "" {
		accountName = event.AccountID
	}

	return messageCard{
		Type:       "MessageCard",
		Context:    "http://schema.org/extensions",
		ThemeColor: teamsColorOrange,
		Summary:    fmt.Sprintf("Cloud Janitor: %d resources tagged for expiration", len(event.Resources)),
		Sections: []messageSection{
			{
				ActivityTitle:    "🏷️ Cloud Janitor: Resources Tagged for Expiration",
				ActivitySubtitle: fmt.Sprintf("Resources tagged with expiration date **%s**", expDate),
				Markdown:         true,
			},
			{
				Facts: []messageFact{
					{Name: "Account", Value: accountName},
					{Name: "Region", Value: event.Region},
					{Name: "Resources", Value: n.formatResourceList(event.Resources)},
				},
				Markdown: true,
			},
			{
				Text:     fmt.Sprintf("⚠️ These resources will be automatically deleted on **%s**. Update the `expiration-date` tag to keep them.", expDate),
				Markdown: true,
			},
		},
	}
}

func (n *TeamsNotifier) buildDeletedCard(event domain.NotificationEvent) messageCard {
	accountName := event.AccountName
	if accountName == "" {
		accountName = event.AccountID
	}

	return messageCard{
		Type:       "MessageCard",
		Context:    "http://schema.org/extensions",
		ThemeColor: teamsColorRed,
		Summary:    fmt.Sprintf("Cloud Janitor: %d expired resources deleted", len(event.Resources)),
		Sections: []messageSection{
			{
				ActivityTitle:    "🗑️ Cloud Janitor: Resources Deleted",
				ActivitySubtitle: "The following expired resources have been deleted",
				Markdown:         true,
			},
			{
				Facts: []messageFact{
					{Name: "Account", Value: accountName},
					{Name: "Region", Value: event.Region},
					{Name: "Resources", Value: n.formatResourceList(event.Resources)},
				},
				Markdown: true,
			},
		},
	}
}

func (n *TeamsNotifier) formatResourceList(resources []domain.Resource) string {
	if len(resources) == 0 {
		return "None"
	}

	var sb strings.Builder
	for i, r := range resources {
		if i > 0 {
			sb.WriteString("<br>")
		}
		if r.Name != "" {
			sb.WriteString(fmt.Sprintf("`%s`: %s (%s)", r.Type, r.ID, r.Name))
		} else {
			sb.WriteString(fmt.Sprintf("`%s`: %s", r.Type, r.ID))
		}
	}
	return sb.String()
}

func (n *TeamsNotifier) send(ctx context.Context, card messageCard) error {
	body, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("marshaling teams message card: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating teams webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending teams webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("teams webhook returned status %d", resp.StatusCode)
	}

	return nil
}
