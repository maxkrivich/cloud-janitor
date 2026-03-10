# AWS Integration Tests Implementation Plan

## Overview

Add comprehensive integration tests for Cloud Janitor that validate the complete workflow against a real AWS test account. Tests will cover all 14 AWS scanners, the complete tag-and-cleanup lifecycle, and exclusion filter behavior.

## Current State Analysis

- **No integration tests exist** - only unit tests with hand-written mocks in `internal/infra/aws/*_test.go`
- **14 scanner repositories** implemented in `internal/infra/aws/`
- **Well-designed for testing** - `RepositoryFactory` and `ClientFactory` allow dependency injection
- **Test patterns established** - table-driven tests, compile-time interface checks

## Desired End State

A complete integration test suite that:
1. Creates real AWS resources with test-identifying tags
2. Validates detect → tag → expire → delete workflow
3. Verifies exclusion filters (`DoNotDelete=true`, `expiration-date=never`) work correctly
4. Guarantees cleanup even on test failures (cost control)
5. Can be run locally with `make test-integration`

### Success Verification
```bash
# All tests pass
TEST_AWS_ACCOUNT_ID=123456789012 make test-integration

# Fast subset passes
TEST_AWS_ACCOUNT_ID=123456789012 make test-integration-fast

# No resources leaked (check after tests)
aws resourcegroupstaggingapi get-resources \
  --tag-filters Key=cloud-janitor-test,Values=true \
  --region us-west-2
```

## What We're NOT Doing

- **CI integration** - tests are local-only initially
- **GCP/Azure** - AWS only
- **Terraform/IaC setup** - tests create their own VPC infrastructure
- **Cost optimization** - prioritizing correctness over minimal resource usage
- **Mocking AWS APIs** - using real AWS for true integration testing

## Implementation Approach

1. **Infrastructure first** - build test framework, cleanup registry, VPC setup
2. **Fast scanners** - EIP, Logs (seconds to create, validate patterns work)
3. **Incrementally add scanners** - from fast to slow, one at a time
4. **Complete workflow test** - end-to-end Janitor service validation
5. **Documentation** - README with IAM policy and setup instructions

---

## Test Pattern: Create → Detect → Tag → Delete

Each integration test follows a consistent pattern that validates the complete Cloud Janitor lifecycle:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Integration Test Flow                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  1. CREATE        Create real AWS resource with test-identifying tags   │
│      │            Register cleanup function immediately                 │
│      │            Wait for resource to be ready                         │
│      ▼                                                                  │
│  2. DETECT        Use repository.List() to find the resource            │
│      │            Verify Status() == StatusUntagged                     │
│      ▼                                                                  │
│  3. TAG           Use repository.Tag() to add expiration-date           │
│      │            Verify Status() == StatusActive                       │
│      ▼                                                                  │
│  4. DELETE        Use repository.Delete() to remove resource            │
│      │            Verify resource no longer appears in List()           │
│      ▼                                                                  │
│  5. CLEANUP       Automatic via cleanup registry (even on failure)      │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Why Create Resources First?

The scanners (repositories) are designed to **detect existing resources** - they don't create anything. To test that detection works correctly, we must:

1. **Create resources ourselves** using AWS SDK directly
2. **Tag them** with `cloud-janitor-test=true` for identification
3. **Register cleanup** immediately after creation (before any assertions)
4. **Then test** the scanner's List/Tag/Delete operations

### Resource Creation Helpers

Each test creates resources directly, but we provide helper functions for common patterns.

**File**: `tests/integration/resource_helpers_test.go`

```go
//go:build integration

package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

// CreateTestResourceResult contains the created resource ID and its cleanup function.
type CreateTestResourceResult struct {
	ID      string
	Cleanup func(ctx context.Context) error
}

// --- Elastic IP ---

// CreateTestEIP allocates an Elastic IP for testing.
// Returns the allocation ID and registers cleanup automatically.
func CreateTestEIP(ctx context.Context, cleanup *CleanupRegistry, extraTags map[string]string) (*CreateTestResourceResult, error) {
	tags := testTags()
	for k, v := range extraTags {
		tags[k] = v
	}

	output, err := clients.EC2.AllocateAddress(ctx, &ec2.AllocateAddressInput{
		Domain: types.DomainTypeVpc,
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeElasticIp,
				Tags:         toEC2Tags(tags),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("allocating EIP: %w", err)
	}

	eipID := *output.AllocationId
	cleanupFn := func(ctx context.Context) error {
		_, err := clients.EC2.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{
			AllocationId: aws.String(eipID),
		})
		return err
	}

	cleanup.Register("EIP "+eipID, PriorityElasticIP, cleanupFn)

	return &CreateTestResourceResult{ID: eipID, Cleanup: cleanupFn}, nil
}

// --- CloudWatch Log Group ---

// CreateTestLogGroup creates a CloudWatch Log Group for testing.
func CreateTestLogGroup(ctx context.Context, cleanup *CleanupRegistry, namePrefix string, extraTags map[string]string) (*CreateTestResourceResult, error) {
	tags := map[string]string{testTagKey: testTagValue}
	for k, v := range extraTags {
		tags[k] = v
	}

	logGroupName := fmt.Sprintf("%s/%d", namePrefix, time.Now().UnixNano())

	_, err := clients.Logs.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(logGroupName),
		Tags:         tags,
	})
	if err != nil {
		return nil, fmt.Errorf("creating log group: %w", err)
	}

	cleanupFn := func(ctx context.Context) error {
		_, err := clients.Logs.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{
			LogGroupName: aws.String(logGroupName),
		})
		return err
	}

	cleanup.Register("LogGroup "+logGroupName, PriorityLogGroup, cleanupFn)

	return &CreateTestResourceResult{ID: logGroupName, Cleanup: cleanupFn}, nil
}

// --- EBS Volume ---

// CreateTestEBSVolume creates an EBS volume for testing.
// Waits for the volume to be available before returning.
func CreateTestEBSVolume(ctx context.Context, cleanup *CleanupRegistry, az string, extraTags map[string]string) (*CreateTestResourceResult, error) {
	tags := testTags()
	for k, v := range extraTags {
		tags[k] = v
	}

	output, err := clients.EC2.CreateVolume(ctx, &ec2.CreateVolumeInput{
		AvailabilityZone: aws.String(az),
		Size:             aws.Int32(1), // 1 GB minimum
		VolumeType:       types.VolumeTypeGp3,
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeVolume,
				Tags:         toEC2Tags(tags),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating volume: %w", err)
	}

	volumeID := *output.VolumeId
	cleanupFn := func(ctx context.Context) error {
		_, err := clients.EC2.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
			VolumeId: aws.String(volumeID),
		})
		return err
	}

	cleanup.Register("Volume "+volumeID, PriorityEBS, cleanupFn)

	// Wait for volume to be available
	err = waitFor(ctx, func() (bool, error) {
		desc, err := clients.EC2.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
			VolumeIds: []string{volumeID},
		})
		if err != nil {
			return false, err
		}
		return desc.Volumes[0].State == types.VolumeStateAvailable, nil
	}, 2*time.Second, 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("waiting for volume %s: %w", volumeID, err)
	}

	return &CreateTestResourceResult{ID: volumeID, Cleanup: cleanupFn}, nil
}

// --- EBS Snapshot ---

// CreateTestSnapshot creates an EBS snapshot from a volume.
// Waits for the snapshot to complete before returning.
func CreateTestSnapshot(ctx context.Context, cleanup *CleanupRegistry, volumeID string, extraTags map[string]string) (*CreateTestResourceResult, error) {
	tags := testTags()
	for k, v := range extraTags {
		tags[k] = v
	}

	output, err := clients.EC2.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
		VolumeId: aws.String(volumeID),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeSnapshot,
				Tags:         toEC2Tags(tags),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating snapshot: %w", err)
	}

	snapshotID := *output.SnapshotId
	cleanupFn := func(ctx context.Context) error {
		_, err := clients.EC2.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
			SnapshotId: aws.String(snapshotID),
		})
		return err
	}

	cleanup.Register("Snapshot "+snapshotID, PrioritySnapshot, cleanupFn)

	// Wait for snapshot to complete
	err = waitFor(ctx, func() (bool, error) {
		desc, err := clients.EC2.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{
			SnapshotIds: []string{snapshotID},
		})
		if err != nil {
			return false, err
		}
		return desc.Snapshots[0].State == types.SnapshotStateCompleted, nil
	}, 5*time.Second, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("waiting for snapshot %s: %w", snapshotID, err)
	}

	return &CreateTestResourceResult{ID: snapshotID, Cleanup: cleanupFn}, nil
}

// --- EC2 Instance ---

// CreateTestEC2Instance launches an EC2 instance for testing.
// Waits for the instance to be running before returning.
func CreateTestEC2Instance(ctx context.Context, cleanup *CleanupRegistry, subnetID string, extraTags map[string]string) (*CreateTestResourceResult, error) {
	tags := testTags()
	for k, v := range extraTags {
		tags[k] = v
	}

	// Find Amazon Linux 2 AMI
	amiOutput, err := clients.EC2.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Owners: []string{"amazon"},
		Filters: []types.Filter{
			{Name: aws.String("name"), Values: []string{"amzn2-ami-hvm-*-x86_64-gp2"}},
			{Name: aws.String("state"), Values: []string{"available"}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("finding AMI: %w", err)
	}
	if len(amiOutput.Images) == 0 {
		return nil, fmt.Errorf("no suitable AMI found")
	}
	amiID := *amiOutput.Images[0].ImageId

	output, err := clients.EC2.RunInstances(ctx, &ec2.RunInstancesInput{
		ImageId:      aws.String(amiID),
		InstanceType: types.InstanceTypeT3Micro,
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
		SubnetId:     aws.String(subnetID),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeInstance,
				Tags:         toEC2Tags(tags),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("launching instance: %w", err)
	}

	instanceID := *output.Instances[0].InstanceId
	cleanupFn := func(ctx context.Context) error {
		_, err := clients.EC2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
			InstanceIds: []string{instanceID},
		})
		return err
	}

	cleanup.Register("EC2 "+instanceID, PriorityEC2, cleanupFn)

	// Wait for instance to be running
	err = waitFor(ctx, func() (bool, error) {
		desc, err := clients.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: []string{instanceID},
		})
		if err != nil {
			return false, err
		}
		state := desc.Reservations[0].Instances[0].State.Name
		return state == types.InstanceStateNameRunning, nil
	}, 5*time.Second, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("waiting for instance %s: %w", instanceID, err)
	}

	return &CreateTestResourceResult{ID: instanceID, Cleanup: cleanupFn}, nil
}

// --- NAT Gateway ---

// CreateTestNATGateway creates a NAT Gateway for testing.
// Also creates the required EIP. Waits for NAT Gateway to be available.
func CreateTestNATGateway(ctx context.Context, cleanup *CleanupRegistry, subnetID string, extraTags map[string]string) (*CreateTestResourceResult, error) {
	// First create EIP for NAT Gateway
	eipResult, err := CreateTestEIP(ctx, cleanup, nil)
	if err != nil {
		return nil, fmt.Errorf("creating EIP for NAT: %w", err)
	}

	tags := testTags()
	for k, v := range extraTags {
		tags[k] = v
	}

	output, err := clients.EC2.CreateNatGateway(ctx, &ec2.CreateNatGatewayInput{
		AllocationId: aws.String(eipResult.ID),
		SubnetId:     aws.String(subnetID),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeNatgateway,
				Tags:         toEC2Tags(tags),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating NAT Gateway: %w", err)
	}

	natID := *output.NatGateway.NatGatewayId
	cleanupFn := func(ctx context.Context) error {
		_, err := clients.EC2.DeleteNatGateway(ctx, &ec2.DeleteNatGatewayInput{
			NatGatewayId: aws.String(natID),
		})
		if err != nil {
			return err
		}
		// Wait for deletion before releasing EIP
		return waitFor(ctx, func() (bool, error) {
			desc, err := clients.EC2.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{
				NatGatewayIds: []string{natID},
			})
			if err != nil {
				return false, err
			}
			if len(desc.NatGateways) == 0 {
				return true, nil
			}
			return desc.NatGateways[0].State == types.NatGatewayStateDeleted, nil
		}, 5*time.Second, 5*time.Minute)
	}

	cleanup.Register("NAT "+natID, PriorityNATGateway, cleanupFn)

	// Wait for NAT Gateway to be available
	err = waitFor(ctx, func() (bool, error) {
		desc, err := clients.EC2.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{
			NatGatewayIds: []string{natID},
		})
		if err != nil {
			return false, err
		}
		return desc.NatGateways[0].State == types.NatGatewayStateAvailable, nil
	}, 10*time.Second, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("waiting for NAT Gateway %s: %w", natID, err)
	}

	return &CreateTestResourceResult{ID: natID, Cleanup: cleanupFn}, nil
}

// --- Application Load Balancer ---

// CreateTestALB creates an Application Load Balancer for testing.
// Waits for ALB to be active before returning.
func CreateTestALB(ctx context.Context, cleanup *CleanupRegistry, subnetIDs []string, extraTags map[string]string) (*CreateTestResourceResult, error) {
	tags := []elbv2types.Tag{
		{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
	}
	for k, v := range extraTags {
		tags = append(tags, elbv2types.Tag{Key: aws.String(k), Value: aws.String(v)})
	}

	albName := fmt.Sprintf("cj-test-%d", time.Now().Unix()%100000)

	output, err := clients.ELBv2.CreateLoadBalancer(ctx, &elbv2.CreateLoadBalancerInput{
		Name:    aws.String(albName),
		Type:    elbv2types.LoadBalancerTypeEnumApplication,
		Scheme:  elbv2types.LoadBalancerSchemeEnumInternal,
		Subnets: subnetIDs,
		Tags:    tags,
	})
	if err != nil {
		return nil, fmt.Errorf("creating ALB: %w", err)
	}

	albARN := *output.LoadBalancers[0].LoadBalancerArn
	cleanupFn := func(ctx context.Context) error {
		_, err := clients.ELBv2.DeleteLoadBalancer(ctx, &elbv2.DeleteLoadBalancerInput{
			LoadBalancerArn: aws.String(albARN),
		})
		return err
	}

	cleanup.Register("ALB "+albARN, PriorityLoadBalancer, cleanupFn)

	// Wait for ALB to be active
	err = waitFor(ctx, func() (bool, error) {
		desc, err := clients.ELBv2.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{
			LoadBalancerArns: []string{albARN},
		})
		if err != nil {
			return false, err
		}
		return desc.LoadBalancers[0].State.Code == elbv2types.LoadBalancerStateEnumActive, nil
	}, 10*time.Second, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("waiting for ALB %s: %w", albARN, err)
	}

	return &CreateTestResourceResult{ID: albARN, Cleanup: cleanupFn}, nil
}
```

