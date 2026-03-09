package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check.
var _ domain.ResourceRepository = (*SnapshotRepository)(nil)

// SnapshotRepository implements ResourceRepository for EBS snapshots.
type SnapshotRepository struct {
	client    *ec2.Client
	accountID string
	region    string
}

// NewSnapshotRepository creates a new SnapshotRepository.
func NewSnapshotRepository(client *ec2.Client, accountID, region string) *SnapshotRepository {
	return &SnapshotRepository{
		client:    client,
		accountID: accountID,
		region:    region,
	}
}

// Type returns the resource type.
func (r *SnapshotRepository) Type() domain.ResourceType {
	return domain.ResourceTypeEBSSnapshot
}

// List returns all EBS snapshots owned by the account in the region.
func (r *SnapshotRepository) List(ctx context.Context, _ string) ([]domain.Resource, error) {
	var resources []domain.Resource

	paginator := ec2.NewDescribeSnapshotsPaginator(r.client, &ec2.DescribeSnapshotsInput{
		OwnerIds: []string{"self"},
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing EBS snapshots: %w", err)
		}

		for _, snapshot := range page.Snapshots {
			resource := r.snapshotToResource(snapshot)
			resources = append(resources, resource)
		}
	}

	return resources, nil
}

// Tag adds the expiration-date tag to an EBS snapshot.
func (r *SnapshotRepository) Tag(ctx context.Context, resourceID string, expirationDate time.Time) error {
	_, err := r.client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: []string{resourceID},
		Tags: []types.Tag{
			{
				Key:   aws.String(ExpirationTagName),
				Value: aws.String(expirationDate.Format(ExpirationDateFormat)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("tagging EBS snapshot %s: %w", resourceID, err)
	}
	return nil
}

// Delete removes an EBS snapshot.
func (r *SnapshotRepository) Delete(ctx context.Context, resourceID string) error {
	_, err := r.client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
		SnapshotId: aws.String(resourceID),
	})
	if err != nil {
		return fmt.Errorf("deleting EBS snapshot %s: %w", resourceID, err)
	}
	return nil
}

func (r *SnapshotRepository) snapshotToResource(snapshot types.Snapshot) domain.Resource {
	resource := domain.Resource{
		ID:        aws.ToString(snapshot.SnapshotId),
		Type:      domain.ResourceTypeEBSSnapshot,
		Region:    r.region,
		AccountID: r.accountID,
		Tags:      make(map[string]string),
	}

	// Parse tags
	for _, tag := range snapshot.Tags {
		key := aws.ToString(tag.Key)
		value := aws.ToString(tag.Value)
		resource.Tags[key] = value

		switch key {
		case "Name":
			resource.Name = value
		case ExpirationTagName:
			if IsNeverExpires(value) {
				resource.NeverExpires = true
			} else {
				resource.ExpirationDate = ParseExpirationDate(value, resource.ID, "Snapshot")
			}
		}
	}

	// Set creation time
	if snapshot.StartTime != nil {
		resource.CreatedAt = *snapshot.StartTime
	}

	// Use description as name if no Name tag
	if resource.Name == "" && snapshot.Description != nil {
		resource.Name = aws.ToString(snapshot.Description)
	}

	return resource
}
