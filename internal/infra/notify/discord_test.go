package notify

import (
	"context"
	"testing"
	"time"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

func TestParseWebhookURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantID    string
		wantToken string
		wantErr   bool
	}{
		{
			name:      "valid discord.com URL",
			url:       "https://discord.com/api/webhooks/123456789012345678/abcdefghijklmnop_QRSTUVWXYZ-1234567890",
			wantID:    "123456789012345678",
			wantToken: "abcdefghijklmnop_QRSTUVWXYZ-1234567890",
			wantErr:   false,
		},
		{
			name:      "valid discordapp.com URL",
			url:       "https://discordapp.com/api/webhooks/987654321098765432/token_with-mixed_CHARS123",
			wantID:    "987654321098765432",
			wantToken: "token_with-mixed_CHARS123",
			wantErr:   false,
		},
		{
			name:    "invalid URL - missing token",
			url:     "https://discord.com/api/webhooks/123456789012345678",
			wantErr: true,
		},
		{
			name:    "invalid URL - wrong domain",
			url:     "https://example.com/api/webhooks/123456789012345678/token",
			wantErr: true,
		},
		{
			name:    "invalid URL - empty",
			url:     "",
			wantErr: true,
		},
		{
			name:    "invalid URL - not a webhook URL",
			url:     "https://discord.com/channels/123/456",
			wantErr: true,
		},
		{
			name:    "invalid URL - non-numeric ID",
			url:     "https://discord.com/api/webhooks/abc/token",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, token, err := parseWebhookURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseWebhookURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if id != tt.wantID {
					t.Errorf("parseWebhookURL() id = %v, want %v", id, tt.wantID)
				}
				if token != tt.wantToken {
					t.Errorf("parseWebhookURL() token = %v, want %v", token, tt.wantToken)
				}
			}
		})
	}
}

func TestNewDiscordNotifierWebhook(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid webhook URL",
			url:     "https://discord.com/api/webhooks/123456789012345678/validtoken123",
			wantErr: false,
		},
		{
			name:    "invalid webhook URL",
			url:     "https://example.com/not-a-webhook",
			wantErr: true,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier, err := NewDiscordNotifierWebhook(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewDiscordNotifierWebhook() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if notifier == nil {
					t.Error("NewDiscordNotifierWebhook() returned nil notifier")
					return
				}
				if notifier.mode != discordModeWebhook {
					t.Errorf("NewDiscordNotifierWebhook() mode = %v, want %v", notifier.mode, discordModeWebhook)
				}
				if notifier.session == nil {
					t.Error("NewDiscordNotifierWebhook() session is nil")
				}
			}
		})
	}
}

func TestNewDiscordNotifierBot(t *testing.T) {
	tests := []struct {
		name      string
		botToken  string
		channelID string
		wantErr   bool
	}{
		{
			name:      "valid bot token without prefix",
			botToken:  "MTIzNDU2Nzg5MDEyMzQ1Njc4.abcdef.ghijklmnopqrstuvwxyz",
			channelID: "123456789012345678",
			wantErr:   false,
		},
		{
			name:      "valid bot token with prefix",
			botToken:  "Bot MTIzNDU2Nzg5MDEyMzQ1Njc4.abcdef.ghijklmnopqrstuvwxyz",
			channelID: "123456789012345678",
			wantErr:   false,
		},
		{
			name:      "empty bot token",
			botToken:  "",
			channelID: "123456789012345678",
			wantErr:   true,
		},
		{
			name:      "empty channel ID",
			botToken:  "valid-token",
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
			notifier, err := NewDiscordNotifierBot(tt.botToken, tt.channelID)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewDiscordNotifierBot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if notifier == nil {
					t.Error("NewDiscordNotifierBot() returned nil notifier")
					return
				}
				if notifier.mode != discordModeBot {
					t.Errorf("NewDiscordNotifierBot() mode = %v, want %v", notifier.mode, discordModeBot)
				}
				if notifier.channelID != tt.channelID {
					t.Errorf("NewDiscordNotifierBot() channelID = %v, want %v", notifier.channelID, tt.channelID)
				}
				if notifier.session == nil {
					t.Error("NewDiscordNotifierBot() session is nil")
				}
			}
		})
	}
}

