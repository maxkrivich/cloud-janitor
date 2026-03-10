package domain_test

import (
	"errors"
	"testing"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

func TestResourceError_Error(t *testing.T) {
	err := domain.NewResourceError("i-123", domain.ResourceTypeEC2, "tagging", domain.ErrTagFailed)

	// Phase 2: ResourceError format puts resource ID first, type in parentheses
	// Format: "operation resourceID (resourceType): error"
	want := "tagging i-123 (ec2): tag operation unsuccessful"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %v, want %v", got, want)
	}
}

func TestResourceError_Error_Format(t *testing.T) {
	tests := []struct {
		name         string
		resourceID   string
		resourceType domain.ResourceType
		operation    string
		err          error
		want         string
	}{
		{
			name:         "EC2 instance tagging",
			resourceID:   "i-1234567890abcdef0",
			resourceType: domain.ResourceTypeEC2,
			operation:    "tagging",
			err:          errors.New("access denied"),
			want:         "tagging i-1234567890abcdef0 (ec2): access denied",
		},
		{
			name:         "EBS volume deletion",
			resourceID:   "vol-0abc123def456789",
			resourceType: domain.ResourceTypeEBS,
			operation:    "deleting",
			err:          errors.New("volume in use"),
			want:         "deleting vol-0abc123def456789 (ebs): volume in use",
		},
		{
			name:         "RDS instance with long ID",
			resourceID:   "my-production-database-instance",
			resourceType: domain.ResourceTypeRDS,
			operation:    "listing",
			err:          errors.New("throttling"),
			want:         "listing my-production-database-instance (rds): throttling",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.NewResourceError(tt.resourceID, tt.resourceType, tt.operation, tt.err)
			got := err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResourceError_Unwrap(t *testing.T) {
	err := domain.NewResourceError("i-123", domain.ResourceTypeEC2, "tagging", domain.ErrTagFailed)

	if !errors.Is(err, domain.ErrTagFailed) {
		t.Errorf("Unwrap() should unwrap to ErrTagFailed")
	}
}

func TestSentinelErrors_NoFailedToPrefix(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "ErrResourceNotFound",
			err:  domain.ErrResourceNotFound,
			want: "resource not found",
		},
		{
			name: "ErrTagFailed",
			err:  domain.ErrTagFailed,
			want: "tag operation unsuccessful",
		},
		{
			name: "ErrDeleteFailed",
			err:  domain.ErrDeleteFailed,
			want: "delete operation unsuccessful",
		},
		{
			name: "ErrNotificationFailed",
			err:  domain.ErrNotificationFailed,
			want: "notification delivery unsuccessful",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("%s.Error() = %q, want %q", tt.name, got, tt.want)
			}
			// Verify no "failed to" prefix per Google guidelines
			if len(got) >= 9 && got[:9] == "failed to" {
				t.Errorf("%s should not start with 'failed to'", tt.name)
			}
		})
	}
}