### Helper Usage Example

Using helpers makes tests cleaner and more focused on validation:

```go
func TestEIPRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx := context.Background()

	t.Run("ListTagDelete", func(t *testing.T) {
		// 1. CREATE - using helper
		result, err := CreateTestEIP(ctx, globalCleanup, nil)
		requireNoError(t, err, "creating test EIP")

		// 2. DETECT - using repository
		repo := awsinfra.NewElasticIPRepository(clients.EC2, testConfig.AccountID, testConfig.Region)
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs")

		found := findResource(resources, result.ID)
		if found == nil {
			t.Fatalf("EIP %s not found", result.ID)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// 3. TAG
		err = repo.Tag(ctx, result.ID, time.Now().AddDate(0, 0, 30))
		requireNoError(t, err, "tagging EIP")

		// 4. DELETE
		err = repo.Delete(ctx, result.ID)
		requireNoError(t, err, "deleting EIP")

		// 5. CLEANUP - automatic via globalCleanup registry
	})

	t.Run("ExcludedResource", func(t *testing.T) {
		// Create EIP with DoNotDelete tag
		result, err := CreateTestEIP(ctx, globalCleanup, map[string]string{
			"DoNotDelete": "true",
		})
		requireNoError(t, err, "creating excluded EIP")

		repo := awsinfra.NewElasticIPRepository(clients.EC2, testConfig.AccountID, testConfig.Region)
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs")

		found := findResource(resources, result.ID)
		if !found.IsExcluded(map[string]string{"DoNotDelete": "true"}) {
			t.Error("resource should be excluded")
		}
	})
}
```

### Resource Creation Summary

| Resource Type | Helper Function | Creation Time | Dependencies |
|--------------|-----------------|---------------|--------------|
| Elastic IP | `CreateTestEIP()` | ~1s | None |
| Log Group | `CreateTestLogGroup()` | ~1s | None |
| EBS Volume | `CreateTestEBSVolume()` | ~30s | Availability Zone |
| EBS Snapshot | `CreateTestSnapshot()` | ~2-3m | EBS Volume |
| EC2 Instance | `CreateTestEC2Instance()` | ~2-3m | Subnet, AMI |
| NAT Gateway | `CreateTestNATGateway()` | ~3-5m | Subnet (creates own EIP) |
| ALB | `CreateTestALB()` | ~2-3m | 2+ Subnets |
| RDS Instance | (inline in test) | ~10-15m | DB Subnet Group |
| ElastiCache | (inline in test) | ~10-15m | Cache Subnet Group |
| EKS Cluster | (inline in test) | ~15-20m | IAM Role, Subnets |

### Key Principles

1. **Create before detect** - Resources must exist before scanners can find them
2. **Register cleanup immediately** - Always register cleanup right after creation, before any assertions
3. **Wait for ready state** - Many AWS resources have async creation; wait before testing
4. **Tag for identification** - All test resources get `cloud-janitor-test=true` tag
5. **Helpers handle dependencies** - e.g., `CreateTestNATGateway()` creates its own EIP

---

## Phase 1: Test Infrastructure

### Overview
Create the foundational test infrastructure: TestMain, configuration, AWS clients, cleanup registry, resource creation helpers, and VPC setup.

### Files Created in Phase 1

| File | Purpose |
|------|---------|
| `tests/integration/main_test.go` | TestMain entry point, setup/teardown |
| `tests/integration/config_test.go` | Test configuration from env vars |
| `tests/integration/cleanup_test.go` | Cleanup registry for guaranteed resource cleanup |
| `tests/integration/clients_test.go` | AWS SDK client factory |
| `tests/integration/helpers_test.go` | Test assertion helpers |
| `tests/integration/resource_helpers_test.go` | Resource creation helpers (Create→Detect pattern) |
| `tests/integration/vpc_test.go` | VPC/subnet infrastructure setup |

### Changes Required

#### 1. Create tests directory structure
**File**: `tests/integration/` (new directory)

```bash
mkdir -p tests/integration
```

#### 2. Test Configuration
**File**: `tests/integration/config_test.go`

```go
//go:build integration

package integration

import (
	"os"
	"testing"
)

// TestConfig holds configuration for integration tests.
type TestConfig struct {
	AccountID        string
	Region           string
	EKSRoleARN       string // Optional: IAM role for EKS clusters
	SageMakerRoleARN string // Optional: IAM role for SageMaker notebooks
}

// Global test configuration
var testConfig TestConfig

// loadTestConfig loads configuration from environment variables.
func loadTestConfig() {
	testConfig = TestConfig{
		AccountID:        os.Getenv("TEST_AWS_ACCOUNT_ID"),
		Region:           os.Getenv("TEST_AWS_REGION"),
		EKSRoleARN:       os.Getenv("TEST_EKS_ROLE_ARN"),
		SageMakerRoleARN: os.Getenv("TEST_SAGEMAKER_ROLE_ARN"),
	}

	if testConfig.Region == "" {
		testConfig.Region = "us-west-2"
	}
}

// skipIfMissingConfig skips the test if required config is missing.
func skipIfMissingConfig(t *testing.T) {
	t.Helper()
	if testConfig.AccountID == "" {
		t.Skip("Skipping: TEST_AWS_ACCOUNT_ID not set")
	}
}

// skipIfMissingEKSRole skips EKS tests if role is not configured.
func skipIfMissingEKSRole(t *testing.T) {
	t.Helper()
	skipIfMissingConfig(t)
	if testConfig.EKSRoleARN == "" {
		t.Skip("Skipping: TEST_EKS_ROLE_ARN not set")
	}
}

// skipIfMissingSageMakerRole skips SageMaker tests if role is not configured.
func skipIfMissingSageMakerRole(t *testing.T) {
	t.Helper()
	skipIfMissingConfig(t)
	if testConfig.SageMakerRoleARN == "" {
		t.Skip("Skipping: TEST_SAGEMAKER_ROLE_ARN not set")
	}
}
```

