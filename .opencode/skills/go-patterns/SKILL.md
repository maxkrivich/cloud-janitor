---
name: go-patterns
description: Go coding patterns and best practices for Cloud Janitor development
license: MIT
metadata:
  language: go
  framework: cobra
---

# Go Patterns for Cloud Janitor

## Error Handling

Always wrap errors with context:

```go
if err != nil {
    return fmt.Errorf("scanning instances in %s: %w", region, err)
}
```

Use custom error types for specific cases:

```go
type ResourceNotFoundError struct {
    ResourceType string
    ResourceID   string
}

func (e *ResourceNotFoundError) Error() string {
    return fmt.Sprintf("%s not found: %s", e.ResourceType, e.ResourceID)
}
```

## Table-Driven Tests

```go
func TestScanner_Scan(t *testing.T) {
    tests := []struct {
        name      string
        input     InputType
        want      []Resource
        wantErr   bool
    }{
        {
            name:  "finds unused resources",
            input: validInput,
            want:  expectedResources,
        },
        {
            name:    "handles empty results",
            input:   emptyInput,
            want:    []Resource{},
        },
        {
            name:    "returns error on API failure",
            input:   badInput,
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := scanner.Scan(ctx, tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Scan() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("Scan() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## AWS API Pagination

```go
paginator := ec2.NewDescribeInstancesPaginator(client, input)
for paginator.HasMorePages() {
    page, err := paginator.NextPage(ctx)
    if err != nil {
        return fmt.Errorf("listing instances: %w", err)
    }
    for _, reservation := range page.Reservations {
        for _, instance := range reservation.Instances {
            // process instance
        }
    }
}
```

## Concurrent Operations

```go
g, ctx := errgroup.WithContext(ctx)
results := make(chan Resource)

for _, region := range regions {
    region := region // capture loop variable
    g.Go(func() error {
        resources, err := s.scanRegion(ctx, region)
        if err != nil {
            return fmt.Errorf("scanning %s: %w", region, err)
        }
        for _, r := range resources {
            select {
            case results <- r:
            case <-ctx.Done():
                return ctx.Err()
            }
        }
        return nil
    })
}

// Close results when all goroutines complete
go func() {
    g.Wait()
    close(results)
}()
```

## Interface Design

Accept interfaces, return concrete types:

```go
// Good: accepts interface for testability
func NewScanner(client EC2Client) *EC2Scanner {
    return &EC2Scanner{client: client}
}

// Interface for mocking
type EC2Client interface {
    DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
}
```

## Context Usage

Always pass context as first parameter:

```go
func (s *Scanner) Scan(ctx context.Context, region string) ([]Resource, error) {
    // Check for cancellation
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    
    // Use context with timeout for API calls
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()
    
    return s.client.DescribeInstances(ctx, input)
}
```
