# Phase 2: Extended AWS Coverage

**Status:** `completed`  
**Created:** 2026-03-09  
**Estimated Effort:** ~43 hours

---

## Summary

Expand Cloud Janitor from 4 AWS scanners to 14 scanners, targeting the highest-cost resources in development accounts based on FinOps analysis.

## Decisions Made

| Decision | Choice |
|----------|--------|
| Total scanners | 10 new (14 total) |
| ECR | Skipped (recommend AWS lifecycle policies) |
| Deletion protection | Global config: `force_delete_protected: false` |
| ElastiCache scope | Both clusters and replication groups |
| EKS node groups | Configurable: `eks_cascade_delete: false` |
| CloudWatch Logs | Skip patterns configurable |

## New Scanners (Priority Order)

| # | Scanner | Resource Type | Priority | Est. Savings |
|---|---------|---------------|----------|--------------|
| 1 | RDS Instances | `aws:rds` | P1 | $300-3000/mo |
| 2 | Load Balancers | `aws:elb` | P1 | $100-500/mo |
| 3 | NAT Gateways | `aws:nat-gateway` | P1 | $100-500/mo |
| 4 | ElastiCache | `aws:elasticache` | P2 | $100-400/mo |
| 5 | OpenSearch | `aws:opensearch` | P2 | $100-500/mo |
| 6 | EKS Clusters | `aws:eks` | P2 | $150-500/mo |
| 7 | Redshift | `aws:redshift` | P3 | $200-2000/mo |
| 8 | SageMaker Notebooks | `aws:sagemaker` | P3 | $50-400/mo |
| 9 | AMIs | `aws:ami` | P3 | $1-20/mo |
| 10 | CloudWatch Logs | `aws:logs` | P3 | $10-100/mo |

## Configuration Schema

```yaml
scanners:
  # Existing (Phase 1)
  ec2: true
  ebs: true
  ebs_snapshots: true
  elastic_ip: true
  
  # Phase 2 - P1
  rds: true
  elb: true
  nat_gateway: true
  
  # Phase 2 - P2
  elasticache: true
  opensearch: true
  eks: true
  
  # Phase 2 - P3
  redshift: true
  sagemaker: true
  ami: true
  logs: true

expiration:
  tag_name: "expiration-date"
  default_days: 30
  
  # NEW: Force delete resources with deletion protection enabled
  force_delete_protected: false
  
  # NEW: EKS cascade delete (delete node groups before cluster)
  eks_cascade_delete: false
  
  # NEW: Skip patterns for CloudWatch Log Groups
  logs_skip_patterns:
    - "/aws/lambda/*"
    - "/aws/eks/*"
    - "/aws/rds/*"
    - "/aws/elasticbeanstalk/*"
```

---

## Implementation Tasks

### Phase 2.1: Foundation

| Task | Description | Files | Status |
|------|-------------|-------|--------|
| 1 | Domain Layer - Add 7 new ResourceType constants | `internal/domain/resource.go` | completed |
| 2 | Config Layer - Add scanner flags, `force_delete_protected`, `eks_cascade_delete`, `logs_skip_patterns` | `internal/infra/config/loader.go` | completed |
| 3 | Client Factory - Add 7 new client methods (RDS, ELBv2, ElastiCache, OpenSearch, EKS, Redshift, SageMaker, CloudWatchLogs) | `internal/infra/aws/client.go` | completed |

### Phase 2.2: P1 Scanners (Highest ROI)

| Task | Scanner | Key APIs | Status |
|------|---------|----------|--------|
| 4 | **RDS** | `DescribeDBInstances`, `AddTagsToResource`, `DeleteDBInstance` | completed |
| 5 | **ELB** | `DescribeLoadBalancers`, `DescribeTags`, `AddTags`, `DeleteLoadBalancer` | completed |
| 6 | **NAT Gateway** | `DescribeNatGateways`, `CreateTags`, `DeleteNatGateway` | completed |

### Phase 2.3: P2 Scanners

| Task | Scanner | Key APIs | Status |
|------|---------|----------|--------|
| 7 | **ElastiCache** | `DescribeCacheClusters`, `DescribeReplicationGroups`, `AddTagsToResource`, `DeleteCacheCluster`, `DeleteReplicationGroup` | completed |
| 8 | **OpenSearch** | `ListDomainNames`, `DescribeDomain`, `AddTags`, `DeleteDomain` | completed |
| 9 | **EKS** | `ListClusters`, `DescribeCluster`, `ListNodegroups`, `DeleteNodegroup`, `TagResource`, `DeleteCluster` | completed |

### Phase 2.4: P3 Scanners

