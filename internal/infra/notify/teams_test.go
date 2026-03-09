package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

func TestTeamsNotifier_InterfaceCompliance(_ *testing.T) {
	// Compile-time interface check
	var _ domain.Notifier = (*TeamsNotifier)(nil)

	// Runtime check
	notifier := NewTeamsNotifier("https://example.webhook.office.com/webhook")
	var _ domain.Notifier = notifier
}

func TestNewTeamsNotifier(t *testing.T) {
	tests := []struct {
		name       string
		webhookURL string
		wantURL    string
	}{
		{
			name:       "creates notifier with webhook URL",
			webhookURL: "https://example.webhook.office.com/webhook/xxx",
			wantURL:    "https://example.webhook.office.com/webhook/xxx",
		},
		{
			name:       "creates notifier with empty URL",
			webhookURL: "",
			wantURL:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := NewTeamsNotifier(tt.webhookURL)
			if notifier == nil {
				t.Fatal("NewTeamsNotifier() returned nil")
			}
			if notifier.webhookURL != tt.wantURL {
				t.Errorf("webhookURL = %v, want %v", notifier.webhookURL, tt.wantURL)
			}
			if notifier.client == nil {
				t.Error("client is nil")
			}
		})
	}
}

func TestWithTeamsHTTPClient(t *testing.T) {
	customClient := &http.Client{Timeout: 30 * time.Second}

	notifier := NewTeamsNotifier("https://example.webhook.office.com/webhook", WithTeamsHTTPClient(customClient))

	if notifier.client != customClient {
		t.Error("WithTeamsHTTPClient() did not set custom client")
	}
}

func TestTeamsNotifier_buildTaggedCard(t *testing.T) {
	expDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	timestamp := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	event := domain.NotificationEvent{
		Type:           domain.NotificationTypeTagged,
		AccountID:      "123456789012",
		AccountName:    "dev-account",
		Region:         "us-east-1",
		Timestamp:      timestamp,
		ExpirationDate: &expDate,
		Resources: []domain.Resource{
			{ID: "i-0abc123", Type: domain.ResourceTypeAWSEC2, Name: "web-server"},
			{ID: "vol-0def456", Type: domain.ResourceTypeAWSEBS, Name: ""},
		},
	}

	notifier := NewTeamsNotifier("https://example.webhook.office.com/webhook")
	card := notifier.buildTaggedCard(event)

	// Verify card structure
	if card.Type != "MessageCard" {
		t.Errorf("Type = %v, want MessageCard", card.Type)
	}
	if card.Context != "http://schema.org/extensions" {
		t.Errorf("Context = %v, want http://schema.org/extensions", card.Context)
	}
	if card.ThemeColor != "FFA500" {
		t.Errorf("ThemeColor = %v, want FFA500 (orange)", card.ThemeColor)
	}
	if card.Summary == "" {
		t.Error("Summary should not be empty")
	}
	if len(card.Sections) == 0 {
		t.Error("Sections should not be empty")
	}

	// Verify sections contain expected data
	foundAccount := false
	foundRegion := false
	foundResources := false
	for _, section := range card.Sections {
		for _, fact := range section.Facts {
			switch fact.Name {
			case "Account":
				foundAccount = true
				if fact.Value != "dev-account" {
					t.Errorf("Account = %v, want dev-account", fact.Value)
				}
			case "Region":
				foundRegion = true
				if fact.Value != "us-east-1" {
					t.Errorf("Region = %v, want us-east-1", fact.Value)
				}
			case "Resources":
				foundResources = true
			}
		}
	}
	if !foundAccount {
		t.Error("Account fact not found in card")
	}
	if !foundRegion {
		t.Error("Region fact not found in card")
	}
	if !foundResources {
		t.Error("Resources fact not found in card")
	}
}