#### 3. Cleanup Registry
**File**: `tests/integration/cleanup_test.go`

```go
//go:build integration

package integration

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Cleanup priority levels (lower = runs first)
const (
	PriorityEKSNodeGroup = 1
	PriorityEKSCluster   = 2
	PriorityNATGateway   = 3
	PriorityLoadBalancer = 4
	PriorityRDS          = 5
	PriorityElastiCache  = 6
	PriorityOpenSearch   = 7
	PriorityRedshift     = 8
	PrioritySubnetGroup  = 9
	PriorityEC2          = 10
	PriorityEBS          = 11
	PrioritySnapshot     = 12
	PriorityAMI          = 13
	PriorityElasticIP    = 14
	PriorityLogGroup     = 15
	PrioritySageMaker    = 16
	PrioritySubnet       = 100
	PriorityRouteTable   = 101
	PriorityIGW          = 102
	PriorityVPC          = 103
)

// CleanupFunc represents a cleanup function with metadata.
type CleanupFunc struct {
	Name     string
	Priority int
	Fn       func(ctx context.Context) error
}

// CleanupRegistry tracks all created resources for guaranteed cleanup.
type CleanupRegistry struct {
	mu       sync.Mutex
	cleanups []CleanupFunc
}

// NewCleanupRegistry creates a new CleanupRegistry.
func NewCleanupRegistry() *CleanupRegistry {
	return &CleanupRegistry{}
}

// Register adds a cleanup function to the registry.
func (r *CleanupRegistry) Register(name string, priority int, fn func(ctx context.Context) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanups = append(r.cleanups, CleanupFunc{
		Name:     name,
		Priority: priority,
		Fn:       fn,
	})
}

// RunAll executes all cleanups in priority order (lowest first), then LIFO within same priority.
// Returns all errors encountered (does not stop on first error).
func (r *CleanupRegistry) RunAll(ctx context.Context) []error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.cleanups) == 0 {
		return nil
	}

	// Sort by priority (ascending)
	sort.SliceStable(r.cleanups, func(i, j int) bool {
		return r.cleanups[i].Priority < r.cleanups[j].Priority
	})

	// Group by priority and reverse within each group (LIFO)
	var sorted []CleanupFunc
	currentPriority := r.cleanups[0].Priority
	var currentGroup []CleanupFunc

	for _, c := range r.cleanups {
		if c.Priority != currentPriority {
			// Reverse current group and add to sorted
			for i := len(currentGroup) - 1; i >= 0; i-- {
				sorted = append(sorted, currentGroup[i])
			}
			currentGroup = nil
			currentPriority = c.Priority
		}
		currentGroup = append(currentGroup, c)
	}
	// Add last group
	for i := len(currentGroup) - 1; i >= 0; i-- {
		sorted = append(sorted, currentGroup[i])
	}

	var errs []error
	for _, c := range sorted {
		fmt.Printf("  Cleaning up: %s\n", c.Name)
		if err := c.Fn(ctx); err != nil {
			errs = append(errs, fmt.Errorf("cleanup %s: %w", c.Name, err))
			// Continue cleaning up other resources
		}
	}

	return errs
}

// Count returns the number of registered cleanup functions.
func (r *CleanupRegistry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.cleanups)
}
```

#### 4. AWS Clients Factory
**File**: `tests/integration/clients_test.go`

```go
//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// testClients holds all AWS clients for integration tests.
type testClients struct {
	EC2         *ec2.Client
	RDS         *rds.Client
	ELBv2       *elasticloadbalancingv2.Client
	ElastiCache *elasticache.Client
	OpenSearch  *opensearch.Client
	EKS         *eks.Client
	Redshift    *redshift.Client
	SageMaker   *sagemaker.Client
	Logs        *cloudwatchlogs.Client
	STS         *sts.Client
}

// Global clients instance
var clients *testClients

// initClients initializes all AWS clients.
func initClients(ctx context.Context) error {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(testConfig.Region))
	if err != nil {
		return err
	}

	clients = &testClients{
		EC2:         ec2.NewFromConfig(cfg),
		RDS:         rds.NewFromConfig(cfg),
		ELBv2:       elasticloadbalancingv2.NewFromConfig(cfg),
		ElastiCache: elasticache.NewFromConfig(cfg),
		OpenSearch:  opensearch.NewFromConfig(cfg),
		EKS:         eks.NewFromConfig(cfg),
		Redshift:    redshift.NewFromConfig(cfg),
		SageMaker:   sagemaker.NewFromConfig(cfg),
		Logs:        cloudwatchlogs.NewFromConfig(cfg),
		STS:         sts.NewFromConfig(cfg),
	}

	return nil
}

// getEC2Client returns the EC2 client, initializing if needed.
func getEC2Client(t *testing.T) *ec2.Client {
	t.Helper()
	if clients == nil || clients.EC2 == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.EC2
}

// getRDSClient returns the RDS client.
func getRDSClient(t *testing.T) *rds.Client {
	t.Helper()
	if clients == nil || clients.RDS == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.RDS
}

// getELBv2Client returns the ELBv2 client.
func getELBv2Client(t *testing.T) *elasticloadbalancingv2.Client {
	t.Helper()
	if clients == nil || clients.ELBv2 == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.ELBv2
}

// getElastiCacheClient returns the ElastiCache client.
func getElastiCacheClient(t *testing.T) *elasticache.Client {
	t.Helper()
	if clients == nil || clients.ElastiCache == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.ElastiCache
}

// getOpenSearchClient returns the OpenSearch client.
func getOpenSearchClient(t *testing.T) *opensearch.Client {
	t.Helper()
	if clients == nil || clients.OpenSearch == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.OpenSearch
}

// getEKSClient returns the EKS client.
func getEKSClient(t *testing.T) *eks.Client {
	t.Helper()
	if clients == nil || clients.EKS == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.EKS
}

// getRedshiftClient returns the Redshift client.
func getRedshiftClient(t *testing.T) *redshift.Client {
	t.Helper()
	if clients == nil || clients.Redshift == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.Redshift
}

// getSageMakerClient returns the SageMaker client.
func getSageMakerClient(t *testing.T) *sagemaker.Client {
	t.Helper()
	if clients == nil || clients.SageMaker == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.SageMaker
}

// getLogsClient returns the CloudWatch Logs client.
func getLogsClient(t *testing.T) *cloudwatchlogs.Client {
	t.Helper()
	if clients == nil || clients.Logs == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.Logs
}
```

#### 5. Test Helpers
**File**: `tests/integration/helpers_test.go`

```go
//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

const (
	// testTagKey is the tag key used to identify test resources.
	testTagKey = "cloud-janitor-test"
	// testTagValue is the tag value used to identify test resources.
	testTagValue = "true"
)

// testTags returns the standard tags for test resources.
func testTags() map[string]string {
	return map[string]string{
		testTagKey: testTagValue,
		"Name":     fmt.Sprintf("cloud-janitor-test-%d", time.Now().Unix()),
	}
}

// mergeTags merges additional tags with test tags.
func mergeTags(base, additional map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range additional {
		result[k] = v
	}
	return result
}

// toEC2Tags converts a map to EC2 tag slice.
func toEC2Tags(tags map[string]string) []types.Tag {
	result := make([]types.Tag, 0, len(tags))
	for k, v := range tags {
		result = append(result, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return result
}

// findResource finds a resource by ID in a slice.
func findResource(resources []domain.Resource, id string) *domain.Resource {
	for i := range resources {
		if resources[i].ID == id {
			return &resources[i]
		}
	}
	return nil
}

// containsResourceID checks if a resource ID is in the slice.
func containsResourceID(resources []domain.Resource, id string) bool {
	return findResource(resources, id) != nil
}

// waitFor polls a condition until it returns true or timeout.
func waitFor(ctx context.Context, condition func() (bool, error), interval, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for condition: %w", ctx.Err())
		case <-ticker.C:
			done, err := condition()
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		}
	}
}

// requireNoError fails the test if err is not nil.
func requireNoError(t *testing.T, err error, msg string, args ...any) {
	t.Helper()
	if err != nil {
		t.Fatalf(msg+": %v", append(args, err)...)
	}
}

// assertContains checks if needle is in haystack.
func assertContains(t *testing.T, haystack []string, needle string, msg string) {
	t.Helper()
	for _, s := range haystack {
		if s == needle {
			return
		}
	}
	t.Errorf("%s: %q not found in %v", msg, needle, haystack)
}

// assertNotContains checks if needle is NOT in haystack.
func assertNotContains(t *testing.T, haystack []string, needle string, msg string) {
	t.Helper()
	for _, s := range haystack {
		if s == needle {
			t.Errorf("%s: %q should not be in %v", msg, needle, haystack)
			return
		}
	}
}

// getResourceIDs extracts IDs from a slice of resources.
func getResourceIDs(resources []domain.Resource) []string {
	ids := make([]string, len(resources))
	for i, r := range resources {
		ids[i] = r.ID
	}
	return ids
}
```

#### 6. Resource Creation Helpers
**File**: `tests/integration/resource_helpers_test.go`

> **Note**: The full implementation of this file is provided in the "Test Pattern: Create → Detect → Tag → Delete" section above. It contains helper functions for creating test resources:
> - `CreateTestEIP()` - Elastic IP
> - `CreateTestLogGroup()` - CloudWatch Log Group
> - `CreateTestEBSVolume()` - EBS Volume
> - `CreateTestSnapshot()` - EBS Snapshot
> - `CreateTestEC2Instance()` - EC2 Instance
> - `CreateTestNATGateway()` - NAT Gateway (with EIP)
> - `CreateTestALB()` - Application Load Balancer

#### 7. VPC Infrastructure
**File**: `tests/integration/vpc_test.go`

