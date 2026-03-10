package domain

import (
	"errors"
	"fmt"
)

// Domain errors.
var (
	// ErrResourceNotFound indicates a resource was not found.
	ErrResourceNotFound = errors.New("resource not found")

	// ErrTagFailed indicates tagging a resource was unsuccessful.
	ErrTagFailed = errors.New("tag operation unsuccessful")

	// ErrDeleteFailed indicates deleting a resource was unsuccessful.
	ErrDeleteFailed = errors.New("delete operation unsuccessful")

	// ErrNotificationFailed indicates sending a notification was unsuccessful.
	ErrNotificationFailed = errors.New("notification delivery unsuccessful")
)

// ResourceError represents an error that occurred while processing a specific resource.
type ResourceError struct {
	ResourceID   string
	ResourceType ResourceType
	Operation    string
	Err          error
}

// Error returns a user-friendly error message with resource ID prominent.
// Format: "operation resourceID (resourceType): error"
func (e *ResourceError) Error() string {
	return fmt.Sprintf("%s %s (%s): %s", e.Operation, e.ResourceID, e.ResourceType, e.Err.Error())
}

func (e *ResourceError) Unwrap() error {
	return e.Err
}

// NewResourceError creates a new ResourceError.
func NewResourceError(resourceID string, resourceType ResourceType, operation string, err error) *ResourceError {
	return &ResourceError{
		ResourceID:   resourceID,
		ResourceType: resourceType,
		Operation:    operation,
		Err:          err,
	}
}
