package usecase

import (
	"context"
	"fmt"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// ListConfig holds configuration for the list use case.
type ListConfig struct {
	// FilterStatus filters resources by status (empty means all).
	FilterStatus []domain.Status

	// FilterType filters resources by type (empty means all).
	FilterType []domain.ResourceType
}

// ListResult contains the results of the list operation.
type ListResult struct {
	// Resources is the list of resources found.
	Resources []domain.Resource

	// Summary contains counts by status.
	Summary ListSummary
}

// ListSummary contains summary counts.
type ListSummary struct {
	Total        int
	Untagged     int
	Active       int
	Expired      int
	NeverExpires int
}

// ListResourcesUseCase lists resources and their expiration status.
type ListResourcesUseCase struct {
	repos  []domain.ResourceRepository
	config ListConfig
}

// NewListResourcesUseCase creates a new ListResourcesUseCase.
func NewListResourcesUseCase(
	repos []domain.ResourceRepository,
	config ListConfig,
) *ListResourcesUseCase {
	return &ListResourcesUseCase{
		repos:  repos,
		config: config,
	}
}

// Execute runs the list operation for all repositories in the specified region.
func (uc *ListResourcesUseCase) Execute(ctx context.Context, region string) (*ListResult, error) {
	result := &ListResult{}

	for _, repo := range uc.repos {
		// Filter by type if specified
		if len(uc.config.FilterType) > 0 {
			found := false
			for _, t := range uc.config.FilterType {
				if t == repo.Type() {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		resources, err := repo.List(ctx, region)
		if err != nil {
			return nil, fmt.Errorf("listing %s resources: %w", repo.Type(), err)
		}

		for _, r := range resources {
			// Filter by status if specified
			if len(uc.config.FilterStatus) > 0 {
				found := false
				for _, s := range uc.config.FilterStatus {
					if s == r.Status() {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			result.Resources = append(result.Resources, r)

			// Update summary
			result.Summary.Total++
			switch r.Status() {
			case domain.StatusUntagged:
				result.Summary.Untagged++
			case domain.StatusActive:
				result.Summary.Active++
			case domain.StatusExpired:
				result.Summary.Expired++
			case domain.StatusNeverExpires:
				result.Summary.NeverExpires++
			}
		}
	}

	return result, nil
}
