# Integration Test IAM Role Assumption Implementation Plan

## Overview

Add IAM role assumption support to the integration test suite, allowing tests to run against AWS accounts that require cross-account access via `sts:AssumeRole`.

## Current State Analysis

- **Integration tests exist** in `tests/integration/` with full coverage of 14 AWS scanners
- **Main app supports role assumption** via `internal/infra/aws/client.go` using `stscreds.NewAssumeRoleProvider`
- **Integration tests use direct credentials** - `config.LoadDefaultConfig()` without role assumption
- **TestConfig** has `AccountID`, `Region`, `EKSRoleARN`, `SageMakerRoleARN` but no general role ARN

## Desired End State

Integration tests support cross-account role assumption:
1. New `TEST_ROLE_ARN` environment variable for specifying a role to assume
2. Tests assume the role before creating AWS clients (if configured)
3. Documentation includes IAM trust policy examples
4. Makefile help text updated with new env var

### Success Verification
```bash
# Run tests with cross-account role assumption
TEST_AWS_ACCOUNT_ID=123456789012 \
TEST_ROLE_ARN=arn:aws:iam::123456789012:role/CloudJanitorIntegrationTest \
make test-integration-fast
```

## What We're NOT Doing

- **Session name customization** - using SDK defaults
- **External ID support** - not needed for this use case
- **Role chaining** - only single role assumption
- **Credential caching** - SDK handles this automatically

---

## Phase 1: Add Role ARN Configuration

### Overview
Add `TEST_ROLE_ARN` to the test configuration and load it from environment variables.

### Changes Required

#### 1. Update TestConfig struct
**File**: `tests/integration/config_test.go`

Add `RoleARN` field to the `TestConfig` struct:

```go
// TestConfig holds configuration for integration tests.
type TestConfig struct {
	AccountID        string
	Region           string
	RoleARN          string // Optional: IAM role to assume for cross-account access
	EKSRoleARN       string // Optional: IAM role for EKS clusters
	SageMakerRoleARN string // Optional: IAM role for SageMaker notebooks
}
```

#### 2. Load TEST_ROLE_ARN in loadTestConfig()
**File**: `tests/integration/config_test.go`

Update the `loadTestConfig()` function to read `TEST_ROLE_ARN`:

```go
func loadTestConfig() {
	testConfig = TestConfig{
		AccountID:        os.Getenv("TEST_AWS_ACCOUNT_ID"),
		Region:           os.Getenv("TEST_AWS_REGION"),
		RoleARN:          os.Getenv("TEST_ROLE_ARN"),
		EKSRoleARN:       os.Getenv("TEST_EKS_ROLE_ARN"),
		SageMakerRoleARN: os.Getenv("TEST_SAGEMAKER_ROLE_ARN"),
	}

	if testConfig.Region == "" {
		testConfig.Region = "us-west-2"
	}
}
```

### Success Criteria

#### Automated Verification:
- [x] `go build -tags=integration ./tests/integration/...` succeeds
- [x] `go vet -tags=integration ./tests/integration/...` passes

#### Manual Verification:
- [x] Verify `TEST_ROLE_ARN` is correctly loaded when set

**Pause for manual confirmation before Phase 2.**

---

## Phase 2: Implement Role Assumption in Client Initialization

### Overview
Modify `initClients()` to assume the configured role before creating AWS service clients.

### Changes Required

#### 1. Update imports
**File**: `tests/integration/clients_test.go`

Add required imports for role assumption:

```go
import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	// ... existing service imports
)
```

#### 2. Modify initClients() to assume role
**File**: `tests/integration/clients_test.go`

Update `initClients()` to use `stscreds.NewAssumeRoleProvider` when `TEST_ROLE_ARN` is set:

```go
// initClients initializes all AWS clients.
// If TEST_ROLE_ARN is set, it assumes that role before creating clients.
func initClients(ctx context.Context) error {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(testConfig.Region))
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	// If a role ARN is configured, assume that role
	if testConfig.RoleARN != "" {
		stsClient := sts.NewFromConfig(cfg)
		creds := stscreds.NewAssumeRoleProvider(stsClient, testConfig.RoleARN)
		cfg.Credentials = aws.NewCredentialsCache(creds)
	}

	clients = &testClients{
		// ... create clients from cfg
	}

	return nil
}
```