```go
//go:build integration

package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// TestInfrastructure holds VPC and networking resources for tests.
type TestInfrastructure struct {
	VPCID            string
	PublicSubnetIDs  []string // 2 subnets in different AZs (for ELB, NAT)
	PrivateSubnetIDs []string // 2 subnets in different AZs (for RDS, ElastiCache)
	InternetGatewayID string
	PublicRouteTableID string
	AvailabilityZones []string
}

// Global test infrastructure
var testInfra *TestInfrastructure

// setupTestInfrastructure creates VPC, subnets, and networking for tests.
func setupTestInfrastructure(ctx context.Context, cleanup *CleanupRegistry) (*TestInfrastructure, error) {
	ec2Client := clients.EC2
	infra := &TestInfrastructure{}

	// Get availability zones
	azOutput, err := ec2Client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		Filters: []types.Filter{
			{Name: aws.String("state"), Values: []string{"available"}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("describing AZs: %w", err)
	}
	if len(azOutput.AvailabilityZones) < 2 {
		return nil, fmt.Errorf("need at least 2 AZs, found %d", len(azOutput.AvailabilityZones))
	}
	infra.AvailabilityZones = []string{
		*azOutput.AvailabilityZones[0].ZoneName,
		*azOutput.AvailabilityZones[1].ZoneName,
	}

	// Create VPC
	vpcOutput, err := ec2Client.CreateVpc(ctx, &ec2.CreateVpcInput{
		CidrBlock: aws.String("10.0.0.0/16"),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeVpc,
				Tags:         toEC2Tags(testTags()),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating VPC: %w", err)
	}
	infra.VPCID = *vpcOutput.Vpc.VpcId
	cleanup.Register("VPC "+infra.VPCID, PriorityVPC, func(ctx context.Context) error {
		_, err := ec2Client.DeleteVpc(ctx, &ec2.DeleteVpcInput{VpcId: aws.String(infra.VPCID)})
		return err
	})

	// Enable DNS hostnames
	_, err = ec2Client.ModifyVpcAttribute(ctx, &ec2.ModifyVpcAttributeInput{
		VpcId:              aws.String(infra.VPCID),
		EnableDnsHostnames: &types.AttributeBooleanValue{Value: aws.Bool(true)},
	})
	if err != nil {
		return nil, fmt.Errorf("enabling DNS hostnames: %w", err)
	}

	// Create Internet Gateway
	igwOutput, err := ec2Client.CreateInternetGateway(ctx, &ec2.CreateInternetGatewayInput{
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeInternetGateway,
				Tags:         toEC2Tags(testTags()),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating IGW: %w", err)
	}
	infra.InternetGatewayID = *igwOutput.InternetGateway.InternetGatewayId
	cleanup.Register("IGW "+infra.InternetGatewayID, PriorityIGW, func(ctx context.Context) error {
		// Detach first
		_, _ = ec2Client.DetachInternetGateway(ctx, &ec2.DetachInternetGatewayInput{
			InternetGatewayId: aws.String(infra.InternetGatewayID),
			VpcId:             aws.String(infra.VPCID),
		})
		_, err := ec2Client.DeleteInternetGateway(ctx, &ec2.DeleteInternetGatewayInput{
			InternetGatewayId: aws.String(infra.InternetGatewayID),
		})
		return err
	})

	// Attach IGW to VPC
	_, err = ec2Client.AttachInternetGateway(ctx, &ec2.AttachInternetGatewayInput{
		InternetGatewayId: aws.String(infra.InternetGatewayID),
		VpcId:             aws.String(infra.VPCID),
	})
	if err != nil {
		return nil, fmt.Errorf("attaching IGW: %w", err)
	}

	// Create public subnets (2 in different AZs)
	publicCIDRs := []string{"10.0.1.0/24", "10.0.2.0/24"}
	for i, cidr := range publicCIDRs {
		subnetOutput, err := ec2Client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
			VpcId:            aws.String(infra.VPCID),
			CidrBlock:        aws.String(cidr),
			AvailabilityZone: aws.String(infra.AvailabilityZones[i]),
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeSubnet,
					Tags:         toEC2Tags(mergeTags(testTags(), map[string]string{"Type": "public"})),
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("creating public subnet %d: %w", i, err)
		}
		subnetID := *subnetOutput.Subnet.SubnetId
		infra.PublicSubnetIDs = append(infra.PublicSubnetIDs, subnetID)
		cleanup.Register("Subnet "+subnetID, PrioritySubnet, func(ctx context.Context) error {
			_, err := ec2Client.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{SubnetId: aws.String(subnetID)})
			return err
		})
	}

	// Create private subnets (2 in different AZs)
	privateCIDRs := []string{"10.0.3.0/24", "10.0.4.0/24"}
	for i, cidr := range privateCIDRs {
		subnetOutput, err := ec2Client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
			VpcId:            aws.String(infra.VPCID),
			CidrBlock:        aws.String(cidr),
			AvailabilityZone: aws.String(infra.AvailabilityZones[i]),
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeSubnet,
					Tags:         toEC2Tags(mergeTags(testTags(), map[string]string{"Type": "private"})),
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("creating private subnet %d: %w", i, err)
		}
		subnetID := *subnetOutput.Subnet.SubnetId
		infra.PrivateSubnetIDs = append(infra.PrivateSubnetIDs, subnetID)
		cleanup.Register("Subnet "+subnetID, PrioritySubnet, func(ctx context.Context) error {
			_, err := ec2Client.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{SubnetId: aws.String(subnetID)})
			return err
		})
	}

	// Create route table for public subnets
	rtOutput, err := ec2Client.CreateRouteTable(ctx, &ec2.CreateRouteTableInput{
		VpcId: aws.String(infra.VPCID),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeRouteTable,
				Tags:         toEC2Tags(testTags()),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating route table: %w", err)
	}
	infra.PublicRouteTableID = *rtOutput.RouteTable.RouteTableId
	cleanup.Register("RouteTable "+infra.PublicRouteTableID, PriorityRouteTable, func(ctx context.Context) error {
		_, err := ec2Client.DeleteRouteTable(ctx, &ec2.DeleteRouteTableInput{
			RouteTableId: aws.String(infra.PublicRouteTableID),
		})
		return err
	})

	// Add route to IGW
	_, err = ec2Client.CreateRoute(ctx, &ec2.CreateRouteInput{
		RouteTableId:         aws.String(infra.PublicRouteTableID),
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		GatewayId:            aws.String(infra.InternetGatewayID),
	})
	if err != nil {
		return nil, fmt.Errorf("creating route to IGW: %w", err)
	}

	// Associate public subnets with route table
	for _, subnetID := range infra.PublicSubnetIDs {
		_, err = ec2Client.AssociateRouteTable(ctx, &ec2.AssociateRouteTableInput{
			RouteTableId: aws.String(infra.PublicRouteTableID),
			SubnetId:     aws.String(subnetID),
		})
		if err != nil {
			return nil, fmt.Errorf("associating subnet %s with route table: %w", subnetID, err)
		}
	}

	// Wait for subnets to be available
	err = waitFor(ctx, func() (bool, error) {
		output, err := ec2Client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
			SubnetIds: append(infra.PublicSubnetIDs, infra.PrivateSubnetIDs...),
		})
		if err != nil {
			return false, err
		}
		for _, subnet := range output.Subnets {
			if subnet.State != types.SubnetStateAvailable {
				return false, nil
			}
		}
		return true, nil
	}, 2*time.Second, 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("waiting for subnets: %w", err)
	}

	return infra, nil
}
```

#### 8. Main Test Entry Point
**File**: `tests/integration/main_test.go`

```go
//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// Global cleanup registry
var globalCleanup *CleanupRegistry

func TestMain(m *testing.M) {
	// Initialize cleanup registry FIRST (before any resources are created)
	globalCleanup = NewCleanupRegistry()

	// Setup context with timeout for entire test suite
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)

	// Track exit code
	code := 1

	// ALWAYS run cleanup, even on panic
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("\nPANIC during tests: %v\n", r)
		}

		// Run cleanup with extended timeout
		fmt.Printf("\n=== Running cleanup (%d resources) ===\n", globalCleanup.Count())
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cleanupCancel()

		if errs := globalCleanup.RunAll(cleanupCtx); len(errs) > 0 {
			fmt.Println("\nCleanup errors:")
			for _, err := range errs {
				fmt.Printf("  - %v\n", err)
			}
		} else if globalCleanup.Count() > 0 {
			fmt.Println("Cleanup completed successfully")
		}

		cancel()
		os.Exit(code)
	}()

	// Load configuration from environment
	loadTestConfig()

	// Skip if no account configured
	if testConfig.AccountID == "" {
		fmt.Println("Skipping integration tests: TEST_AWS_ACCOUNT_ID not set")
		fmt.Println("Usage: TEST_AWS_ACCOUNT_ID=123456789012 make test-integration")
		code = 0
		return
	}

	fmt.Printf("=== Integration Test Configuration ===\n")
	fmt.Printf("  Account ID: %s\n", testConfig.AccountID)
	fmt.Printf("  Region: %s\n", testConfig.Region)
	if testConfig.EKSRoleARN != "" {
		fmt.Printf("  EKS Role: %s\n", testConfig.EKSRoleARN)
	}
	if testConfig.SageMakerRoleARN != "" {
		fmt.Printf("  SageMaker Role: %s\n", testConfig.SageMakerRoleARN)
	}
	fmt.Println()

	// Initialize AWS clients
	fmt.Println("Initializing AWS clients...")
	if err := initClients(ctx); err != nil {
		fmt.Printf("Failed to initialize AWS clients: %v\n", err)
		return
	}

	// Verify AWS credentials
	output, err := clients.STS.GetCallerIdentity(ctx, nil)
	if err != nil {
		fmt.Printf("Failed to verify AWS credentials: %v\n", err)
		return
	}
	fmt.Printf("Authenticated as: %s\n", *output.Arn)

	// Verify account ID matches
	if *output.Account != testConfig.AccountID {
		fmt.Printf("Account mismatch: expected %s, got %s\n", testConfig.AccountID, *output.Account)
		return
	}

	// Setup test infrastructure (VPC, subnets)
	fmt.Println("\nSetting up test infrastructure (VPC, subnets)...")
	testInfra, err = setupTestInfrastructure(ctx, globalCleanup)
	if err != nil {
		fmt.Printf("Failed to setup test infrastructure: %v\n", err)
		return
	}
	fmt.Printf("  VPC: %s\n", testInfra.VPCID)
	fmt.Printf("  Public Subnets: %v\n", testInfra.PublicSubnetIDs)
	fmt.Printf("  Private Subnets: %v\n", testInfra.PrivateSubnetIDs)
	fmt.Printf("  Availability Zones: %v\n", testInfra.AvailabilityZones)
	fmt.Println()

	// Run tests
	fmt.Println("=== Running Tests ===")
	code = m.Run()
}
```

