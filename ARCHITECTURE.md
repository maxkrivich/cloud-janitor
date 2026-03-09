# Cloud Janitor - Architecture

## System Overview

Cloud Janitor uses **onion architecture** (also known as clean/hexagonal architecture) to ensure business logic is independent of external systems like cloud provider APIs, notification services, and storage.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           CLI Layer (cmd/)                              │
│  Cobra commands: run, tag, cleanup, list, version                       │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                      Application Layer (internal/app/)                  │
│  Use cases: TagResourcesUseCase, CleanupResourcesUseCase                │
│  Orchestrates domain logic, depends only on domain interfaces           │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                       Domain Layer (internal/domain/)                   │
│  Entities: Resource, Account                                            │
│  Interfaces: ResourceRepository, Notifier, Provider                     │
│  Pure business logic, no external dependencies                          │
└─────────────────────────────────────────────────────────────────────────┘
                                    ▲
                                    │
┌─────────────────────────────────────────────────────────────────────────┐
│                  Infrastructure Layer (internal/infra/)                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │     AWS     │  │     GCP     │  │    Azure    │  │   Notify    │     │
│  │  Provider   │  │  Provider   │  │  Provider   │  │  Adapters   │     │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘     │
│  Implements domain interfaces, handles external API calls               │
└─────────────────────────────────────────────────────────────────────────┘
```

## Onion Architecture Layers

### 1. Domain Layer (Core)
The innermost layer contains business entities and interfaces. It has **zero external dependencies**.

```go
// internal/domain/resource.go
type Resource struct {
    ID             string
    Type           ResourceType
    Region         string
    AccountID      string
    Name           string
    ExpirationDate *time.Time
    CreatedAt      time.Time
    Tags           map[string]string
}

func (r Resource) Status() Status {
    if r.ExpirationDate == nil {
        return StatusUntagged
    }
    if r.ExpirationDate.Before(time.Now()) {
        return StatusExpired
    }
    return StatusActive
}

// internal/domain/repository.go
type ResourceRepository interface {
    List(ctx context.Context, region string) ([]Resource, error)
    Tag(ctx context.Context, resourceID string, expirationDate time.Time) error
    Delete(ctx context.Context, resourceID string) error
}

// internal/domain/notifier.go
type Notifier interface {
    NotifyTagged(ctx context.Context, resources []Resource) error
    NotifyDeleted(ctx context.Context, resources []Resource) error
}
```

### 2. Application Layer (Use Cases)
Orchestrates domain logic. Depends only on domain interfaces, not implementations.

```go
// internal/app/usecase/tag.go
type TagResourcesUseCase struct {
    repo     domain.ResourceRepository
    notifier domain.Notifier
    config   Config
}

func (uc *TagResourcesUseCase) Execute(ctx context.Context, region string) (*TagResult, error) {
    resources, err := uc.repo.List(ctx, region)
    if err != nil {
        return nil, fmt.Errorf("listing resources: %w", err)
    }

    var tagged []domain.Resource
    for _, r := range resources {
        if r.Status() == domain.StatusUntagged {
            expDate := time.Now().AddDate(0, 0, uc.config.DefaultDays)
            if err := uc.repo.Tag(ctx, r.ID, expDate); err != nil {
                // log error, continue with other resources
                continue
            }
            r.ExpirationDate = &expDate
            tagged = append(tagged, r)
        }
    }

    // Notify about newly tagged resources
    if len(tagged) > 0 {
        if err := uc.notifier.NotifyTagged(ctx, tagged); err != nil {
            // log error, don't fail the operation
        }
    }

    return &TagResult{Tagged: tagged}, nil
}
```

### 3. Infrastructure Layer (Adapters)
Implements domain interfaces. All external API calls live here.

```go
// internal/infra/aws/ec2_repository.go
type EC2Repository struct {
    client *ec2.Client
}

func (r *EC2Repository) List(ctx context.Context, region string) ([]domain.Resource, error) {
    // AWS SDK calls here
}

func (r *EC2Repository) Tag(ctx context.Context, resourceID string, expDate time.Time) error {
    // AWS SDK calls here
}

func (r *EC2Repository) Delete(ctx context.Context, resourceID string) error {
    // AWS SDK calls here
}

// internal/infra/slack/notifier.go
type SlackNotifier struct {
    webhookURL string
    client     *http.Client
}

