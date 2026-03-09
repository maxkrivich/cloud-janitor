package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check.
var _ domain.ResourceRepository = (*SageMakerRepository)(nil)

// sageMakerClient defines the interface for SageMaker client operations used by SageMakerRepository.
// This allows for mocking in tests.
type sageMakerClient interface {
	ListNotebookInstances(ctx context.Context, params *sagemaker.ListNotebookInstancesInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListNotebookInstancesOutput, error)
	ListTags(ctx context.Context, params *sagemaker.ListTagsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListTagsOutput, error)
	AddTags(ctx context.Context, params *sagemaker.AddTagsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.AddTagsOutput, error)
	DescribeNotebookInstance(ctx context.Context, params *sagemaker.DescribeNotebookInstanceInput, optFns ...func(*sagemaker.Options)) (*sagemaker.DescribeNotebookInstanceOutput, error)
	StopNotebookInstance(ctx context.Context, params *sagemaker.StopNotebookInstanceInput, optFns ...func(*sagemaker.Options)) (*sagemaker.StopNotebookInstanceOutput, error)
	DeleteNotebookInstance(ctx context.Context, params *sagemaker.DeleteNotebookInstanceInput, optFns ...func(*sagemaker.Options)) (*sagemaker.DeleteNotebookInstanceOutput, error)
}

// SageMakerRepository implements ResourceRepository for SageMaker notebook instances.
type SageMakerRepository struct {
	client    sageMakerClient
	accountID string
	region    string
}

// NewSageMakerRepository creates a new SageMakerRepository.
func NewSageMakerRepository(client *sagemaker.Client, accountID, region string) *SageMakerRepository {
	return &SageMakerRepository{
		client:    client,
		accountID: accountID,
		region:    region,
	}
}

// Type returns the resource type.
func (r *SageMakerRepository) Type() domain.ResourceType {
	return domain.ResourceTypeSageMaker
}

// List returns all SageMaker notebook instances in the region.
func (r *SageMakerRepository) List(ctx context.Context, _ string) ([]domain.Resource, error) {
	var resources []domain.Resource

	var nextToken *string
	for {
		output, err := r.client.ListNotebookInstances(ctx, &sagemaker.ListNotebookInstancesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("listing SageMaker notebook instances: %w", err)
		}

		for _, instance := range output.NotebookInstances {
			// Skip instances that are being deleted or have failed
			if shouldSkipNotebookStatus(instance.NotebookInstanceStatus) {
				continue
			}

			// Get tags for the instance
			tags := r.getTags(ctx, aws.ToString(instance.NotebookInstanceArn))

			resource := r.instanceToResource(instance, tags)
			resources = append(resources, resource)
		}

		nextToken = output.NextToken
		if nextToken == nil {
			break
		}
	}

	return resources, nil
}

// Tag adds the expiration-date tag to a SageMaker notebook instance.
// SageMaker uses ARN-based tagging: arn:aws:sagemaker:{region}:{account}:notebook-instance/{notebook-name}
func (r *SageMakerRepository) Tag(ctx context.Context, resourceID string, expirationDate time.Time) error {
	arn := r.buildARN(resourceID)

	_, err := r.client.AddTags(ctx, &sagemaker.AddTagsInput{
		ResourceArn: aws.String(arn),
		Tags: []types.Tag{
			{
				Key:   aws.String(ExpirationTagName),
				Value: aws.String(expirationDate.Format(ExpirationDateFormat)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("tagging SageMaker notebook %s: %w", resourceID, err)
	}
	return nil
}

// Delete removes a SageMaker notebook instance.
// Note: SageMaker notebooks must be in Stopped or Failed state before deletion.
// If the notebook is running, an error is returned asking the user to stop it first.
func (r *SageMakerRepository) Delete(ctx context.Context, resourceID string) error {
	// First, check the current status of the notebook
	describeOutput, err := r.client.DescribeNotebookInstance(ctx, &sagemaker.DescribeNotebookInstanceInput{
		NotebookInstanceName: aws.String(resourceID),
	})
	if err != nil {
		return fmt.Errorf("describing SageMaker notebook %s: %w", resourceID, err)
	}

	status := describeOutput.NotebookInstanceStatus
	if !canDeleteNotebook(status) {
		return fmt.Errorf("cannot delete notebook %s in %s state; stop the notebook first or wait for current operation to complete", resourceID, status)
	}

	// Delete the notebook instance
	_, err = r.client.DeleteNotebookInstance(ctx, &sagemaker.DeleteNotebookInstanceInput{
		NotebookInstanceName: aws.String(resourceID),
	})
	if err != nil {
		return fmt.Errorf("deleting SageMaker notebook %s: %w", resourceID, err)
	}

	return nil
}

// buildARN constructs the ARN for a SageMaker notebook instance.
// Format: arn:aws:sagemaker:{region}:{account}:notebook-instance/{notebook-name}
func (r *SageMakerRepository) buildARN(resourceID string) string {
	return fmt.Sprintf("arn:aws:sagemaker:%s:%s:notebook-instance/%s", r.region, r.accountID, resourceID)
}

// getTags retrieves tags for a SageMaker resource.
// Errors are handled gracefully - if tags cannot be retrieved, an empty slice is returned.
func (r *SageMakerRepository) getTags(ctx context.Context, arn string) []types.Tag {
	output, err := r.client.ListTags(ctx, &sagemaker.ListTagsInput{
		ResourceArn: aws.String(arn),
	})
	if err != nil {
		// Log error but don't fail the operation
		return nil
	}
	return output.Tags
}

// instanceToResource converts a SageMaker NotebookInstanceSummary to a domain.Resource.
func (r *SageMakerRepository) instanceToResource(instance types.NotebookInstanceSummary, tags []types.Tag) domain.Resource {
	resource := domain.Resource{
		ID:        aws.ToString(instance.NotebookInstanceName),
		Type:      domain.ResourceTypeSageMaker,
		Region:    r.region,
		AccountID: r.accountID,
		Tags:      make(map[string]string),
	}

	// Set creation time
	if instance.CreationTime != nil {
		resource.CreatedAt = *instance.CreationTime
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
			if IsNeverExpires(value) {
				resource.NeverExpires = true
			} else {
				resource.ExpirationDate = ParseExpirationDate(value, resource.ID, "SageMaker")
			}
		}
	}

	return resource
}

// shouldSkipNotebookStatus returns true if the notebook instance should be skipped during listing.
// We skip instances that are already being deleted or have failed.
func shouldSkipNotebookStatus(status types.NotebookInstanceStatus) bool {
	switch status {
	case types.NotebookInstanceStatusDeleting,
		types.NotebookInstanceStatusFailed:
		return true
	default:
		return false
	}
}

// canDeleteNotebook returns true if the notebook instance can be deleted.
// SageMaker notebooks can only be deleted when in Stopped or Failed state.
func canDeleteNotebook(status types.NotebookInstanceStatus) bool {
	switch status {
	case types.NotebookInstanceStatusStopped,
		types.NotebookInstanceStatusFailed:
		return true
	default:
		return false
	}
}
