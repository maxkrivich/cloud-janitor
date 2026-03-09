package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

const (
	// maxTagBatchSize is the maximum number of ARNs that can be passed to DescribeTags.
	maxTagBatchSize = 20
)

// Compile-time interface check.
var _ domain.ResourceRepository = (*ELBRepository)(nil)

// elbClient defines the interface for ELBv2 client operations used by ELBRepository.
// This allows for mocking in tests.
type elbClient interface {
	DescribeLoadBalancers(ctx context.Context, params *elasticloadbalancingv2.DescribeLoadBalancersInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error)
	DescribeTags(ctx context.Context, params *elasticloadbalancingv2.DescribeTagsInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeTagsOutput, error)
	AddTags(ctx context.Context, params *elasticloadbalancingv2.AddTagsInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.AddTagsOutput, error)
	DeleteLoadBalancer(ctx context.Context, params *elasticloadbalancingv2.DeleteLoadBalancerInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DeleteLoadBalancerOutput, error)
}

// ELBRepository implements ResourceRepository for Elastic Load Balancers (ALB/NLB/GWLB).
type ELBRepository struct {
	client    elbClient
	accountID string
	region    string
}

// NewELBRepository creates a new ELBRepository.
func NewELBRepository(client *elasticloadbalancingv2.Client, accountID, region string) *ELBRepository {
	return &ELBRepository{
		client:    client,
		accountID: accountID,
		region:    region,
	}
}

// Type returns the resource type.
func (r *ELBRepository) Type() domain.ResourceType {
	return domain.ResourceTypeELB
}

// List returns all load balancers in the region.
// Only load balancers with state "active" or "active_impaired" are returned.
// Load balancers in "provisioning" or "failed" states are skipped.
func (r *ELBRepository) List(ctx context.Context, _ string) ([]domain.Resource, error) {
	loadBalancers, err := r.listLoadBalancers(ctx)
	if err != nil {
		return nil, err
	}

	if len(loadBalancers) == 0 {
		return nil, nil
	}

	// Collect ARNs for tag fetching
	arns := make([]string, len(loadBalancers))
	for i, lb := range loadBalancers {
		arns[i] = aws.ToString(lb.LoadBalancerArn)
	}

	// Fetch tags in batches
	tagMap, err := r.fetchTags(ctx, arns)
	if err != nil {
		return nil, err
	}

	// Convert load balancers to domain resources
	resources := make([]domain.Resource, 0, len(loadBalancers))
	for _, lb := range loadBalancers {
		arn := aws.ToString(lb.LoadBalancerArn)
		tags := tagMap[arn]
		resource := r.loadBalancerToResource(lb, tags)
		resources = append(resources, resource)
	}

	return resources, nil
}

// Tag adds the expiration-date tag to a load balancer.
// The resourceID should be the full ARN of the load balancer.
func (r *ELBRepository) Tag(ctx context.Context, resourceID string, expirationDate time.Time) error {
	_, err := r.client.AddTags(ctx, &elasticloadbalancingv2.AddTagsInput{
		ResourceArns: []string{resourceID},
		Tags: []types.Tag{
			{
				Key:   aws.String(ExpirationTagName),
				Value: aws.String(expirationDate.Format(ExpirationDateFormat)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("tagging ELB %s: %w", resourceID, err)
	}
	return nil
}

// Delete removes a load balancer.
// The resourceID should be the full ARN of the load balancer.
func (r *ELBRepository) Delete(ctx context.Context, resourceID string) error {
	_, err := r.client.DeleteLoadBalancer(ctx, &elasticloadbalancingv2.DeleteLoadBalancerInput{
		LoadBalancerArn: aws.String(resourceID),
	})
	if err != nil {
		return fmt.Errorf("deleting ELB %s: %w", resourceID, err)
	}
	return nil
}

// listLoadBalancers retrieves all load balancers with valid states.
func (r *ELBRepository) listLoadBalancers(ctx context.Context) ([]types.LoadBalancer, error) {
	var loadBalancers []types.LoadBalancer

	paginator := elasticloadbalancingv2.NewDescribeLoadBalancersPaginator(r.client, &elasticloadbalancingv2.DescribeLoadBalancersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing ELBs: %w", err)
		}

		for _, lb := range page.LoadBalancers {
			// Skip load balancers with invalid states
			if !r.isValidState(lb.State) {
				continue
			}
			loadBalancers = append(loadBalancers, lb)
		}
	}

	return loadBalancers, nil
}

// isValidState checks if the load balancer state is valid for processing.
// Only "active" and "active_impaired" states are valid.
func (r *ELBRepository) isValidState(state *types.LoadBalancerState) bool {
	if state == nil {
		return false
	}
	switch state.Code {
	case types.LoadBalancerStateEnumActive, types.LoadBalancerStateEnumActiveImpaired:
		return true
	default:
		return false
	}
}

// fetchTags retrieves tags for load balancers in batches.
// The DescribeTags API accepts a maximum of 20 ARNs per call.
func (r *ELBRepository) fetchTags(ctx context.Context, arns []string) (map[string][]types.Tag, error) {
	tagMap := make(map[string][]types.Tag)

	// Process ARNs in batches of maxTagBatchSize
	for i := 0; i < len(arns); i += maxTagBatchSize {
		end := i + maxTagBatchSize
		if end > len(arns) {
			end = len(arns)
		}
		batch := arns[i:end]

		output, err := r.client.DescribeTags(ctx, &elasticloadbalancingv2.DescribeTagsInput{
			ResourceArns: batch,
		})
		if err != nil {
			return nil, fmt.Errorf("describing tags for ELBs: %w", err)
		}

		for _, td := range output.TagDescriptions {
			arn := aws.ToString(td.ResourceArn)
			tagMap[arn] = td.Tags
		}
	}

	return tagMap, nil
}

// loadBalancerToResource converts an ELBv2 LoadBalancer to a domain.Resource.
func (r *ELBRepository) loadBalancerToResource(lb types.LoadBalancer, tags []types.Tag) domain.Resource {
	resource := domain.Resource{
		ID:        aws.ToString(lb.LoadBalancerArn),
		Type:      domain.ResourceTypeELB,
		Region:    r.region,
		AccountID: r.accountID,
		Name:      aws.ToString(lb.LoadBalancerName),
		Tags:      make(map[string]string),
	}

	// Parse tags
	for _, tag := range tags {
		key := aws.ToString(tag.Key)
		value := aws.ToString(tag.Value)
		resource.Tags[key] = value

		if key == ExpirationTagName {
			if IsNeverExpires(value) {
				resource.NeverExpires = true
			} else {
				resource.ExpirationDate = ParseExpirationDate(value, resource.ID, "ELB")
			}
		}
	}

	// Set creation time
	if lb.CreatedTime != nil {
		resource.CreatedAt = *lb.CreatedTime
	}

	return resource
}