func (n *SlackNotifier) NotifyTagged(ctx context.Context, resources []domain.Resource) error {
    // Slack API calls here
}
```

### 4. CLI Layer (Entry Point)
Wires everything together using dependency injection.

```go
// cmd/run.go
func runCmd() *cobra.Command {
    return &cobra.Command{
        Use: "run",
        RunE: func(cmd *cobra.Command, args []string) error {
            // Create infrastructure implementations
            awsClient := aws.NewClient(cfg)
            ec2Repo := awsinfra.NewEC2Repository(awsClient)
            notifier := buildNotifier(cfg) // Slack, Discord, or Multi

            // Inject into use case
            tagUC := usecase.NewTagResourcesUseCase(ec2Repo, notifier, cfg)
            cleanupUC := usecase.NewCleanupResourcesUseCase(ec2Repo, notifier, cfg)

            // Execute
            tagResult, _ := tagUC.Execute(ctx, region)
            cleanupResult, _ := cleanupUC.Execute(ctx, region)

            return nil
        },
    }
}
```

## Directory Structure

```
cloud-janitor/
├── cmd/                           # CLI Layer
│   ├── root.go                    # Root command, global flags
│   ├── run.go                     # Main run command (tag + cleanup)
│   ├── tag.go                     # Tag-only command
│   ├── cleanup.go                 # Cleanup-only command
│   ├── list.go                    # List resources command
│   └── version.go                 # Version command
│
├── internal/
│   ├── domain/                    # Domain Layer (Core)
│   │   ├── resource.go            # Resource entity
│   │   ├── account.go             # Account entity
│   │   ├── repository.go          # ResourceRepository interface
│   │   ├── notifier.go            # Notifier interface
│   │   ├── provider.go            # Provider interface, CloudProvider type
│   │   └── errors.go              # Domain errors
│   │
│   ├── app/                       # Application Layer
│   │   ├── usecase/
│   │   │   ├── tag.go             # TagResourcesUseCase
│   │   │   ├── cleanup.go         # CleanupResourcesUseCase
│   │   │   └── list.go            # ListResourcesUseCase
│   │   └── service/
│   │       └── janitor.go         # Orchestrates multiple use cases
│   │
│   ├── infra/                     # Infrastructure Layer
│   │   ├── aws/                   # AWS adapters
│   │   │   ├── provider.go        # AWS Provider implementation
│   │   │   ├── client.go          # AWS client factory
│   │   │   ├── ec2_repository.go  # EC2 ResourceRepository impl
│   │   │   ├── ebs_repository.go  # EBS ResourceRepository impl
│   │   │   ├── eip_repository.go  # Elastic IP ResourceRepository impl
│   │   │   └── sts.go             # AssumeRole helper
│   │   │
│   │   ├── gcp/                   # GCP adapters (planned)
│   │   │   ├── provider.go        # GCP Provider implementation
│   │   │   ├── client.go          # GCP client factory
│   │   │   ├── instance_repository.go  # Compute Instance impl
│   │   │   ├── disk_repository.go      # Persistent Disk impl
│   │   │   └── ip_repository.go        # Static IP impl
│   │   │
│   │   ├── azure/                 # Azure adapters (planned)
│   │   │   ├── provider.go        # Azure Provider implementation
│   │   │   ├── client.go          # Azure client factory
│   │   │   ├── vm_repository.go   # VM impl
│   │   │   ├── disk_repository.go # Managed Disk impl
│   │   │   └── ip_repository.go   # Public IP impl
│   │   │
│   │   ├── provider/              # Provider registry
│   │   │   └── registry.go        # Maps provider name → Provider impl
│   │   │
│   │   ├── notify/                # Notification adapters
│   │   │   ├── slack.go           # Slack Notifier impl
│   │   │   ├── discord.go         # Discord Notifier impl
│   │   │   ├── webhook.go         # Generic webhook Notifier impl
│   │   │   ├── multi.go           # Multi-notifier (fan-out)
│   │   │   └── noop.go            # No-op Notifier (for dry-run)
│   │   │
│   │   └── config/                # Configuration loading
│   │       └── loader.go
│   │
│   └── output/                    # Output formatters
│       ├── formatter.go
│       ├── table.go
│       └── json.go
│
├── main.go                        # Entry point
├── go.mod
├── go.sum
└── Makefile
```

## Domain Interfaces

### ResourceRepository

```go
// internal/domain/repository.go
package domain

import (
    "context"
    "time"
)

// ResourceRepository defines operations for managing cloud resources.
// Each resource type (EC2, EBS, etc.) has its own implementation.
type ResourceRepository interface {
    // Type returns the resource type this repository manages
    Type() ResourceType
    
    // List returns all resources of this type in the specified region
    List(ctx context.Context, region string) ([]Resource, error)
    
    // Tag adds the expiration-date tag to a resource
    Tag(ctx context.Context, resourceID string, expirationDate time.Time) error
    
    // Delete removes the resource
    Delete(ctx context.Context, resourceID string) error
}

