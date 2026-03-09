# Plan: Microsoft Teams Webhook Notifier

**Status**: `implemented`
**Created**: 2026-03-09
**Author**: AI Assistant

## Overview

Add Microsoft Teams notification support using Incoming Webhooks. This follows the existing notifier pattern (Slack, Discord, Webhook) and uses the simple MessageCard format for notifications when resources are tagged for expiration or deleted.

### User Requirements
- **Message Format**: Simple MessageCard (legacy format, not Adaptive Cards)
- **Authentication**: Webhook URL only (no Graph API/Bot support)
- **Scope**: Teams notifier only (not bundled with other notifiers)

## Architecture & Design

### Teams Webhook API

Microsoft Teams Incoming Webhooks accept JSON payloads in MessageCard format:

```json
{
    "@type": "MessageCard",
    "@context": "http://schema.org/extensions",
    "themeColor": "FFA500",
    "summary": "Cloud Janitor: Resources Tagged",
    "sections": [{
        "activityTitle": "🏷️ Resources Tagged for Expiration",
        "activitySubtitle": "Account: dev-account • Region: us-east-1",
        "facts": [
            {"name": "Expiration Date", "value": "2026-04-08"},
            {"name": "Resources", "value": "3 resources tagged"}
        ],
        "markdown": true
    }]
}
```

### Message Format Design

#### Tagged Resources Notification

```
┌─────────────────────────────────────────────────────────┐
│ 🏷️ Resources Tagged for Expiration                     │  <- activityTitle
│ Account: dev-account • Region: us-east-1               │  <- activitySubtitle
├─────────────────────────────────────────────────────────┤
│ Expiration Date    2026-04-08                          │  <- facts
│ Resources          3 resources tagged                  │
├─────────────────────────────────────────────────────────┤
│ aws:ec2: i-0abc123 (web-server)                        │  <- text (code block)
│ aws:ebs: vol-0def456                                   │
│ aws:eip: eipalloc-0ghi789                              │
├─────────────────────────────────────────────────────────┤
│ ⚠️ Update expiration-date tag to keep resources.       │  <- text footer
└─────────────────────────────────────────────────────────┘
```

#### Color Coding
- **Tagged**: Orange (`FFA500`) - Warning, action may be needed
- **Deleted**: Red (`FF0000`) - Resources removed

### Code Structure

```go
// internal/infra/notify/teams.go

// Compile-time interface check
var _ domain.Notifier = (*TeamsNotifier)(nil)

type TeamsNotifier struct {
    webhookURL string
    client     *http.Client
}

type TeamsOption func(*TeamsNotifier)

// Constructor
func NewTeamsNotifier(webhookURL string, opts ...TeamsOption) *TeamsNotifier

// Interface methods
func (n *TeamsNotifier) NotifyTagged(ctx context.Context, event domain.NotificationEvent) error
func (n *TeamsNotifier) NotifyDeleted(ctx context.Context, event domain.NotificationEvent) error

// Internal helpers
func (n *TeamsNotifier) buildTaggedCard(event domain.NotificationEvent) messageCard
func (n *TeamsNotifier) buildDeletedCard(event domain.NotificationEvent) messageCard
func (n *TeamsNotifier) send(ctx context.Context, card messageCard) error
```

### MessageCard Struct

```go
// MessageCard represents a Microsoft Teams connector card
type messageCard struct {
    Type       string            `json:"@type"`
    Context    string            `json:"@context"`
    ThemeColor string            `json:"themeColor"`
    Summary    string            `json:"summary"`
    Sections   []messageSection  `json:"sections"`
}

type messageSection struct {
    ActivityTitle    string        `json:"activityTitle,omitempty"`
    ActivitySubtitle string        `json:"activitySubtitle,omitempty"`
    Facts            []messageFact `json:"facts,omitempty"`
    Text             string        `json:"text,omitempty"`
    Markdown         bool          `json:"markdown"`
}

type messageFact struct {
    Name  string `json:"name"`
    Value string `json:"value"`
}
```

### Configuration

```yaml
notifications:
  teams:
    enabled: true
    webhook_url: "${TEAMS_WEBHOOK_URL}"
```

### Rate Limiting Considerations

Teams webhooks have strict rate limits:
- 4 requests per second
- 60 requests per 30 seconds
- 100 requests per hour

For Cloud Janitor's use case (daily batch runs), this is unlikely to be an issue. However, we should:
- Handle HTTP 429 responses gracefully
- Log warnings when rate limited