### Success Criteria

#### Automated Verification:
- [x] `go build -tags=integration ./tests/integration/...` succeeds
- [x] `go vet -tags=integration ./tests/integration/...` passes
- [x] Files created in correct location

#### Manual Verification:
- [ ] Run `TEST_AWS_ACCOUNT_ID=<id> go test -v -tags=integration ./tests/integration/... -run TestMain` and verify:
  - VPC is created
  - Subnets are created (4 total)
  - All resources are cleaned up after test

**Implementation Note**: After completing this phase and all automated verification passes, pause for manual confirmation before proceeding to Phase 2.

---

## Phase 2: Fast Scanner Tests (EIP, Logs)

### Overview
Implement tests for the fastest-to-create resources: Elastic IPs and CloudWatch Log Groups. These validate the test patterns work before implementing slower scanners.

### Changes Required

#### 1. Elastic IP Tests
**File**: `tests/integration/eip_test.go`

```go
//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

func TestEIPRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ec2Client := getEC2Client(t)

	t.Run("ListTagDelete", func(t *testing.T) {
		// Create test EIP
		allocOutput, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
			Domain: types.DomainTypeVpc,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeElasticIp,
					Tags:         toEC2Tags(testTags()),
				},
			},
		})
		requireNoError(t, err, "allocating EIP")
		eipID := *allocOutput.AllocationId

		// Register cleanup
		globalCleanup.Register("EIP "+eipID, PriorityElasticIP, func(ctx context.Context) error {
			_, err := ec2Client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(eipID)})
			return err
		})

		// Create repository
		repo := awsinfra.NewElasticIPRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		// Test List - should find untagged resource
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs")

		found := findResource(resources, eipID)
		if found == nil {
			t.Fatalf("EIP %s not found in list", eipID)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, eipID, expDate)
		requireNoError(t, err, "tagging EIP")

		// Verify tag applied
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs after tag")

		found = findResource(resources, eipID)
		if found == nil {
			t.Fatalf("EIP %s not found after tagging", eipID)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive, got %v", found.Status())
		}

		// Test Delete
		err = repo.Delete(ctx, eipID)
		requireNoError(t, err, "deleting EIP")

		// Verify deleted
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs after delete")

		if findResource(resources, eipID) != nil {
			t.Error("EIP should be deleted")
		}
	})

	t.Run("ExcludedResourceNotTagged", func(t *testing.T) {
		// Create EIP with DoNotDelete tag
		allocOutput, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
			Domain: types.DomainTypeVpc,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeElasticIp,
					Tags: toEC2Tags(mergeTags(testTags(), map[string]string{
						"DoNotDelete": "true",
					})),
				},
			},
		})
		requireNoError(t, err, "allocating excluded EIP")
		eipID := *allocOutput.AllocationId

		globalCleanup.Register("EIP "+eipID, PriorityElasticIP, func(ctx context.Context) error {
			_, err := ec2Client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(eipID)})
			return err
		})

		repo := awsinfra.NewElasticIPRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		// List and verify excluded status
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs")

		found := findResource(resources, eipID)
		if found == nil {
			t.Fatalf("excluded EIP %s not found", eipID)
		}

		// Verify IsExcluded works
		excludeTags := map[string]string{"DoNotDelete": "true"}
		if !found.IsExcluded(excludeTags) {
			t.Error("resource should be excluded with DoNotDelete=true tag")
		}
	})

	t.Run("NeverExpiresResource", func(t *testing.T) {
		// Create EIP with expiration-date=never
		allocOutput, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
			Domain: types.DomainTypeVpc,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeElasticIp,
					Tags: toEC2Tags(mergeTags(testTags(), map[string]string{
						"expiration-date": "never",
					})),
				},
			},
		})
		requireNoError(t, err, "allocating never-expires EIP")
		eipID := *allocOutput.AllocationId

		globalCleanup.Register("EIP "+eipID, PriorityElasticIP, func(ctx context.Context) error {
			_, err := ec2Client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(eipID)})
			return err
		})

		repo := awsinfra.NewElasticIPRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs")

		found := findResource(resources, eipID)
		if found == nil {
			t.Fatalf("never-expires EIP %s not found", eipID)
		}
		if found.Status() != domain.StatusNeverExpires {
			t.Errorf("expected StatusNeverExpires, got %v", found.Status())
		}
	})
}
```

#### 2. CloudWatch Logs Tests
**File**: `tests/integration/logs_test.go`

```go
//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"

	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

func TestLogsRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	logsClient := getLogsClient(t)

	t.Run("ListTagDelete", func(t *testing.T) {
		logGroupName := fmt.Sprintf("/cloud-janitor-test/%d", time.Now().UnixNano())

		// Create log group
		_, err := logsClient.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(logGroupName),
			Tags: map[string]string{
				testTagKey: testTagValue,
			},
		})
		requireNoError(t, err, "creating log group")

		globalCleanup.Register("LogGroup "+logGroupName, PriorityLogGroup, func(ctx context.Context) error {
			_, err := logsClient.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{
				LogGroupName: aws.String(logGroupName),
			})
			return err
		})

		repo := awsinfra.NewLogsRepository(logsClient, testConfig.AccountID, testConfig.Region, nil)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing log groups")

		found := findResource(resources, logGroupName)
		if found == nil {
			t.Fatalf("log group %s not found", logGroupName)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, logGroupName, expDate)
		requireNoError(t, err, "tagging log group")

		// Verify tag applied
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after tag")

		found = findResource(resources, logGroupName)
		if found == nil {
			t.Fatalf("log group %s not found after tagging", logGroupName)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive, got %v", found.Status())
		}

		// Test Delete
		err = repo.Delete(ctx, logGroupName)
		requireNoError(t, err, "deleting log group")

		// Verify deleted
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after delete")

		if findResource(resources, logGroupName) != nil {
			t.Error("log group should be deleted")
		}
	})

	t.Run("SkipPatternsWork", func(t *testing.T) {
		// Create log group matching skip pattern
		logGroupName := fmt.Sprintf("/aws/lambda/test-%d", time.Now().UnixNano())

		_, err := logsClient.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(logGroupName),
			Tags: map[string]string{
				testTagKey: testTagValue,
			},
		})
		requireNoError(t, err, "creating lambda log group")

		globalCleanup.Register("LogGroup "+logGroupName, PriorityLogGroup, func(ctx context.Context) error {
			_, err := logsClient.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{
				LogGroupName: aws.String(logGroupName),
			})
			return err
		})

		// Create repo with skip patterns
		skipPatterns := []string{"/aws/lambda/*"}
		repo := awsinfra.NewLogsRepository(logsClient, testConfig.AccountID, testConfig.Region, skipPatterns)

		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing with skip patterns")

		// Should NOT find the log group (skipped)
		if findResource(resources, logGroupName) != nil {
			t.Error("log group should be skipped by pattern /aws/lambda/*")
		}
	})
}
```

### Success Criteria

#### Automated Verification:
- [x] `go build -tags=integration ./tests/integration/...` succeeds
- [x] `make lint` passes

#### Manual Verification:
- [ ] Run EIP tests: `TEST_AWS_ACCOUNT_ID=<id> go test -v -tags=integration ./tests/integration/... -run TestEIP`
- [ ] Run Logs tests: `TEST_AWS_ACCOUNT_ID=<id> go test -v -tags=integration ./tests/integration/... -run TestLogs`
- [ ] Verify resources are cleaned up after tests

**Implementation Note**: After completing this phase and all automated verification passes, pause for manual confirmation before proceeding to Phase 3.

---

## Phase 3: Quick Scanner Tests (Snapshot, AMI, EBS)

### Overview
Implement tests for resources that take 1-3 minutes to create: EBS Snapshots, AMIs, and EBS Volumes.

### Changes Required

#### 1. EBS Snapshot Tests
**File**: `tests/integration/snapshot_test.go`

```go
//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

func TestSnapshotRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	ec2Client := getEC2Client(t)

	t.Run("ListTagDelete", func(t *testing.T) {
		// First create a volume to snapshot
		volumeOutput, err := ec2Client.CreateVolume(ctx, &ec2.CreateVolumeInput{
			AvailabilityZone: aws.String(testInfra.AvailabilityZones[0]),
			Size:             aws.Int32(1), // 1 GB minimum
			VolumeType:       types.VolumeTypeGp3,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeVolume,
					Tags:         toEC2Tags(testTags()),
				},
			},
		})
		requireNoError(t, err, "creating volume")
		volumeID := *volumeOutput.VolumeId

		globalCleanup.Register("Volume "+volumeID, PriorityEBS, func(ctx context.Context) error {
			_, err := ec2Client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: aws.String(volumeID)})
			return err
		})

		// Wait for volume to be available
		err = waitFor(ctx, func() (bool, error) {
			output, err := ec2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
				VolumeIds: []string{volumeID},
			})
			if err != nil {
				return false, err
			}
			return output.Volumes[0].State == types.VolumeStateAvailable, nil
		}, 2*time.Second, 60*time.Second)
		requireNoError(t, err, "waiting for volume")

		// Create snapshot
		snapshotOutput, err := ec2Client.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
			VolumeId: aws.String(volumeID),
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeSnapshot,
					Tags:         toEC2Tags(testTags()),
				},
			},
		})
		requireNoError(t, err, "creating snapshot")
		snapshotID := *snapshotOutput.SnapshotId

		globalCleanup.Register("Snapshot "+snapshotID, PrioritySnapshot, func(ctx context.Context) error {
			_, err := ec2Client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{SnapshotId: aws.String(snapshotID)})
			return err
		})

		// Wait for snapshot to complete
		err = waitFor(ctx, func() (bool, error) {
			output, err := ec2Client.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{
				SnapshotIds: []string{snapshotID},
			})
			if err != nil {
				return false, err
			}
			return output.Snapshots[0].State == types.SnapshotStateCompleted, nil
		}, 5*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for snapshot")

		repo := awsinfra.NewSnapshotRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing snapshots")

		found := findResource(resources, snapshotID)
		if found == nil {
			t.Fatalf("snapshot %s not found", snapshotID)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, snapshotID, expDate)
		requireNoError(t, err, "tagging snapshot")

		// Test Delete
		err = repo.Delete(ctx, snapshotID)
		requireNoError(t, err, "deleting snapshot")

		// Verify deleted
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after delete")

		if findResource(resources, snapshotID) != nil {
			t.Error("snapshot should be deleted")
		}
	})
}
```

