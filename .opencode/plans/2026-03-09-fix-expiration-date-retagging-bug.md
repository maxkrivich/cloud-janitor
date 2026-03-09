# Plan: Fix Expiration Date Re-tagging Bug

**Status**: `implemented`
**Created**: 2026-03-09

## Problem

Resources with existing `expiration-date` tags were being re-tagged instead of being evaluated for deletion. When a resource had an `expiration-date` tag (e.g., `2026-03-01`) and today's date was past that (e.g., `2026-03-09`), Cloud Janitor should DELETE the resource. Instead, it re-tagged it with a new expiration date 30 days from now.

## Root Cause

Two separate bugs caused this behavior:

### Bug 1: Silent Date Parsing Failures (All 14 Repositories)

In all repository files, date parsing failures were silent:

```go
if t, err := time.Parse(ExpirationDateFormat, value); err == nil {
    resource.ExpirationDate = &t
}
```

When `time.Parse()` failed (due to whitespace, wrong format, etc.), the error was ignored, `ExpirationDate` remained `nil`, and `Status()` returned `StatusUntagged`, causing the resource to be re-tagged.

### Bug 2: CloudWatch Logs ARN Suffix Mismatch

AWS `DescribeLogGroups` API returns ARNs with a `:*` suffix:
```
arn:aws:logs:us-east-1:123456789012:log-group:/my-app/logs:*
```

But `ListTagsForResource` requires ARNs **without** the `:*` suffix:
```
arn:aws:logs:us-east-1:123456789012:log-group:/my-app/logs
```

The `fetchTags` function passed the ARN directly, causing `ListTagsForResource` to fail. Since errors were silently handled (returning empty tags), the resource appeared as "untagged".

## Solution

### Fix 1: Centralized Date Parsing with Warnings

Created `internal/infra/aws/tag_helpers.go` with helper functions:

- `ParseExpirationDate(value, resourceID, resourceType string) *time.Time` - Trims whitespace before parsing and logs warnings on failure
- `IsNeverExpires(value string) bool` - Checks for "never" value with whitespace handling

Updated all 14 repository files to use the new helpers.

### Fix 2: Strip ARN Suffix for CloudWatch Logs

In `logs_repository.go`, added `strings.TrimSuffix(arn, ":*")` before calling `ListTagsForResource`, and added warning logging when tag fetch fails.

## Files Created

- `internal/infra/aws/tag_helpers.go` - Helper functions for date parsing
- `internal/infra/aws/tag_helpers_test.go` - Tests for helper functions

## Files Modified

### Repositories Updated to Use Helpers
- `internal/infra/aws/ec2_repository.go`
- `internal/infra/aws/ebs_repository.go`
- `internal/infra/aws/snapshot_repository.go`
- `internal/infra/aws/eip_repository.go`
- `internal/infra/aws/rds_repository.go`
- `internal/infra/aws/elb_repository.go`
- `internal/infra/aws/natgw_repository.go`
- `internal/infra/aws/elasticache_repository.go` (2 locations)
- `internal/infra/aws/opensearch_repository.go`
- `internal/infra/aws/eks_repository.go`
- `internal/infra/aws/redshift_repository.go`
- `internal/infra/aws/sagemaker_repository.go`
- `internal/infra/aws/ami_repository.go`
- `internal/infra/aws/logs_repository.go` (also ARN suffix fix)

### Tests Updated
- `internal/infra/aws/logs_repository_test.go` - Added test for ARN with `:*` suffix

## Testing

```bash
# Run all tests
go test ./...

# Build
go build ./...

# Manual verification with CloudWatch Logs
./cloud-janitor list --regions us-east-1
```

All tests pass and the bug is confirmed fixed with manual testing.

## Commit

```
fix(aws): prevent re-tagging of resources with existing expiration dates

Two bugs caused resources with valid expiration-date tags to be
incorrectly identified as "untagged" and re-tagged:

1. Date parsing silently failed when tag values had whitespace or
   invalid formats, leaving ExpirationDate as nil. Created shared
   helper functions (ParseExpirationDate, IsNeverExpires) that trim
   whitespace and log warnings on parse failures. Applied fix to all
   14 repository files.

2. CloudWatch Logs fetchTags failed silently because DescribeLogGroups
   returns ARNs with :* suffix, but ListTagsForResource requires ARNs
   without it. Fixed by stripping the suffix before fetching tags.

Added comprehensive tests for the new helper functions and ARN handling.
```