func TestTeamsNotifier_buildTaggedCard_FallbackToAccountID(t *testing.T) {
	expDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	timestamp := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	event := domain.NotificationEvent{
		Type:           domain.NotificationTypeTagged,
		AccountID:      "123456789012",
		AccountName:    "", // Empty account name
		Region:         "us-east-1",
		Timestamp:      timestamp,
		ExpirationDate: &expDate,
		Resources: []domain.Resource{
			{ID: "i-0abc123", Type: domain.ResourceTypeAWSEC2, Name: "web-server"},
		},
	}

	notifier := NewTeamsNotifier("https://example.webhook.office.com/webhook")
	card := notifier.buildTaggedCard(event)

	// Verify account falls back to AccountID
	for _, section := range card.Sections {
		for _, fact := range section.Facts {
			if fact.Name == "Account" && fact.Value != "123456789012" {
				t.Errorf("Account should fallback to ID, got %v", fact.Value)
			}
		}
	}
}

func TestTeamsNotifier_buildDeletedCard(t *testing.T) {
	timestamp := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	event := domain.NotificationEvent{
		Type:        domain.NotificationTypeDeleted,
		AccountID:   "123456789012",
		AccountName: "dev-account",
		Region:      "eu-west-1",
		Timestamp:   timestamp,
		Resources: []domain.Resource{
			{ID: "i-expired123", Type: domain.ResourceTypeAWSEC2, Name: "old-server"},
		},
	}

	notifier := NewTeamsNotifier("https://example.webhook.office.com/webhook")
	card := notifier.buildDeletedCard(event)

	// Verify card structure
	if card.Type != "MessageCard" {
		t.Errorf("Type = %v, want MessageCard", card.Type)
	}
	if card.ThemeColor != "FF0000" {
		t.Errorf("ThemeColor = %v, want FF0000 (red)", card.ThemeColor)
	}
	if len(card.Sections) == 0 {
		t.Error("Sections should not be empty")
	}
}

func TestTeamsNotifier_buildDeletedCard_FallbackToAccountID(t *testing.T) {
	timestamp := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	event := domain.NotificationEvent{
		Type:        domain.NotificationTypeDeleted,
		AccountID:   "987654321098",
		AccountName: "", // Empty account name
		Region:      "eu-west-1",
		Timestamp:   timestamp,
		Resources: []domain.Resource{
			{ID: "i-expired123", Type: domain.ResourceTypeAWSEC2, Name: "old-server"},
		},
	}

	notifier := NewTeamsNotifier("https://example.webhook.office.com/webhook")
	card := notifier.buildDeletedCard(event)

	// Verify account falls back to AccountID
	for _, section := range card.Sections {
		for _, fact := range section.Facts {
			if fact.Name == "Account" && fact.Value != "987654321098" {
				t.Errorf("Account should fallback to ID, got %v", fact.Value)
			}
		}
	}
}

func TestTeamsNotifier_formatResourceList(t *testing.T) {
	notifier := NewTeamsNotifier("https://example.webhook.office.com/webhook")

	tests := []struct {
		name      string
		resources []domain.Resource
		wantParts []string
	}{
		{
			name: "formats resources with names",
			resources: []domain.Resource{
				{ID: "i-abc123", Type: domain.ResourceTypeAWSEC2, Name: "web-server"},
				{ID: "vol-def456", Type: domain.ResourceTypeAWSEBS, Name: "data-volume"},
			},
			wantParts: []string{"aws:ec2", "i-abc123", "web-server", "aws:ebs", "vol-def456", "data-volume"},
		},
		{
			name: "formats resources without names",
			resources: []domain.Resource{
				{ID: "i-abc123", Type: domain.ResourceTypeAWSEC2, Name: ""},
				{ID: "vol-def456", Type: domain.ResourceTypeAWSEBS, Name: ""},
			},
			wantParts: []string{"aws:ec2", "i-abc123", "aws:ebs", "vol-def456"},
		},
		{
			name:      "handles empty resources",
			resources: []domain.Resource{},
			wantParts: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := notifier.formatResourceList(tt.resources)
			for _, part := range tt.wantParts {
				if !containsSubstring(result, part) {
					t.Errorf("formatResourceList() missing %q in result: %s", part, result)
				}
			}
		})
	}
}

