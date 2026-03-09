package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/maxkrivich/cloud-janitor/internal/app/usecase"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// mockRepository implements domain.ResourceRepository for testing.
type mockRepository struct {
	resourceType domain.ResourceType
	resources    []domain.Resource
	tagErr       error
	deleteErr    error
	tagCalls     []tagCall
	deleteCalls  []string
}

type tagCall struct {
	resourceID     string
	expirationDate time.Time
}

func (m *mockRepository) Type() domain.ResourceType {
	return m.resourceType
}

func (m *mockRepository) List(_ context.Context, _ string) ([]domain.Resource, error) {
	return m.resources, nil
}

func (m *mockRepository) Tag(_ context.Context, resourceID string, expirationDate time.Time) error {
	m.tagCalls = append(m.tagCalls, tagCall{resourceID, expirationDate})
	return m.tagErr
}

func (m *mockRepository) Delete(_ context.Context, resourceID string) error {
	m.deleteCalls = append(m.deleteCalls, resourceID)
	return m.deleteErr
}

// mockNotifier implements domain.Notifier for testing.
type mockNotifier struct {
	taggedEvents  []domain.NotificationEvent
	deletedEvents []domain.NotificationEvent
	taggedErr     error
	deletedErr    error
}

func (m *mockNotifier) NotifyTagged(_ context.Context, event domain.NotificationEvent) error {
	m.taggedEvents = append(m.taggedEvents, event)
	return m.taggedErr
}

func (m *mockNotifier) NotifyDeleted(_ context.Context, event domain.NotificationEvent) error {
	m.deletedEvents = append(m.deletedEvents, event)
	return m.deletedErr
}

func TestTagResourcesUseCase_Execute(t *testing.T) {
	now := time.Now()
	future := now.AddDate(0, 0, 10)

	tests := []struct {
		name        string
		resources   []domain.Resource
		config      usecase.TagConfig
		wantTagged  int
		wantSkipped int
	}{
		{
			name: "tags untagged resources",
			resources: []domain.Resource{
				{ID: "i-1", Type: domain.ResourceTypeEC2},
				{ID: "i-2", Type: domain.ResourceTypeEC2},
			},
			config: usecase.TagConfig{
				DefaultDays: 30,
				TagName:     "expiration-date",
			},
			wantTagged:  2,
			wantSkipped: 0,
		},
		{
			name: "skips already tagged resources",
			resources: []domain.Resource{
				{ID: "i-1", Type: domain.ResourceTypeEC2},
				{ID: "i-2", Type: domain.ResourceTypeEC2, ExpirationDate: &future},
			},
			config: usecase.TagConfig{
				DefaultDays: 30,
				TagName:     "expiration-date",
			},
			wantTagged:  1,
			wantSkipped: 1,
		},
		{
			name: "skips resources marked never expire",
			resources: []domain.Resource{
				{ID: "i-1", Type: domain.ResourceTypeEC2},
				{ID: "i-2", Type: domain.ResourceTypeEC2, NeverExpires: true},
			},
			config: usecase.TagConfig{
				DefaultDays: 30,
				TagName:     "expiration-date",
			},
			wantTagged:  1,
			wantSkipped: 1,
		},
		{
			name: "skips excluded resources",
			resources: []domain.Resource{
				{ID: "i-1", Type: domain.ResourceTypeEC2, Tags: map[string]string{}},
				{ID: "i-2", Type: domain.ResourceTypeEC2, Tags: map[string]string{"Environment": "production"}},
			},
			config: usecase.TagConfig{
				DefaultDays: 30,
				TagName:     "expiration-date",
				ExcludeTags: map[string]string{"Environment": "production"},
			},
			wantTagged:  1,
			wantSkipped: 1,
		},
		{
			name: "dry run does not tag",
			resources: []domain.Resource{
				{ID: "i-1", Type: domain.ResourceTypeEC2},
			},
			config: usecase.TagConfig{
				DefaultDays: 30,
				TagName:     "expiration-date",
				DryRun:      true,
			},
			wantTagged:  1, // Still counted as tagged
			wantSkipped: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				resourceType: domain.ResourceTypeEC2,
				resources:    tt.resources,
			}
			notifier := &mockNotifier{}

			uc := usecase.NewTagResourcesUseCase(
				[]domain.ResourceRepository{repo},
				notifier,
				tt.config,
			)

			result, err := uc.Execute(context.Background(), "123456789012", "test-account", "us-east-1")
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			if len(result.Tagged) != tt.wantTagged {
				t.Errorf("Tagged = %d, want %d", len(result.Tagged), tt.wantTagged)
			}

			if len(result.Skipped) != tt.wantSkipped {
				t.Errorf("Skipped = %d, want %d", len(result.Skipped), tt.wantSkipped)
			}

			// Verify Tag was called (unless dry run)
			if !tt.config.DryRun && len(repo.tagCalls) != tt.wantTagged {
				t.Errorf("Tag calls = %d, want %d", len(repo.tagCalls), tt.wantTagged)
			}

			// Verify notification was sent (unless dry run or no tagged resources)
			if !tt.config.DryRun && tt.wantTagged > 0 && len(notifier.taggedEvents) != 1 {
				t.Errorf("NotifyTagged calls = %d, want 1", len(notifier.taggedEvents))
			}
		})
	}
}