// ResourceType identifies the type of cloud resource
type ResourceType string

const (
    // AWS resource types (priority order)
    ResourceTypeAWSEC2       ResourceType = "aws:ec2"         // Priority 1
    ResourceTypeAWSRDS       ResourceType = "aws:rds"         // Priority 2
    ResourceTypeAWSElasticIP ResourceType = "aws:eip"         // Priority 3
    ResourceTypeAWSEBS       ResourceType = "aws:ebs"         // Priority 4
    ResourceTypeAWSELB       ResourceType = "aws:elb"         // Priority 5
    ResourceTypeAWSSnapshot  ResourceType = "aws:snapshot"    // Priority 6
    ResourceTypeAWSECR       ResourceType = "aws:ecr"         // Priority 7
    ResourceTypeAWSAMI       ResourceType = "aws:ami"         // Priority 8
    
    // GCP resource types (planned)
    ResourceTypeGCPInstance  ResourceType = "gcp:compute-instance"
    ResourceTypeGCPCloudSQL  ResourceType = "gcp:cloud-sql"
    ResourceTypeGCPStaticIP  ResourceType = "gcp:static-ip"
    ResourceTypeGCPDisk      ResourceType = "gcp:disk"
    ResourceTypeGCPSnapshot  ResourceType = "gcp:snapshot"
    
    // Azure resource types (planned)
    ResourceTypeAzureVM       ResourceType = "azure:vm"
    ResourceTypeAzureSQL      ResourceType = "azure:sql"
    ResourceTypeAzurePublicIP ResourceType = "azure:public-ip"
    ResourceTypeAzureDisk     ResourceType = "azure:disk"
    ResourceTypeAzureSnapshot ResourceType = "azure:snapshot"
)

// CostCategory indicates how the resource is billed
type CostCategory string

const (
    CostCategoryCompute CostCategory = "compute" // Hourly charges, even when idle
    CostCategoryStorage CostCategory = "storage" // Monthly per-GB charges
)
```

### ResourceCatalog

The ResourceCatalog provides a prioritized list of supported resources per cloud provider. Resources are ordered by cost impact (80/20 rule) to maximize savings.

```go
// internal/domain/catalog.go
package domain

// ResourceDefinition describes a cloud resource type with its cost characteristics.
type ResourceDefinition struct {
    Type         ResourceType
    Provider     CloudProvider
    CostCategory CostCategory
    Priority     int    // Lower = more expensive (1 = highest priority)
    Description  string
}

// ResourceCatalog provides the prioritized list of supported resources.
type ResourceCatalog struct{}

// AWSResources returns AWS resources ordered by cost priority.
func (c *ResourceCatalog) AWSResources() []ResourceDefinition

// GCPResources returns GCP resources ordered by cost priority.
func (c *ResourceCatalog) GCPResources() []ResourceDefinition

// AzureResources returns Azure resources ordered by cost priority.
func (c *ResourceCatalog) AzureResources() []ResourceDefinition

// GetDefinition returns the definition for a specific resource type.
func (c *ResourceCatalog) GetDefinition(resourceType ResourceType) (ResourceDefinition, bool)
```

### Notifier

```go
// internal/domain/notifier.go
package domain

import "context"

// Notifier sends notifications about resource lifecycle events.
// Implementations: Slack, Discord, Webhook, Multi (fan-out), Noop
type Notifier interface {
    // NotifyTagged sends notification when resources are tagged with expiration date.
    // Message should prompt user to take action if they want to keep the resource.
    NotifyTagged(ctx context.Context, resources []Resource) error
    
    // NotifyDeleted sends notification when expired resources are deleted.
    NotifyDeleted(ctx context.Context, resources []Resource) error
}

// NotificationEvent represents a notification payload
type NotificationEvent struct {
    Type      NotificationType
    Resources []Resource
    Account   string
    Region    string
    Timestamp time.Time
}

type NotificationType string

const (
    NotificationTypeTagged  NotificationType = "tagged"
    NotificationTypeDeleted NotificationType = "deleted"
)
```

## Multi-Cloud Provider Abstraction

Cloud Janitor is designed to support multiple cloud providers. The architecture uses provider-specific resource types and environment-based authentication.

### Provider Interface

```go
// internal/domain/provider.go
package domain

// CloudProvider identifies the cloud platform
type CloudProvider string

const (
    ProviderAWS   CloudProvider = "aws"
    ProviderGCP   CloudProvider = "gcp"
    ProviderAzure CloudProvider = "azure"
)

