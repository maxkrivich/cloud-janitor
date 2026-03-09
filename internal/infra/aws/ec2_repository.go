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

const (
	// ExpirationTagName is the name of the tag used to track expiration dates.
	ExpirationTagName = "expiration-date"

	// ExpirationDateFormat is the format used for expiration dates.
	ExpirationDateFormat = "2006-01-02"

	// NeverExpiresValue is the value that indicates a resource should never expire.
	NeverExpiresValue = "never"
)

// Compile-time interface check.
var _ domain.ResourceRepository = (*EC2Repository)(nil)

// EC2Repository implements ResourceRepository for EC2 instances.
type EC2Repository struct {
	client    *ec2.Client
	accountID string
	region    string
}

// NewEC2Repository creates a new EC2Repository.
func NewEC2Repository(client *ec2.Client, accountID, region string) *EC2Repository {
	return &EC2Repository{
		client:    client,
		accountID: accountID,
		region:    region,
	}
}

// Type returns the resource type.
func (r *EC2Repository) Type() domain.ResourceType {
	return domain.ResourceTypeEC2
}

// List returns all EC2 instances in the region.
func (r *EC2Repository) List(ctx context.Context, _ string) ([]domain.Resource, error) {
	var resources []domain.Resource

	paginator := ec2.NewDescribeInstancesPaginator(r.client, &ec2.DescribeInstancesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing EC2 instances: %w", err)
		}

		for _, reservation := range page.Reservations {
			for _, instance := range reservation.Instances {
				// Skip terminated instances
				if instance.State.Name == types.InstanceStateNameTerminated {
					continue
				}

				resource := r.instanceToResource(instance)
				resources = append(resources, resource)
			}
		}
	}

	return resources, nil
}

// Tag adds the expiration-date tag to an EC2 instance.
func (r *EC2Repository) Tag(ctx context.Context, resourceID string, expirationDate time.Time) error {
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
		return fmt.Errorf("tagging EC2 instance %s: %w", resourceID, err)
	}
	return nil
}

// Delete terminates an EC2 instance.
func (r *EC2Repository) Delete(ctx context.Context, resourceID string) error {
	_, err := r.client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{resourceID},
	})
	if err != nil {
		return fmt.Errorf("terminating EC2 instance %s: %w", resourceID, err)
	}
	return nil
}

func (r *EC2Repository) instanceToResource(instance types.Instance) domain.Resource {
	resource := domain.Resource{
		ID:        aws.ToString(instance.InstanceId),
		Type:      domain.ResourceTypeEC2,
		Region:    r.region,
		AccountID: r.accountID,
		Tags:      make(map[string]string),
	}

	// Parse tags
	for _, tag := range instance.Tags {
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
				resource.ExpirationDate = ParseExpirationDate(value, resource.ID, "EC2")
			}
		}
	}

	// Set creation time
	if instance.LaunchTime != nil {
		resource.CreatedAt = *instance.LaunchTime
	}

	return resource
}