func TestTeamsNotifier_NotifyTagged(t *testing.T) {
	expDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	timestamp := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	event := domain.NotificationEvent{
		Type:           domain.NotificationTypeTagged,
		AccountID:      "123456789012",
		AccountName:    "dev-account",
		Region:         "us-east-1",
		Timestamp:      timestamp,
		ExpirationDate: &expDate,
		Resources: []domain.Resource{
			{ID: "i-0abc123", Type: domain.ResourceTypeAWSEC2, Name: "web-server"},
		},
	}

	// Create test server
	var receivedCard messageCard
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		if err := json.NewDecoder(r.Body).Decode(&receivedCard); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewTeamsNotifier(server.URL)
	err := notifier.NotifyTagged(context.Background(), event)

	if err != nil {
		t.Fatalf("NotifyTagged() error = %v", err)
	}

	// Verify the card was sent correctly
	if receivedCard.Type != "MessageCard" {
		t.Errorf("received card Type = %v, want MessageCard", receivedCard.Type)
	}
	if receivedCard.ThemeColor != "FFA500" {
		t.Errorf("received card ThemeColor = %v, want FFA500", receivedCard.ThemeColor)
	}
}

func TestTeamsNotifier_NotifyDeleted(t *testing.T) {
	timestamp := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	event := domain.NotificationEvent{
		Type:        domain.NotificationTypeDeleted,
		AccountID:   "123456789012",
		AccountName: "dev-account",
		Region:      "us-east-1",
		Timestamp:   timestamp,
		Resources: []domain.Resource{
			{ID: "i-expired123", Type: domain.ResourceTypeAWSEC2, Name: "old-server"},
		},
	}

	// Create test server
	var receivedCard messageCard
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedCard); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewTeamsNotifier(server.URL)
	err := notifier.NotifyDeleted(context.Background(), event)

	if err != nil {
		t.Fatalf("NotifyDeleted() error = %v", err)
	}

	// Verify the card was sent correctly
	if receivedCard.ThemeColor != "FF0000" {
		t.Errorf("received card ThemeColor = %v, want FF0000", receivedCard.ThemeColor)
	}
}

func TestTeamsNotifier_send_HTTPError(t *testing.T) {
	// Create test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	notifier := NewTeamsNotifier(server.URL)

	event := domain.NotificationEvent{
		Type:      domain.NotificationTypeTagged,
		AccountID: "123",
		Region:    "us-east-1",
		Timestamp: time.Now(),
		Resources: []domain.Resource{},
	}

	err := notifier.NotifyTagged(context.Background(), event)
	if err == nil {
		t.Error("expected error for HTTP 500 response, got nil")
	}
}

func TestTeamsNotifier_send_NetworkError(t *testing.T) {
	// Use an invalid URL to trigger network error
	notifier := NewTeamsNotifier("http://localhost:99999/invalid")

	event := domain.NotificationEvent{
		Type:      domain.NotificationTypeTagged,
		AccountID: "123",
		Region:    "us-east-1",
		Timestamp: time.Now(),
		Resources: []domain.Resource{},
	}

	err := notifier.NotifyTagged(context.Background(), event)
	if err == nil {
		t.Error("expected error for network failure, got nil")
	}
}

func TestTeamsNotifier_send_ContextCancellation(t *testing.T) {
	// Create a server that blocks
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewTeamsNotifier(server.URL)

	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	event := domain.NotificationEvent{
		Type:      domain.NotificationTypeTagged,
		AccountID: "123",
		Region:    "us-east-1",
		Timestamp: time.Now(),
		Resources: []domain.Resource{},
	}

	err := notifier.NotifyTagged(ctx, event)
	if err == nil {
		t.Error("expected error for canceled context, got nil")
	}
}

// containsSubstring checks if s contains substr.
func containsSubstring(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && searchSubstring(s, substr))
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
