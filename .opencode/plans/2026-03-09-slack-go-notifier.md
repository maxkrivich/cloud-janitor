# Plan: Replace Slack Notifier with slack-go/slack Library

**Status**: `implemented`
**Created**: 2026-03-09
**Author**: AI Assistant

## Overview

Replace the existing Slack webhook notifier (raw HTTP) with the `slack-go/slack` library for:
- Built-in rate limiting and retries
- Typed Slack API structures
- Block Kit support for rich message formatting
- Multiple authentication modes (Webhook, Bot Token, App Token)

This follows the same pattern as the Discord notifier refactor, preferring open-source client libraries over raw HTTP implementations.

## Scope

### In Scope
- Replace `internal/infra/notify/slack.go` with `slack-go/slack`-based implementation
- Support three authentication modes:
  - Webhook URL (simple, current approach)
  - Bot Token (send to any channel)
  - App Token (Socket Mode ready for future)
- Upgrade message format to Block Kit for richer notifications
- Update configuration to support all auth methods
- Maintain the same `domain.Notifier` interface
- Unit tests

### Out of Scope
- Socket Mode / real-time events (future feature)
- Interactive components (buttons, modals)
- Slash commands
- File uploads

## Architecture & Design

### Authentication Modes

| Mode | Config | Use Case | Library Method |
|------|--------|----------|----------------|
| **Webhook** | `webhook_url` | Simple, no Slack app required | `slack.PostWebhook()` |
| **Bot Token** | `bot_token` + `channel_id` | Send to any channel | `client.PostMessage()` |
| **App Token** | `app_token` + `bot_token` + `channel_id` | Socket Mode ready | `client.PostMessage()` |

### Configuration Changes

```yaml
notifications:
  slack:
    enabled: true
    
    # Option 1: Webhook (simple, current approach)
    webhook_url: "https://hooks.slack.com/services/XXX/YYY/ZZZ"
    
    # Option 2: Bot token (send to any channel)
    bot_token: "${SLACK_BOT_TOKEN}"
    channel_id: "C0123456789"
    
    # Option 3: App token (Socket Mode ready)
    app_token: "${SLACK_APP_TOKEN}"  # xapp-...
    bot_token: "${SLACK_BOT_TOKEN}"  # xoxb-...
    channel_id: "C0123456789"
    
    # Optional: channel override (works with webhook too)
    channel: "#cloud-janitor"
```

### Code Structure

```go
// internal/infra/notify/slack.go

type slackMode int

const (
    slackModeWebhook slackMode = iota + 1
    slackModeBot
    slackModeApp
)

type SlackNotifier struct {
    client    *slack.Client
    webhookURL string
    channelID  string
    channel    string  // Optional override
    mode       slackMode
}

// Constructors
func NewSlackNotifierWebhook(webhookURL string, opts ...SlackOption) *SlackNotifier
func NewSlackNotifierBot(botToken, channelID string, opts ...SlackOption) (*SlackNotifier, error)
func NewSlackNotifierApp(appToken, botToken, channelID string, opts ...SlackOption) (*SlackNotifier, error)
```

### Block Kit Message Format

#### Tagged Resources Notification

```go
blocks := []slack.Block{
    // Header
    slack.NewHeaderBlock(
        slack.NewTextBlockObject("plain_text", "🏷️ Cloud Janitor: Resources Tagged for Expiration", true, false),
    ),
    
    // Context (Account & Region)
    slack.NewContextBlock("context",
        slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*Account:* %s", accountName), false, false),
        slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*Region:* %s", region), false, false),
    ),
    
    // Divider
    slack.NewDividerBlock(),
    
    // Section with expiration info
    slack.NewSectionBlock(
        slack.NewTextBlockObject("mrkdwn", 
            fmt.Sprintf("The following resources have been tagged with expiration date *%s*:", expDate),
            false, false),
        nil, nil,
    ),
    
    // Resource table (as code block in section)
    slack.NewSectionBlock(
        slack.NewTextBlockObject("mrkdwn", resourceTable, false, false),
        nil, nil,
    ),
    
    // Divider
    slack.NewDividerBlock(),
    
    // Warning footer
    slack.NewContextBlock("footer",
        slack.NewTextBlockObject("mrkdwn", 
            fmt.Sprintf("⚠️ These resources will be automatically deleted on *%s*. Update the `expiration-date` tag to keep them.", expDate),
            false, false),
    ),
}
```

#### Visual Preview

```
┌─────────────────────────────────────────────────────┐
│ 🏷️ Cloud Janitor: Resources Tagged for Expiration  │  <- Header
├─────────────────────────────────────────────────────┤
│ Account: dev-account  •  Region: us-east-1         │  <- Context
├─────────────────────────────────────────────────────┤
│ Tagged with expiration date 2026-04-08:            │  <- Section
│                                                     │
│ ```                                                 │
│ Type     | Resource ID      | Name                 │  <- Code block
│ aws:ec2  | i-0abc123def456  | web-server-1         │
│ aws:ebs  | vol-0def789abc   | data-volume          │
│ ```                                                 │
├─────────────────────────────────────────────────────┤
│ ⚠️ Will be deleted on 2026-04-08. Update tag.      │  <- Footer
└─────────────────────────────────────────────────────┘
```