#### 2. AMI Tests
**File**: `tests/integration/ami_test.go`

```go
//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

func TestAMIRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	ec2Client := getEC2Client(t)

	t.Run("ListTagDelete", func(t *testing.T) {
		// First create a snapshot to register as AMI
		volumeOutput, err := ec2Client.CreateVolume(ctx, &ec2.CreateVolumeInput{
			AvailabilityZone: aws.String(testInfra.AvailabilityZones[0]),
			Size:             aws.Int32(1),
			VolumeType:       types.VolumeTypeGp3,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeVolume,
					Tags:         toEC2Tags(testTags()),
				},
			},
		})
		requireNoError(t, err, "creating volume for AMI")
		volumeID := *volumeOutput.VolumeId

		globalCleanup.Register("Volume "+volumeID, PriorityEBS, func(ctx context.Context) error {
			_, err := ec2Client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: aws.String(volumeID)})
			return err
		})

		// Wait for volume
		err = waitFor(ctx, func() (bool, error) {
			output, err := ec2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{VolumeIds: []string{volumeID}})
			if err != nil {
				return false, err
			}
			return output.Volumes[0].State == types.VolumeStateAvailable, nil
		}, 2*time.Second, 60*time.Second)
		requireNoError(t, err, "waiting for volume")

		// Create snapshot
		snapshotOutput, err := ec2Client.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
			VolumeId: aws.String(volumeID),
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeSnapshot,
					Tags:         toEC2Tags(testTags()),
				},
			},
		})
		requireNoError(t, err, "creating snapshot for AMI")
		snapshotID := *snapshotOutput.SnapshotId

		globalCleanup.Register("Snapshot "+snapshotID, PrioritySnapshot, func(ctx context.Context) error {
			_, err := ec2Client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{SnapshotId: aws.String(snapshotID)})
			return err
		})

		// Wait for snapshot to complete
		err = waitFor(ctx, func() (bool, error) {
			output, err := ec2Client.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{SnapshotIds: []string{snapshotID}})
			if err != nil {
				return false, err
			}
			return output.Snapshots[0].State == types.SnapshotStateCompleted, nil
		}, 5*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for snapshot")

		// Register AMI
		amiName := fmt.Sprintf("cloud-janitor-test-%d", time.Now().UnixNano())
		amiOutput, err := ec2Client.RegisterImage(ctx, &ec2.RegisterImageInput{
			Name:               aws.String(amiName),
			Architecture:       types.ArchitectureValuesX8664,
			RootDeviceName:     aws.String("/dev/xvda"),
			VirtualizationType: aws.String("hvm"),
			BlockDeviceMappings: []types.BlockDeviceMapping{
				{
					DeviceName: aws.String("/dev/xvda"),
					Ebs: &types.EbsBlockDevice{
						SnapshotId:          aws.String(snapshotID),
						DeleteOnTermination: aws.Bool(true),
					},
				},
			},
		})
		requireNoError(t, err, "registering AMI")
		amiID := *amiOutput.ImageId

		// Tag the AMI
		_, err = ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []string{amiID},
			Tags:      toEC2Tags(testTags()),
		})
		requireNoError(t, err, "tagging AMI")

		globalCleanup.Register("AMI "+amiID, PriorityAMI, func(ctx context.Context) error {
			_, err := ec2Client.DeregisterImage(ctx, &ec2.DeregisterImageInput{ImageId: aws.String(amiID)})
			return err
		})

		// Wait for AMI to be available
		err = waitFor(ctx, func() (bool, error) {
			output, err := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{ImageIds: []string{amiID}})
			if err != nil {
				return false, err
			}
			if len(output.Images) == 0 {
				return false, nil
			}
			return output.Images[0].State == types.ImageStateAvailable, nil
		}, 5*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for AMI")

		repo := awsinfra.NewAMIRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing AMIs")

		found := findResource(resources, amiID)
		if found == nil {
			t.Fatalf("AMI %s not found", amiID)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, amiID, expDate)
		requireNoError(t, err, "tagging AMI")

		// Test Delete (deregister)
		err = repo.Delete(ctx, amiID)
		requireNoError(t, err, "deleting AMI")

		// Verify deregistered
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after delete")

		if findResource(resources, amiID) != nil {
			t.Error("AMI should be deregistered")
		}
	})
}
```

#### 3. EBS Volume Tests
**File**: `tests/integration/ebs_test.go`

```go
//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

func TestEBSRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	ec2Client := getEC2Client(t)

	t.Run("ListTagDelete", func(t *testing.T) {
		// Create volume
		volumeOutput, err := ec2Client.CreateVolume(ctx, &ec2.CreateVolumeInput{
			AvailabilityZone: aws.String(testInfra.AvailabilityZones[0]),
			Size:             aws.Int32(1),
			VolumeType:       types.VolumeTypeGp3,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeVolume,
					Tags:         toEC2Tags(testTags()),
				},
			},
		})
		requireNoError(t, err, "creating volume")
		volumeID := *volumeOutput.VolumeId

		globalCleanup.Register("Volume "+volumeID, PriorityEBS, func(ctx context.Context) error {
			_, err := ec2Client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: aws.String(volumeID)})
			return err
		})

		// Wait for volume to be available
		err = waitFor(ctx, func() (bool, error) {
			output, err := ec2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
				VolumeIds: []string{volumeID},
			})
			if err != nil {
				return false, err
			}
			return output.Volumes[0].State == types.VolumeStateAvailable, nil
		}, 2*time.Second, 60*time.Second)
		requireNoError(t, err, "waiting for volume")

		repo := awsinfra.NewEBSRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing volumes")

		found := findResource(resources, volumeID)
		if found == nil {
			t.Fatalf("volume %s not found", volumeID)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, volumeID, expDate)
		requireNoError(t, err, "tagging volume")

		// Verify tag
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after tag")

		found = findResource(resources, volumeID)
		if found == nil {
			t.Fatalf("volume %s not found after tag", volumeID)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive, got %v", found.Status())
		}

		// Test Delete
		err = repo.Delete(ctx, volumeID)
		requireNoError(t, err, "deleting volume")

		// Verify deleted
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after delete")

		if findResource(resources, volumeID) != nil {
			t.Error("volume should be deleted")
		}
	})

	t.Run("SkipsAttachedVolumes", func(t *testing.T) {
		// EBS volumes attached to instances should still be listed
		// but deleting them should fail (tested in unit tests)
		// Integration test just verifies List works with attached volumes
		t.Skip("Requires EC2 instance - covered in ec2_test.go")
	})
}
```

### Success Criteria

#### Automated Verification:
- [x] `go build -tags=integration ./tests/integration/...` succeeds
- [x] `go vet -tags=integration ./tests/integration/...` passes

#### Manual Verification:
- [ ] Run: `TEST_AWS_ACCOUNT_ID=<id> go test -v -tags=integration ./tests/integration/... -run "TestSnapshot|TestAMI|TestEBS"`
- [ ] All resources cleaned up after tests

**Implementation Note**: After completing this phase and all automated verification passes, pause for manual confirmation before proceeding to Phase 4.

---

## Phase 4: Medium Scanner Tests (EC2, NAT Gateway, ELB)

### Overview
Implement tests for resources that take 2-5 minutes to create: EC2 instances, NAT Gateways, and Load Balancers.

### Changes Required

#### 1. EC2 Instance Tests
**File**: `tests/integration/ec2_test.go`

```go
//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

func TestEC2Repository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	ec2Client := getEC2Client(t)

	// Find a suitable AMI (Amazon Linux 2)
	amiOutput, err := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Owners: []string{"amazon"},
		Filters: []types.Filter{
			{Name: aws.String("name"), Values: []string{"amzn2-ami-hvm-*-x86_64-gp2"}},
			{Name: aws.String("state"), Values: []string{"available"}},
		},
	})
	requireNoError(t, err, "finding AMI")
	if len(amiOutput.Images) == 0 {
		t.Fatal("no suitable AMI found")
	}
	amiID := *amiOutput.Images[0].ImageId

	t.Run("ListTagDelete", func(t *testing.T) {
		// Launch instance
		runOutput, err := ec2Client.RunInstances(ctx, &ec2.RunInstancesInput{
			ImageId:      aws.String(amiID),
			InstanceType: types.InstanceTypeT3Micro,
			MinCount:     aws.Int32(1),
			MaxCount:     aws.Int32(1),
			SubnetId:     aws.String(testInfra.PublicSubnetIDs[0]),
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeInstance,
					Tags:         toEC2Tags(testTags()),
				},
			},
		})
		requireNoError(t, err, "launching instance")
		instanceID := *runOutput.Instances[0].InstanceId

		globalCleanup.Register("EC2 "+instanceID, PriorityEC2, func(ctx context.Context) error {
			_, err := ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
				InstanceIds: []string{instanceID},
			})
			return err
		})

		// Wait for instance to be running
		err = waitFor(ctx, func() (bool, error) {
			output, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
				InstanceIds: []string{instanceID},
			})
			if err != nil {
				return false, err
			}
			state := output.Reservations[0].Instances[0].State.Name
			return state == types.InstanceStateNameRunning, nil
		}, 5*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for instance")

		repo := awsinfra.NewEC2Repository(ec2Client, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing instances")

		found := findResource(resources, instanceID)
		if found == nil {
			t.Fatalf("instance %s not found", instanceID)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, instanceID, expDate)
		requireNoError(t, err, "tagging instance")

		// Test Delete (terminate)
		err = repo.Delete(ctx, instanceID)
		requireNoError(t, err, "terminating instance")

		// Wait for termination
		err = waitFor(ctx, func() (bool, error) {
			output, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
				InstanceIds: []string{instanceID},
			})
			if err != nil {
				return false, err
			}
			state := output.Reservations[0].Instances[0].State.Name
			return state == types.InstanceStateNameTerminated, nil
		}, 5*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for termination")
	})
}
```