### Success Criteria

#### Automated Verification:
- [x] `go build -tags=integration ./tests/integration/...` succeeds
- [x] `go vet -tags=integration ./tests/integration/...` passes
- [x] No lint errors in modified files

#### Manual Verification:
- [ ] Test with valid role ARN - tests should authenticate with assumed role
- [ ] Test without role ARN - tests should use default credentials
- [ ] Test with invalid role ARN - should fail with clear error message

**Pause for manual confirmation before Phase 3.**

---

## Phase 3: Documentation Updates

### Overview
Update README and Makefile with documentation for the new `TEST_ROLE_ARN` feature.

### Changes Required

#### 1. Update README environment variables table
**File**: `tests/integration/README.md`

Add `TEST_ROLE_ARN` to the environment variables table:

```markdown
| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TEST_AWS_ACCOUNT_ID` | Yes | - | AWS account ID for testing |
| `TEST_AWS_REGION` | No | `us-west-2` | AWS region to run tests in |
| `TEST_ROLE_ARN` | No | - | IAM role ARN to assume for cross-account access |
| `TEST_EKS_ROLE_ARN` | No | - | IAM role ARN for EKS cluster |
| `TEST_SAGEMAKER_ROLE_ARN` | No | - | IAM role ARN for SageMaker notebooks |
```

#### 2. Add Cross-Account Access section to README
**File**: `tests/integration/README.md`

Add new section after IAM Policy:

```markdown
## Cross-Account Access

If you need to run integration tests against a different AWS account (e.g., a dedicated test account), you can use IAM role assumption with the `TEST_ROLE_ARN` environment variable.

### Setup

1. **Create an IAM role** in the target test account with the IAM policy above
2. **Add a trust policy** to allow your source account/user to assume the role
3. **Set the `TEST_ROLE_ARN`** environment variable when running tests

### Trust Policy

Add this trust policy to the IAM role in the target account:

\`\`\`json
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
\`\`\`

### Usage

\`\`\`bash
TEST_AWS_ACCOUNT_ID=123456789012 \
TEST_ROLE_ARN=arn:aws:iam::123456789012:role/CloudJanitorIntegrationTest \
make test-integration-fast
\`\`\`
```

#### 3. Update Makefile help text
**File**: `Makefile`

Update the `test-integration` target to include `TEST_ROLE_ARN`:

```makefile
test-integration:
	@if [ -z "$(TEST_AWS_ACCOUNT_ID)" ]; then \
		echo "Optional environment variables:"; \
		echo "  TEST_AWS_REGION         - AWS region (default: us-west-2)"; \
		echo "  TEST_ROLE_ARN           - IAM role to assume for cross-account access"; \
		echo "  TEST_EKS_ROLE_ARN       - IAM role for EKS clusters"; \
		echo "  TEST_SAGEMAKER_ROLE_ARN - IAM role for SageMaker notebooks"; \
		...
	fi
	@if [ -n "$(TEST_ROLE_ARN)" ]; then echo "Role: $(TEST_ROLE_ARN)"; fi
```

### Success Criteria

#### Automated Verification:
- [x] `make help` shows updated help text
- [x] README renders correctly

#### Manual Verification:
- [x] Documentation is clear and complete
- [x] Trust policy example is correct
- [x] Usage example works as documented

---

## Implementation Summary

### Files Modified

| File | Change |
|------|--------|
| `tests/integration/config_test.go` | Added `RoleARN` field and loading from env |
| `tests/integration/clients_test.go` | Added role assumption in `initClients()` |
| `tests/integration/README.md` | Added env var docs and Cross-Account section |
| `Makefile` | Updated help text with `TEST_ROLE_ARN` |

### Pattern Used

The implementation follows the same pattern as the main application in `internal/infra/aws/client.go:55-58`:

```go
if account.RoleARN != "" {
	stsClient := sts.NewFromConfig(cfg)
	creds := stscreds.NewAssumeRoleProvider(stsClient, account.RoleARN)
	cfg.Credentials = aws.NewCredentialsCache(creds)
}
```

This ensures consistency across the codebase and uses the recommended AWS SDK v2 approach for role assumption.
