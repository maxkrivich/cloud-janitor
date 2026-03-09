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
var _ domain.ResourceRepository = (*NATGatewayRepository)(nil)

// natGatewayClient defines the interface for EC2 client operations used by NATGatewayRepository.
// This allows for mocking in tests.
type natGatewayClient interface {
	DescribeNatGateways(ctx context.Context, params *ec2.DescribeNatGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error)
	CreateTags(ctx context.Context, params *ec2.CreateTagsInput, optFns ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error)
	DeleteNatGateway(ctx context.Context, params *ec2.DeleteNatGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteNatGatewayOutput, error)
}

// NATGatewayRepository implements ResourceRepository for NAT Gateways.
type NATGatewayRepository struct {
	client    natGatewayClient
	accountID string
	region    string
}

// NewNATGatewayRepository creates a new NATGatewayRepository.
func NewNATGatewayRepository(client *ec2.Client, accountID, region string) *NATGatewayRepository {
	return &NATGatewayRepository{
		client:    client,
		accountID: accountID,
		region:    region,
	}
}

// Type returns the resource type.
func (r *NATGatewayRepository) Type() domain.ResourceType {
	return domain.ResourceTypeNATGateway
}

// List returns all NAT Gateways in the region.
// Only NAT Gateways with state "pending" or "available" are returned.
// NAT Gateways in "deleting", "deleted", or "failed" states are skipped.
func (r *NATGatewayRepository) List(ctx context.Context, _ string) ([]domain.Resource, error) {
	var resources []domain.Resource

	paginator := ec2.NewDescribeNatGatewaysPaginator(r.client, &ec2.DescribeNatGatewaysInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing NAT Gateways: %w", err)
		}

		for _, natGateway := range page.NatGateways {
			// Skip NAT Gateways with invalid states
			if !r.isValidState(natGateway.State) {
				continue
			}

			resource := r.natGatewayToResource(natGateway)
			resources = append(resources, resource)
		}
	}

	return resources, nil
}

// Tag adds the expiration-date tag to a NAT Gateway.
func (r *NATGatewayRepository) Tag(ctx context.Context, resourceID string, expirationDate time.Time) error {
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
		return fmt.Errorf("tagging NAT Gateway %s: %w", resourceID, err)
	}
	return nil
}

// Delete removes a NAT Gateway.
func (r *NATGatewayRepository) Delete(ctx context.Context, resourceID string) error {
	_, err := r.client.DeleteNatGateway(ctx, &ec2.DeleteNatGatewayInput{
		NatGatewayId: aws.String(resourceID),
	})
	if err != nil {
		return fmt.Errorf("deleting NAT Gateway %s: %w", resourceID, err)
	}
	return nil
}

// isValidState checks if the NAT Gateway state is valid for processing.
// Only "pending" and "available" states are valid.
func (r *NATGatewayRepository) isValidState(state types.NatGatewayState) bool {
	switch state {
	case types.NatGatewayStatePending, types.NatGatewayStateAvailable:
		return true
	default:
		return false
	}
}

// natGatewayToResource converts an EC2 NatGateway to a domain.Resource.
func (r *NATGatewayRepository) natGatewayToResource(natGateway types.NatGateway) domain.Resource {
	resource := domain.Resource{
		ID:        aws.ToString(natGateway.NatGatewayId),
		Type:      domain.ResourceTypeNATGateway,
		Region:    r.region,
		AccountID: r.accountID,
		Tags:      make(map[string]string),
	}

	// Parse tags
	for _, tag := range natGateway.Tags {
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
				resource.ExpirationDate = ParseExpirationDate(value, resource.ID, "NATGateway")
			}
		}
	}

	// Set creation time
	if natGateway.CreateTime != nil {
		resource.CreatedAt = *natGateway.CreateTime
	}

	return resource
}