### Benefits of slack-go/slack

1. **Built-in Rate Limiting**: Automatic retry with backoff for 429 errors
2. **Typed Structures**: `slack.Block`, `slack.Attachment`, etc.
3. **Block Kit Support**: Rich, structured message formatting
4. **Multiple Auth Modes**: Webhook, Bot, App Token
5. **Well-maintained**: 4.9k stars, active development, BSD-2-Clause license
6. **Testable**: Includes `slacktest` package for mocking

## Tasks

### Task 1: Add slack-go/slack Dependency
- [x] Run `go get github.com/slack-go/slack`
- [x] Verify dependency added to `go.mod` and `go.sum`

### Task 2: Implement Webhook Mode
- [x] Implement `NewSlackNotifierWebhook` constructor
- [x] Use `slack.PostWebhook()` for sending
- [x] Build Block Kit messages for tagged/deleted notifications
- [x] Add compile-time interface check

### Task 3: Implement Bot Token Mode
- [x] Implement `NewSlackNotifierBot` constructor
- [x] Create `slack.Client` with bot token
- [x] Use `client.PostMessage()` with blocks
- [x] Support channel ID and optional channel override

### Task 4: Implement App Token Mode
- [x] Implement `NewSlackNotifierApp` constructor
- [x] Create `slack.Client` with app token option
- [x] Same sending logic as bot mode
- [x] Prepare for future Socket Mode integration

### Task 5: Block Kit Message Builder
- [x] Create `buildTaggedBlocks()` method
- [x] Create `buildDeletedBlocks()` method
- [x] Format resource table with proper alignment
- [x] Add header, context, sections, dividers, footer

### Task 6: Functional Options
- [x] `WithSlackChannel` - channel override
- [ ] `WithSlackHTTPClient` - custom HTTP client for testing (skipped - not needed)
- [x] `WithSlackDebug` - enable debug logging

### Task 7: Unit Tests
- [x] Test webhook URL validation
- [x] Test Block Kit message building
- [x] Test `NotifyTagged` with mocked client
- [x] Test `NotifyDeleted` with mocked client
- [x] Test all three authentication modes
- [x] Test error handling

### Task 8: Update Configuration Loader
- [x] Update `SlackConfig` struct for new options
- [x] Add `bot_token`, `app_token`, `channel_id` fields
- [x] Expand environment variables for sensitive fields
- [x] Update `buildNotifier` in `cmd/root.go`

### Task 9: Documentation
- [x] Add configuration examples in comments
- [ ] Update any relevant docs (not needed - code is self-documenting)

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `go.mod` | Modify | Add `github.com/slack-go/slack` |
| `internal/infra/notify/slack.go` | Replace | New slack-go implementation |
| `internal/infra/notify/slack_test.go` | Create | Unit tests |
| `internal/infra/config/loader.go` | Modify | Update Slack config struct |
| `cmd/root.go` | Modify | Update buildNotifier for new constructors |

## Open Questions

1. **Webhook vs PostWebhook**: The `slack-go/slack` library has `PostWebhook()` for incoming webhooks. Does it support Block Kit? 
   - Answer: Yes, `WebhookMessage` supports `Blocks` field.

2. **Channel Override**: Should channel override work with all modes or just webhook?
   - Recommendation: Support for all modes where applicable.

3. **Error Details**: Should we parse Slack error responses for better error messages?
   - Recommendation: Yes, the library provides typed errors.

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| New dependency increases binary size | `slack-go/slack` is well-optimized |
| Breaking change to existing configs | Webhook URL config remains the same |
| Block Kit complexity | Encapsulate in builder methods |
| Rate limiting edge cases | Library handles automatically; add retries option |

## Success Criteria

- [x] Existing webhook URL config continues to work
- [x] Bot token config works for sending to channels
- [x] App token config works (prepared for Socket Mode)
- [x] Block Kit messages render correctly in Slack
- [x] Rate limiting is handled automatically
- [x] All unit tests pass
- [x] No breaking changes to `domain.Notifier` interface

## Example Usage

```go
// Webhook mode (simple)
notifier := notify.NewSlackNotifierWebhook(
    "https://hooks.slack.com/services/XXX/YYY/ZZZ",
    notify.WithSlackChannel("#cloud-janitor"),
)

// Bot mode (send to any channel)
notifier, err := notify.NewSlackNotifierBot(
    os.Getenv("SLACK_BOT_TOKEN"),
    "C0123456789",
)

// App mode (Socket Mode ready)
notifier, err := notify.NewSlackNotifierApp(
    os.Getenv("SLACK_APP_TOKEN"),
    os.Getenv("SLACK_BOT_TOKEN"),
    "C0123456789",
)

// All implement domain.Notifier
var _ domain.Notifier = notifier
```
