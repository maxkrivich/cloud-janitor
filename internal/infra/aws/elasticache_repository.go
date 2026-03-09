package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticache/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

const (
	// elastiCacheClusterPrefix is the prefix for cache cluster resource IDs.
	elastiCacheClusterPrefix = "cluster:"

	// elastiCacheReplicationGroupPrefix is the prefix for replication group resource IDs.
	elastiCacheReplicationGroupPrefix = "replication-group:"
)

// Compile-time interface check.
var _ domain.ResourceRepository = (*ElastiCacheRepository)(nil)

// elastiCacheClient defines the interface for ElastiCache client operations used by ElastiCacheRepository.
// This allows for mocking in tests.
type elastiCacheClient interface {
	DescribeCacheClusters(ctx context.Context, params *elasticache.DescribeCacheClustersInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error)
	DescribeReplicationGroups(ctx context.Context, params *elasticache.DescribeReplicationGroupsInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeReplicationGroupsOutput, error)
	ListTagsForResource(ctx context.Context, params *elasticache.ListTagsForResourceInput, optFns ...func(*elasticache.Options)) (*elasticache.ListTagsForResourceOutput, error)
	AddTagsToResource(ctx context.Context, params *elasticache.AddTagsToResourceInput, optFns ...func(*elasticache.Options)) (*elasticache.AddTagsToResourceOutput, error)
	DeleteCacheCluster(ctx context.Context, params *elasticache.DeleteCacheClusterInput, optFns ...func(*elasticache.Options)) (*elasticache.DeleteCacheClusterOutput, error)
	DeleteReplicationGroup(ctx context.Context, params *elasticache.DeleteReplicationGroupInput, optFns ...func(*elasticache.Options)) (*elasticache.DeleteReplicationGroupOutput, error)
}

// ElastiCacheRepository implements ResourceRepository for ElastiCache clusters and replication groups.
type ElastiCacheRepository struct {
	client    elastiCacheClient
	accountID string
	region    string
}

// NewElastiCacheRepository creates a new ElastiCacheRepository.
func NewElastiCacheRepository(client *elasticache.Client, accountID, region string) *ElastiCacheRepository {
	return &ElastiCacheRepository{
		client:    client,
		accountID: accountID,
		region:    region,
	}
}

// Type returns the resource type.
func (r *ElastiCacheRepository) Type() domain.ResourceType {
	return domain.ResourceTypeElastiCache
}

// List returns all ElastiCache clusters and replication groups in the region.
// Cache clusters that are part of a replication group are excluded (only the replication group is returned).
func (r *ElastiCacheRepository) List(ctx context.Context, _ string) ([]domain.Resource, error) {
	var resources []domain.Resource

	// List standalone cache clusters (not part of replication groups)
	clusters, err := r.listCacheClusters(ctx)
	if err != nil {
		return nil, err
	}
	resources = append(resources, clusters...)

	// List replication groups
	replGroups, err := r.listReplicationGroups(ctx)
	if err != nil {
		return nil, err
	}
	resources = append(resources, replGroups...)

	return resources, nil
}