// Provider creates ResourceRepositories for a specific cloud platform.
// Each cloud provider (AWS, GCP, Azure) implements this interface.
type Provider interface {
    // Name returns the provider identifier
    Name() CloudProvider
    
    // CreateRepositories returns all resource repositories for an account/project.
    // The returned repositories implement ResourceRepository interface.
    CreateRepositories(ctx context.Context, account Account) ([]ResourceRepository, error)
}
```

### Provider Registry

```go
// internal/infra/provider/registry.go
package provider

// Registry manages cloud provider implementations
type Registry struct {
    providers map[domain.CloudProvider]domain.Provider
}

func NewRegistry() *Registry {
    return &Registry{
        providers: make(map[domain.CloudProvider]domain.Provider),
    }
}

func (r *Registry) Register(p domain.Provider) {
    r.providers[p.Name()] = p
}

func (r *Registry) Get(name domain.CloudProvider) (domain.Provider, error) {
    p, ok := r.providers[name]
    if !ok {
        return nil, fmt.Errorf("provider %s not registered", name)
    }
    return p, nil
}

// CreateRepositories creates repositories for an account using the appropriate provider
func (r *Registry) CreateRepositories(ctx context.Context, account domain.Account) ([]domain.ResourceRepository, error) {
    p, err := r.Get(account.Provider)
    if err != nil {
        return nil, err
    }
    return p.CreateRepositories(ctx, account)
}
```

### Authentication Strategy

Each provider uses environment-based authentication:

| Provider | Environment Variables | Notes |
|----------|----------------------|-------|
| AWS | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`, `AWS_PROFILE` | Also supports assume-role via config |
| GCP | `GOOGLE_APPLICATION_CREDENTIALS`, `CLOUDSDK_CORE_PROJECT` | Service account JSON file path |
| Azure | `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_SUBSCRIPTION_ID` | Service principal credentials |

### Expiration Mechanism by Provider

