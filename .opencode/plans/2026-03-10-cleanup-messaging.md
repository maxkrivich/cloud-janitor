# Cleanup Messaging Improvement Plan [IMPLEMENTED]

## Overview

Improve integration test cleanup output to show friendly messages when resources are already deleted, instead of verbose AWS error stack traces.

## Current State Analysis

- **File**: `tests/integration/cleanup_test.go`
- **Problem**: When `ListTagDelete` tests explicitly call `repo.Delete()` to test delete functionality, the cleanup registry still tries to delete those resources at test suite end
- **Result**: AWS returns "NotFound" errors which clutter the test output with scary-looking error messages
- **Root Cause**: This is expected behavior (safety-first cleanup design), not a bug

### Current Output (Before)
```
  Cleaning up: Volume vol-06e633f3effac48e
Cleanup errors:
- cleanup Volume vol-06e633f3effac48e: operation error EC2: DeleteVolume, https response error StatusCode: 400, RequestID: ..., api error InvalidVolume.NotFound: The volume "vol-06e633f3effac48e" does not exist.
```

## Desired End State

Clean, informative cleanup output that:
1. Shows `(already deleted)` for resources that were already removed
2. Shows `done` for successfully cleaned resources
3. Shows `FAILED` for real errors
4. Prints a summary with counts at the end

### Expected Output (After)
```
  Cleaning up: Volume vol-06e633f3effac48e (already deleted)
  Cleaning up: Snapshot snap-0f1fb619596370f6 (already deleted)
  Cleaning up: EIP eipalloc-0e9682c9d023d983 done
  Cleaning up: LogGroup /cloud-janitor-test/... (already deleted)

Cleanup complete: 1 succeeded, 3 already deleted, 0 failed
```

## What We're NOT Doing

- **Not adding `Unregister()` method** - Would require updating every test
- **Not making cleanups check existence first** - Extra API calls, more complex
- **Not hiding all errors** - Only suppressing known "NotFound" patterns

---

## Phase 1: Add AWS NotFound Error Detection

### Overview
Add a helper function to detect AWS "resource not found" errors using the smithy API error interface.

### Changes Required

#### 1. Update imports
**File**: `tests/integration/cleanup_test.go`

Add required imports:
```go
import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/aws/smithy-go"
)
```

#### 2. Add isNotFoundError helper
**File**: `tests/integration/cleanup_test.go`
**Location**: After `Register()` function, before `RunAll()`

```go
// isNotFoundError checks if the error is an AWS "resource not found" error.
// These errors are expected during cleanup when tests explicitly delete resources.
func isNotFoundError(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		// AWS "not found" error patterns
		return strings.HasSuffix(code, ".NotFound") ||
			strings.HasSuffix(code, "NotFound") ||
			code == "ResourceNotFoundException"
	}
	return false
}
```

### AWS Error Codes Covered

| Service | Error Code |
|---------|------------|
| EC2 (Volume) | `InvalidVolume.NotFound` |
| EC2 (Snapshot) | `InvalidSnapshot.NotFound` |
| EC2 (EIP) | `InvalidAllocationID.NotFound` |
| EC2 (Instance) | `InvalidInstanceID.NotFound` |
| EC2 (NAT GW) | `InvalidNatGatewayID.NotFound` |
| EC2 (AMI) | `InvalidAMIID.NotFound` |
| CloudWatch Logs | `ResourceNotFoundException` |
| RDS | `DBInstanceNotFound` |
| ELB | `LoadBalancerNotFound` |

### Success Criteria

#### Automated Verification:
- [x] `go build -tags=integration ./tests/integration/...` succeeds
- [x] `go vet -tags=integration ./tests/integration/...` passes

#### Manual Verification:
- [ ] Helper function correctly identifies NotFound errors

---

## Phase 2: Update RunAll() with Improved Messaging

### Overview
Modify the cleanup loop to show friendly status messages and track counts for a summary.

### Changes Required

#### 1. Update RunAll() loop
**File**: `tests/integration/cleanup_test.go`
**Location**: Replace the cleanup loop in `RunAll()` function

**Before:**
```go
var errs []error
for _, c := range sorted {
    fmt.Printf("  Cleaning up: %s\n", c.Name)
    if err := c.Fn(ctx); err != nil {
        errs = append(errs, fmt.Errorf("cleanup %s: %w", c.Name, err))
        // Continue cleaning up other resources
    }
}

return errs
```

**After:**
```go
var errs []error
var succeeded, alreadyDeleted, failed int

for _, c := range sorted {
    fmt.Printf("  Cleaning up: %s", c.Name)
    if err := c.Fn(ctx); err != nil {
        if isNotFoundError(err) {
            fmt.Printf(" (already deleted)\n")
            alreadyDeleted++
        } else {
            fmt.Printf(" FAILED\n")
            errs = append(errs, fmt.Errorf("cleanup %s: %w", c.Name, err))
            failed++
        }
    } else {
        fmt.Printf(" done\n")
        succeeded++
    }
}

fmt.Printf("\nCleanup complete: %d succeeded, %d already deleted, %d failed\n", succeeded, alreadyDeleted, failed)

return errs
```

### Success Criteria

#### Automated Verification:
- [x] `go build -tags=integration ./tests/integration/...` succeeds
- [x] `go vet -tags=integration ./tests/integration/...` passes

#### Manual Verification:
- [ ] Run integration tests and verify:
  - Resources deleted by tests show `(already deleted)`
  - Resources cleaned by cleanup show `done`
  - Summary line shows correct counts
  - No "NotFound" errors in error list

---

## Testing Strategy

### Manual Testing Steps

1. Run fast integration tests:
   ```bash
   TEST_AWS_ACCOUNT_ID=<account> make test-integration-fast
   ```

2. Verify cleanup output shows:
   - `(already deleted)` for EIP, Volume, Snapshot that were deleted in tests
   - `done` for NeverExpires/Excluded resources that weren't deleted
   - Summary line with counts

3. Verify no "NotFound" errors appear in the final error list

---

## Implementation Status

- [x] Phase 1: Add AWS NotFound Error Detection
- [x] Phase 2: Update RunAll() with Improved Messaging
- [x] Manual verification complete

## References

- Research: Conducted inline during conversation (no separate research document)
- File modified: `tests/integration/cleanup_test.go:69-141`