func TestDiscordNotifier_buildTaggedEmbed(t *testing.T) {
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

	notifier, err := NewDiscordNotifierWebhook("https://discord.com/api/webhooks/123/token")
	if err != nil {
		t.Fatalf("failed to create notifier: %v", err)
	}

	embed := notifier.buildTaggedEmbed(event)

	if embed.Title != "Cloud Janitor: Resources Tagged for Expiration" {
		t.Errorf("unexpected title: %s", embed.Title)
	}

	if embed.Color != 0xFFA500 {
		t.Errorf("unexpected color: %d, want %d", embed.Color, 0xFFA500)
	}

	if len(embed.Fields) != 3 {
		t.Errorf("unexpected field count: %d, want 3", len(embed.Fields))
	}

	// Check account field
	if embed.Fields[0].Name != "Account" || embed.Fields[0].Value != "dev-account" {
		t.Errorf("unexpected account field: %+v", embed.Fields[0])
	}

	// Check region field
	if embed.Fields[1].Name != "Region" || embed.Fields[1].Value != "us-east-1" {
		t.Errorf("unexpected region field: %+v", embed.Fields[1])
	}

	// Check footer
	if embed.Footer == nil {
		t.Error("footer is nil")
	}

	// Check timestamp
	if embed.Timestamp != timestamp.Format(time.RFC3339) {
		t.Errorf("unexpected timestamp: %s", embed.Timestamp)
	}
}

func TestDiscordNotifier_buildDeletedEmbed(t *testing.T) {
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

	notifier, err := NewDiscordNotifierWebhook("https://discord.com/api/webhooks/123/token")
	if err != nil {
		t.Fatalf("failed to create notifier: %v", err)
	}

	embed := notifier.buildDeletedEmbed(event)

	if embed.Title != "Cloud Janitor: Resources Deleted" {
		t.Errorf("unexpected title: %s", embed.Title)
	}

	if embed.Color != 0xFF0000 {
		t.Errorf("unexpected color: %d, want %d (red)", embed.Color, 0xFF0000)
	}

	// Should fall back to AccountID when AccountName is empty
	if embed.Fields[0].Value != "123456789012" {
		t.Errorf("expected account fallback to ID, got: %s", embed.Fields[0].Value)
	}

	// Deleted messages don't have a footer
	if embed.Footer != nil {
		t.Error("deleted embed should not have footer")
	}
}

func TestDiscordNotifier_formatResourceList(t *testing.T) {
	notifier, _ := NewDiscordNotifierWebhook("https://discord.com/api/webhooks/123/token")

	resources := []domain.Resource{
		{ID: "i-abc123", Type: "aws:ec2", Name: "web-server"},
		{ID: "vol-def456", Type: "aws:ebs", Name: ""},
	}

	result := notifier.formatResourceList(resources)

	// Should be wrapped in code block
	if result[:4] != "```\n" {
		t.Errorf("resource list should start with code block, got: %s", result[:4])
	}
	if result[len(result)-3:] != "```" {
		t.Errorf("resource list should end with code block")
	}

	// Should contain resource info
	if !contains(result, "aws:ec2: i-abc123 (web-server)") {
		t.Errorf("resource list missing ec2 resource: %s", result)
	}
	if !contains(result, "aws:ebs: vol-def456") {
		t.Errorf("resource list missing ebs resource: %s", result)
	}
}

func TestDiscordNotifier_InterfaceCompliance(t *testing.T) {
	// Verify both constructors return types that implement domain.Notifier
	var _ domain.Notifier = (*DiscordNotifier)(nil)

	webhookNotifier, err := NewDiscordNotifierWebhook("https://discord.com/api/webhooks/123/token")
	if err != nil {
		t.Fatalf("failed to create webhook notifier: %v", err)
	}
	var _ domain.Notifier = webhookNotifier

	botNotifier, err := NewDiscordNotifierBot("test-token", "channel-id")
	if err != nil {
		t.Fatalf("failed to create bot notifier: %v", err)
	}
	var _ domain.Notifier = botNotifier
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Note: Integration tests for actual Discord API calls would require
// mocking the discordgo session or using a test webhook.
// The sendViaWebhook and sendViaBot methods are tested via integration tests
// or manual verification with real Discord webhooks/bots.

func TestNotifyTagged_InvalidMode(t *testing.T) {
	notifier := &DiscordNotifier{
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
