---
description: Plan implementation of a new AWS service scanner
agent: plan
---

Create an implementation plan for adding a new AWS scanner for: **$ARGUMENTS**

## Analysis Questions

1. What AWS API calls are needed to list resources?
2. What criteria determine if a resource is "unused"?
3. How do we estimate the cost of each resource?
4. What configuration options should users have?

## Plan Structure

### Scanner Implementation
- `pkg/aws/scanners/<service>.go` - Scanner struct and methods
- Implement `Scanner` interface (Type, Scan, EstimateCost)
- Handle pagination for list operations
- Extract relevant resource metadata

### Testing
- `pkg/aws/scanners/<service>_test.go` - Unit tests
- Mock AWS client for testing
- Test cases: happy path, empty results, API errors, pagination

### Integration
- Register scanner in `pkg/aws/provider.go`
- Add configuration options in `internal/config/config.go`
- Update documentation

## Reference

Review existing scanners in `pkg/aws/scanners/` for patterns to follow.
