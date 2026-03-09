package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// CleanupConfig holds configuration for the cleanup use case.
type CleanupConfig struct {
	// ExcludeTags are tag key-value pairs that exclude resources from cleanup.
	ExcludeTags map[string]string

	// DryRun indicates whether to simulate without making changes.
	DryRun bool
}

// CleanupResult contains the results of the cleanup operation.
type CleanupResult struct {
	// Deleted is the list of resources that were deleted.
	Deleted []domain.Resource

	// Skipped is the list of resources that were skipped (not expired or excluded).
	Skipped []domain.Resource

	// Errors is the list of errors that occurred during cleanup.
	Errors []*domain.ResourceError
}

// CleanupResourcesUseCase deletes expired resources.
type CleanupResourcesUseCase struct {
	repos    []domain.ResourceRepository
	notifier domain.Notifier
	config   CleanupConfig
	now      func() time.Time
}

// NewCleanupResourcesUseCase creates a new CleanupResourcesUseCase.
func NewCleanupResourcesUseCase(
	repos []domain.ResourceRepository,
	notifier domain.Notifier,
	config CleanupConfig,
) *CleanupResourcesUseCase {
	return &CleanupResourcesUseCase{
		repos:    repos,
		notifier: notifier,
		config:   config,
		now:      time.Now,
	}
}

// Execute runs the cleanup operation for all repositories in the specified region.
func (uc *CleanupResourcesUseCase) Execute(ctx context.Context, accountID, accountName, region string) (*CleanupResult, error) {
	result := &CleanupResult{}

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

			// Only delete expired resources
			if r.Status() != domain.StatusExpired {
				result.Skipped = append(result.Skipped, r)
				continue
			}

			// Delete the resource
			if !uc.config.DryRun {
				if err := repo.Delete(ctx, r.ID); err != nil {
					result.Errors = append(result.Errors, domain.NewResourceError(
						r.ID, r.Type, "deleting", err,
					))
					continue
				}
			}

			result.Deleted = append(result.Deleted, r)
		}
	}

	// Send notification about deleted resources
	if len(result.Deleted) > 0 && !uc.config.DryRun {
		event := domain.NotificationEvent{
			Type:        domain.NotificationTypeDeleted,
			Resources:   result.Deleted,
			AccountID:   accountID,
			AccountName: accountName,
			Region:      region,
			Timestamp:   uc.now(),
		}
		if err := uc.notifier.NotifyDeleted(ctx, event); err != nil {
			// Log but don't fail the operation
			_ = err
		}
	}

	return result, nil
}
