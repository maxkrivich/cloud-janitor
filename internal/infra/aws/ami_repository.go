package aws

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// amiClient defines the EC2 API operations needed for AMI management.
type amiClient interface {
	DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
	CreateTags(ctx context.Context, params *ec2.CreateTagsInput, optFns ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error)
	DeregisterImage(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error)
	DeleteSnapshot(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error)
}

// Compile-time interface check.
var _ domain.ResourceRepository = (*AMIRepository)(nil)

// AMIRepository implements ResourceRepository for Amazon Machine Images.
type AMIRepository struct {
	client    amiClient
	accountID string
	region    string
}

// NewAMIRepository creates a new AMIRepository.
func NewAMIRepository(client amiClient, accountID, region string) *AMIRepository {
	return &AMIRepository{
		client:    client,
		accountID: accountID,
		region:    region,
	}
}

// Type returns the resource type.
func (r *AMIRepository) Type() domain.ResourceType {
	return domain.ResourceTypeAMI
}

// List returns all AMIs owned by the account in the region.
func (r *AMIRepository) List(ctx context.Context, _ string) ([]domain.Resource, error) {
	output, err := r.client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Owners: []string{"self"},
	})
	if err != nil {
		return nil, fmt.Errorf("listing AMIs: %w", err)
	}

	resources := make([]domain.Resource, 0, len(output.Images))
	for _, image := range output.Images {
		// Skip invalid states
		if !r.isValidState(image.State) {
			continue
		}

		resource := r.imageToResource(image)
		resources = append(resources, resource)
	}

	return resources, nil
}

// Tag adds the expiration-date tag to an AMI.
func (r *AMIRepository) Tag(ctx context.Context, resourceID string, expirationDate time.Time) error {
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
		return fmt.Errorf("tagging AMI %s: %w", resourceID, err)
	}
	return nil
}

// Delete deregisters an AMI and deletes its associated EBS snapshots.
func (r *AMIRepository) Delete(ctx context.Context, resourceID string) error {
	// First, get the AMI details to find associated snapshots
	output, err := r.client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		ImageIds: []string{resourceID},
	})
	if err != nil {
		return fmt.Errorf("describing AMI %s: %w", resourceID, err)
	}

	if len(output.Images) == 0 {
		return fmt.Errorf("AMI %s not found", resourceID)
	}

	image := output.Images[0]

	// Extract snapshot IDs before deregistering
	snapshotIDs := r.extractSnapshotIDs(image.BlockDeviceMappings)

	// Deregister the AMI
	_, err = r.client.DeregisterImage(ctx, &ec2.DeregisterImageInput{
		ImageId: aws.String(resourceID),
	})
	if err != nil {
		return fmt.Errorf("deregistering AMI %s: %w", resourceID, err)
	}

	// Delete associated snapshots
	// Note: Some snapshots may be shared or used by other AMIs, so we ignore errors
	for _, snapshotID := range snapshotIDs {
		_, err := r.client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
			SnapshotId: aws.String(snapshotID),
		})
		if err != nil {
			// Log warning but don't fail - snapshot might be in use by another AMI
			log.Printf("warning: failed to delete snapshot %s for AMI %s: %v", snapshotID, resourceID, err)
		}
	}

	return nil
}

// isValidState returns true if the AMI state is valid for management.
// Valid states: available, pending
// Invalid states: deregistered, invalid, transient, failed, error
func (r *AMIRepository) isValidState(state types.ImageState) bool {
	switch state {
	case types.ImageStateAvailable, types.ImageStatePending:
		return true
	case types.ImageStateDeregistered, types.ImageStateInvalid, types.ImageStateTransient, types.ImageStateFailed, types.ImageStateError:
		return false
	default:
		return false
	}
}

// imageToResource converts an EC2 Image to a domain Resource.
func (r *AMIRepository) imageToResource(image types.Image) domain.Resource {
	resource := domain.Resource{
		ID:        aws.ToString(image.ImageId),
		Type:      domain.ResourceTypeAMI,
		Region:    r.region,
		AccountID: r.accountID,
		Tags:      make(map[string]string),
	}

	// Use AMI name as default name
	resource.Name = aws.ToString(image.Name)

	// Parse tags
	for _, tag := range image.Tags {
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

	// Parse creation date
	if image.CreationDate != nil {
		if t, err := time.Parse(time.RFC3339, *image.CreationDate); err == nil {
			resource.CreatedAt = t
		}
	}

	return resource
}

// extractSnapshotIDs extracts EBS snapshot IDs from block device mappings.
func (r *AMIRepository) extractSnapshotIDs(mappings []types.BlockDeviceMapping) []string {
	var snapshotIDs []string
	for _, mapping := range mappings {
		if mapping.Ebs != nil && mapping.Ebs.SnapshotId != nil {
			snapshotID := aws.ToString(mapping.Ebs.SnapshotId)
			if snapshotID != "" {
				snapshotIDs = append(snapshotIDs, snapshotID)
			}
		}
	}
	return snapshotIDs
}
