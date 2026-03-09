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
var _ domain.ResourceRepository = (*ElasticIPRepository)(nil)

// ElasticIPRepository implements ResourceRepository for Elastic IPs.
type ElasticIPRepository struct {
	client    *ec2.Client
	accountID string
	region    string
}

// NewElasticIPRepository creates a new ElasticIPRepository.
func NewElasticIPRepository(client *ec2.Client, accountID, region string) *ElasticIPRepository {
	return &ElasticIPRepository{
		client:    client,
		accountID: accountID,
		region:    region,
	}
}

// Type returns the resource type.
func (r *ElasticIPRepository) Type() domain.ResourceType {
	return domain.ResourceTypeElasticIP
}

// List returns all Elastic IPs in the region.
func (r *ElasticIPRepository) List(ctx context.Context, _ string) ([]domain.Resource, error) {
	output, err := r.client.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{})
	if err != nil {
		return nil, fmt.Errorf("listing Elastic IPs: %w", err)
	}

	resources := make([]domain.Resource, 0, len(output.Addresses))
	for _, address := range output.Addresses {
		resource := r.addressToResource(address)
		resources = append(resources, resource)
	}

	return resources, nil
}

// Tag adds the expiration-date tag to an Elastic IP.
func (r *ElasticIPRepository) Tag(ctx context.Context, resourceID string, expirationDate time.Time) error {
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
		return fmt.Errorf("tagging Elastic IP %s: %w", resourceID, err)
	}
	return nil
}

// Delete releases an Elastic IP.
func (r *ElasticIPRepository) Delete(ctx context.Context, resourceID string) error {
	_, err := r.client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{
		AllocationId: aws.String(resourceID),
	})
	if err != nil {
		return fmt.Errorf("releasing Elastic IP %s: %w", resourceID, err)
	}
	return nil
}

func (r *ElasticIPRepository) addressToResource(address types.Address) domain.Resource {
	// Use AllocationId as the resource ID
	resourceID := aws.ToString(address.AllocationId)

	resource := domain.Resource{
		ID:        resourceID,
		Type:      domain.ResourceTypeElasticIP,
		Region:    r.region,
		AccountID: r.accountID,
		Tags:      make(map[string]string),
	}

	// Use the public IP as name if no Name tag
	if address.PublicIp != nil {
		resource.Name = aws.ToString(address.PublicIp)
	}

	// Parse tags
	for _, tag := range address.Tags {
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

	// Add association info to tags for visibility
	if address.InstanceId != nil {
		resource.Tags["AssociatedInstance"] = aws.ToString(address.InstanceId)
	}

	return resource
}
