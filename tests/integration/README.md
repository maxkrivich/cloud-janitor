# Cloud Janitor Integration Tests

This directory contains integration tests that run against a real AWS account. These tests validate the complete Cloud Janitor workflow including resource detection, tagging, and cleanup.

## Prerequisites

1. **AWS Account**: A dedicated test/development AWS account (NOT production)
2. **AWS Credentials**: Valid AWS credentials with sufficient permissions
3. **Go 1.21+**: Required to run the tests

## Quick Start

```bash
# Run fast tests only (~5 minutes, ~$0.10 cost)
TEST_AWS_ACCOUNT_ID=123456789012 make test-integration-fast

# Run complete workflow test (~10 minutes)
TEST_AWS_ACCOUNT_ID=123456789012 make test-integration-workflow

# Run all integration tests (~90 minutes, ~$1-2 cost)
TEST_AWS_ACCOUNT_ID=123456789012 make test-integration
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TEST_AWS_ACCOUNT_ID` | Yes | - | AWS account ID for testing |
| `TEST_AWS_REGION` | No | `us-west-2` | AWS region to run tests in |
| `TEST_ROLE_ARN` | No | - | IAM role ARN to assume for cross-account access |
| `TEST_EKS_ROLE_ARN` | No | - | IAM role ARN for EKS cluster (skips EKS tests if not set) |
| `TEST_SAGEMAKER_ROLE_ARN` | No | - | IAM role ARN for SageMaker notebooks (skips tests if not set) |

## IAM Policy

The following IAM policy grants minimum required permissions for running integration tests:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "EC2Permissions",
      "Effect": "Allow",
      "Action": [
        "ec2:AllocateAddress",
        "ec2:ReleaseAddress",
        "ec2:DescribeAddresses",
        "ec2:CreateVolume",
        "ec2:DeleteVolume",
        "ec2:DescribeVolumes",
        "ec2:CreateSnapshot",
        "ec2:DeleteSnapshot",
        "ec2:DescribeSnapshots",
        "ec2:CreateImage",
        "ec2:RegisterImage",
        "ec2:DeregisterImage",
        "ec2:DescribeImages",
        "ec2:RunInstances",
        "ec2:TerminateInstances",
        "ec2:DescribeInstances",
        "ec2:CreateNatGateway",
        "ec2:DeleteNatGateway",
        "ec2:DescribeNatGateways",
        "ec2:CreateVpc",
        "ec2:DeleteVpc",
        "ec2:DescribeVpcs",
        "ec2:ModifyVpcAttribute",
        "ec2:CreateSubnet",
        "ec2:DeleteSubnet",
        "ec2:DescribeSubnets",
        "ec2:CreateInternetGateway",
        "ec2:DeleteInternetGateway",
        "ec2:AttachInternetGateway",
        "ec2:DetachInternetGateway",
        "ec2:DescribeInternetGateways",
        "ec2:CreateRouteTable",
        "ec2:DeleteRouteTable",
        "ec2:CreateRoute",
        "ec2:AssociateRouteTable",
        "ec2:DisassociateRouteTable",
        "ec2:DescribeRouteTables",
        "ec2:CreateTags",
        "ec2:DescribeTags",
        "ec2:DescribeAvailabilityZones"
      ],
      "Resource": "*"
    },
    {
      "Sid": "ELBPermissions",
      "Effect": "Allow",
      "Action": [
        "elasticloadbalancing:CreateLoadBalancer",
        "elasticloadbalancing:DeleteLoadBalancer",
        "elasticloadbalancing:DescribeLoadBalancers",
        "elasticloadbalancing:DescribeTags",
        "elasticloadbalancing:AddTags"
      ],
      "Resource": "*"
    },
    {
      "Sid": "CloudWatchLogsPermissions",
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogGroup",
        "logs:DeleteLogGroup",
        "logs:DescribeLogGroups",
        "logs:ListTagsForResource",
        "logs:TagResource"
      ],
      "Resource": "*"
    },
    {
      "Sid": "RDSPermissions",
      "Effect": "Allow",
      "Action": [
        "rds:CreateDBInstance",
        "rds:DeleteDBInstance",
        "rds:DescribeDBInstances",
        "rds:ModifyDBInstance",
        "rds:ListTagsForResource",
        "rds:AddTagsToResource"
      ],
      "Resource": "*"
    },
    {
      "Sid": "ElastiCachePermissions",
      "Effect": "Allow",
      "Action": [
        "elasticache:CreateCacheCluster",
        "elasticache:DeleteCacheCluster",
        "elasticache:DescribeCacheClusters",
        "elasticache:ListTagsForResource",
        "elasticache:AddTagsToResource"
      ],
      "Resource": "*"
    },
    {
      "Sid": "RedshiftPermissions",
      "Effect": "Allow",
      "Action": [
        "redshift:CreateCluster",
        "redshift:DeleteCluster",
        "redshift:DescribeClusters",
        "redshift:CreateTags"
      ],
      "Resource": "*"
    },
    {
      "Sid": "OpenSearchPermissions",
      "Effect": "Allow",
      "Action": [
        "es:CreateDomain",
        "es:DeleteDomain",
        "es:DescribeDomain",
        "es:ListDomainNames",
        "es:ListTags",
        "es:AddTags"
      ],
      "Resource": "*"
    },
    {
      "Sid": "EKSPermissions",
      "Effect": "Allow",
      "Action": [
        "eks:CreateCluster",
        "eks:DeleteCluster",
        "eks:DescribeCluster",
        "eks:ListClusters",
        "eks:TagResource"
      ],
      "Resource": "*"
    },
    {
      "Sid": "SageMakerPermissions",
      "Effect": "Allow",
      "Action": [
        "sagemaker:CreateNotebookInstance",
        "sagemaker:DeleteNotebookInstance",
        "sagemaker:DescribeNotebookInstance",
        "sagemaker:ListNotebookInstances",
        "sagemaker:StopNotebookInstance",
        "sagemaker:ListTags",
        "sagemaker:AddTags"
      ],
      "Resource": "*"
    },
    {
      "Sid": "IAMPassRole",
      "Effect": "Allow",
      "Action": "iam:PassRole",
      "Resource": [
        "arn:aws:iam::*:role/*EKS*",
        "arn:aws:iam::*:role/*SageMaker*"
      ]
    }
  ]
}
```

## Cross-Account Access

If you need to run integration tests against a different AWS account (e.g., a dedicated test account), you can use IAM role assumption with the `TEST_ROLE_ARN` environment variable.

### Setup

1. **Create an IAM role** in the target test account with the IAM policy above
2. **Add a trust policy** to allow your source account/user to assume the role
3. **Set the `TEST_ROLE_ARN`** environment variable when running tests

### Trust Policy

Add this trust policy to the IAM role in the target account:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::SOURCE_ACCOUNT_ID:root"
      },
      "Action": "sts:AssumeRole",
      "Condition": {}
    }
  ]
}
```

