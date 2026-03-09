package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check.
var _ domain.ResourceRepository = (*RDSRepository)(nil)

// rdsClient defines the interface for RDS client operations used by RDSRepository.
// This allows for mocking in tests.
type rdsClient interface {
	DescribeDBInstances(ctx context.Context, params *rds.DescribeDBInstancesInput, optFns ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
	AddTagsToResource(ctx context.Context, params *rds.AddTagsToResourceInput, optFns ...func(*rds.Options)) (*rds.AddTagsToResourceOutput, error)
	ModifyDBInstance(ctx context.Context, params *rds.ModifyDBInstanceInput, optFns ...func(*rds.Options)) (*rds.ModifyDBInstanceOutput, error)
	DeleteDBInstance(ctx context.Context, params *rds.DeleteDBInstanceInput, optFns ...func(*rds.Options)) (*rds.DeleteDBInstanceOutput, error)
}

// RDSRepository implements ResourceRepository for RDS database instances.
type RDSRepository struct {
	client               rdsClient
	accountID            string
	region               string
	forceDeleteProtected bool
}

// NewRDSRepository creates a new RDSRepository.
func NewRDSRepository(client *rds.Client, accountID, region string, forceDeleteProtected bool) *RDSRepository {
	return &RDSRepository{
		client:               client,
		accountID:            accountID,
		region:               region,
		forceDeleteProtected: forceDeleteProtected,
	}
}

// Type returns the resource type.
func (r *RDSRepository) Type() domain.ResourceType {
	return domain.ResourceTypeRDS
}

// List returns all RDS instances in the region.
func (r *RDSRepository) List(ctx context.Context, _ string) ([]domain.Resource, error) {
	var resources []domain.Resource

	paginator := rds.NewDescribeDBInstancesPaginator(r.client, &rds.DescribeDBInstancesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing RDS instances: %w", err)
		}

		for _, instance := range page.DBInstances {
			// Skip instances that are being deleted or already deleted
			status := aws.ToString(instance.DBInstanceStatus)
			if status == "deleting" || status == "deleted" {
				continue
			}

			resource := r.instanceToResource(instance)
			resources = append(resources, resource)
		}
	}

	return resources, nil
}

// Tag adds the expiration-date tag to an RDS instance.
// RDS uses ARN-based tagging: arn:aws:rds:{region}:{account}:db:{db-instance-id}
func (r *RDSRepository) Tag(ctx context.Context, resourceID string, expirationDate time.Time) error {
	arn := r.buildARN(resourceID)

	_, err := r.client.AddTagsToResource(ctx, &rds.AddTagsToResourceInput{
		ResourceName: aws.String(arn),
		Tags: []types.Tag{
			{
				Key:   aws.String(ExpirationTagName),
				Value: aws.String(expirationDate.Format(ExpirationDateFormat)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("tagging RDS instance %s: %w", resourceID, err)
	}
	return nil
}

// Delete removes an RDS instance.
// If forceDeleteProtected is true and the instance has deletion protection enabled,
// it will first disable the protection before deleting.
func (r *RDSRepository) Delete(ctx context.Context, resourceID string) error {
	// Check if deletion protection is enabled
	isProtected, err := r.isDeletionProtected(ctx, resourceID)
	if err != nil {
		return fmt.Errorf("checking deletion protection for RDS instance %s: %w", resourceID, err)
	}

	if isProtected {
		if !r.forceDeleteProtected {
			return fmt.Errorf("RDS instance %s has deletion protection enabled; set forceDeleteProtected to override", resourceID)
		}

		// Disable deletion protection first
		if disableErr := r.disableDeletionProtection(ctx, resourceID); disableErr != nil {
			return fmt.Errorf("disabling deletion protection for RDS instance %s: %w", resourceID, disableErr)
		}
	}

	// Delete the instance
	_, err = r.client.DeleteDBInstance(ctx, &rds.DeleteDBInstanceInput{
		DBInstanceIdentifier: aws.String(resourceID),
		SkipFinalSnapshot:    aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("deleting RDS instance %s: %w", resourceID, err)
	}

	return nil
}

// buildARN constructs the ARN for an RDS instance.
// Format: arn:aws:rds:{region}:{account}:db:{db-instance-id}
func (r *RDSRepository) buildARN(resourceID string) string {
	return fmt.Sprintf("arn:aws:rds:%s:%s:db:%s", r.region, r.accountID, resourceID)
}

// isDeletionProtected checks if an RDS instance has deletion protection enabled.
func (r *RDSRepository) isDeletionProtected(ctx context.Context, resourceID string) (bool, error) {
	output, err := r.client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(resourceID),
	})
	if err != nil {
		return false, fmt.Errorf("describing RDS instance: %w", err)
	}

	if len(output.DBInstances) == 0 {
		return false, fmt.Errorf("RDS instance %s not found", resourceID)
	}

	return aws.ToBool(output.DBInstances[0].DeletionProtection), nil
}

// disableDeletionProtection disables deletion protection on an RDS instance.
func (r *RDSRepository) disableDeletionProtection(ctx context.Context, resourceID string) error {
	_, err := r.client.ModifyDBInstance(ctx, &rds.ModifyDBInstanceInput{
		DBInstanceIdentifier: aws.String(resourceID),
		DeletionProtection:   aws.Bool(false),
	})
	if err != nil {
		return fmt.Errorf("modifying RDS instance: %w", err)
	}
	return nil
}

// instanceToResource converts an RDS DBInstance to a domain.Resource.
func (r *RDSRepository) instanceToResource(instance types.DBInstance) domain.Resource {
	resource := domain.Resource{
		ID:        aws.ToString(instance.DBInstanceIdentifier),
		Type:      domain.ResourceTypeRDS,
		Region:    r.region,
		AccountID: r.accountID,
		Tags:      make(map[string]string),
	}

	// Parse tags (RDS uses TagList, not Tags)
	for _, tag := range instance.TagList {
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
				resource.ExpirationDate = ParseExpirationDate(value, resource.ID, "RDS")
			}
		}
	}

	// Set creation time
	if instance.InstanceCreateTime != nil {
		resource.CreatedAt = *instance.InstanceCreateTime
	}

	return resource
}
