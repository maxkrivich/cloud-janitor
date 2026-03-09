# Plan: Fix AssumeRole Bug

**Status**: `implemented`
**Created**: 2026-03-09

## Problem

When a role ARN is specified in the config, Cloud Janitor is not calling `sts:AssumeRole`. Instead, it uses the IAM user's credentials directly, resulting in `AccessDenied` errors.

**Evidence**: CloudTrail shows `ec2:DescribeInstances` calls from the IAM user but no `sts:AssumeRole` calls.

## Root Cause

In `internal/infra/config/loader.go`, `viper.SetConfigName()` was called twice:
```go
v.SetConfigName("cloud-janitor")
v.SetConfigName(".cloud-janitor")  // This overwrites the first one!
```

The second call overwrote the first, so viper was only looking for `.cloud-janitor.yaml` instead of `cloud-janitor.yaml`. The config file was never loaded, resulting in an empty accounts array, which meant no role was ever assumed.

## Solution

Removed the duplicate `SetConfigName(".cloud-janitor")` call. Now viper correctly finds `cloud-janitor.yaml`.

## Files Modified

- `internal/infra/config/loader.go` - Removed duplicate SetConfigName call

## Testing

```bash
./cloud-janitor list --regions us-east-1
```

Now correctly:
1. Loads `cloud-janitor.yaml`
2. Parses the accounts with RoleARN
3. Calls `sts:AssumeRole` before making EC2 API calls
4. Lists resources using the assumed role's permissions
