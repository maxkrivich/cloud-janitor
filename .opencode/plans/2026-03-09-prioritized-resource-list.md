# Plan: Prioritized Resource List (80/20 Cost Savings)

**Status**: `implemented`
**Created**: 2026-03-09
**Author**: AI Assistant

## Problem

Different cloud providers have different pricing models. Not all resources have equal cost impact. To maximize cost savings with minimal effort (80/20 rule), Cloud Janitor should focus on the most expensive resources first.

## Solution

Create a hardcoded, prioritized list of resource types per cloud provider, ordered by typical cost impact in development accounts. This list:
- Includes both **compute resources** (hourly costs) and **storage resources** (monthly costs)
- Is easily extensible for future resource types
- Lives in the domain layer as it represents core business logic

## AWS Resource Priority List

Based on typical dev account waste patterns and AWS pricing:

| Priority | Resource Type | Cost Type | Why Expensive | Est. Monthly Cost |
|----------|--------------|-----------|---------------|-------------------|
| 1 | EC2 Instances | Hourly | Charged even when idle | $50-500+ |
| 2 | RDS Instances | Hourly | Database instances are expensive | $100-1000+ |
| 3 | Elastic IPs (unattached) | Hourly | $0.005/hr when NOT attached | $3.60 each |
| 4 | EBS Volumes (unattached) | Storage | Persist after instance termination | $10-100 |
| 5 | Load Balancers (ALB/NLB) | Hourly | $16+/month minimum | $20-50 each |
| 6 | EBS Snapshots | Storage | Often forgotten, accumulate | $5-50 |
| 7 | ECR Images | Storage | Old images pile up | $1-20 |
| 8 | AMIs | Storage | Custom images rarely cleaned | $1-10 |

**Note**: NAT Gateways excluded from initial scope (very expensive but complex dependencies).

## GCP Resource Priority List (Future)

| Priority | Resource Type | Cost Type | Why Expensive |
|----------|--------------|-----------|---------------|
| 1 | Compute Instances | Hourly | Same as EC2 |
| 2 | Cloud SQL Instances | Hourly | Same as RDS |
| 3 | Static IPs (unattached) | Hourly | Similar to Elastic IP |
| 4 | Persistent Disks | Storage | Same as EBS |
| 5 | Snapshots | Storage | Same as EBS Snapshots |

## Azure Resource Priority List (Future)

| Priority | Resource Type | Cost Type | Why Expensive |
|----------|--------------|-----------|---------------|
| 1 | Virtual Machines | Hourly | Same as EC2 |
| 2 | Azure SQL | Hourly | Same as RDS |
| 3 | Public IPs (unassociated) | Hourly | Similar to Elastic IP |
| 4 | Managed Disks | Storage | Same as EBS |
| 5 | Snapshots | Storage | Same as EBS Snapshots |

## Implementation

### Phase 1: Documentation (This PR)
- [x] Update PRODUCT.md with prioritized resource list rationale
- [x] Update ARCHITECTURE.md with resource type definitions

### Phase 2: Code Implementation
- [x] Add `CostCategory` enum (Compute, Storage) to domain
- [x] Add `CloudProvider` type to domain  
- [x] Add provider-prefixed `ResourceType` constants
- [x] Create `ResourceCatalog` in domain with hardcoded lists
- [x] Add unit tests for ResourceCatalog
- [ ] Update scanners config to use catalog (future - when adding new scanners)

## Architecture Changes

### Domain Layer Addition

```go
// internal/domain/resource_catalog.go

// CostCategory indicates how the resource is billed
type CostCategory string

const (
    CostCategoryCompute CostCategory = "compute" // Hourly charges
    CostCategoryStorage CostCategory = "storage" // Monthly/GB charges
)

// ResourceDefinition describes a cloud resource type
type ResourceDefinition struct {
    Type         ResourceType
    Provider     CloudProvider
    CostCategory CostCategory
    Priority     int    // Lower = more expensive (1 = highest priority)
    Description  string
}

// ResourceCatalog provides the prioritized list of supported resources
type ResourceCatalog struct {
    definitions []ResourceDefinition
}

// AWSResources returns AWS resources ordered by cost priority
func (c *ResourceCatalog) AWSResources() []ResourceDefinition {
    return []ResourceDefinition{
        {ResourceTypeAWSEC2, ProviderAWS, CostCategoryCompute, 1, "EC2 Instances"},
        {ResourceTypeAWSRDS, ProviderAWS, CostCategoryCompute, 2, "RDS Instances"},
        {ResourceTypeAWSElasticIP, ProviderAWS, CostCategoryCompute, 3, "Elastic IPs"},
        {ResourceTypeAWSEBS, ProviderAWS, CostCategoryStorage, 4, "EBS Volumes"},
        {ResourceTypeAWSELB, ProviderAWS, CostCategoryCompute, 5, "Load Balancers"},
        {ResourceTypeAWSSnapshot, ProviderAWS, CostCategoryStorage, 6, "EBS Snapshots"},
        {ResourceTypeAWSECR, ProviderAWS, CostCategoryStorage, 7, "ECR Images"},
        {ResourceTypeAWSAMI, ProviderAWS, CostCategoryStorage, 8, "AMIs"},
    }
}
```

## Non-Goals

- Priority does not affect processing order (all resources processed the same way)
- No cost estimation or calculation
- No dynamic priority based on actual resource size/usage

## Success Criteria

- Clear documentation of which resources are supported and why
- Easy to add new resource types in the future
- Developers understand the 80/20 rationale
