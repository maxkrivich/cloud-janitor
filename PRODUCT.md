# Cloud Janitor - Product Vision

## Overview

Cloud Janitor is an automated cleanup tool for cloud development accounts. It uses a tag-based expiration system to automatically remove unused resources after a grace period, reducing cloud costs with minimal manual intervention.

**Supported Cloud Providers:**
- **AWS** - Full support (current)
- **GCP** - Planned
- **Azure** - Planned

## Problem Statement

Development cloud accounts accumulate unused resources over time:
- Forgotten EC2 instances / GCP Compute instances / Azure VMs from completed experiments
- Orphaned EBS volumes / Persistent Disks / Azure Disks from terminated instances
- Old snapshots and images no longer needed
- Test load balancers left running
- Unattached Elastic IPs / Static IPs / Public IPs

Manual cleanup is tedious and often neglected, leading to unnecessary costs.

## Solution: Tag-Based Expiration

Cloud Janitor implements a simple two-step process:

```
┌─────────────────────────────────────────────────────────────────┐
│                    Cloud Janitor Process                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Step 1: SCAN                                                   │
│  ─────────────────                                              │
│  For each resource without `expiration-date` tag:               │
│    → Add tag: expiration-date = today + 30 days                 │
│                                                                 │
│  Step 2: CLEANUP                                                │
│  ─────────────────                                              │
│  For each resource with `expiration-date` tag:                  │
│    → If expiration date has passed → DELETE resource            │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### How It Works

1. **Daily Scheduled Run**: Cloud Janitor runs daily via Github Actions (or other CI)
2. **Tag New Resources**: Resources without an `expiration-date` tag get one set to 30 days from now
3. **Delete Expired Resources**: Resources past their expiration date are automatically deleted
4. **Preserve Important Resources**: Users can extend expiration by updating the tag manually

### The `expiration-date` Tag

- **Format**: `YYYY-MM-DD` (e.g., `2024-02-15`)
- **Default Grace Period**: 30 days
- **User Override**: Manually set a future date to extend the resource lifetime
- **Permanent Resources**: Set to `never` to exclude from cleanup

## Target Users

- **Platform Teams**: Configure and deploy Cloud Janitor across development accounts
- **Developers**: Understand the expiration system; extend tags when needed
- **FinOps Teams**: Monitor cost savings from automated cleanup

## Supported Resources

Cloud Janitor focuses on the **most expensive resources first** (80/20 rule). Each cloud provider has a prioritized list of resources ordered by typical cost impact in development accounts. This ensures maximum cost savings with minimal implementation effort.

Resources are categorized by cost type:
- **Compute**: Charged hourly, even when idle (highest impact)
- **Storage**: Charged monthly per GB (accumulates over time)

### AWS

| Priority | Resource Type | Cost Type | Status | Est. Monthly Cost |
|----------|--------------|-----------|--------|-------------------|
| 1 | **EC2 Instances** | Compute | ✅ Complete | $50-500+ |
| 2 | **RDS Instances** | Compute | 🔜 Phase 2 | $100-1000+ |
| 3 | **Elastic IPs** (unattached) | Compute | ✅ Complete | $3.60 each |
| 4 | **EBS Volumes** | Storage | ✅ Complete | $10-100 |
| 5 | **Load Balancers** (ALB/NLB) | Compute | 🔜 Phase 2 | $20-50 each |
| 6 | **EBS Snapshots** | Storage | ✅ Complete | $5-50 |
| 7 | **ECR Images** | Storage | 🔜 Phase 3 | $1-20 |
| 8 | **AMIs** | Storage | 🔜 Phase 3 | $1-10 |

### GCP (Planned)

| Priority | Resource Type | Cost Type | Status |
|----------|--------------|-----------|--------|
| 1 | **Compute Instances** | Compute | 🔜 Phase 4 |
| 2 | **Cloud SQL Instances** | Compute | 🔜 Phase 4 |
| 3 | **Static IPs** (unattached) | Compute | 🔜 Phase 4 |
| 4 | **Persistent Disks** | Storage | 🔜 Phase 4 |
| 5 | **Snapshots** | Storage | 🔜 Phase 4 |

### Azure (Planned)

| Priority | Resource Type | Cost Type | Status |
|----------|--------------|-----------|--------|
| 1 | **Virtual Machines** | Compute | 🔜 Phase 5 |
| 2 | **Azure SQL** | Compute | 🔜 Phase 5 |
| 3 | **Public IPs** (unassociated) | Compute | 🔜 Phase 5 |
| 4 | **Managed Disks** | Storage | 🔜 Phase 5 |
| 5 | **Snapshots** | Storage | 🔜 Phase 5 |

## Configuration

```yaml
# cloud-janitor.yaml
aws:
  accounts:
    - id: "123456789012"
      role: "arn:aws:iam::123456789012:role/CloudJanitor"
    - id: "987654321098"
      role: "arn:aws:iam::987654321098:role/CloudJanitor"
  regions:
    - us-east-1
    - us-west-2
    - eu-west-1

