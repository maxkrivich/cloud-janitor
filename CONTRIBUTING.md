# Contributing to Cloud Janitor

This project follows the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md). Please read it before contributing.

## Development Setup

### Prerequisites

- Go 1.21 or later
- Make
- AWS credentials (for integration tests)
- golangci-lint (for linting)

### Getting Started

```bash
# Clone the repository
git clone https://github.com/maxkrivich/cloud-janitor.git
cd cloud-janitor

# Install dependencies
go mod download

# Run tests
make test

# Build the binary
make build

# Run linter
make lint
```

## Code Style (Uber Go Style Guide)

### Interface Compliance

Verify interface compliance at compile time:

```go
// Good - compile-time interface check
type SlackNotifier struct {
    webhookURL string
}

var _ domain.Notifier = (*SlackNotifier)(nil)

func (n *SlackNotifier) NotifyTagged(ctx context.Context, resources []domain.Resource) error {
    // ...
}
```

### Error Handling

#### Error Wrapping

Use `%w` for errors that callers may need to match, `%v` otherwise:

```go
// Good - allows caller to use errors.Is/As
if err != nil {
    return fmt.Errorf("tagging EC2 %s: %w", id, err)
}

// Good - context without "failed to" prefix
s, err := store.New()
if err != nil {
    return fmt.Errorf("new store: %w", err)
}
```

#### Error Naming

```go
// Exported errors use Err prefix
var ErrResourceNotFound = errors.New("resource not found")

// Custom error types use Error suffix
type ValidationError struct {
    Field string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("invalid field: %s", e.Field)
}
```

#### Handle Errors Once

Don't log and return - do one or the other:

```go
// Bad
if err != nil {
    log.Printf("failed to get user: %v", err)
    return err
}

// Good - wrap and return
if err != nil {
    return fmt.Errorf("get user %q: %w", id, err)
}

// Good - log and degrade gracefully
if err := emitMetrics(); err != nil {
    log.Printf("emit metrics: %v", err)
    // continue execution
}
```

### Naming Conventions

```go
// Interfaces: use -er suffix or descriptive noun
type Notifier interface {}
type ResourceRepository interface {}

// Structs: descriptive nouns
type EC2Repository struct {}
type SlackNotifier struct {}

// Unexported globals: prefix with underscore
var _defaultTimeout = 30 * time.Second

// Enums: start at 1 (zero value = unset)
type Status int

const (
    StatusUntagged Status = iota + 1
    StatusActive
    StatusExpired
)
```

### Struct Initialization

Always use field names:

```go
// Bad
resource := Resource{"i-123", "ec2", "us-east-1"}

// Good
resource := Resource{
    ID:     "i-123",
    Type:   ResourceTypeEC2,
    Region: "us-east-1",
}
```

### Function Grouping

Order functions logically:
1. Exported functions first
2. Grouped by receiver
3. Sorted by dependency (called functions below callers)

```go
// Constructor first
func NewEC2Repository(client *ec2.Client) *EC2Repository { ... }

// Exported methods
func (r *EC2Repository) List(ctx context.Context, region string) ([]Resource, error) { ... }
func (r *EC2Repository) Tag(ctx context.Context, id string, exp time.Time) error { ... }
func (r *EC2Repository) Delete(ctx context.Context, id string) error { ... }

// Unexported helpers last
func (r *EC2Repository) parseInstance(inst types.Instance) Resource { ... }
```

### Reduce Nesting

Handle errors first, keep happy path un-indented:

```go
// Bad
if err == nil {
    // success logic
    // more success logic
} else {
    return err
}

// Good
if err != nil {
    return err
}
// success logic
// more success logic
```

### Avoid Mutable Globals

Use dependency injection instead:

```go
// Bad
var _timeNow = time.Now

func getExpiration() time.Time {
    return _timeNow().AddDate(0, 0, 30)
}

// Good
type ExpirationCalculator struct {
    now func() time.Time
}

func (c *ExpirationCalculator) GetExpiration() time.Time {
    return c.now().AddDate(0, 0, 30)
}
```

### Functional Options

Use for optional configuration:

```go
type Option func(*Client)

func WithTimeout(d time.Duration) Option {
    return func(c *Client) {
        c.timeout = d
    }
}

func WithRegion(region string) Option {
    return func(c *Client) {
        c.region = region
    }
}

func NewClient(opts ...Option) *Client {
    c := &Client{
        timeout: 30 * time.Second,
        region:  "us-east-1",
    }
    for _, opt := range opts {
        opt(c)
    }
    return c
}
```

### Channel Size

Channels should be unbuffered or size 1:

```go
// Good
done := make(chan struct{})
results := make(chan Resource, 1)

// Bad - arbitrary buffer size
c := make(chan int, 64)
```

### Goroutine Management

Never fire-and-forget goroutines. Always ensure they can be stopped:

```go
// Good - goroutine with controlled lifecycle
func (s *Scanner) Start(ctx context.Context) error {
    g, ctx := errgroup.WithContext(ctx)
    
    for _, region := range s.regions {
        region := region // capture loop variable
        g.Go(func() error {
            return s.scanRegion(ctx, region)
        })
    }
    
    return g.Wait()
}
```