| Provider | Mechanism | Key Name | Format |
|----------|-----------|----------|--------|
| AWS | Tags | `expiration-date` | `YYYY-MM-DD` |
| GCP | Labels | `expiration-date` | `YYYY-MM-DD` (labels don't support special chars) |
| Azure | Tags | `expiration-date` | `YYYY-MM-DD` |
```

## Notification System

### Configuration

```yaml
# cloud-janitor.yaml
notifications:
  # Enable/disable notifications
  enabled: true
  
  # Notification channels (multiple can be enabled)
  slack:
    enabled: true
    webhook_url: "https://hooks.slack.com/services/XXX/YYY/ZZZ"
    channel: "#cloud-janitor"  # Optional override
    
  discord:
    enabled: false
    webhook_url: "https://discord.com/api/webhooks/XXX/YYY"
    
  webhook:
    enabled: false
    url: "https://your-service.com/webhooks/cloud-janitor"
    headers:
      Authorization: "Bearer ${WEBHOOK_TOKEN}"
    
  # Message settings
  mention_on_tag: true        # @channel when resources are tagged
  include_resource_details: true
```

### Message Format

When resources are tagged, a notification is sent:

```
🏷️ Cloud Janitor: Resources Tagged for Expiration

Account: 123456789012 (dev-account)
Region: us-east-1

The following resources have been tagged with expiration date 2024-03-15:

| Type | Resource ID          | Name         |
|------|----------------------|--------------|
| EC2  | i-0abc123def456789  | dev-server-1 |
| EBS  | vol-0abc123def45678 | data-volume  |

⚠️ These resources will be automatically deleted on 2024-03-15.

To keep a resource, update its `expiration-date` tag to a future date or `never`.
```

### Multi-Notifier (Fan-out)

```go
// internal/infra/notify/multi.go
package notify

// MultiNotifier fans out notifications to multiple notifiers
type MultiNotifier struct {
    notifiers []domain.Notifier
}

func NewMultiNotifier(notifiers ...domain.Notifier) *MultiNotifier {
    return &MultiNotifier{notifiers: notifiers}
}

func (m *MultiNotifier) NotifyTagged(ctx context.Context, resources []domain.Resource) error {
    var errs []error
    for _, n := range m.notifiers {
        if err := n.NotifyTagged(ctx, resources); err != nil {
            errs = append(errs, err)
        }
    }
    return errors.Join(errs...)
}
```

## Process Flow

```
┌────────────────────────────────────────────────────────────────────────┐
│                         Daily Run Flow                                  │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐             │
│  │ Load Config  │───▶│ For Each     │───▶│ For Each     │             │
│  │              │    │ Account      │    │ Region       │             │
│  └──────────────┘    └──────────────┘    └──────────────┘             │
│                                                 │                      │
│                                                 ▼                      │
│                      ┌──────────────────────────────────────┐         │
│                      │       For Each Repository            │         │
│                      └──────────────────────────────────────┘         │
│                                    │                                   │
│                    ┌───────────────┴───────────────┐                  │
│                    ▼                               ▼                  │
│         ┌─────────────────────┐       ┌─────────────────────┐        │
│         │ Step 1: TAG         │       │ Step 2: CLEANUP     │        │
│         │                     │       │                     │        │
│         │ List() → filter     │       │ List() → filter     │        │
│         │ untagged → Tag()    │       │ expired → Delete()  │        │
│         │                     │       │                     │        │
│         │ NotifyTagged() ────────────────────────────────────▶ Slack │
│         └─────────────────────┘       └─────────────────────┘        │
│                    │                               │                  │
│                    └───────────────┬───────────────┘                  │
│                                    ▼                                   │
│                      ┌──────────────────────────────┐                 │
│                      │     Output Results           │                 │
│                      └──────────────────────────────┘                 │
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘
```

## Configuration

```yaml
# cloud-janitor.yaml
aws:
  accounts:
    - id: "123456789012"
      name: "dev-account"
      role: "arn:aws:iam::123456789012:role/CloudJanitor"
    - id: "987654321098"
      name: "staging-account"
      role: "arn:aws:iam::987654321098:role/CloudJanitor"
  regions:
    - us-east-1
    - us-west-2
    - eu-west-1

expiration:
  tag_name: "expiration-date"
  date_format: "2006-01-02"
  default_days: 30
  
  exclude_tags:
    - key: "Environment"
      value: "production"
    - key: "DoNotDelete"
      value: "true"
    - key: "expiration-date"
      value: "never"

scanners:
  ec2: true
  ebs: true
  ebs_snapshots: true
  elastic_ip: true

notifications:
  enabled: true
  slack:
    enabled: true
    webhook_url: "${SLACK_WEBHOOK_URL}"
  discord:
    enabled: false
    webhook_url: "${DISCORD_WEBHOOK_URL}"

output:
  format: table
  verbose: false
  
dry_run: false
```

## Benefits of Onion Architecture

### 1. Testability
- Domain logic can be tested without AWS or Slack
- Use mock implementations of interfaces
- Fast, reliable unit tests

```go
func TestTagResourcesUseCase(t *testing.T) {
    // Use mock implementations
    mockRepo := &MockResourceRepository{
        resources: []domain.Resource{{ID: "i-123", Status: domain.StatusUntagged}},
    }
    mockNotifier := &MockNotifier{}
    
    uc := usecase.NewTagResourcesUseCase(mockRepo, mockNotifier, cfg)
    result, err := uc.Execute(ctx, "us-east-1")
    
    assert.NoError(t, err)
    assert.Len(t, result.Tagged, 1)
    assert.True(t, mockNotifier.NotifyTaggedCalled)
}
```

### 2. Swappable Implementations
- Switch from Slack to Discord by changing one line
- Swap AWS SDK v1 to v2 without touching business logic
- Add new cloud providers (GCP, Azure) without modifying use cases
- Add new notification channels without modifying use cases

### 3. Clear Dependencies
- Domain layer has zero external imports
- Application layer depends only on domain interfaces
- Infrastructure implements domain interfaces

### 4. Maintainability
- Changes to AWS API don't affect business logic
- Notification logic is centralized
- Easy to understand code flow

## IAM Permissions

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "EC2Permissions",
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeInstances",
        "ec2:DescribeVolumes",
        "ec2:DescribeSnapshots",
        "ec2:DescribeAddresses",
        "ec2:CreateTags",
        "ec2:TerminateInstances",
        "ec2:DeleteVolume",
        "ec2:DeleteSnapshot",
        "ec2:ReleaseAddress"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AssumeRole",
      "Effect": "Allow",
      "Action": "sts:AssumeRole",
      "Resource": "arn:aws:iam::*:role/CloudJanitor"
    }
  ]
}
```

## Error Handling

- Use wrapped errors: `fmt.Errorf("tagging EC2 %s: %w", id, err)`
- Continue on individual resource errors (don't fail entire run)
- Notification errors are logged but don't fail the operation
- Aggregate and report all errors at the end

## Security Considerations

- Never log credentials or sensitive data
- Webhook URLs should be loaded from environment variables
- Use IAM roles with minimum required permissions
- Audit log for compliance requirements