expiration:
  tag_name: "expiration-date"
  default_days: 30
  
  # Resources matching these tags are never expired
  exclude_tags:
    - "Environment=production"
    - "DoNotDelete=true"
    - "expiration-date=never"

scanners:
  ec2: true
  ebs: true
  ebs_snapshots: true
  elastic_ip: true

output:
  format: table  # table, json
  
dry_run: false  # Set to true to preview without making changes
```

## CLI Commands

```bash
# Run full scan and cleanup cycle
cloud-janitor run

# Preview what would happen (no changes)
cloud-janitor run --dry-run

# Only tag resources (no deletion)
cloud-janitor tag

# Only delete expired resources (no tagging)
cloud-janitor cleanup

# Show resources and their expiration status
cloud-janitor list

# Scan specific account/region
cloud-janitor run --account 123456789012 --region us-east-1
```

## Non-Goals

- **Production accounts**: Only for development/test accounts
- **Real-time monitoring**: Runs on schedule, not continuously
- **Cost estimation**: Focus is on cleanup, not cost analysis

## Success Metrics

- **Resources Cleaned**: Number of resources deleted per run
- **Cost Savings**: Monthly savings from deleted resources
- **Coverage**: Percentage of dev accounts with Cloud Janitor enabled
- **False Positives**: Resources deleted that shouldn't have been (target: 0)

## Notifications

When resources are tagged for expiration, Cloud Janitor sends notifications to configured channels (Slack, Discord, or custom webhooks). This gives resource owners time to take action before deletion.

### Notification Flow

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   Resource   │     │    Cloud     │     │    Slack/    │
│   Tagged     │────▶│   Janitor    │────▶│   Discord    │
└──────────────┘     └──────────────┘     └──────────────┘
                                                 │
                                                 ▼
                                          ┌──────────────┐
                                          │  Developer   │
                                          │  Takes Action│
                                          └──────────────┘
```

### Example Notification

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

### Notification Configuration

```yaml
notifications:
  enabled: true
  
  slack:
    enabled: true
    webhook_url: "${SLACK_WEBHOOK_URL}"
    channel: "#cloud-janitor"
    
  discord:
    enabled: false
    webhook_url: "${DISCORD_WEBHOOK_URL}"
    
  webhook:
    enabled: false
    url: "https://your-service.com/webhooks/cloud-janitor"
```

## Safety Measures

1. **30-Day Grace Period**: New resources get 30 days before deletion
2. **Notifications**: Alerts sent when resources are tagged (take action before deletion)
3. **Dry-Run Mode**: Preview all actions before executing
4. **Exclude Tags**: Protect resources with specific tags
5. **Audit Logging**: Full log of all tag and delete operations
6. **Dev Accounts Only**: Configuration explicitly lists allowed accounts

## Roadmap

### Phase 1: MVP (Complete)
- Core tagging and cleanup logic
- EC2, EBS, Elastic IP, EBS Snapshots scanners
- Configuration file support
- Dry-run mode
- Basic CLI (run, list)
- Slack notifications (webhook, bot token, app token modes)
- Discord notifications (webhook, bot token modes)
- Generic webhook notifications

### Phase 2: Extended AWS Coverage
- Additional scanners (ELB, RDS)
- Multi-account support via assume role

### Phase 3: Extended Notifications (In Progress)
- ✅ Microsoft Teams notifier
- Google Chat notifier
- Telegram notifier

### Phase 4: Multi-Cloud Foundation
- Provider abstraction layer
- Provider-specific resource types (aws:ec2, gcp:compute-instance, azure:vm)
- Multi-provider configuration support
- Environment-based authentication for all providers

### Phase 5: GCP Support
- GCP Compute Instance scanner
- GCP Persistent Disk scanner
- GCP Snapshot scanner
- GCP Static IP scanner
- Label-based expiration (GCP equivalent of tags)

### Phase 6: Azure Support
- Azure VM scanner
- Azure Managed Disk scanner
- Azure Snapshot scanner
- Azure Public IP scanner
- Tag-based expiration

### Phase 7: Observability
- Metrics export (Prometheus/CloudWatch)
- Dashboard for cleanup statistics
- Weekly summary reports
