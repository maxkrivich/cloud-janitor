// Package domain contains the core business entities and interfaces.
// This layer has no external dependencies and defines the heart of the application.
package domain

import "time"

// ResourceType identifies the type of cloud resource.
type ResourceType string

const (
	ResourceTypeEC2         ResourceType = "ec2"
	ResourceTypeEBS         ResourceType = "ebs"
	ResourceTypeEBSSnapshot ResourceType = "ebs_snapshot"
	ResourceTypeElasticIP   ResourceType = "elastic_ip"
	ResourceTypeELB         ResourceType = "elb"
	ResourceTypeRDS         ResourceType = "rds"
)

// String returns the string representation of the resource type.
func (rt ResourceType) String() string {
	return string(rt)
}

// Status represents the expiration status of a resource.
type Status int

const (
	// StatusUntagged indicates the resource has no expiration-date tag.
	StatusUntagged Status = iota + 1
	// StatusActive indicates the resource has an expiration date in the future.
	StatusActive
	// StatusExpired indicates the resource's expiration date has passed.
	StatusExpired
	// StatusNeverExpires indicates the resource is marked to never expire.
	StatusNeverExpires
)

// String returns the string representation of the status.
func (s Status) String() string {
	switch s {
	case StatusUntagged:
		return "untagged"
	case StatusActive:
		return "active"
	case StatusExpired:
		return "expired"
	case StatusNeverExpires:
		return "never_expires"
	default:
		return "unknown"
	}
}

// Resource represents an AWS resource with expiration tracking.
type Resource struct {
	// ID is the unique identifier for the resource (e.g., i-0abc123).
	ID string

	// Type is the kind of resource (ec2, ebs, etc.).
	Type ResourceType

	// Region is the AWS region where the resource exists.
	Region string

	// AccountID is the AWS account ID that owns the resource.
	AccountID string

	// Name is the human-readable name of the resource (from Name tag).
	Name string

	// ExpirationDate is when the resource is scheduled for deletion.
	// nil means untagged, will be tagged on next run.
	ExpirationDate *time.Time

	// NeverExpires indicates if the resource is marked to never expire.
	NeverExpires bool

	// CreatedAt is when the resource was created.
	CreatedAt time.Time

	// Tags contains all tags associated with the resource.
	Tags map[string]string
}

// Status returns the expiration status of the resource.
func (r Resource) Status() Status {
	if r.NeverExpires {
		return StatusNeverExpires
	}
	if r.ExpirationDate == nil {
		return StatusUntagged
	}
	if r.ExpirationDate.Before(time.Now()) {
		return StatusExpired
	}
	return StatusActive
}

// DaysUntilExpiration returns the number of days until the resource expires.
// Returns -1 if the resource is untagged or never expires.
// Returns 0 if expired or expiring today.
func (r Resource) DaysUntilExpiration() int {
	if r.ExpirationDate == nil || r.NeverExpires {
		return -1
	}
	days := int(time.Until(*r.ExpirationDate).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// IsExcluded checks if the resource should be excluded from cleanup
// based on the provided exclude tag patterns.
func (r Resource) IsExcluded(excludeTags map[string]string) bool {
	for key, value := range excludeTags {
		if tagValue, ok := r.Tags[key]; ok && tagValue == value {
			return true
		}
	}
	return false
}