#### 2. NAT Gateway Tests
**File**: `tests/integration/natgw_test.go`

```go
//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

func TestNATGatewayRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	ec2Client := getEC2Client(t)

	t.Run("ListTagDelete", func(t *testing.T) {
		// First allocate an EIP for NAT Gateway
		allocOutput, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
			Domain: types.DomainTypeVpc,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeElasticIp,
					Tags:         toEC2Tags(testTags()),
				},
			},
		})
		requireNoError(t, err, "allocating EIP for NAT")
		eipAllocID := *allocOutput.AllocationId

		globalCleanup.Register("EIP "+eipAllocID, PriorityElasticIP, func(ctx context.Context) error {
			_, err := ec2Client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(eipAllocID)})
			return err
		})

		// Create NAT Gateway
		natOutput, err := ec2Client.CreateNatGateway(ctx, &ec2.CreateNatGatewayInput{
			AllocationId: aws.String(eipAllocID),
			SubnetId:     aws.String(testInfra.PublicSubnetIDs[0]),
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeNatgateway,
					Tags:         toEC2Tags(testTags()),
				},
			},
		})
		requireNoError(t, err, "creating NAT Gateway")
		natID := *natOutput.NatGateway.NatGatewayId

		globalCleanup.Register("NAT "+natID, PriorityNATGateway, func(ctx context.Context) error {
			_, err := ec2Client.DeleteNatGateway(ctx, &ec2.DeleteNatGatewayInput{NatGatewayId: aws.String(natID)})
			if err != nil {
				return err
			}
			// Wait for deletion
			return waitFor(ctx, func() (bool, error) {
				output, err := ec2Client.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{
					NatGatewayIds: []string{natID},
				})
				if err != nil {
					return false, err
				}
				if len(output.NatGateways) == 0 {
					return true, nil
				}
				state := output.NatGateways[0].State
				return state == types.NatGatewayStateDeleted, nil
			}, 5*time.Second, 5*time.Minute)
		})

		// Wait for NAT Gateway to be available
		err = waitFor(ctx, func() (bool, error) {
			output, err := ec2Client.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{
				NatGatewayIds: []string{natID},
			})
			if err != nil {
				return false, err
			}
			state := output.NatGateways[0].State
			return state == types.NatGatewayStateAvailable, nil
		}, 10*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for NAT Gateway")

		repo := awsinfra.NewNATGatewayRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing NAT Gateways")

		found := findResource(resources, natID)
		if found == nil {
			t.Fatalf("NAT Gateway %s not found", natID)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, natID, expDate)
		requireNoError(t, err, "tagging NAT Gateway")

		// Test Delete
		err = repo.Delete(ctx, natID)
		requireNoError(t, err, "deleting NAT Gateway")

		// Wait for deletion
		err = waitFor(ctx, func() (bool, error) {
			output, err := ec2Client.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{
				NatGatewayIds: []string{natID},
			})
			if err != nil {
				return false, err
			}
			state := output.NatGateways[0].State
			return state == types.NatGatewayStateDeleted, nil
		}, 10*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for NAT deletion")
	})
}
```

#### 3. ELB (ALB) Tests
**File**: `tests/integration/elb_test.go`

```go
//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

func TestELBRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	elbClient := getELBv2Client(t)

	t.Run("ListTagDelete_ALB", func(t *testing.T) {
		// Create ALB
		createOutput, err := elbClient.CreateLoadBalancer(ctx, &elasticloadbalancingv2.CreateLoadBalancerInput{
			Name:   aws.String("cj-test-alb"),
			Type:   types.LoadBalancerTypeEnumApplication,
			Scheme: types.LoadBalancerSchemeEnumInternal,
			Subnets: testInfra.PrivateSubnetIDs,
			Tags: []types.Tag{
				{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
				{Key: aws.String("Name"), Value: aws.String("cloud-janitor-test-alb")},
			},
		})
		requireNoError(t, err, "creating ALB")
		albARN := *createOutput.LoadBalancers[0].LoadBalancerArn

		globalCleanup.Register("ALB "+albARN, PriorityLoadBalancer, func(ctx context.Context) error {
			_, err := elbClient.DeleteLoadBalancer(ctx, &elasticloadbalancingv2.DeleteLoadBalancerInput{
				LoadBalancerArn: aws.String(albARN),
			})
			return err
		})

		// Wait for ALB to be active
		err = waitFor(ctx, func() (bool, error) {
			output, err := elbClient.DescribeLoadBalancers(ctx, &elasticloadbalancingv2.DescribeLoadBalancersInput{
				LoadBalancerArns: []string{albARN},
			})
			if err != nil {
				return false, err
			}
			state := output.LoadBalancers[0].State.Code
			return state == types.LoadBalancerStateEnumActive, nil
		}, 10*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for ALB")

		repo := awsinfra.NewELBRepository(elbClient, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing load balancers")

		found := findResource(resources, albARN)
		if found == nil {
			t.Fatalf("ALB %s not found", albARN)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, albARN, expDate)
		requireNoError(t, err, "tagging ALB")

		// Test Delete
		err = repo.Delete(ctx, albARN)
		requireNoError(t, err, "deleting ALB")

		// Wait for deletion
		err = waitFor(ctx, func() (bool, error) {
			output, err := elbClient.DescribeLoadBalancers(ctx, &elasticloadbalancingv2.DescribeLoadBalancersInput{
				LoadBalancerArns: []string{albARN},
			})
			if err != nil {
				// LoadBalancer not found = deleted
				return true, nil
			}
			return len(output.LoadBalancers) == 0, nil
		}, 10*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for ALB deletion")
	})
}
```

### Success Criteria

#### Automated Verification:
- [x] `go build -tags=integration ./tests/integration/...` succeeds
- [x] `go vet -tags=integration ./tests/integration/...` passes

#### Manual Verification:
- [ ] Run: `TEST_AWS_ACCOUNT_ID=<id> go test -v -tags=integration ./tests/integration/... -run "TestEC2|TestNAT|TestELB"`
- [ ] All resources cleaned up

**Implementation Note**: After completing this phase, pause for manual confirmation before proceeding to Phase 5.

---

## Phase 5: Slow Scanner Tests (RDS, ElastiCache, Others)

### Overview
Implement tests for resources that take 10-20+ minutes to create. Each test file follows the same pattern established in earlier phases.

### Changes Required

Create the following test files with the same patterns (List, Tag, Delete):

1. **`tests/integration/rds_test.go`** - RDS instances (requires DB subnet group)
2. **`tests/integration/elasticache_test.go`** - ElastiCache clusters (requires cache subnet group)  
3. **`tests/integration/redshift_test.go`** - Redshift clusters (requires cluster subnet group)
4. **`tests/integration/sagemaker_test.go`** - SageMaker notebooks (requires IAM role)
5. **`tests/integration/opensearch_test.go`** - OpenSearch domains
6. **`tests/integration/eks_test.go`** - EKS clusters (requires IAM role, tests cascade delete)

Each file should:
- Create required subnet groups with cleanup registration
- Create the resource with test tags
- Test List, Tag, Delete operations
- Handle long wait times (10-20 minutes)
- Skip if required IAM roles not configured

### Success Criteria

#### Automated Verification:
- [x] All files compile: `go build -tags=integration ./tests/integration/...`
- [x] `go vet -tags=integration ./tests/integration/...` passes

#### Manual Verification:
- [ ] Run individual slow tests (one at a time due to cost)
- [ ] Verify cleanup works even for long-running resource creation

**Implementation Note**: Slow scanner tests can be implemented incrementally. Each can be added and tested independently.

---

## Phase 6: Complete Workflow Test

### Overview
Implement the end-to-end workflow test that validates the complete Cloud Janitor cycle: detect untagged → tag → expire → cleanup.

### Changes Required

**File**: `tests/integration/workflow_test.go`

