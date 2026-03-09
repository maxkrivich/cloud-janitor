package notify

import (
	"context"
	"errors"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check.
var _ domain.Notifier = (*MultiNotifier)(nil)

// MultiNotifier fans out notifications to multiple notifiers.
type MultiNotifier struct {
	notifiers []domain.Notifier
}

// NewMultiNotifier creates a new MultiNotifier.
func NewMultiNotifier(notifiers ...domain.Notifier) *MultiNotifier {
	return &MultiNotifier{notifiers: notifiers}
}

// NotifyTagged sends notification to all notifiers.
func (m *MultiNotifier) NotifyTagged(ctx context.Context, event domain.NotificationEvent) error {
	var errs []error
	for _, n := range m.notifiers {
		if err := n.NotifyTagged(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// NotifyDeleted sends notification to all notifiers.
func (m *MultiNotifier) NotifyDeleted(ctx context.Context, event domain.NotificationEvent) error {
	var errs []error
	for _, n := range m.notifiers {
		if err := n.NotifyDeleted(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
