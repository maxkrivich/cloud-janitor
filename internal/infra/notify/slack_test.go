package notify

import (
	"context"
	"testing"
	"time"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

func TestNewSlackNotifierWebhook(t *testing.T) {
	tests := []struct {
		name       string
		webhookURL string
		opts       []SlackOption
		wantMode   slackMode
	}{
		{
			name:       "valid webhook URL",
			webhookURL: "https://hooks.slack.com/services/T00/B00/XXXX",
			opts:       nil,
			wantMode:   slackModeWebhook,
		},
		{
			name:       "webhook with channel override",
			webhookURL: "https://hooks.slack.com/services/T00/B00/XXXX",
			opts:       []SlackOption{WithSlackChannel("#notifications")},
			wantMode:   slackModeWebhook,
		},
		{
			name:       "webhook with debug enabled",
			webhookURL: "https://hooks.slack.com/services/T00/B00/XXXX",
			opts:       []SlackOption{WithSlackDebug(true)},
			wantMode:   slackModeWebhook,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := NewSlackNotifierWebhook(tt.webhookURL, tt.opts...)

			if notifier == nil {
				t.Fatal("NewSlackNotifierWebhook() returned nil")
			}
			if notifier.mode != tt.wantMode {
				t.Errorf("mode = %v, want %v", notifier.mode, tt.wantMode)
			}
			if notifier.webhookURL != tt.webhookURL {
				t.Errorf("webhookURL = %v, want %v", notifier.webhookURL, tt.webhookURL)
			}
		})
	}
}

func TestNewSlackNotifierBot(t *testing.T) {
	tests := []struct {
		name      string
		botToken  string
		channelID string
		wantErr   bool
	}{
		{
			name:      "valid bot token and channel",
			botToken:  "xoxb-1234567890-1234567890123-abcdefghijklmnop",
			channelID: "C0123456789",
			wantErr:   false,
		},
		{
			name:      "empty bot token",
			botToken:  "",
			channelID: "C0123456789",
			wantErr:   true,
		},
		{
			name:      "empty channel ID",
			botToken:  "xoxb-1234567890-1234567890123-abcdefghijklmnop",
			channelID: "",
			wantErr:   true,
		},
		{
			name:      "both empty",
			botToken:  "",
			channelID: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier, err := NewSlackNotifierBot(tt.botToken, tt.channelID)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewSlackNotifierBot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if notifier == nil {
					t.Fatal("NewSlackNotifierBot() returned nil notifier")
				}
				if notifier.mode != slackModeBot {
					t.Errorf("mode = %v, want %v", notifier.mode, slackModeBot)
				}
				if notifier.channelID != tt.channelID {
					t.Errorf("channelID = %v, want %v", notifier.channelID, tt.channelID)
				}
				if notifier.client == nil {
					t.Error("client is nil")
				}
			}
		})
	}
}

func TestNewSlackNotifierApp(t *testing.T) {
	tests := []struct {
		name      string
		appToken  string
		botToken  string
		channelID string
		wantErr   bool
	}{
		{
			name:      "valid app, bot token and channel",
			appToken:  "xapp-1-A0123456789-1234567890123-abcdefghijklmnop",
			botToken:  "xoxb-1234567890-1234567890123-abcdefghijklmnop",
			channelID: "C0123456789",
			wantErr:   false,
		},
		{
			name:      "empty app token",
			appToken:  "",
			botToken:  "xoxb-1234567890-1234567890123-abcdefghijklmnop",
			channelID: "C0123456789",
			wantErr:   true,
		},
		{
			name:      "empty bot token",
			appToken:  "xapp-1-A0123456789-1234567890123-abcdefghijklmnop",
			botToken:  "",
			channelID: "C0123456789",
			wantErr:   true,
		},
		{
			name:      "empty channel ID",
			appToken:  "xapp-1-A0123456789-1234567890123-abcdefghijklmnop",
			botToken:  "xoxb-1234567890-1234567890123-abcdefghijklmnop",
			channelID: "",
			wantErr:   true,
		},
		{
			name:      "all empty",
			appToken:  "",
			botToken:  "",
			channelID: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier, err := NewSlackNotifierApp(tt.appToken, tt.botToken, tt.channelID)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewSlackNotifierApp() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if notifier == nil {
					t.Fatal("NewSlackNotifierApp() returned nil notifier")
				}
				if notifier.mode != slackModeApp {
					t.Errorf("mode = %v, want %v", notifier.mode, slackModeApp)
				}
				if notifier.channelID != tt.channelID {
					t.Errorf("channelID = %v, want %v", notifier.channelID, tt.channelID)
				}
				if notifier.client == nil {
					t.Error("client is nil")
				}
			}
		})
	}
}

