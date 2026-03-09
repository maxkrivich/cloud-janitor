// Package usecase contains application use cases that orchestrate domain logic.
package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// TagConfig holds configuration for the tag use case.
type TagConfig struct {
	// DefaultDays is the number of days until expiration for newly tagged resources.
	DefaultDays int

	// TagName is the name of the expiration tag.
	TagName string

	// ExcludeTags are tag key-value pairs that exclude resources from tagging.
	ExcludeTags map[string]string

	// DryRun indicates whether to simulate without making changes.
	DryRun bool
}

// TagResult contains the results of the tag operation.
type TagResult struct {
	// Tagged is the list of resources that were tagged.
	Tagged []domain.Resource

	// Skipped is the list of resources that were skipped (already tagged or excluded).
	Skipped []domain.Resource

	// Errors is the list of errors that occurred during tagging.
	Errors []*domain.ResourceError
}

// TagResourcesUseCase tags untagged resources with an expiration date.
type TagResourcesUseCase struct {
	repos    []domain.ResourceRepository
	notifier domain.Notifier
	config   TagConfig
	now      func() time.Time
}

// NewTagResourcesUseCase creates a new TagResourcesUseCase.
func NewTagResourcesUseCase(
	repos []domain.ResourceRepository,
	notifier domain.Notifier,
	config TagConfig,
) *TagResourcesUseCase {
	return &TagResourcesUseCase{
		repos:    repos,
		notifier: notifier,
		config:   config,
		now:      time.Now,
	}
}

// Execute runs the tag operation for all repositories in the specified region.
func (uc *TagResourcesUseCase) Execute(ctx context.Context, accountID, accountName, region string) (*TagResult, error) {
	result := &TagResult{}
	expirationDate := uc.now().AddDate(0, 0, uc.config.DefaultDays)

	for _, repo := range uc.repos {
		resources, err := repo.List(ctx, region)
		if err != nil {
			return nil, fmt.Errorf("listing %s resources: %w", repo.Type(), err)
		}

		for _, r := range resources {
			// Skip resources that are excluded
			if r.IsExcluded(uc.config.ExcludeTags) {
				result.Skipped = append(result.Skipped, r)
				continue
			}

			// Skip resources that already have an expiration date or never expire
			if r.Status() != domain.StatusUntagged {
				result.Skipped = append(result.Skipped, r)
				continue
			}

			// Tag the resource
			if !uc.config.DryRun {
				if err := repo.Tag(ctx, r.ID, expirationDate); err != nil {
					result.Errors = append(result.Errors, domain.NewResourceError(
						r.ID, r.Type, "tagging", err,
					))
					continue
				}
			}

			r.ExpirationDate = &expirationDate
			result.Tagged = append(result.Tagged, r)
		}
	}

	// Send notification about tagged resources
	if len(result.Tagged) > 0 && !uc.config.DryRun {
		event := domain.NotificationEvent{
			Type:           domain.NotificationTypeTagged,
			Resources:      result.Tagged,
			AccountID:      accountID,
			AccountName:    accountName,
			Region:         region,
			Timestamp:      uc.now(),
			ExpirationDate: &expirationDate,
		}
		if err := uc.notifier.NotifyTagged(ctx, event); err != nil {
			// Log but don't fail the operation
			// The tagging was successful, notification is best-effort
			_ = err
		}
	}

	return result, nil
}