// Tag adds the expiration-date tag to an ElastiCache resource.
// The resourceID format is "cluster:{cluster-id}" or "replication-group:{group-id}".
func (r *ElastiCacheRepository) Tag(ctx context.Context, resourceID string, expirationDate time.Time) error {
	arn := r.buildARN(resourceID)

	_, err := r.client.AddTagsToResource(ctx, &elasticache.AddTagsToResourceInput{
		ResourceName: aws.String(arn),
		Tags: []types.Tag{
			{
				Key:   aws.String(ExpirationTagName),
				Value: aws.String(expirationDate.Format(ExpirationDateFormat)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("tagging ElastiCache %s: %w", resourceID, err)
	}
	return nil
}

// Delete removes an ElastiCache cluster or replication group.
// The resourceID format is "cluster:{cluster-id}" or "replication-group:{group-id}".
func (r *ElastiCacheRepository) Delete(ctx context.Context, resourceID string) error {
	resourceType, id, err := r.parseResourceID(resourceID)
	if err != nil {
		return err
	}

	switch resourceType {
	case "cluster":
		return r.deleteCacheCluster(ctx, id)
	case "replication-group":
		return r.deleteReplicationGroup(ctx, id)
	default:
		return fmt.Errorf("unknown ElastiCache resource type: %s", resourceType)
	}
}

// listCacheClusters retrieves all standalone cache clusters (not part of replication groups).
func (r *ElastiCacheRepository) listCacheClusters(ctx context.Context) ([]domain.Resource, error) {
	var resources []domain.Resource

	paginator := elasticache.NewDescribeCacheClustersPaginator(r.client, &elasticache.DescribeCacheClustersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing ElastiCache clusters: %w", err)
		}

		for _, cluster := range page.CacheClusters {
			// Skip clusters that are part of a replication group
			if cluster.ReplicationGroupId != nil && *cluster.ReplicationGroupId != "" {
				continue
			}

			// Skip clusters in invalid states
			status := aws.ToString(cluster.CacheClusterStatus)
			if !r.isValidClusterState(status) {
				continue
			}

			// Fetch tags for this cluster
			arn := aws.ToString(cluster.ARN)
			tags, err := r.listTags(ctx, arn)
			if err != nil {
				return nil, fmt.Errorf("listing tags for ElastiCache cluster %s: %w", aws.ToString(cluster.CacheClusterId), err)
			}

			resource := r.clusterToResource(cluster, tags)
			resources = append(resources, resource)
		}
	}

	return resources, nil
}

// listReplicationGroups retrieves all replication groups.
func (r *ElastiCacheRepository) listReplicationGroups(ctx context.Context) ([]domain.Resource, error) {
	var resources []domain.Resource

	paginator := elasticache.NewDescribeReplicationGroupsPaginator(r.client, &elasticache.DescribeReplicationGroupsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing ElastiCache replication groups: %w", err)
		}

		for _, replGroup := range page.ReplicationGroups {
			// Skip replication groups in invalid states
			status := aws.ToString(replGroup.Status)
			if !r.isValidReplicationGroupState(status) {
				continue
			}

			// Fetch tags for this replication group
			arn := aws.ToString(replGroup.ARN)
			tags, err := r.listTags(ctx, arn)
			if err != nil {
				return nil, fmt.Errorf("listing tags for ElastiCache replication group %s: %w", aws.ToString(replGroup.ReplicationGroupId), err)
			}

			resource := r.replicationGroupToResource(replGroup, tags)
			resources = append(resources, resource)
		}
	}

	return resources, nil
}

// listTags retrieves tags for an ElastiCache resource.
func (r *ElastiCacheRepository) listTags(ctx context.Context, arn string) ([]types.Tag, error) {
	output, err := r.client.ListTagsForResource(ctx, &elasticache.ListTagsForResourceInput{
		ResourceName: aws.String(arn),
	})
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	return output.TagList, nil
}

// deleteCacheCluster deletes a standalone cache cluster.
func (r *ElastiCacheRepository) deleteCacheCluster(ctx context.Context, clusterID string) error {
	_, err := r.client.DeleteCacheCluster(ctx, &elasticache.DeleteCacheClusterInput{
		CacheClusterId: aws.String(clusterID),
		// FinalSnapshotIdentifier is intentionally nil to skip final snapshot
	})
	if err != nil {
		return fmt.Errorf("deleting ElastiCache cluster %s: %w", clusterID, err)
	}
	return nil
}

// deleteReplicationGroup deletes a replication group.
func (r *ElastiCacheRepository) deleteReplicationGroup(ctx context.Context, groupID string) error {
	_, err := r.client.DeleteReplicationGroup(ctx, &elasticache.DeleteReplicationGroupInput{
		ReplicationGroupId: aws.String(groupID),
		// FinalSnapshotIdentifier is intentionally nil to skip final snapshot
	})
	if err != nil {
		return fmt.Errorf("deleting ElastiCache replication group %s: %w", groupID, err)
	}
	return nil
}

// buildARN constructs the ARN for an ElastiCache resource.
// The resourceID format is "cluster:{cluster-id}" or "replication-group:{group-id}".
// ARN format: arn:aws:elasticache:{region}:{account}:cluster:{cluster-id}
//
//	or: arn:aws:elasticache:{region}:{account}:replicationgroup:{group-id}
func (r *ElastiCacheRepository) buildARN(resourceID string) string {
	if strings.HasPrefix(resourceID, elastiCacheClusterPrefix) {
		clusterID := strings.TrimPrefix(resourceID, elastiCacheClusterPrefix)
		return fmt.Sprintf("arn:aws:elasticache:%s:%s:cluster:%s", r.region, r.accountID, clusterID)
	}
	if strings.HasPrefix(resourceID, elastiCacheReplicationGroupPrefix) {
		groupID := strings.TrimPrefix(resourceID, elastiCacheReplicationGroupPrefix)
		return fmt.Sprintf("arn:aws:elasticache:%s:%s:replicationgroup:%s", r.region, r.accountID, groupID)
	}
	// Default to cluster ARN format
	return fmt.Sprintf("arn:aws:elasticache:%s:%s:cluster:%s", r.region, r.accountID, resourceID)
}

// parseResourceID parses the resource ID and returns the type and actual ID.
// The resourceID format is "cluster:{cluster-id}" or "replication-group:{group-id}".
func (r *ElastiCacheRepository) parseResourceID(resourceID string) (resourceType, id string, err error) {
	idx := strings.Index(resourceID, ":")
	if idx == -1 {
		return "", "", fmt.Errorf("invalid ElastiCache resource ID format: %s", resourceID)
	}
	return resourceID[:idx], resourceID[idx+1:], nil
}

// isValidClusterState checks if the cache cluster state is valid for processing.
// Invalid states: deleting, deleted, create-failed, incompatible-network, restore-failed
func (r *ElastiCacheRepository) isValidClusterState(status string) bool {
	switch status {
	case "deleting", "deleted", "create-failed", "incompatible-network", "restore-failed":
		return false
	default:
		return true
	}
}

// isValidReplicationGroupState checks if the replication group state is valid for processing.
// Invalid states: deleting, create-failed
func (r *ElastiCacheRepository) isValidReplicationGroupState(status string) bool {
	switch status {
	case "deleting", "create-failed":
		return false
	default:
		return true
	}
}

// clusterToResource converts an ElastiCache CacheCluster to a domain.Resource.
func (r *ElastiCacheRepository) clusterToResource(cluster types.CacheCluster, tags []types.Tag) domain.Resource {
	clusterID := aws.ToString(cluster.CacheClusterId)

	resource := domain.Resource{
		ID:        elastiCacheClusterPrefix + clusterID,
		Type:      domain.ResourceTypeElastiCache,
		Region:    r.region,
		AccountID: r.accountID,
		Tags:      make(map[string]string),
	}

	// Parse tags
	for _, tag := range tags {
		key := aws.ToString(tag.Key)
		value := aws.ToString(tag.Value)
		resource.Tags[key] = value

		switch key {
		case "Name":
			resource.Name = value
		case ExpirationTagName:
			if value == NeverExpiresValue {
				resource.NeverExpires = true
			} else {
				if t, err := time.Parse(ExpirationDateFormat, value); err == nil {
					resource.ExpirationDate = &t
				}
			}
		}
	}

	// Set creation time if available
	if cluster.CacheClusterCreateTime != nil {
		resource.CreatedAt = *cluster.CacheClusterCreateTime
	}

	return resource
}

// replicationGroupToResource converts an ElastiCache ReplicationGroup to a domain.Resource.
func (r *ElastiCacheRepository) replicationGroupToResource(replGroup types.ReplicationGroup, tags []types.Tag) domain.Resource {
	groupID := aws.ToString(replGroup.ReplicationGroupId)

	resource := domain.Resource{
		ID:        elastiCacheReplicationGroupPrefix + groupID,
		Type:      domain.ResourceTypeElastiCache,
		Region:    r.region,
		AccountID: r.accountID,
		Tags:      make(map[string]string),
	}

	// Parse tags
	for _, tag := range tags {
		key := aws.ToString(tag.Key)
		value := aws.ToString(tag.Value)
		resource.Tags[key] = value

		switch key {
		case "Name":
			resource.Name = value
		case ExpirationTagName:
			if value == NeverExpiresValue {
				resource.NeverExpires = true
			} else {
				if t, err := time.Parse(ExpirationDateFormat, value); err == nil {
					resource.ExpirationDate = &t
				}
			}
		}
	}

	return resource
}