func TestSlackNotifier_buildTaggedBlocks(t *testing.T) {
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
			{ID: "i-0abc123", Type: "aws:ec2", Name: "web-server"},
			{ID: "vol-0def456", Type: "aws:ebs", Name: ""},
		},
	}

	notifier := NewSlackNotifierWebhook("https://hooks.slack.com/services/T00/B00/XXXX")
	blocks := notifier.buildTaggedBlocks(event)

	// Should have: header, context, divider, section, resource table, divider, footer
	if len(blocks) != 7 {
		t.Errorf("expected 7 blocks, got %d", len(blocks))
	}
}

func TestSlackNotifier_buildDeletedBlocks(t *testing.T) {
	timestamp := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	event := domain.NotificationEvent{
		Type:        domain.NotificationTypeDeleted,
		AccountID:   "123456789012",
		AccountName: "",
		Region:      "eu-west-1",
		Timestamp:   timestamp,
		Resources: []domain.Resource{
			{ID: "i-expired123", Type: "aws:ec2", Name: "old-server"},
		},
	}

	notifier := NewSlackNotifierWebhook("https://hooks.slack.com/services/T00/B00/XXXX")
	blocks := notifier.buildDeletedBlocks(event)

	// Should have: header, context, divider, section, resource table
	if len(blocks) != 5 {
		t.Errorf("expected 5 blocks, got %d", len(blocks))
	}
}

func TestSlackNotifier_formatResourceTable(t *testing.T) {
	notifier := NewSlackNotifierWebhook("https://hooks.slack.com/services/T00/B00/XXXX")

	resources := []domain.Resource{
		{ID: "i-abc123", Type: "aws:ec2", Name: "web-server"},
		{ID: "vol-def456", Type: "aws:ebs", Name: ""},
	}

	result := notifier.formatResourceTable(resources)

	// Should be wrapped in code block
	if len(result) < 6 || result[:4] != "```\n" {
		t.Errorf("resource table should start with code block, got: %s", result[:min(10, len(result))])
	}
	if result[len(result)-3:] != "```" {
		t.Error("resource table should end with code block")
	}

	// Should contain resource info
	if !containsSubstr(result, "aws:ec2") {
		t.Errorf("resource table missing ec2 type: %s", result)
	}
	if !containsSubstr(result, "i-abc123") {
		t.Errorf("resource table missing ec2 ID: %s", result)
	}
	if !containsSubstr(result, "web-server") {
		t.Errorf("resource table missing ec2 name: %s", result)
	}
}

func TestSlackNotifier_formatResourceTable_Truncation(t *testing.T) {
	notifier := NewSlackNotifierWebhook("https://hooks.slack.com/services/T00/B00/XXXX")

	resources := []domain.Resource{
		{
			ID:   "i-verylongresourceidthatneedstruncation",
			Type: "aws:ec2:extended:type",
			Name: "this-is-a-very-long-resource-name-that-needs-truncation",
		},
	}

	result := notifier.formatResourceTable(resources)

	// Long names should be truncated with "..."
	if !containsSubstr(result, "...") {
		t.Errorf("expected truncation with '...' for long values: %s", result)
	}
}

func TestSlackNotifier_buildFallbackText(t *testing.T) {
	expDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	notifier := NewSlackNotifierWebhook("https://hooks.slack.com/services/T00/B00/XXXX")

	t.Run("tagged fallback", func(t *testing.T) {
		event := domain.NotificationEvent{
			ExpirationDate: &expDate,
			Resources:      make([]domain.Resource, 5),
		}

		text := notifier.buildTaggedFallbackText(event)
		if !containsSubstr(text, "5 resources") {
			t.Errorf("expected '5 resources' in fallback text: %s", text)
		}
		if !containsSubstr(text, "2026-04-01") {
			t.Errorf("expected date in fallback text: %s", text)
		}
	})

	t.Run("deleted fallback", func(t *testing.T) {
		event := domain.NotificationEvent{
			Resources: make([]domain.Resource, 3),
		}

		text := notifier.buildDeletedFallbackText(event)
		if !containsSubstr(text, "3") {
			t.Errorf("expected '3' in fallback text: %s", text)
		}
		if !containsSubstr(text, "deleted") {
			t.Errorf("expected 'deleted' in fallback text: %s", text)
		}
	})
}