```go
//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/maxkrivich/cloud-janitor/internal/app/service"
	"github.com/maxkrivich/cloud-janitor/internal/app/usecase"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
	"github.com/maxkrivich/cloud-janitor/internal/infra/notify"
)

func TestCompleteWorkflow(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	ec2Client := getEC2Client(t)

	// === SETUP: Create 3 EIPs with different states ===

	// 1. Normal EIP (should be tagged, then deleted)
	normalEIP, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
		Domain: types.DomainTypeVpc,
		TagSpecifications: []types.TagSpecification{
			{ResourceType: types.ResourceTypeElasticIp, Tags: toEC2Tags(testTags())},
		},
	})
	requireNoError(t, err, "creating normal EIP")
	normalID := *normalEIP.AllocationId
	globalCleanup.Register("EIP "+normalID, PriorityElasticIP, func(ctx context.Context) error {
		_, err := ec2Client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(normalID)})
		return err
	})

	// 2. Excluded EIP (DoNotDelete=true, should be skipped)
	excludedEIP, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
		Domain: types.DomainTypeVpc,
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeElasticIp,
				Tags: toEC2Tags(mergeTags(testTags(), map[string]string{
					"DoNotDelete": "true",
				})),
			},
		},
	})
	requireNoError(t, err, "creating excluded EIP")
	excludedID := *excludedEIP.AllocationId
	globalCleanup.Register("EIP "+excludedID, PriorityElasticIP, func(ctx context.Context) error {
		_, err := ec2Client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(excludedID)})
		return err
	})

	// 3. Never-expires EIP (expiration-date=never, should be skipped)
	neverExpiresEIP, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
		Domain: types.DomainTypeVpc,
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeElasticIp,
				Tags: toEC2Tags(mergeTags(testTags(), map[string]string{
					"expiration-date": "never",
				})),
			},
		},
	})
	requireNoError(t, err, "creating never-expires EIP")
	neverExpiresID := *neverExpiresEIP.AllocationId
	globalCleanup.Register("EIP "+neverExpiresID, PriorityElasticIP, func(ctx context.Context) error {
		_, err := ec2Client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(neverExpiresID)})
		return err
	})

	// === PHASE 1: DETECT & TAG ===
	t.Run("Phase1_DetectAndTag", func(t *testing.T) {
		// Create Janitor with only EIP scanner enabled
		janitor := createJanitorForEIPOnly(t, usecase.TagConfig{
			DefaultDays: 30,
			ExcludeTags: map[string]string{"DoNotDelete": "true"},
		}, usecase.CleanupConfig{
			ExcludeTags: map[string]string{"DoNotDelete": "true"},
		})

		result, err := janitor.Tag(ctx)
		requireNoError(t, err, "tagging resources")

		// Get tagged resource IDs
		taggedIDs := getTaggedResourceIDs(result)

		// Normal EIP should be tagged
		assertContains(t, taggedIDs, normalID, "normal EIP should be tagged")

		// Excluded EIP should NOT be tagged
		assertNotContains(t, taggedIDs, excludedID, "excluded EIP should NOT be tagged")

		// Never-expires EIP should NOT be tagged (already has expiration-date)
		assertNotContains(t, taggedIDs, neverExpiresID, "never-expires EIP should NOT be tagged")
	})

	// === PHASE 2: VERIFY TAGGING APPLIED ===
	t.Run("Phase2_VerifyTagsApplied", func(t *testing.T) {
		repo := awsinfra.NewElasticIPRepository(ec2Client, testConfig.AccountID, testConfig.Region)
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs")

		normal := findResource(resources, normalID)
		if normal == nil {
			t.Fatalf("normal EIP not found")
		}
		if normal.Status() != domain.StatusActive {
			t.Errorf("normal EIP: expected StatusActive, got %v", normal.Status())
		}

		excluded := findResource(resources, excludedID)
		if excluded == nil {
			t.Fatalf("excluded EIP not found")
		}
		if excluded.Status() != domain.StatusUntagged {
			t.Errorf("excluded EIP: expected StatusUntagged, got %v", excluded.Status())
		}

		neverExpires := findResource(resources, neverExpiresID)
		if neverExpires == nil {
			t.Fatalf("never-expires EIP not found")
		}
		if neverExpires.Status() != domain.StatusNeverExpires {
			t.Errorf("never-expires EIP: expected StatusNeverExpires, got %v", neverExpires.Status())
		}
	})

	// === PHASE 3: SIMULATE EXPIRATION ===
	t.Run("Phase3_SimulateExpiration", func(t *testing.T) {
		// Set normal EIP's expiration to yesterday
		repo := awsinfra.NewElasticIPRepository(ec2Client, testConfig.AccountID, testConfig.Region)
		yesterday := time.Now().AddDate(0, 0, -1)
		err := repo.Tag(ctx, normalID, yesterday)
		requireNoError(t, err, "setting past expiration date")
	})

	// === PHASE 4: CLEANUP EXPIRED ===
	t.Run("Phase4_CleanupExpired", func(t *testing.T) {
		janitor := createJanitorForEIPOnly(t, usecase.TagConfig{
			DefaultDays: 30,
			ExcludeTags: map[string]string{"DoNotDelete": "true"},
		}, usecase.CleanupConfig{
			ExcludeTags: map[string]string{"DoNotDelete": "true"},
		})

		result, err := janitor.Cleanup(ctx)
		requireNoError(t, err, "cleaning up resources")

		deletedIDs := getDeletedResourceIDs(result)

		// Normal (now expired) EIP should be deleted
		assertContains(t, deletedIDs, normalID, "expired EIP should be deleted")

		// Excluded EIP should NOT be deleted
		assertNotContains(t, deletedIDs, excludedID, "excluded EIP should NOT be deleted")

		// Never-expires EIP should NOT be deleted
		assertNotContains(t, deletedIDs, neverExpiresID, "never-expires EIP should NOT be deleted")
	})

	// === PHASE 5: VERIFY FINAL STATE ===
	t.Run("Phase5_VerifyFinalState", func(t *testing.T) {
		repo := awsinfra.NewElasticIPRepository(ec2Client, testConfig.AccountID, testConfig.Region)
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs")

		// Normal EIP should be gone
		if findResource(resources, normalID) != nil {
			t.Error("expired EIP should be deleted")
		}

		// Excluded EIP should still exist
		if findResource(resources, excludedID) == nil {
			t.Error("excluded EIP should still exist")
		}

		// Never-expires EIP should still exist
		if findResource(resources, neverExpiresID) == nil {
			t.Error("never-expires EIP should still exist")
		}
	})
}

// createJanitorForEIPOnly creates a Janitor configured for EIP-only testing.
func createJanitorForEIPOnly(t *testing.T, tagConfig usecase.TagConfig, cleanupConfig usecase.CleanupConfig) *service.Janitor {
	t.Helper()

	account := domain.Account{
		ID:   testConfig.AccountID,
		Name: "integration-test",
	}

	// Create repository factory that only returns EIP repository
	repoFactory := func(ctx context.Context, acc domain.Account) ([]domain.ResourceRepository, error) {
		return []domain.ResourceRepository{
			awsinfra.NewElasticIPRepository(clients.EC2, acc.ID, testConfig.Region),
		}, nil
	}

	config := service.JanitorConfig{
		Accounts:      []domain.Account{account},
		Regions:       []string{testConfig.Region},
		TagConfig:     tagConfig,
		CleanupConfig: cleanupConfig,
	}

	return service.NewJanitor(config, notify.NewNoopNotifier(), repoFactory)
}

// getTaggedResourceIDs extracts IDs of tagged resources from result.
func getTaggedResourceIDs(result *service.RunResult) []string {
	var ids []string
	for _, tr := range result.TagResults {
		for _, r := range tr.Tagged {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

// getDeletedResourceIDs extracts IDs of deleted resources from result.
func getDeletedResourceIDs(result *service.RunResult) []string {
	var ids []string
	for _, cr := range result.CleanupResults {
		for _, r := range cr.Deleted {
			ids = append(ids, r.ID)
		}
	}
	return ids
}
```

### Success Criteria

#### Automated Verification:
- [ ] `go build -tags=integration ./tests/integration/...` succeeds
- [ ] `make lint` passes

#### Manual Verification:
- [ ] Run: `TEST_AWS_ACCOUNT_ID=<id> go test -v -tags=integration ./tests/integration/... -run TestCompleteWorkflow`
- [ ] Verify all 5 phases pass
- [ ] Verify resources cleaned up

**Implementation Note**: This is the critical validation test. It must pass before the integration test suite is considered complete.

---

## Phase 7: Documentation & Makefile

### Overview
Add README documentation and Makefile targets.

### Changes Required

#### 1. Update Makefile
**File**: `Makefile` (add to existing)

```makefile
## test-integration: Run integration tests (requires AWS credentials)
test-integration:
	@if [ -z "$(TEST_AWS_ACCOUNT_ID)" ]; then \
		echo "ERROR: TEST_AWS_ACCOUNT_ID is required"; \
		echo ""; \
		echo "Usage:"; \
		echo "  TEST_AWS_ACCOUNT_ID=123456789012 make test-integration"; \
		echo ""; \
		echo "Optional environment variables:"; \
		echo "  TEST_AWS_REGION        - AWS region (default: us-west-2)"; \
		echo "  TEST_EKS_ROLE_ARN      - IAM role for EKS clusters"; \
		echo "  TEST_SAGEMAKER_ROLE_ARN - IAM role for SageMaker notebooks"; \
		exit 1; \
	fi
	@echo "=== Running Integration Tests ==="
	@echo "Account: $(TEST_AWS_ACCOUNT_ID)"
	@echo "Region: $(or $(TEST_AWS_REGION),us-west-2)"
	@echo ""
	@echo "WARNING: This will create real AWS resources."
	@echo "Estimated cost: ~$$1-2 USD"
	@echo ""
	$(GOTEST) -v -tags=integration -timeout=90m -parallel=4 ./tests/integration/...

## test-integration-fast: Run fast integration tests only (< 5 min, ~$0.10)
test-integration-fast:
	@if [ -z "$(TEST_AWS_ACCOUNT_ID)" ]; then \
		echo "ERROR: TEST_AWS_ACCOUNT_ID is required"; \
		exit 1; \
	fi
	@echo "Running fast integration tests..."
	$(GOTEST) -v -tags=integration -timeout=30m -run="TestEIP|TestLogs|TestSnapshot|TestAMI|TestEBS" ./tests/integration/...

## test-integration-workflow: Run workflow test only
test-integration-workflow:
	@if [ -z "$(TEST_AWS_ACCOUNT_ID)" ]; then \
		echo "ERROR: TEST_AWS_ACCOUNT_ID is required"; \
		exit 1; \
	fi
	$(GOTEST) -v -tags=integration -timeout=30m -run="TestCompleteWorkflow" ./tests/integration/...
```

#### 2. Create README
**File**: `tests/integration/README.md`

Create comprehensive README with:
- Setup instructions
- Full IAM policy JSON
- Environment variable reference
- Cost estimates table
- Troubleshooting guide

### Success Criteria

#### Automated Verification:
- [ ] `make test-integration` shows help when no account ID
- [ ] Documentation is complete

#### Manual Verification:
- [ ] Run `make test-integration-fast` end-to-end
- [ ] Run `make test-integration-workflow` end-to-end

---

## Testing Strategy

### Unit Tests (Existing)
- Mock-based tests in `internal/infra/aws/*_test.go`
- Fast, run on every commit
- `make test`

### Integration Tests (New)
- Real AWS resources
- Run manually or in dedicated CI job
- `make test-integration`

### Test Scenarios Covered

| Scenario | Test Location |
|----------|---------------|
| List untagged resources | Each scanner `*_test.go` |
| Tag new resources | Each scanner `*_test.go` |
| Delete expired resources | Each scanner `*_test.go` |
| Skip `DoNotDelete=true` | `eip_test.go`, `workflow_test.go` |
| Skip `expiration-date=never` | `eip_test.go`, `workflow_test.go` |
| Complete workflow | `workflow_test.go` |
| Skip patterns (logs) | `logs_test.go` |
| Cleanup on failure | `cleanup_test.go` via `main_test.go` |

---

## References

- Existing test patterns: `internal/infra/aws/rds_repository_test.go`
- Client factory: `internal/infra/aws/client.go`
- Repository factory: `internal/infra/aws/repository_factory.go`
- Janitor service: `internal/app/service/janitor.go`
- Domain types: `internal/domain/resource.go`

---

> ⚠️ **DO NOT IMPLEMENT** - This plan requires explicit `/implement` command to begin execution.
