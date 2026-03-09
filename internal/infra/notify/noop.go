package notify

import (
	"context"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check.
var _ domain.Notifier = (*NoopNotifier)(nil)

// NoopNotifier is a no-operation notifier used for dry-run mode.
type NoopNotifier struct{}

// NewNoopNotifier creates a new NoopNotifier.
func NewNoopNotifier() *NoopNotifier {
	return &NoopNotifier{}
}

// NotifyTagged does nothing.
func (n *NoopNotifier) NotifyTagged(_ context.Context, _ domain.NotificationEvent) error {
	return nil
}

// NotifyDeleted does nothing.
func (n *NoopNotifier) NotifyDeleted(_ context.Context, _ domain.NotificationEvent) error {
	return nil
}