Replace `SOURCE_ACCOUNT_ID` with the AWS account ID where you'll run the tests from.

For more restrictive access, you can limit to specific users or roles:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": [
          "arn:aws:iam::SOURCE_ACCOUNT_ID:user/developer",
          "arn:aws:iam::SOURCE_ACCOUNT_ID:role/CI-Role"
        ]
      },
      "Action": "sts:AssumeRole",
      "Condition": {}
    }
  ]
}
```

### Usage

```bash
# Run tests using cross-account role assumption
TEST_AWS_ACCOUNT_ID=123456789012 \
TEST_ROLE_ARN=arn:aws:iam::123456789012:role/CloudJanitorIntegrationTest \
make test-integration-fast

# The caller must have permission to assume the role
# Add this to your source account IAM policy:
# {
#   "Effect": "Allow",
#   "Action": "sts:AssumeRole",
#   "Resource": "arn:aws:iam::123456789012:role/CloudJanitorIntegrationTest"
# }
```

## Test Categories

### Fast Tests (~5 minutes, ~$0.10)
Resources that create/delete in seconds:
- **EIP**: Elastic IPs (instant)
- **Logs**: CloudWatch Log Groups (instant)
- **Snapshot**: EBS Snapshots (~1-2 min)
- **AMI**: Amazon Machine Images (~2-3 min)
- **EBS**: EBS Volumes (~1 min)

```bash
TEST_AWS_ACCOUNT_ID=123456789012 make test-integration-fast
```

### Medium Tests (~15-30 minutes, ~$0.50)
Resources that take a few minutes:
- **EC2**: EC2 Instances (~2-3 min)
- **NAT Gateway**: NAT Gateways (~3-5 min)
- **ELB**: Application/Network Load Balancers (~3-5 min)

### Slow Tests (~60-90 minutes, ~$1-2)
Resources that take 10-30 minutes to provision:
- **RDS**: MySQL instances (~10-15 min)
- **ElastiCache**: Redis clusters (~10-15 min)
- **Redshift**: Redshift clusters (~10-15 min)
- **OpenSearch**: OpenSearch domains (~15-20 min)
- **SageMaker**: Notebook instances (~5-10 min)
- **EKS**: EKS clusters (~15-20 min)

### Workflow Test (~10 minutes)
End-to-end validation of the complete Cloud Janitor lifecycle:
1. Create resources (normal, excluded, never-expires)
2. Tag untagged resources
3. Verify exclusion filters
4. Simulate expiration
5. Cleanup expired resources
6. Verify final state

```bash
TEST_AWS_ACCOUNT_ID=123456789012 make test-integration-workflow
```

## Cost Estimates

| Test Suite | Duration | Estimated Cost |
|------------|----------|----------------|
| Fast tests | ~5 min | ~$0.10 |
| Workflow test | ~10 min | ~$0.05 |
| Medium tests | ~30 min | ~$0.50 |
| Slow tests | ~60 min | ~$1.50 |
| **All tests** | **~90 min** | **~$2.00** |

Costs are approximate and may vary based on region and current pricing.

## Cleanup Guarantee

Tests use a cleanup registry with panic recovery to ensure resources are deleted even if tests fail:

1. Resources are registered for cleanup immediately after creation
2. Cleanup runs in priority order (dependencies first)
3. Cleanup executes even on test panics or failures
4. Failed cleanups are logged but don't prevent other cleanups

To manually verify no resources were leaked:

```bash
# Check for leftover test resources
aws resourcegroupstaggingapi get-resources \
  --tag-filters Key=cloud-janitor-test,Values=true \
  --region us-west-2