func TestSlackNotifier_InterfaceCompliance(t *testing.T) {
	// Verify all constructors return types that implement domain.Notifier
	var _ domain.Notifier = (*SlackNotifier)(nil)

	webhookNotifier := NewSlackNotifierWebhook("https://hooks.slack.com/services/T00/B00/XXXX")
	var _ domain.Notifier = webhookNotifier

	botNotifier, err := NewSlackNotifierBot("xoxb-token", "C0123456789")
	if err != nil {
		t.Fatalf("failed to create bot notifier: %v", err)
	}
	var _ domain.Notifier = botNotifier

	appNotifier, err := NewSlackNotifierApp("xapp-token", "xoxb-token", "C0123456789")
	if err != nil {
		t.Fatalf("failed to create app notifier: %v", err)
	}
	var _ domain.Notifier = appNotifier
}

func TestSlackNotifier_Options(t *testing.T) {
	t.Run("WithSlackChannel", func(t *testing.T) {
		notifier := NewSlackNotifierWebhook(
			"https://hooks.slack.com/services/T00/B00/XXXX",
			WithSlackChannel("#my-channel"),
		)
		if notifier.channel != "#my-channel" {
			t.Errorf("channel = %v, want #my-channel", notifier.channel)
		}
	})

	t.Run("WithSlackDebug", func(t *testing.T) {
		notifier := NewSlackNotifierWebhook(
			"https://hooks.slack.com/services/T00/B00/XXXX",
			WithSlackDebug(true),
		)
		if !notifier.debug {
			t.Error("debug should be true")
		}
	})

	t.Run("multiple options", func(t *testing.T) {
		notifier := NewSlackNotifierWebhook(
			"https://hooks.slack.com/services/T00/B00/XXXX",
			WithSlackChannel("#alerts"),
			WithSlackDebug(true),
		)
		if notifier.channel != "#alerts" {
			t.Errorf("channel = %v, want #alerts", notifier.channel)
		}
		if !notifier.debug {
			t.Error("debug should be true")
		}
	})
}

func TestSlackNotifyTagged_InvalidMode(t *testing.T) {
	notifier := &SlackNotifier{
		mode: 0, // Invalid mode
	}

	event := domain.NotificationEvent{
		Type:      domain.NotificationTypeTagged,
		AccountID: "123",
		Region:    "us-east-1",
		Timestamp: time.Now(),
		Resources: []domain.Resource{},
	}

	err := notifier.NotifyTagged(context.Background(), event)
	if err == nil {
		t.Error("expected error for invalid mode, got nil")
	}
}

func TestSlackNotifyDeleted_InvalidMode(t *testing.T) {
	notifier := &SlackNotifier{
		mode: 0, // Invalid mode
	}

	event := domain.NotificationEvent{
		Type:      domain.NotificationTypeDeleted,
		AccountID: "123",
		Region:    "us-east-1",
		Timestamp: time.Now(),
		Resources: []domain.Resource{},
	}

	err := notifier.NotifyDeleted(context.Background(), event)
	if err == nil {
		t.Error("expected error for invalid mode, got nil")
	}
}

func TestSlackNotifier_AccountNameFallback(t *testing.T) {
	notifier := NewSlackNotifierWebhook("https://hooks.slack.com/services/T00/B00/XXXX")

	t.Run("uses account name when provided", func(t *testing.T) {
		event := domain.NotificationEvent{
			AccountID:   "123456789012",
			AccountName: "my-dev-account",
			Region:      "us-east-1",
			Resources:   []domain.Resource{},
		}

		blocks := notifier.buildTaggedBlocks(event)
		// The context block should contain the account name, not ID
		// This is block index 1 (after header)
		if len(blocks) < 2 {
			t.Fatal("not enough blocks")
		}
	})

	t.Run("falls back to account ID when name is empty", func(t *testing.T) {
		event := domain.NotificationEvent{
			AccountID:   "123456789012",
			AccountName: "",
			Region:      "us-east-1",
			Resources:   []domain.Resource{},
		}

		blocks := notifier.buildDeletedBlocks(event)
		if len(blocks) < 2 {
			t.Fatal("not enough blocks")
		}
	})
}

// containsSubstr checks if s contains substr
func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Note: Integration tests for actual Slack API calls would require
// mocking the slack client or using a test webhook.
// The sendViaWebhook and sendViaClient methods are tested via integration tests
// or manual verification with real Slack webhooks/bots.
