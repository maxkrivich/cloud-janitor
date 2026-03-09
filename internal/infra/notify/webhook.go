package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check.
var _ domain.Notifier = (*WebhookNotifier)(nil)

// WebhookNotifier sends notifications to a generic webhook endpoint.
type WebhookNotifier struct {
	url     string
	headers map[string]string
	client  *http.Client
}

// NewWebhookNotifier creates a new WebhookNotifier.
func NewWebhookNotifier(url string, opts ...WebhookOption) *WebhookNotifier {
	n := &WebhookNotifier{
		url:     url,
		headers: make(map[string]string),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(n)
	}
	return n
}

// WebhookOption configures a WebhookNotifier.
type WebhookOption func(*WebhookNotifier)

// WithWebhookHeaders sets custom headers for the webhook request.
func WithWebhookHeaders(headers map[string]string) WebhookOption {
	return func(n *WebhookNotifier) {
		n.headers = headers
	}
}

// WithWebhookHTTPClient sets a custom HTTP client.
func WithWebhookHTTPClient(client *http.Client) WebhookOption {
	return func(n *WebhookNotifier) {
		n.client = client
	}
}

// NotifyTagged sends a notification about tagged resources.
func (n *WebhookNotifier) NotifyTagged(ctx context.Context, event domain.NotificationEvent) error {
	payload := n.buildPayload("tagged", event)
	return n.send(ctx, payload)
}

// NotifyDeleted sends a notification about deleted resources.
func (n *WebhookNotifier) NotifyDeleted(ctx context.Context, event domain.NotificationEvent) error {
	payload := n.buildPayload("deleted", event)
	return n.send(ctx, payload)
}

func (n *WebhookNotifier) buildPayload(eventType string, event domain.NotificationEvent) webhookPayload {
	resources := make([]webhookResource, 0, len(event.Resources))
	for _, r := range event.Resources {
		resources = append(resources, webhookResource{
			ID:     r.ID,
			Type:   string(r.Type),
			Name:   r.Name,
			Region: r.Region,
			Tags:   r.Tags,
		})
	}

	payload := webhookPayload{
		Event:       eventType,
		AccountID:   event.AccountID,
		AccountName: event.AccountName,
		Region:      event.Region,
		Timestamp:   event.Timestamp.Format(time.RFC3339),
		Resources:   resources,
	}

	if event.ExpirationDate != nil {
		expDate := event.ExpirationDate.Format("2006-01-02")
		payload.ExpirationDate = &expDate
	}

	return payload
}

func (n *WebhookNotifier) send(ctx context.Context, payload webhookPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	for key, value := range n.headers {
		req.Header.Set(key, value)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

type webhookPayload struct {
	Event          string            `json:"event"`
	AccountID      string            `json:"account_id"`
	AccountName    string            `json:"account_name,omitempty"`
	Region         string            `json:"region"`
	Timestamp      string            `json:"timestamp"`
	ExpirationDate *string           `json:"expiration_date,omitempty"`
	Resources      []webhookResource `json:"resources"`
}

type webhookResource struct {
	ID     string            `json:"id"`
	Type   string            `json:"type"`
	Name   string            `json:"name,omitempty"`
	Region string            `json:"region"`
	Tags   map[string]string `json:"tags,omitempty"`
}
