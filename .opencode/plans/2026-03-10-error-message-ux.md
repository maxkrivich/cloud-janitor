# Error Message UX Improvement Plan

## Overview
Improve error message UX by applying Google's Technical Writing guidelines to domain errors and table formatter output. Focus on high-impact, user-facing areas only.

## Current State Analysis
- Domain sentinel errors use "failed to" prefix (Google advises against)
- `ResourceError.Error()` outputs `"operation type id: error"` format - functional but not user-friendly
- Table formatter shows flat error list with simple bullet points
- Good patterns exist in AWS repositories to model after (SageMaker, RDS, EKS)

## Desired End State
- Error messages answer: "What went wrong?" and "How to fix it?"
- No "failed to" prefixes in user-facing messages
- Clearer, more readable error output in CLI
- Consistent messaging style across domain layer

## What We're NOT Doing
- Updating all ~500 error points across AWS repositories
- Adding error categorization (actionable/transient/permission)
- Adding error codes for programmatic handling
- Adding `--verbose` flag for error detail levels
- Changing JSON formatter output

## Implementation Approach
Update 3 key areas: domain sentinel errors, ResourceError formatting, and table formatter display. Each change is small and testable independently.

---

## Phase 1: Update Domain Sentinel Errors

### Overview
Remove "failed to" prefix from sentinel errors, making them clearer and more direct per Google guidelines.

### Changes Required

#### 1. Domain Errors
**File**: `internal/domain/errors.go`
**Changes**: Update sentinel error messages

Current:
```go
var (
    ErrResourceNotFound    = errors.New("resource not found")
    ErrTagFailed           = errors.New("failed to tag resource")
    ErrDeleteFailed        = errors.New("failed to delete resource")
    ErrNotificationFailed  = errors.New("failed to send notification")
)
```

New:
```go
var (
    ErrResourceNotFound    = errors.New("resource not found")
    ErrTagFailed           = errors.New("tag operation unsuccessful")
    ErrDeleteFailed        = errors.New("delete operation unsuccessful")
    ErrNotificationFailed  = errors.New("notification delivery unsuccessful")
)
```

### Success Criteria

#### Automated Verification:
- [x] Tests pass: `make test`
- [x] Linting passes: `make lint` (domain package passes; pre-existing issues in other files)
- [x] Build succeeds: `make build`

#### Manual Verification:
- [x] Grep for "failed to" in domain/errors.go returns no results

**Implementation Note**: After completing this phase and all automated verification passes, pause for manual confirmation before proceeding to Phase 2.

---

## Phase 2: Improve ResourceError Format

### Overview
Make `ResourceError.Error()` output more readable and structured.

### Changes Required

#### 1. ResourceError.Error() method
**File**: `internal/domain/errors.go`
**Changes**: Improve error string format

Current:
```go
func (e *ResourceError) Error() string {
    return e.Operation + " " + string(e.ResourceType) + " " + e.ResourceID + ": " + e.Err.Error()
}
```

New:
```go
func (e *ResourceError) Error() string {
    return fmt.Sprintf("%s %s (%s): %s", e.Operation, e.ResourceID, e.ResourceType, e.Err.Error())
}
```

This changes output from:
```
tagging aws:ec2 i-1234567890abcdef0: access denied
```

To:
```
tagging i-1234567890abcdef0 (aws:ec2): access denied
```

The resource ID is more prominent (users care about which resource), and the type is in parentheses for context.

#### 2. Add fmt import if not present
**File**: `internal/domain/errors.go`
**Changes**: Ensure `fmt` is imported

### Success Criteria

#### Automated Verification:
- [x] Tests pass: `make test`
- [x] Linting passes: `make lint`
- [x] Build succeeds: `make build`

#### Manual Verification:
- [x] Error output format is more readable

**Implementation Note**: After completing this phase and all automated verification passes, pause for manual confirmation before proceeding to Phase 3.

---

## Phase 3: Enhance Table Formatter Error Display

### Overview
Improve error section formatting in table output for better readability.

### Changes Required

#### 1. Error display section
**File**: `internal/output/table.go`
**Changes**: Improve error section header and formatting

Current (lines 115-136):
```go
// Errors
errorCount := result.TotalErrors()
if errorCount > 0 {
    fmt.Fprintf(w, "\nErrors (%d):\n", errorCount)
    fmt.Fprintln(w, strings.Repeat("-", 50))

    for _, err := range result.Errors {
        fmt.Fprintf(w, "  - %v\n", err)
    }
    // ... more error loops
}
```

New:
```go
// Errors
errorCount := result.TotalErrors()
if errorCount > 0 {
    fmt.Fprintf(w, "\nEncountered %d error(s):\n", errorCount)
    fmt.Fprintln(w, strings.Repeat("-", 50))

    for _, err := range result.Errors {
        fmt.Fprintf(w, "  • %v\n", err)
    }

    for key, tagResult := range result.TagResults {
        for _, err := range tagResult.Errors {
            fmt.Fprintf(w, "  • [%s] %v\n", key, err)
        }
    }

    for key, cleanupResult := range result.CleanupResults {
        for _, err := range cleanupResult.Errors {
            fmt.Fprintf(w, "  • [%s] %v\n", key, err)
        }
    }

    fmt.Fprintln(w)
    fmt.Fprintln(w, "Tip: Use --dry-run to preview operations, or check IAM permissions if access denied.")
}
```

Changes:
- Header: "Errors (N):" → "Encountered N error(s):" (more natural language)
- Bullet: `-` → `•` (cleaner visual)
- Added helpful tip at the end (actionable guidance per Google guidelines)

### Success Criteria

#### Automated Verification:
- [x] Tests pass: `make test`
- [x] Linting passes: `make lint`
- [x] Build succeeds: `make build`

#### Manual Verification:
- [ ] Run `cloud-janitor run --dry-run` with invalid credentials to see error output
- [ ] Verify tip message appears after errors

---

## Testing Strategy

### Unit Tests
- `internal/domain/errors_test.go`: Verify `ResourceError.Error()` format
- `internal/output/table_test.go`: Verify error section formatting (if tests exist)

### Manual Testing Steps
1. Run with invalid AWS credentials: `AWS_ACCESS_KEY_ID=invalid cloud-janitor list`
2. Verify error message doesn't say "failed to"
3. Verify tip message appears
4. Verify resource ID is prominent in error output

## References
- Research: `.opencode/research/2026-03-10-tui-error-messages.md`
- Google Error Message Guidelines: https://developers.google.com/tech-writing/error-messages/summary
- Good pattern example: `internal/infra/aws/sagemaker_repository.go:119`

---

> **DO NOT IMPLEMENT** - This plan requires explicit `/implement` command to begin execution.