| Task | Scanner | Key APIs | Status |
|------|---------|----------|--------|
| 10 | **Redshift** | `DescribeClusters`, `CreateTags`, `DeleteCluster` | completed |
| 11 | **SageMaker** | `ListNotebookInstances`, `AddTags`, `StopNotebookInstance`, `DeleteNotebookInstance` | completed |
| 12 | **AMI** | `DescribeImages`, `CreateTags`, `DeregisterImage`, `DeleteSnapshot` | completed |
| 13 | **CloudWatch Logs** | `DescribeLogGroups`, `ListTagsForResource`, `TagResource`, `DeleteLogGroup` | completed |

### Phase 2.5: Integration

| Task | Description | Files | Status |
|------|-------------|-------|--------|
| 14 | Repository Factory - Wire all new scanners | `internal/infra/aws/repository_factory.go` | completed |
| 15 | Documentation - Update PRODUCT.md, ARCHITECTURE.md, README | Multiple | pending |

---

## New Files

```
internal/infra/aws/
├── rds_repository.go
├── rds_repository_test.go
├── elb_repository.go
├── elb_repository_test.go
├── natgw_repository.go
├── natgw_repository_test.go
├── elasticache_repository.go
├── elasticache_repository_test.go
├── opensearch_repository.go
├── opensearch_repository_test.go
├── eks_repository.go
├── eks_repository_test.go
├── redshift_repository.go
├── redshift_repository_test.go
├── sagemaker_repository.go
├── sagemaker_repository_test.go
├── ami_repository.go
├── ami_repository_test.go
├── logs_repository.go
├── logs_repository_test.go
```

---

## Scanner Implementation Notes

### RDS Scanner (Task 4)
- Check `DeletionProtection` field before delete
- If `force_delete_protected: true`, call `ModifyDBInstance` to disable protection first
- Use `SkipFinalSnapshot: true` on delete
- Tags are ARN-based via `AddTagsToResource`

### ELB Scanner (Task 5)
- Covers both ALB and NLB (elbv2 API)
- Tags require separate `DescribeTags` call per load balancer
- No deletion protection to worry about

### NAT Gateway Scanner (Task 6)
- Uses existing EC2 client
- Filter out `deleted` and `deleting` states in List
- Tags use EC2-style `CreateTags`

### ElastiCache Scanner (Task 7)
- Must handle both standalone cache clusters AND replication groups
- List both types, return as unified resource list
- Delete method must detect type and call appropriate API

### OpenSearch Scanner (Task 8)
- No paginator for `ListDomainNames`
- Need to call `DescribeDomain` for each to get tags/details
- Deletion is async and can take 10+ minutes

### EKS Scanner (Task 9)
- Check for node groups before delete via `ListNodegroups`
- If `eks_cascade_delete: true`: delete each nodegroup, wait, then delete cluster
- If `eks_cascade_delete: false`: skip cluster and log warning if nodegroups exist

### Redshift Scanner (Task 10)
- Use `SkipFinalClusterSnapshot: true` on delete
- No native deletion protection (some orgs use tags)

### SageMaker Scanner (Task 11)
- Check notebook status before delete
- If `InService`: call `StopNotebookInstance` and wait for `Stopped` state
- Then call `DeleteNotebookInstance`

### AMI Scanner (Task 12)
- Get associated snapshots from `BlockDeviceMappings`
- Call `DeregisterImage` first
- Then delete each associated EBS snapshot

### CloudWatch Logs Scanner (Task 13)
- Tags require separate `ListTagsForResource` call per log group
- Filter log groups matching `logs_skip_patterns` (glob matching)
- Default skip patterns: `/aws/lambda/*`, `/aws/eks/*`, `/aws/rds/*`, `/aws/elasticbeanstalk/*`

---

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| RDS/Redshift data loss | 30-day grace period; `force_delete_protected: false` default; notifications |
| EKS deletion fails | `eks_cascade_delete: false` default; skip and warn if node groups exist |
| OpenSearch slow deletion | Async delete; don't wait for completion |
| SageMaker state issues | Auto-stop running notebooks with timeout |
| AMI orphaned snapshots | Delete associated snapshots after deregister |
| CloudWatch Logs volume | Skip patterns for AWS-managed log groups |
| API rate limits | Existing parallel scanning; pagination |

---

## Estimated Effort

| Phase | Tasks | Hours |
|-------|-------|-------|
| 2.1 Foundation | 1-3 | ~5h |
| 2.2 P1 Scanners | 4-6 | ~10h |
| 2.3 P2 Scanners | 7-9 | ~12h |
| 2.4 P3 Scanners | 10-13 | ~13h |
| 2.5 Integration | 14-15 | ~3h |
| **Total** | **15 tasks** | **~43h** |
