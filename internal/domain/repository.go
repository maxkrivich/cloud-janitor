package domain

import (
	"context"
	"time"
)

// ResourceRepository defines operations for managing cloud resources.
// Each resource type (EC2, EBS, etc.) has its own implementation.
type ResourceRepository interface {
	// Type returns the resource type this repository manages.
	Type() ResourceType

	// List returns all resources of this type in the specified region.
	List(ctx context.Context, region string) ([]Resource, error)

	// Tag adds the expiration-date tag to a resource.
	Tag(ctx context.Context, resourceID string, expirationDate time.Time) error

	// Delete removes the resource.
	Delete(ctx context.Context, resourceID string) error
}