func TestTagResourcesUseCase_Execute_TagError(t *testing.T) {
	repo := &mockRepository{
		resourceType: domain.ResourceTypeEC2,
		resources: []domain.Resource{
			{ID: "i-1", Type: domain.ResourceTypeEC2},
			{ID: "i-2", Type: domain.ResourceTypeEC2},
		},
		tagErr: errors.New("tag failed"),
	}
	notifier := &mockNotifier{}

	uc := usecase.NewTagResourcesUseCase(
		[]domain.ResourceRepository{repo},
		notifier,
		usecase.TagConfig{DefaultDays: 30},
	)

	result, err := uc.Execute(context.Background(), "123456789012", "test-account", "us-east-1")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Both resources should have errors
	if len(result.Errors) != 2 {
		t.Errorf("Errors = %d, want 2", len(result.Errors))
	}

	// No resources should be tagged
	if len(result.Tagged) != 0 {
		t.Errorf("Tagged = %d, want 0", len(result.Tagged))
	}
}

func TestCleanupResourcesUseCase_Execute(t *testing.T) {
	now := time.Now()
	past := now.AddDate(0, 0, -1)
	future := now.AddDate(0, 0, 10)

	tests := []struct {
		name        string
		resources   []domain.Resource
		config      usecase.CleanupConfig
		wantDeleted int
		wantSkipped int
	}{
		{
			name: "deletes expired resources",
			resources: []domain.Resource{
				{ID: "i-1", Type: domain.ResourceTypeEC2, ExpirationDate: &past},
				{ID: "i-2", Type: domain.ResourceTypeEC2, ExpirationDate: &past},
			},
			config:      usecase.CleanupConfig{},
			wantDeleted: 2,
			wantSkipped: 0,
		},
		{
			name: "skips active resources",
			resources: []domain.Resource{
				{ID: "i-1", Type: domain.ResourceTypeEC2, ExpirationDate: &past},
				{ID: "i-2", Type: domain.ResourceTypeEC2, ExpirationDate: &future},
			},
			config:      usecase.CleanupConfig{},
			wantDeleted: 1,
			wantSkipped: 1,
		},
		{
			name: "skips untagged resources",
			resources: []domain.Resource{
				{ID: "i-1", Type: domain.ResourceTypeEC2, ExpirationDate: &past},
				{ID: "i-2", Type: domain.ResourceTypeEC2},
			},
			config:      usecase.CleanupConfig{},
			wantDeleted: 1,
			wantSkipped: 1,
		},
		{
			name: "skips excluded resources",
			resources: []domain.Resource{
				{ID: "i-1", Type: domain.ResourceTypeEC2, ExpirationDate: &past, Tags: map[string]string{}},
				{ID: "i-2", Type: domain.ResourceTypeEC2, ExpirationDate: &past, Tags: map[string]string{"DoNotDelete": "true"}},
			},
			config: usecase.CleanupConfig{
				ExcludeTags: map[string]string{"DoNotDelete": "true"},
			},
			wantDeleted: 1,
			wantSkipped: 1,
		},
		{
			name: "dry run does not delete",
			resources: []domain.Resource{
				{ID: "i-1", Type: domain.ResourceTypeEC2, ExpirationDate: &past},
			},
			config: usecase.CleanupConfig{
				DryRun: true,
			},
			wantDeleted: 1, // Still counted as deleted
			wantSkipped: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				resourceType: domain.ResourceTypeEC2,
				resources:    tt.resources,
			}
			notifier := &mockNotifier{}

			uc := usecase.NewCleanupResourcesUseCase(
				[]domain.ResourceRepository{repo},
				notifier,
				tt.config,
			)

			result, err := uc.Execute(context.Background(), "123456789012", "test-account", "us-east-1")
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			if len(result.Deleted) != tt.wantDeleted {
				t.Errorf("Deleted = %d, want %d", len(result.Deleted), tt.wantDeleted)
			}

			if len(result.Skipped) != tt.wantSkipped {
				t.Errorf("Skipped = %d, want %d", len(result.Skipped), tt.wantSkipped)
			}

			// Verify Delete was called (unless dry run)
			if !tt.config.DryRun && len(repo.deleteCalls) != tt.wantDeleted {
				t.Errorf("Delete calls = %d, want %d", len(repo.deleteCalls), tt.wantDeleted)
			}
		})
	}
}

func TestListResourcesUseCase_Execute(t *testing.T) {
	now := time.Now()
	past := now.AddDate(0, 0, -1)
	future := now.AddDate(0, 0, 10)

	tests := []struct {
		name      string
		resources []domain.Resource
		config    usecase.ListConfig
		wantCount int
	}{
		{
			name: "lists all resources",
			resources: []domain.Resource{
				{ID: "i-1", Type: domain.ResourceTypeEC2},
				{ID: "i-2", Type: domain.ResourceTypeEC2, ExpirationDate: &future},
				{ID: "i-3", Type: domain.ResourceTypeEC2, ExpirationDate: &past},
			},
			config:    usecase.ListConfig{},
			wantCount: 3,
		},
		{
			name: "filters by status",
			resources: []domain.Resource{
				{ID: "i-1", Type: domain.ResourceTypeEC2},
				{ID: "i-2", Type: domain.ResourceTypeEC2, ExpirationDate: &future},
				{ID: "i-3", Type: domain.ResourceTypeEC2, ExpirationDate: &past},
			},
			config: usecase.ListConfig{
				FilterStatus: []domain.Status{domain.StatusExpired},
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				resourceType: domain.ResourceTypeEC2,
				resources:    tt.resources,
			}

			uc := usecase.NewListResourcesUseCase(
				[]domain.ResourceRepository{repo},
				tt.config,
			)

			result, err := uc.Execute(context.Background(), "us-east-1")
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			if len(result.Resources) != tt.wantCount {
				t.Errorf("Resources = %d, want %d", len(result.Resources), tt.wantCount)
			}
		})
	}
}
