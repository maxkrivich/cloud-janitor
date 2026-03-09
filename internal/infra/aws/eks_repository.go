package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check.
var _ domain.ResourceRepository = (*EKSRepository)(nil)

// eksClient defines the interface for EKS client operations used by EKSRepository.
// This allows for mocking in tests.
type eksClient interface {
	ListClusters(ctx context.Context, params *eks.ListClustersInput, optFns ...func(*eks.Options)) (*eks.ListClustersOutput, error)
	DescribeCluster(ctx context.Context, params *eks.DescribeClusterInput, optFns ...func(*eks.Options)) (*eks.DescribeClusterOutput, error)
	TagResource(ctx context.Context, params *eks.TagResourceInput, optFns ...func(*eks.Options)) (*eks.TagResourceOutput, error)
	ListNodegroups(ctx context.Context, params *eks.ListNodegroupsInput, optFns ...func(*eks.Options)) (*eks.ListNodegroupsOutput, error)
	DeleteNodegroup(ctx context.Context, params *eks.DeleteNodegroupInput, optFns ...func(*eks.Options)) (*eks.DeleteNodegroupOutput, error)
	DeleteCluster(ctx context.Context, params *eks.DeleteClusterInput, optFns ...func(*eks.Options)) (*eks.DeleteClusterOutput, error)
}

// EKSRepository implements ResourceRepository for EKS clusters.
type EKSRepository struct {
	client        eksClient
	accountID     string
	region        string
	cascadeDelete bool
}

// NewEKSRepository creates a new EKSRepository.
// If cascadeDelete is true, node groups will be deleted before the cluster.
// If cascadeDelete is false, clusters with node groups will be skipped with an error.
func NewEKSRepository(client *eks.Client, accountID, region string, cascadeDelete bool) *EKSRepository {
	return &EKSRepository{
		client:        client,
		accountID:     accountID,
		region:        region,
		cascadeDelete: cascadeDelete,
	}
}

// Type returns the resource type.
func (r *EKSRepository) Type() domain.ResourceType {
	return domain.ResourceTypeEKS
}

// List returns all EKS clusters in the region.
func (r *EKSRepository) List(ctx context.Context, _ string) ([]domain.Resource, error) {
	var resources []domain.Resource

	paginator := eks.NewListClustersPaginator(r.client, &eks.ListClustersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing EKS clusters: %w", err)
		}

		for _, clusterName := range page.Clusters {
			// Describe the cluster to get details and tags
			describeOutput, err := r.client.DescribeCluster(ctx, &eks.DescribeClusterInput{
				Name: aws.String(clusterName),
			})
			if err != nil {
				return nil, fmt.Errorf("describing EKS cluster %s: %w", clusterName, err)
			}

			cluster := describeOutput.Cluster

			// Skip clusters in invalid states
			if !r.isValidClusterState(cluster.Status) {
				continue
			}

			resource := r.clusterToResource(cluster)
			resources = append(resources, resource)
		}
	}

	return resources, nil
}

// Tag adds the expiration-date tag to an EKS cluster.
// EKS uses ARN-based tagging via TagResource.
func (r *EKSRepository) Tag(ctx context.Context, resourceID string, expirationDate time.Time) error {
	// Get the cluster ARN
	arn, err := r.getClusterARN(ctx, resourceID)
	if err != nil {
		return fmt.Errorf("getting EKS cluster ARN for %s: %w", resourceID, err)
	}

	_, err = r.client.TagResource(ctx, &eks.TagResourceInput{
		ResourceArn: aws.String(arn),
		Tags: map[string]string{
			ExpirationTagName: expirationDate.Format(ExpirationDateFormat),
		},
	})
	if err != nil {
		return fmt.Errorf("tagging EKS cluster %s: %w", resourceID, err)
	}
	return nil
}

// Delete removes an EKS cluster.
// If cascadeDelete is true and the cluster has node groups, they will be deleted first.
// If cascadeDelete is false and the cluster has node groups, an error is returned.
func (r *EKSRepository) Delete(ctx context.Context, resourceID string) error {
	// List node groups for this cluster
	nodegroups, err := r.listNodegroups(ctx, resourceID)
	if err != nil {
		return fmt.Errorf("listing node groups for EKS cluster %s: %w", resourceID, err)
	}

	// Handle node groups
	if len(nodegroups) > 0 {
		if !r.cascadeDelete {
			return fmt.Errorf("EKS cluster %s has %d node groups; set cascadeDelete to delete them first", resourceID, len(nodegroups))
		}

		// Delete all node groups first
		for _, ng := range nodegroups {
			if delErr := r.deleteNodegroup(ctx, resourceID, ng); delErr != nil {
				return fmt.Errorf("deleting node group %s for EKS cluster %s: %w", ng, resourceID, delErr)
			}
		}
	}

	// Delete the cluster
	_, err = r.client.DeleteCluster(ctx, &eks.DeleteClusterInput{
		Name: aws.String(resourceID),
	})
	if err != nil {
		return fmt.Errorf("deleting EKS cluster %s: %w", resourceID, err)
	}

	return nil
}

// getClusterARN retrieves the ARN for an EKS cluster by describing it.
func (r *EKSRepository) getClusterARN(ctx context.Context, clusterName string) (string, error) {
	output, err := r.client.DescribeCluster(ctx, &eks.DescribeClusterInput{
		Name: aws.String(clusterName),
	})
	if err != nil {
		return "", fmt.Errorf("describing cluster: %w", err)
	}
	return aws.ToString(output.Cluster.Arn), nil
}

// listNodegroups returns all node groups for a cluster.
func (r *EKSRepository) listNodegroups(ctx context.Context, clusterName string) ([]string, error) {
	output, err := r.client.ListNodegroups(ctx, &eks.ListNodegroupsInput{
		ClusterName: aws.String(clusterName),
	})
	if err != nil {
		return nil, fmt.Errorf("listing node groups: %w", err)
	}
	return output.Nodegroups, nil
}

// deleteNodegroup deletes a single node group.
func (r *EKSRepository) deleteNodegroup(ctx context.Context, clusterName, nodegroupName string) error {
	_, err := r.client.DeleteNodegroup(ctx, &eks.DeleteNodegroupInput{
		ClusterName:   aws.String(clusterName),
		NodegroupName: aws.String(nodegroupName),
	})
	if err != nil {
		return fmt.Errorf("calling DeleteNodegroup API: %w", err)
	}
	return nil
}

// isValidClusterState checks if the cluster state is valid for processing.
// Invalid states: DELETING, FAILED
func (r *EKSRepository) isValidClusterState(status types.ClusterStatus) bool {
	switch status {
	case types.ClusterStatusDeleting, types.ClusterStatusFailed:
		return false
	default:
		return true
	}
}

// clusterToResource converts an EKS Cluster to a domain.Resource.
func (r *EKSRepository) clusterToResource(cluster *types.Cluster) domain.Resource {
	clusterName := aws.ToString(cluster.Name)

	resource := domain.Resource{
		ID:        clusterName,
		Type:      domain.ResourceTypeEKS,
		Region:    r.region,
		AccountID: r.accountID,
		Tags:      make(map[string]string),
	}

	// Handle nil tags map
	if cluster.Tags == nil {
		return resource
	}

	// Copy tags and parse special tags
	for key, value := range cluster.Tags {
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
	if cluster.CreatedAt != nil {
		resource.CreatedAt = *cluster.CreatedAt
	}

	return resource
}
