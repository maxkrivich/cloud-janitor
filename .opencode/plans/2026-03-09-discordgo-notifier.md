# Plan: Replace Discord Notifier with discordgo Library

**Status**: `implemented`
**Created**: 2026-03-09
**Author**: AI Assistant

## Overview

Replace the existing Discord webhook notifier (raw HTTP) with the `discordgo` library for cleaner webhook handling, built-in rate limiting, and typed Discord API structures. Support both webhook URLs (simple) and bot tokens (advanced) for flexibility.

## Scope

### In Scope
- Replace `internal/infra/notify/discord.go` with `discordgo`-based implementation
- Use `discordgo` typed structures for embeds and messages
- Leverage `discordgo` built-in rate limiting
- Support webhook URL authentication (current approach)
- Support bot token authentication (new, optional)
- Update configuration to support both auth methods
- Maintain the same `domain.Notifier` interface
- Unit tests with mocked Discord interactions

### Out of Scope
- Bot features (slash commands, reactions, etc.)
- Real-time Discord event handling (WebSocket)
- Voice features
- Interactive message components (buttons, etc.)

## Architecture & Design

### Authentication Modes

| Mode | Config | Use Case |
|------|--------|----------|
| **Webhook** | `webhook_url` | Simple, no Discord app required |
| **Bot Token** | `bot_token` + `channel_id` | Richer features, requires Discord app |

### Configuration Changes

```yaml
notifications:
  discord:
    enabled: true
    # Option 1: Webhook (simple, current approach)
    webhook_url: "https://discord.com/api/webhooks/XXX/YYY"
    
    # Option 2: Bot token (advanced)
    bot_token: "${DISCORD_BOT_TOKEN}"
    channel_id: "123456789012345678"
```

### Code Structure

```go
// internal/infra/notify/discord.go

// DiscordNotifier sends notifications to Discord using discordgo.
type DiscordNotifier struct {
    session    *discordgo.Session
    webhookID  string
    webhookToken string
    channelID  string  // Used when bot_token is provided
    mode       discordMode
}

type discordMode int

const (
    discordModeWebhook discordMode = iota + 1
    discordModeBot
)

// NewDiscordNotifierWebhook creates a notifier using webhook URL.
func NewDiscordNotifierWebhook(webhookURL string, opts ...DiscordOption) (*DiscordNotifier, error)

// NewDiscordNotifierBot creates a notifier using bot token.
func NewDiscordNotifierBot(botToken, channelID string, opts ...DiscordOption) (*DiscordNotifier, error)
```

### Message Format (Using discordgo Embeds)

```go
embed := &discordgo.MessageEmbed{
    Title:       "Cloud Janitor: Resources Tagged for Expiration",
    Description: "The following resources have been tagged...",
    Color:       0xFFA500, // Orange
    Fields: []*discordgo.MessageEmbedField{
        {Name: "Account", Value: accountName, Inline: true},
        {Name: "Region", Value: event.Region, Inline: true},
        {Name: "Resources", Value: resourceList, Inline: false},
    },
    Footer: &discordgo.MessageEmbedFooter{
        Text: "Update expiration-date tag to keep resources",
    },
    Timestamp: event.Timestamp.Format(time.RFC3339),
}
```

### Benefits of discordgo

1. **Built-in Rate Limiting**: Handles Discord's rate limits automatically
2. **Typed Structures**: `discordgo.MessageEmbed`, `discordgo.WebhookParams` etc.
3. **Webhook Parsing**: Can parse webhook URL to extract ID and token
4. **Future-proof**: Easy to add bot features later if needed
5. **Well-maintained**: 5.8k stars, active development

## Tasks

### Task 1: Add discordgo Dependency
- [x] Run `go get github.com/bwmarrin/discordgo`
- [x] Verify dependency added to `go.mod` and `go.sum`

### Task 2: Implement Webhook Mode
- [x] Create webhook URL parser (extract ID and token)
- [x] Implement `NewDiscordNotifierWebhook` constructor
- [x] Implement `NotifyTagged` using `discordgo.WebhookExecute`
- [x] Implement `NotifyDeleted` using `discordgo.WebhookExecute`
- [x] Build embed messages using `discordgo.MessageEmbed`
- [x] Add compile-time interface check

### Task 3: Implement Bot Token Mode
- [x] Implement `NewDiscordNotifierBot` constructor
- [x] Use `discordgo.ChannelMessageSendEmbed` for bot mode
- [x] Handle session lifecycle (no Open/Close needed for REST-only)

### Task 4: Functional Options
- [x] `WithHTTPClient` - custom HTTP client for testing
- [x] Preserve existing options pattern

### Task 5: Unit Tests
- [x] Test webhook URL parsing
- [x] Test embed message building
- [x] Test `NotifyTagged` with mocked session
- [x] Test `NotifyDeleted` with mocked session
- [x] Test error handling (invalid webhook URL, API errors)

### Task 6: Update Configuration Loader
- [x] Update config struct for new Discord options
- [x] Support both `webhook_url` and `bot_token` + `channel_id`
- [ ] Validate that one auth method is provided (not both, not neither) - skipped, handled gracefully in buildNotifier

### Task 7: Documentation
- [ ] Update ARCHITECTURE.md with new Discord config options - optional, config comments sufficient
- [x] Add example configuration in comments

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `go.mod` | Modify | Add `github.com/bwmarrin/discordgo` |
| `internal/infra/notify/discord.go` | Replace | New discordgo-based implementation |
| `internal/infra/notify/discord_test.go` | Create | Unit tests |
| `internal/infra/config/loader.go` | Modify | Update Discord config parsing |

## Open Questions

1. **Webhook URL Parsing**: The `discordgo` library doesn't have a built-in webhook URL parser. Should we:
   - Parse manually (regex or string split)
   - Use a helper function

2. **Session Reuse**: Should we create a single `discordgo.Session` and reuse it, or create per-request?
   - Recommendation: Single session, reused (handles rate limiting state)

3. **Close Session**: Since we're only using REST API (not WebSocket), do we need to call `session.Close()`?
   - Answer: No, `Close()` only affects WebSocket. REST-only usage doesn't require it.

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| New dependency increases binary size | `discordgo` is relatively small; monitor size |
| Breaking change to existing configs | Webhook URL config remains the same |
| Rate limiting edge cases | `discordgo` handles this; add retry logic if needed |

## Success Criteria

- [x] Existing webhook URL config continues to work
- [x] Bot token config works for sending to a channel
- [x] Rate limiting is handled automatically
- [x] All unit tests pass
- [x] Message format matches current output (embeds)
- [x] No breaking changes to `domain.Notifier` interface

## Example Usage

```go
// Webhook mode (simple)
notifier, err := notify.NewDiscordNotifierWebhook(
    "https://discord.com/api/webhooks/123456/abcdef",
)

// Bot mode (advanced)
notifier, err := notify.NewDiscordNotifierBot(
    os.Getenv("DISCORD_BOT_TOKEN"),
    "channel-id-here",
)

// Both implement domain.Notifier
var _ domain.Notifier = notifier
```