```

## Test Tags

All test resources are created with the following tags for identification:

| Tag | Value | Purpose |
|-----|-------|---------|
| `cloud-janitor-test` | `true` | Identifies test resources |
| `Name` | `cloud-janitor-test-{timestamp}` | Human-readable identifier |

## Troubleshooting

### Tests Skip with "missing config"
Ensure `TEST_AWS_ACCOUNT_ID` is set:
```bash
export TEST_AWS_ACCOUNT_ID=123456789012
```

### EKS/SageMaker tests skip
These require IAM roles to be pre-created:
```bash
export TEST_EKS_ROLE_ARN=arn:aws:iam::123456789012:role/EKSClusterRole
export TEST_SAGEMAKER_ROLE_ARN=arn:aws:iam::123456789012:role/SageMakerExecutionRole
```

### Permission Denied errors
Verify your IAM policy includes all required permissions (see above).

### Resources not cleaned up
If tests are interrupted (Ctrl+C), manually clean up:
```bash
# Find and delete test resources
aws ec2 describe-addresses --filters "Name=tag:cloud-janitor-test,Values=true" --query 'Addresses[].AllocationId' --output text | xargs -n1 aws ec2 release-address --allocation-id

# Or use the tagging API to find all test resources
aws resourcegroupstaggingapi get-resources \
  --tag-filters Key=cloud-janitor-test,Values=true \
  --region us-west-2
```

### Timeout errors
Slow resources (RDS, EKS, OpenSearch) may take longer than expected. The default timeout is 90 minutes for all tests. For individual test runs, you can increase the timeout:

```bash
go test -v -tags=integration -timeout=120m -run=TestRDS ./tests/integration/...
```

## Running Individual Tests

```bash
# Run specific scanner test
TEST_AWS_ACCOUNT_ID=123456789012 \
  go test -v -tags=integration -timeout=30m \
  -run=TestEIPRepository ./tests/integration/...

# Run with verbose AWS SDK logging
AWS_SDK_LOG_LEVEL=debug TEST_AWS_ACCOUNT_ID=123456789012 \
  make test-integration-fast
```

## Architecture

```
tests/integration/
├── main_test.go           # TestMain, setup/teardown
├── config_test.go         # Test configuration from env vars
├── cleanup_test.go        # Cleanup registry with priority-based execution
├── clients_test.go        # AWS SDK client factory
├── helpers_test.go        # Test assertion helpers
├── resource_helpers_test.go # Resource creation helpers
├── vpc_test.go            # VPC/subnet infrastructure setup
├── workflow_test.go       # Complete E2E workflow test
└── *_test.go              # Individual scanner tests (14 files)
```
