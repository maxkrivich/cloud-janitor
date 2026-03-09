package domain

import "errors"

// Domain errors.
var (
	// ErrResourceNotFound indicates a resource was not found.
	ErrResourceNotFound = errors.New("resource not found")

	// ErrTagFailed indicates tagging a resource failed.
	ErrTagFailed = errors.New("failed to tag resource")

	// ErrDeleteFailed indicates deleting a resource failed.
	ErrDeleteFailed = errors.New("failed to delete resource")

	// ErrNotificationFailed indicates sending a notification failed.
	ErrNotificationFailed = errors.New("failed to send notification")
)

// ResourceError represents an error that occurred while processing a specific resource.
type ResourceError struct {
	ResourceID   string
	ResourceType ResourceType
	Operation    string
	Err          error
}

func (e *ResourceError) Error() string {
	return e.Operation + " " + string(e.ResourceType) + " " + e.ResourceID + ": " + e.Err.Error()
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
