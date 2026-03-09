// Package domain contains the core business entities and interfaces.
// This layer has no external dependencies and defines the heart of the application.
package domain

import "time"

// ResourceType identifies the type of cloud resource.
// Resource types are prefixed with the provider name for clarity.
type ResourceType string

const (
	// AWS resource types (ordered by cost priority, 1 = most expensive).
	ResourceTypeAWSEC2         ResourceType = "aws:ec2"         // Priority 1: Compute - EC2 instances
	ResourceTypeAWSRDS         ResourceType = "aws:rds"         // Priority 2: Compute - RDS database instances
	ResourceTypeAWSNATGateway  ResourceType = "aws:nat-gateway" // Priority 3: Compute - NAT Gateways
	ResourceTypeAWSELB         ResourceType = "aws:elb"         // Priority 4: Compute - Load Balancers (ALB/NLB)
	ResourceTypeAWSElasticIP   ResourceType = "aws:eip"         // Priority 5: Compute - Elastic IPs
	ResourceTypeAWSElastiCache ResourceType = "aws:elasticache" // Priority 6: Compute - ElastiCache clusters
	ResourceTypeAWSOpenSearch  ResourceType = "aws:opensearch"  // Priority 7: Compute - OpenSearch domains
	ResourceTypeAWSEKS         ResourceType = "aws:eks"         // Priority 8: Compute - EKS clusters
	ResourceTypeAWSRedshift    ResourceType = "aws:redshift"    // Priority 9: Compute - Redshift clusters
	ResourceTypeAWSSageMaker   ResourceType = "aws:sagemaker"   // Priority 10: Compute - SageMaker notebooks
	ResourceTypeAWSEBS         ResourceType = "aws:ebs"         // Priority 11: Storage - EBS volumes
	ResourceTypeAWSSnapshot    ResourceType = "aws:snapshot"    // Priority 12: Storage - EBS snapshots
	ResourceTypeAWSAMI         ResourceType = "aws:ami"         // Priority 13: Storage - AMIs
	ResourceTypeAWSLogs        ResourceType = "aws:logs"        // Priority 14: Storage - CloudWatch Log Groups
	ResourceTypeAWSECR         ResourceType = "aws:ecr"         // Priority 15: Storage - ECR images (not implemented)

	// GCP resource types (ordered by cost priority).
	ResourceTypeGCPInstance ResourceType = "gcp:compute-instance" // Priority 1: Compute
	ResourceTypeGCPCloudSQL ResourceType = "gcp:cloud-sql"        // Priority 2: Compute
	ResourceTypeGCPStaticIP ResourceType = "gcp:static-ip"        // Priority 3: Compute
	ResourceTypeGCPDisk     ResourceType = "gcp:disk"             // Priority 4: Storage
	ResourceTypeGCPSnapshot ResourceType = "gcp:snapshot"         // Priority 5: Storage

	// Azure resource types (ordered by cost priority).
	ResourceTypeAzureVM       ResourceType = "azure:vm"        // Priority 1: Compute
	ResourceTypeAzureSQL      ResourceType = "azure:sql"       // Priority 2: Compute
	ResourceTypeAzurePublicIP ResourceType = "azure:public-ip" // Priority 3: Compute
	ResourceTypeAzureDisk     ResourceType = "azure:disk"      // Priority 4: Storage
	ResourceTypeAzureSnapshot ResourceType = "azure:snapshot"  // Priority 5: Storage

	// Legacy resource types (deprecated, use provider-prefixed types).
	// These are kept for backward compatibility with existing code.
	ResourceTypeEC2         ResourceType = "ec2"
	ResourceTypeEBS         ResourceType = "ebs"
	ResourceTypeEBSSnapshot ResourceType = "ebs_snapshot"
	ResourceTypeElasticIP   ResourceType = "elastic_ip"
	ResourceTypeELB         ResourceType = "elb"
	ResourceTypeRDS         ResourceType = "rds"
	ResourceTypeNATGateway  ResourceType = "nat_gateway"
	ResourceTypeElastiCache ResourceType = "elasticache"
	ResourceTypeOpenSearch  ResourceType = "opensearch"
	ResourceTypeEKS         ResourceType = "eks"
	ResourceTypeRedshift    ResourceType = "redshift"
	ResourceTypeSageMaker   ResourceType = "sagemaker"
	ResourceTypeLogs        ResourceType = "logs"
	ResourceTypeAMI         ResourceType = "ami"
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
