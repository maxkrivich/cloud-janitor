# Cloud Janitor

[![CI](https://github.com/maxkrivich/cloud-janitor/actions/workflows/ci.yml/badge.svg)](https://github.com/maxkrivich/cloud-janitor/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/maxkrivich/cloud-janitor)](https://goreportcard.com/report/github.com/maxkrivich/cloud-janitor)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Automated AWS resource cleanup tool using tag-based expiration. Cloud Janitor helps reduce cloud costs by automatically removing unused resources from development accounts after a configurable grace period.

## How It Works

Cloud Janitor runs a two-step process:

```
┌─────────────────────────────────────────────────────────────┐
│                  Cloud Janitor Process                      │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Step 1: TAG                                                │
│  ─────────────────                                          │
│  Resources without `expiration-date` tag:                   │
│    → Add tag: expiration-date = today + 30 days             │
│    → Send notification to Slack/Discord                     │
│                                                             │
│  Step 2: CLEANUP                                            │
│  ─────────────────                                          │
│  Resources with `expiration-date` in the past:              │
│    → DELETE resource                                        │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

This gives teams a 30-day grace period to either:
- Extend the expiration date if the resource is still needed
- Set `expiration-date=never` to permanently exclude from cleanup
- Do nothing and let the resource be automatically cleaned up

## Features

- **Tag-based expiration**: Resources get a 30-day grace period before deletion
- **Multi-account support**: Scan multiple AWS accounts via assume role
- **Multiple resource types**: EC2, EBS volumes, EBS snapshots, Elastic IPs
- **Notifications**: Slack, Discord, and generic webhook support
- **Dry-run mode**: Preview changes without making them
- **Flexible output**: Table or JSON format

## Installation

### From Release

Download the latest release from the [releases page](https://github.com/maxkrivich/cloud-janitor/releases).

```bash
# macOS (Apple Silicon)
curl -L https://github.com/maxkrivich/cloud-janitor/releases/latest/download/cloud-janitor_darwin_arm64.tar.gz | tar xz
sudo mv cloud-janitor /usr/local/bin/

# macOS (Intel)
curl -L https://github.com/maxkrivich/cloud-janitor/releases/latest/download/cloud-janitor_darwin_amd64.tar.gz | tar xz
sudo mv cloud-janitor /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/maxkrivich/cloud-janitor/releases/latest/download/cloud-janitor_linux_amd64.tar.gz | tar xz
sudo mv cloud-janitor /usr/local/bin/
```

### From Source

```bash
go install github.com/maxkrivich/cloud-janitor@latest
```

### Docker

```bash
docker pull ghcr.io/maxkrivich/cloud-janitor:latest

# Run with AWS credentials
docker run --rm \
  -v ~/.aws:/root/.aws:ro \
  -e AWS_PROFILE=dev \
  ghcr.io/maxkrivich/cloud-janitor:latest run --dry-run
```

## Quick Start

1. **Set up AWS credentials**

   Cloud Janitor uses the standard AWS credential chain. Ensure you have credentials configured:

   ```bash
   export AWS_PROFILE=dev-account
   # or
   export AWS_ACCESS_KEY_ID=...
   export AWS_SECRET_ACCESS_KEY=...
   ```

2. **Preview what would happen**

   ```bash
   cloud-janitor run --dry-run
   ```

3. **Run the cleanup**

   ```bash
   cloud-janitor run
   ```

## Usage

### Commands

```bash
# Run full tag and cleanup cycle
cloud-janitor run

# Only tag resources (no deletion)
cloud-janitor tag

# Only delete expired resources (no tagging)
cloud-janitor cleanup

# List resources and their expiration status
cloud-janitor list

# Show version
cloud-janitor version
```

### Global Flags

```bash
--config string      Config file (default: ./cloud-janitor.yaml)
--dry-run            Preview changes without making them
--verbose, -v        Verbose output
--output, -o string  Output format: table, json (default: table)
--regions, -r        AWS regions to scan (overrides config)
--accounts, -a       AWS account IDs to scan (overrides config)
```

### Examples

```bash
# Preview changes in dry-run mode
cloud-janitor run --dry-run

# Run only in specific regions
cloud-janitor run --regions us-east-1,us-west-2

# List only expired resources
cloud-janitor list --status expired

# List only EC2 instances
cloud-janitor list --types ec2

# Output as JSON
cloud-janitor list --output json
```

## Configuration

Create a `cloud-janitor.yaml` file (see [cloud-janitor.example.yaml](cloud-janitor.example.yaml) for a full example):

```yaml
aws:
  accounts:
    - id: "123456789012"
      name: "dev-account"
      role: "arn:aws:iam::123456789012:role/CloudJanitor"
  regions:
    - us-east-1
    - us-west-2

expiration:
  tag_name: "expiration-date"
  default_days: 30
  exclude_tags:
    - key: "Environment"
      value: "production"

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

dry_run: false
```

### Environment Variables

All configuration options can be set via environment variables with the `CLOUD_JANITOR_` prefix:

```bash
export CLOUD_JANITOR_DRY_RUN=true
export CLOUD_JANITOR_AWS_REGIONS=us-east-1,us-west-2
export SLACK_WEBHOOK_URL=https://hooks.slack.com/services/...
```

## Notifications

When resources are tagged for expiration, Cloud Janitor sends notifications to help teams take action:

### Slack Example

```
🏷️ Cloud Janitor: Resources Tagged for Expiration

Account: 123456789012 (dev-account)
Region: us-east-1

| Type | Resource ID         | Name         | Expires    |
|------|---------------------|--------------|------------|
| EC2  | i-0abc123def456789 | dev-server-1 | 2024-03-15 |
| EBS  | vol-0abc123def4567 | data-volume  | 2024-03-15 |

⚠️ These resources will be deleted on 2024-03-15.

To keep a resource, update its `expiration-date` tag to a future date or `never`.
```

## AWS IAM Permissions

Cloud Janitor requires the following IAM permissions:

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

## CI/CD Integration

### TeamCity / GitHub Actions

Run Cloud Janitor daily to keep development accounts clean:

```yaml
# .github/workflows/cleanup.yml
name: Cloud Cleanup
on:
  schedule:
    - cron: '0 6 * * *'  # Daily at 6 AM UTC
  workflow_dispatch:

jobs:
  cleanup:
    runs-on: ubuntu-latest
    steps:
      - uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::123456789012:role/CloudJanitor
          aws-region: us-east-1

      - name: Run Cloud Janitor
        run: |
          curl -L https://github.com/maxkrivich/cloud-janitor/releases/latest/download/cloud-janitor_linux_amd64.tar.gz | tar xz
          ./cloud-janitor run
        env:
          SLACK_WEBHOOK_URL: ${{ secrets.SLACK_WEBHOOK_URL }}
```

## Safety Measures

1. **30-Day Grace Period**: New resources get 30 days before deletion
2. **Notifications**: Alerts sent when resources are tagged
3. **Dry-Run Mode**: Preview all actions before executing
4. **Exclude Tags**: Protect resources with specific tags
5. **Dev Accounts Only**: Designed for development/test accounts

## Development

```bash
# Clone the repository
git clone https://github.com/maxkrivich/cloud-janitor.git
cd cloud-janitor

# Install dependencies
go mod download

# Run tests
make test

# Build
make build

# Run linter
make lint
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed development guidelines.

## Architecture

Cloud Janitor uses onion architecture to keep business logic independent of external systems. See [ARCHITECTURE.md](ARCHITECTURE.md) for details.

## License

MIT License - see [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) first.
