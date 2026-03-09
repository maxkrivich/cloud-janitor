package domain

import (
	"context"
	"time"
)

// Notifier sends notifications about resource lifecycle events.
// Implementations: Slack, Discord, Webhook, Multi (fan-out), Noop.
type Notifier interface {
	// NotifyTagged sends notification when resources are tagged with expiration date.
	// The message should prompt users to take action if they want to keep the resource.
	NotifyTagged(ctx context.Context, event NotificationEvent) error

	// NotifyDeleted sends notification when expired resources are deleted.
	NotifyDeleted(ctx context.Context, event NotificationEvent) error
}

// NotificationType represents the type of notification event.
type NotificationType string

const (
	// NotificationTypeTagged indicates resources were tagged for expiration.
	NotificationTypeTagged NotificationType = "tagged"
	// NotificationTypeDeleted indicates resources were deleted.
	NotificationTypeDeleted NotificationType = "deleted"
)

// NotificationEvent represents a notification payload.
type NotificationEvent struct {
	// Type is the kind of notification (tagged, deleted).
	Type NotificationType

	// Resources is the list of affected resources.
	Resources []Resource

	// AccountID is the AWS account ID.
	AccountID string

	// AccountName is the human-readable account name.
	AccountName string

	// Region is the AWS region.
	Region string

	// Timestamp is when the event occurred.
	Timestamp time.Time

	// ExpirationDate is the expiration date set (for tagged events).
	ExpirationDate *time.Time
}