## Tasks

### Task 1: Create TeamsNotifier Implementation
- [x] Create `internal/infra/notify/teams.go`
- [x] Add compile-time interface check
- [x] Implement `TeamsNotifier` struct with `webhookURL` and `client` fields
- [x] Implement `NewTeamsNotifier` constructor with functional options
- [x] Add `WithTeamsHTTPClient` option for testing

### Task 2: Implement MessageCard Builder
- [x] Define `messageCard`, `messageSection`, `messageFact` structs
- [x] Implement `buildTaggedCard` method
- [x] Implement `buildDeletedCard` method
- [x] Implement `formatResourceList` helper (code block format)
- [x] Use appropriate theme colors (orange for tagged, red for deleted)

### Task 3: Implement Send Method
- [x] Implement `send` method with HTTP POST
- [x] Set `Content-Type: application/json`
- [x] Handle non-2xx responses with descriptive errors
- [x] Handle rate limiting (429) with warning log

### Task 4: Implement Notifier Interface
- [x] Implement `NotifyTagged` method
- [x] Implement `NotifyDeleted` method
- [x] Handle account name fallback to ID

### Task 5: Add Configuration Support
- [x] Add `TeamsConfig` struct to `internal/infra/config/loader.go`
- [x] Add `Teams` field to `NotificationsConfig`
- [x] Add environment variable expansion for `webhook_url`

### Task 6: Wire Up in buildNotifier
- [x] Update `buildNotifier` in `cmd/root.go`
- [x] Add Teams notifier creation logic
- [x] Handle errors with warning message

### Task 7: Write Unit Tests
- [x] Create `internal/infra/notify/teams_test.go`
- [x] Test `NewTeamsNotifier` constructor
- [x] Test `buildTaggedCard` output structure
- [x] Test `buildDeletedCard` output structure
- [x] Test `formatResourceList` formatting
- [x] Test interface compliance
- [x] Test options (`WithTeamsHTTPClient`)
- [x] Test error handling for invalid responses

### Task 8: Update Documentation
- [x] Add Teams config example in code comments
- [x] Update plan status to `implemented`

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/infra/notify/teams.go` | Create | Teams notifier implementation |
| `internal/infra/notify/teams_test.go` | Create | Unit tests |
| `internal/infra/config/loader.go` | Modify | Add `TeamsConfig` struct |
| `cmd/root.go` | Modify | Add Teams to `buildNotifier` |

## Open Questions

1. **Deprecation Notice**: Microsoft is deprecating Office 365 Connectors in favor of Workflows/Power Automate. Should we:
   - Proceed with MessageCard (works now, may need migration later)
   - Use Adaptive Cards via Workflows API (future-proof but more complex)
   
   **Decision**: Proceed with MessageCard for now. The deprecation timeline is long, and this matches the "simple" requirement. Can add Workflows support later if needed.

2. **Image/Icon**: Should we include an icon in the card? Teams MessageCards support `activityImage`.
   
   **Recommendation**: Skip for MVP. Keep it simple.

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Office 365 Connectors deprecation | Medium | MessageCard format still works; can migrate to Workflows later |
| Rate limiting on large batches | Low | Daily runs unlikely to hit limits; add retry logic |
| Webhook URL exposure | Medium | Use environment variables; document security |

## Success Criteria

- [x] Teams webhook URL config works
- [x] MessageCard notifications render correctly in Teams
- [x] Tagged notifications show orange theme color
- [x] Deleted notifications show red theme color
- [x] Resource list displays clearly
- [x] All unit tests pass
- [x] No breaking changes to existing notifiers

## Example Usage

```go
// Create Teams notifier
notifier := notify.NewTeamsNotifier(
    "https://outlook.office.com/webhook/xxx/IncomingWebhook/yyy/zzz",
)

// Implements domain.Notifier
var _ domain.Notifier = notifier

// Send notification
err := notifier.NotifyTagged(ctx, domain.NotificationEvent{
    AccountID:      "123456789012",
    AccountName:    "dev-account",
    Region:         "us-east-1",
    ExpirationDate: &expDate,
    Resources:      resources,
})
```

## Configuration Example

```yaml
# cloud-janitor.yaml
notifications:
  enabled: true
  
  teams:
    enabled: true
    webhook_url: "${TEAMS_WEBHOOK_URL}"
    
  # Can be used alongside other notifiers
  slack:
    enabled: true
    webhook_url: "${SLACK_WEBHOOK_URL}"
```
