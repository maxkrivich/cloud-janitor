// Package service contains application services that orchestrate use cases.
package service

import (
	"context"
	"fmt"

	"github.com/maxkrivich/cloud-janitor/internal/app/usecase"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// JanitorConfig holds configuration for the Janitor service.
type JanitorConfig struct {
	// Accounts is the list of AWS accounts to process.
	Accounts []domain.Account

	// Regions is the list of AWS regions to process.
	Regions []string

	// TagConfig is the configuration for tagging.
	TagConfig usecase.TagConfig

	// CleanupConfig is the configuration for cleanup.
	CleanupConfig usecase.CleanupConfig

	// ListConfig is the configuration for listing.
	ListConfig usecase.ListConfig
}

// RegionResult holds the result of processing a single account/region.
// Each worker returns its own RegionResult, avoiding shared state.
type RegionResult struct {
	// AccountID is the AWS account that was processed.
	AccountID string

	// AccountName is the human-readable account name.
	AccountName string

	// Region is the AWS region that was processed.
	Region string

	// TagResult contains the tagging results (nil if tagging was not performed).
	TagResult *usecase.TagResult

	// CleanupResult contains the cleanup results (nil if cleanup was not performed).
	CleanupResult *usecase.CleanupResult

	// Err contains any error that occurred during processing.
	Err error
}

// Key returns a unique identifier for this region result.
func (r RegionResult) Key() string {
	return fmt.Sprintf("%s/%s", r.AccountID, r.Region)
}

// RunResult contains the aggregated results of a full run (tag + cleanup).
// This struct is populated by mergeResults and is not accessed concurrently.
type RunResult struct {
	// TagResults contains tag results per account/region.
	TagResults map[string]*usecase.TagResult

	// CleanupResults contains cleanup results per account/region.
	CleanupResults map[string]*usecase.CleanupResult

	// Errors contains any errors that occurred.
	Errors []error
}

// NewRunResult creates a new RunResult.
func NewRunResult() *RunResult {
	return &RunResult{
		TagResults:     make(map[string]*usecase.TagResult),
		CleanupResults: make(map[string]*usecase.CleanupResult),
	}
}

// TotalTagged returns the total number of tagged resources.
func (r *RunResult) TotalTagged() int {
	total := 0
	for _, tr := range r.TagResults {
		total += len(tr.Tagged)
	}
	return total
}

// TotalDeleted returns the total number of deleted resources.
func (r *RunResult) TotalDeleted() int {
	total := 0
	for _, cr := range r.CleanupResults {
		total += len(cr.Deleted)
	}
	return total
}

// TotalErrors returns the total number of errors.
func (r *RunResult) TotalErrors() int {
	total := len(r.Errors)
	for _, tr := range r.TagResults {
		total += len(tr.Errors)
	}
	for _, cr := range r.CleanupResults {
		total += len(cr.Errors)
	}
	return total
}

// RepositoryFactory creates repositories for a specific account.
type RepositoryFactory func(ctx context.Context, account domain.Account) ([]domain.ResourceRepository, error)

// Janitor orchestrates the tag and cleanup operations across accounts and regions.
type Janitor struct {
	config         JanitorConfig
	notifier       domain.Notifier
	repoFactory    RepositoryFactory
	maxConcurrency int
}

// NewJanitor creates a new Janitor service.
func NewJanitor(
	config JanitorConfig,
	notifier domain.Notifier,
	repoFactory RepositoryFactory,
) *Janitor {
	return &Janitor{
		config:         config,
		notifier:       notifier,
		repoFactory:    repoFactory,
		maxConcurrency: 5,
	}
}

// WithMaxConcurrency sets the maximum number of concurrent operations.
func (j *Janitor) WithMaxConcurrency(n int) *Janitor {
	j.maxConcurrency = n
	return j
}

// Run executes the full tag and cleanup cycle.
func (j *Janitor) Run(ctx context.Context) (*RunResult, error) {
	regionResults := j.processAllRegions(ctx, true, true)
	return mergeResults(regionResults), nil
}

// Tag executes only the tagging operation.
func (j *Janitor) Tag(ctx context.Context) (*RunResult, error) {
	regionResults := j.processAllRegions(ctx, true, false)
	return mergeResults(regionResults), nil
}

// Cleanup executes only the cleanup operation.
func (j *Janitor) Cleanup(ctx context.Context) (*RunResult, error) {
	regionResults := j.processAllRegions(ctx, false, true)
	return mergeResults(regionResults), nil
}

// List returns all resources across accounts and regions.
func (j *Janitor) List(ctx context.Context) (*usecase.ListResult, error) {
	combined := &usecase.ListResult{}

	for _, account := range j.config.Accounts {
		repos, err := j.repoFactory(ctx, account)
		if err != nil {
			return nil, fmt.Errorf("creating repos for account %s: %w", account.ID, err)
		}

		listUC := usecase.NewListResourcesUseCase(repos, j.config.ListConfig)

		for _, region := range j.config.Regions {
			result, err := listUC.Execute(ctx, region)
			if err != nil {
				return nil, fmt.Errorf("listing %s/%s: %w", account.ID, region, err)
			}

			combined.Resources = append(combined.Resources, result.Resources...)
			combined.Summary.Total += result.Summary.Total
			combined.Summary.Untagged += result.Summary.Untagged
			combined.Summary.Active += result.Summary.Active
			combined.Summary.Expired += result.Summary.Expired
			combined.Summary.NeverExpires += result.Summary.NeverExpires
		}
	}

	return combined, nil
}

// processAllRegions processes all account/region combinations and returns isolated results.
func (j *Janitor) processAllRegions(ctx context.Context, doTag, doCleanup bool) []RegionResult {
	var results []RegionResult

	for _, account := range j.config.Accounts {
		for _, region := range j.config.Regions {
			result := j.processRegion(ctx, account, region, doTag, doCleanup)
			results = append(results, result)
		}
	}

	return results
}

// processRegion processes a single account/region and returns an isolated result.
// This function owns all data it creates - no shared state is modified.
func (j *Janitor) processRegion(
	ctx context.Context,
	account domain.Account,
	region string,
	doTag, doCleanup bool,
) RegionResult {
	result := RegionResult{
		AccountID:   account.ID,
		AccountName: account.Name,
		Region:      region,
	}

	repos, err := j.repoFactory(ctx, account)
	if err != nil {
		result.Err = fmt.Errorf("creating repositories: %w", err)
		return result
	}

	// Step 1: Tag untagged resources
	if doTag {
		tagUC := usecase.NewTagResourcesUseCase(repos, j.notifier, j.config.TagConfig)
		tagResult, err := tagUC.Execute(ctx, account.ID, account.Name, region)
		if err != nil {
			result.Err = fmt.Errorf("tagging %s/%s: %w", account.ID, region, err)
			return result
		}
		result.TagResult = tagResult
	}

	// Step 2: Cleanup expired resources
	if doCleanup {
		cleanupUC := usecase.NewCleanupResourcesUseCase(repos, j.notifier, j.config.CleanupConfig)
		cleanupResult, err := cleanupUC.Execute(ctx, account.ID, account.Name, region)
		if err != nil {
			result.Err = fmt.Errorf("cleaning %s/%s: %w", account.ID, region, err)
			return result
		}
		result.CleanupResult = cleanupResult
	}

	return result
}

// mergeResults combines isolated RegionResults into a single RunResult.
// This is a pure function - it only reads from the input slice and writes to a new struct.
func mergeResults(results []RegionResult) *RunResult {
	final := NewRunResult()

	for _, r := range results {
		key := r.Key()

		if r.Err != nil {
			final.Errors = append(final.Errors, r.Err)
			continue
		}

		if r.TagResult != nil {
			final.TagResults[key] = r.TagResult
		}

		if r.CleanupResult != nil {
			final.CleanupResults[key] = r.CleanupResult
		}
	}

	return final
}
