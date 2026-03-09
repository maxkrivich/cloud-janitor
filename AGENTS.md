# Cloud Janitor

Cloud Janitor is a Go CLI tool for automated AWS resource cleanup. It uses tag-based expiration to automatically remove unused resources from development accounts after a 30-day grace period.

## How It Works

Cloud Janitor runs daily (via TeamCity or CI) and performs a two-step process:

1. **Tag**: Resources without an `expiration-date` tag get tagged with a date 30 days from now
2. **Cleanup**: Resources with an `expiration-date` in the past are deleted

This ensures all resources in dev accounts are automatically cleaned up unless someone explicitly extends the expiration date.

## Project Context

Before making changes, review:

- @PRODUCT.md - Product vision, features, and roadmap
- @ARCHITECTURE.md - System design, interfaces, and directory structure
- @CONTRIBUTING.md - Go coding standards, testing, and PR process

## Technical Stack

- **Language**: Go 1.21+
- **CLI Framework**: Cobra
- **Module**: `github.com/maxkrivich/cloud-janitor`
- **Cloud Provider**: AWS (initially)

## Key Interfaces

When implementing new features, follow these core interfaces:

```go
// Scanner interface - implement for each resource type
type Scanner interface {
    // Type returns the resource type identifier (ec2, ebs, etc.)
    Type() string
    
    // List returns all resources of this type in the region
    List(ctx context.Context, region string) ([]Resource, error)
    
    // Tag adds the expiration-date tag to a resource
    Tag(ctx context.Context, resourceID string, expirationDate time.Time) error
    
    // Delete removes the resource
    Delete(ctx context.Context, resourceID string) error
}

// Resource represents an AWS resource with expiration tracking
type Resource struct {
    ID             string
    Type           string
    Region         string
    AccountID      string
    Name           string
    ExpirationDate *time.Time  // nil = untagged, will be tagged
    Tags           map[string]string
}
```

## Directory Structure

```
cmd/           # Cobra commands (root, run, tag, cleanup, list, version)
internal/      # Private packages (config, engine, output)
pkg/           # Public packages (provider, aws, resource)
```

## Development Guidelines

### Go Best Practices
- Use `context.Context` for cancellation and timeouts
- Wrap errors with context: `fmt.Errorf("scanning %s: %w", region, err)`
- Use table-driven tests for comprehensive coverage
- Keep functions focused (<50 lines when possible)
- Document all exported types and functions

### Testing Requirements
- Write tests before or alongside implementation (TDD preferred)
- Use table-driven tests for multiple scenarios
- Mock external AWS API calls
- Aim for >80% coverage on business logic

### Error Handling
- Never ignore errors
- Provide actionable error messages
- Aggregate errors from parallel operations (don't fail fast)

### Security
- Never log credentials or sensitive data
- Use IAM roles and temporary credentials
- Validate all user inputs

## Common Tasks

### Adding a New Scanner
1. Create `pkg/aws/scanners/<service>.go`
2. Implement `Scanner` interface (List, Tag, Delete)
3. Register in provider
4. Add tests
5. Update documentation

### Adding a New Command
1. Create `cmd/<command>.go`
2. Register with root command
3. Add tests
4. Update help text

## Coding Patterns

### AWS API Calls
```go
// Use pagination for list operations
paginator := ec2.NewDescribeInstancesPaginator(client, input)
for paginator.HasMorePages() {
    page, err := paginator.NextPage(ctx)
    if err != nil {
        return fmt.Errorf("listing instances: %w", err)
    }
    // process page.Reservations
}
```

### Concurrent Scanning
```go
// Scan regions in parallel with error aggregation
g, ctx := errgroup.WithContext(ctx)
for _, region := range regions {
    region := region
    g.Go(func() error {
        return s.scanRegion(ctx, region)
    })
}
if err := g.Wait(); err != nil {
    return err
}
```

---

Suggest updates to the referenced documents if you find incomplete or conflicting information.
