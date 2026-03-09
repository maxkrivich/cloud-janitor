package domain_test

import (
	"errors"
	"testing"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

func TestResourceError_Error(t *testing.T) {
	err := domain.NewResourceError("i-123", domain.ResourceTypeEC2, "tagging", domain.ErrTagFailed)

	want := "tagging ec2 i-123: failed to tag resource"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %v, want %v", got, want)
	}
}

func TestResourceError_Unwrap(t *testing.T) {
	err := domain.NewResourceError("i-123", domain.ResourceTypeEC2, "tagging", domain.ErrTagFailed)

	if !errors.Is(err, domain.ErrTagFailed) {
		t.Errorf("Unwrap() should unwrap to ErrTagFailed")
	}
}