### Exit Only in Main

```go
// Bad - exit in helper function
func readConfig(path string) Config {
    data, err := os.ReadFile(path)
    if err != nil {
        log.Fatal(err) // Don't do this
    }
    // ...
}

// Good - return errors, exit in main
func readConfig(path string) (Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return Config{}, fmt.Errorf("read config: %w", err)
    }
    // ...
}

func main() {
    if err := run(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

### Import Organization

```go
import (
    // Standard library
    "context"
    "fmt"
    "time"

    // External packages
    "github.com/aws/aws-sdk-go-v2/service/ec2"
    "github.com/spf13/cobra"

    // Internal packages
    "github.com/maxkrivich/cloud-janitor/internal/domain"
)
```

## Testing

### Table-Driven Tests

```go
func TestResource_Status(t *testing.T) {
    now := time.Now()
    past := now.AddDate(0, 0, -1)
    future := now.AddDate(0, 0, 1)

    tests := []struct {
        name     string
        resource Resource
        want     Status
    }{
        {
            name:     "untagged resource",
            resource: Resource{ID: "i-123", ExpirationDate: nil},
            want:     StatusUntagged,
        },
        {
            name:     "expired resource",
            resource: Resource{ID: "i-123", ExpirationDate: &past},
            want:     StatusExpired,
        },
        {
            name:     "active resource",
            resource: Resource{ID: "i-123", ExpirationDate: &future},
            want:     StatusActive,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := tt.resource.Status()
            if got != tt.want {
                t.Errorf("Status() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Running Tests

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run specific package
go test -v ./internal/domain/...

# Run integration tests (requires AWS credentials)
make test-integration
```

## Adding New Features

### Adding a New Repository (Scanner)

1. Create file: `internal/infra/aws/<service>_repository.go`
2. Implement the `domain.ResourceRepository` interface
3. Add compile-time interface check
4. Register in provider
5. Write tests
6. Update documentation

```go
// internal/infra/aws/rds_repository.go
package aws

import (
    "context"
    "time"

    "github.com/aws/aws-sdk-go-v2/service/rds"
    "github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check
var _ domain.ResourceRepository = (*RDSRepository)(nil)

type RDSRepository struct {
    client *rds.Client
}

func NewRDSRepository(client *rds.Client) *RDSRepository {
    return &RDSRepository{client: client}
}

func (r *RDSRepository) Type() domain.ResourceType {
    return domain.ResourceTypeRDS
}

func (r *RDSRepository) List(ctx context.Context, region string) ([]domain.Resource, error) {
    // Implementation
}

func (r *RDSRepository) Tag(ctx context.Context, id string, exp time.Time) error {
    // Implementation
}

func (r *RDSRepository) Delete(ctx context.Context, id string) error {
    // Implementation
}
```

### Adding a New Notifier

1. Create file: `internal/infra/notify/<service>.go`
2. Implement the `domain.Notifier` interface
3. Add compile-time interface check
4. Write tests

```go
// internal/infra/notify/teams.go
package notify

import (
    "context"

    "github.com/maxkrivich/cloud-janitor/internal/domain"
)

var _ domain.Notifier = (*TeamsNotifier)(nil)

type TeamsNotifier struct {
    webhookURL string
}

func NewTeamsNotifier(webhookURL string) *TeamsNotifier {
    return &TeamsNotifier{webhookURL: webhookURL}
}

func (n *TeamsNotifier) NotifyTagged(ctx context.Context, resources []domain.Resource) error {
    // Implementation
}

func (n *TeamsNotifier) NotifyDeleted(ctx context.Context, resources []domain.Resource) error {
    // Implementation
}
```

## Git Workflow

### Branch Naming

- `feature/<description>` - New features
- `fix/<description>` - Bug fixes
- `docs/<description>` - Documentation updates
- `refactor/<description>` - Code refactoring

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

Examples:
```
feat(scanner): add RDS repository for database instances

fix(aws): handle pagination for large instance lists

docs(readme): add Docker usage instructions
```

### Pull Request Process

1. Create a feature branch from `main`
2. Make your changes with tests
3. Run `make lint test` locally
4. Push and create a pull request
5. Ensure CI passes
6. Request review from maintainers
7. Address feedback
8. Squash and merge when approved

## Code Review Checklist

- [ ] Follows Uber Go Style Guide
- [ ] Interface compliance verified at compile time
- [ ] Errors wrapped with context (no "failed to" prefix)
- [ ] No mutable globals - uses dependency injection
- [ ] Table-driven tests for multiple scenarios
- [ ] Goroutines have controlled lifecycles
- [ ] No panics in library code
- [ ] Exit calls only in `main()`

## Release Process

1. Update version in `cmd/version.go`
2. Update CHANGELOG.md
3. Create git tag: `git tag v1.0.0`
4. Push tag: `git push origin v1.0.0`
5. CI builds and publishes release

## Getting Help

- Open an issue for bugs or feature requests
- Start a discussion for questions
- Check the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md) for style questions
