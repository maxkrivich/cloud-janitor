package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check.
var _ domain.ResourceRepository = (*RedshiftRepository)(nil)

// redshiftClient defines the interface for Redshift client operations used by RedshiftRepository.
// This allows for mocking in tests.
type redshiftClient interface {
	DescribeClusters(ctx context.Context, params *redshift.DescribeClustersInput, optFns ...func(*redshift.Options)) (*redshift.DescribeClustersOutput, error)
	CreateTags(ctx context.Context, params *redshift.CreateTagsInput, optFns ...func(*redshift.Options)) (*redshift.CreateTagsOutput, error)
	DeleteCluster(ctx context.Context, params *redshift.DeleteClusterInput, optFns ...func(*redshift.Options)) (*redshift.DeleteClusterOutput, error)
}

// RedshiftRepository implements ResourceRepository for Redshift clusters.
type RedshiftRepository struct {
	client    redshiftClient
	accountID string
	region    string
}

// NewRedshiftRepository creates a new RedshiftRepository.
func NewRedshiftRepository(client *redshift.Client, accountID, region string) *RedshiftRepository {
	return &RedshiftRepository{
		client:    client,
		accountID: accountID,
		region:    region,
	}
}

// Type returns the resource type.
func (r *RedshiftRepository) Type() domain.ResourceType {
	return domain.ResourceTypeRedshift
}

// List returns all Redshift clusters in the region.
func (r *RedshiftRepository) List(ctx context.Context, _ string) ([]domain.Resource, error) {
	var resources []domain.Resource

	paginator := redshift.NewDescribeClustersPaginator(r.client, &redshift.DescribeClustersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing Redshift clusters: %w", err)
		}

		for _, cluster := range page.Clusters {
			// Skip clusters in invalid states
			status := aws.ToString(cluster.ClusterStatus)
			if !r.isValidClusterState(status) {
				continue
			}

			resource := r.clusterToResource(cluster)
			resources = append(resources, resource)
		}
	}

	return resources, nil
}

// Tag adds the expiration-date tag to a Redshift cluster.
// Redshift uses ARN-based tagging: arn:aws:redshift:{region}:{account}:cluster:{cluster-identifier}
func (r *RedshiftRepository) Tag(ctx context.Context, resourceID string, expirationDate time.Time) error {
	arn := r.buildARN(resourceID)

	_, err := r.client.CreateTags(ctx, &redshift.CreateTagsInput{
		ResourceName: aws.String(arn),
		Tags: []types.Tag{
			{
				Key:   aws.String(ExpirationTagName),
				Value: aws.String(expirationDate.Format(ExpirationDateFormat)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("tagging Redshift cluster %s: %w", resourceID, err)
	}
	return nil
}

// Delete removes a Redshift cluster.
// Uses SkipFinalClusterSnapshot to avoid creating a snapshot before deletion.
func (r *RedshiftRepository) Delete(ctx context.Context, resourceID string) error {
	_, err := r.client.DeleteCluster(ctx, &redshift.DeleteClusterInput{
		ClusterIdentifier:        aws.String(resourceID),
		SkipFinalClusterSnapshot: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("deleting Redshift cluster %s: %w", resourceID, err)
	}
	return nil
}

// buildARN constructs the ARN for a Redshift cluster.
// Format: arn:aws:redshift:{region}:{account}:cluster:{cluster-identifier}
func (r *RedshiftRepository) buildARN(resourceID string) string {
	return fmt.Sprintf("arn:aws:redshift:%s:%s:cluster:%s", r.region, r.accountID, resourceID)
}

// isValidClusterState checks if the cluster state is valid for processing.
// Invalid states: deleting, final-snapshot
func (r *RedshiftRepository) isValidClusterState(status string) bool {
	switch status {
	case "deleting", "final-snapshot":
		return false
	default:
		return true
	}
}

// clusterToResource converts a Redshift Cluster to a domain.Resource.
func (r *RedshiftRepository) clusterToResource(cluster types.Cluster) domain.Resource {
	resource := domain.Resource{
		ID:        aws.ToString(cluster.ClusterIdentifier),
		Type:      domain.ResourceTypeRedshift,
		Region:    r.region,
		AccountID: r.accountID,
		Tags:      make(map[string]string),
	}

	// Parse tags (Redshift includes tags in the cluster response)
	for _, tag := range cluster.Tags {
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

	// Set creation time
	if cluster.ClusterCreateTime != nil {
		resource.CreatedAt = *cluster.ClusterCreateTime
	}

	return resource
}
