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
var _ domain.ResourceRepository = (*EBSRepository)(nil)

// EBSRepository implements ResourceRepository for EBS volumes.
type EBSRepository struct {
	client    *ec2.Client
	accountID string
	region    string
}

// NewEBSRepository creates a new EBSRepository.
func NewEBSRepository(client *ec2.Client, accountID, region string) *EBSRepository {
	return &EBSRepository{
		client:    client,
		accountID: accountID,
		region:    region,
	}
}

// Type returns the resource type.
func (r *EBSRepository) Type() domain.ResourceType {
	return domain.ResourceTypeEBS
}

// List returns all EBS volumes in the region.
func (r *EBSRepository) List(ctx context.Context, _ string) ([]domain.Resource, error) {
	var resources []domain.Resource

	paginator := ec2.NewDescribeVolumesPaginator(r.client, &ec2.DescribeVolumesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing EBS volumes: %w", err)
		}

		for _, volume := range page.Volumes {
			resource := r.volumeToResource(volume)
			resources = append(resources, resource)
		}
	}

	return resources, nil
}

// Tag adds the expiration-date tag to an EBS volume.
func (r *EBSRepository) Tag(ctx context.Context, resourceID string, expirationDate time.Time) error {
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
		return fmt.Errorf("tagging EBS volume %s: %w", resourceID, err)
	}
	return nil
}

// Delete removes an EBS volume.
func (r *EBSRepository) Delete(ctx context.Context, resourceID string) error {
	_, err := r.client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
		VolumeId: aws.String(resourceID),
	})
	if err != nil {
		return fmt.Errorf("deleting EBS volume %s: %w", resourceID, err)
	}
	return nil
}

func (r *EBSRepository) volumeToResource(volume types.Volume) domain.Resource {
	resource := domain.Resource{
		ID:        aws.ToString(volume.VolumeId),
		Type:      domain.ResourceTypeEBS,
		Region:    r.region,
		AccountID: r.accountID,
		Tags:      make(map[string]string),
	}

	// Parse tags
	for _, tag := range volume.Tags {
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
	if volume.CreateTime != nil {
		resource.CreatedAt = *volume.CreateTime
	}

	return resource
}
