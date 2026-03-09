package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check.
var _ domain.ResourceRepository = (*OpenSearchRepository)(nil)

// openSearchClient defines the interface for OpenSearch client operations used by OpenSearchRepository.
// This allows for mocking in tests.
type openSearchClient interface {
	ListDomainNames(ctx context.Context, params *opensearch.ListDomainNamesInput, optFns ...func(*opensearch.Options)) (*opensearch.ListDomainNamesOutput, error)
	DescribeDomain(ctx context.Context, params *opensearch.DescribeDomainInput, optFns ...func(*opensearch.Options)) (*opensearch.DescribeDomainOutput, error)
	ListTags(ctx context.Context, params *opensearch.ListTagsInput, optFns ...func(*opensearch.Options)) (*opensearch.ListTagsOutput, error)
	AddTags(ctx context.Context, params *opensearch.AddTagsInput, optFns ...func(*opensearch.Options)) (*opensearch.AddTagsOutput, error)
	DeleteDomain(ctx context.Context, params *opensearch.DeleteDomainInput, optFns ...func(*opensearch.Options)) (*opensearch.DeleteDomainOutput, error)
}

// OpenSearchRepository implements ResourceRepository for OpenSearch domains.
type OpenSearchRepository struct {
	client    openSearchClient
	accountID string
	region    string
}

// NewOpenSearchRepository creates a new OpenSearchRepository.
func NewOpenSearchRepository(client *opensearch.Client, accountID, region string) *OpenSearchRepository {
	return &OpenSearchRepository{
		client:    client,
		accountID: accountID,
		region:    region,
	}
}

// Type returns the resource type.
func (r *OpenSearchRepository) Type() domain.ResourceType {
	return domain.ResourceTypeOpenSearch
}

// List returns all OpenSearch domains in the region.
func (r *OpenSearchRepository) List(ctx context.Context, _ string) ([]domain.Resource, error) {
	// List all domain names (no pagination available for this API)
	listOutput, err := r.client.ListDomainNames(ctx, &opensearch.ListDomainNamesInput{})
	if err != nil {
		return nil, fmt.Errorf("listing OpenSearch domain names: %w", err)
	}

	// Pre-allocate resources slice based on domain count
	resources := make([]domain.Resource, 0, len(listOutput.DomainNames))

	// Get details for each domain
	for _, domainInfo := range listOutput.DomainNames {
		domainName := aws.ToString(domainInfo.DomainName)

		// Describe the domain to get full details
		describeOutput, err := r.client.DescribeDomain(ctx, &opensearch.DescribeDomainInput{
			DomainName: aws.String(domainName),
		})
		if err != nil {
			return nil, fmt.Errorf("describing OpenSearch domain %s: %w", domainName, err)
		}

		domainStatus := describeOutput.DomainStatus
		if domainStatus == nil {
			continue
		}

		// Skip deleted domains
		if aws.ToBool(domainStatus.Deleted) {
			continue
		}

		// Fetch tags for this domain
		arn := aws.ToString(domainStatus.ARN)
		tags, err := r.listTags(ctx, arn)
		if err != nil {
			return nil, fmt.Errorf("listing tags for OpenSearch domain %s: %w", domainName, err)
		}

		resource := r.domainToResource(domainStatus, tags)
		resources = append(resources, resource)
	}

	return resources, nil
}

// Tag adds the expiration-date tag to an OpenSearch domain.
// OpenSearch uses ARN-based tagging: arn:aws:es:{region}:{account}:domain/{domain-name}
func (r *OpenSearchRepository) Tag(ctx context.Context, resourceID string, expirationDate time.Time) error {
	arn := r.buildARN(resourceID)

	_, err := r.client.AddTags(ctx, &opensearch.AddTagsInput{
		ARN: aws.String(arn),
		TagList: []types.Tag{
			{
				Key:   aws.String(ExpirationTagName),
				Value: aws.String(expirationDate.Format(ExpirationDateFormat)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("tagging OpenSearch domain %s: %w", resourceID, err)
	}
	return nil
}

// Delete removes an OpenSearch domain.
// Deletion is async and can take 10+ minutes (we don't wait for completion).
func (r *OpenSearchRepository) Delete(ctx context.Context, resourceID string) error {
	_, err := r.client.DeleteDomain(ctx, &opensearch.DeleteDomainInput{
		DomainName: aws.String(resourceID),
	})
	if err != nil {
		return fmt.Errorf("deleting OpenSearch domain %s: %w", resourceID, err)
	}
	return nil
}

// buildARN constructs the ARN for an OpenSearch domain.
// Format: arn:aws:es:{region}:{account}:domain/{domain-name}
func (r *OpenSearchRepository) buildARN(resourceID string) string {
	return fmt.Sprintf("arn:aws:es:%s:%s:domain/%s", r.region, r.accountID, resourceID)
}

// listTags retrieves tags for an OpenSearch domain.
func (r *OpenSearchRepository) listTags(ctx context.Context, arn string) ([]types.Tag, error) {
	output, err := r.client.ListTags(ctx, &opensearch.ListTagsInput{
		ARN: aws.String(arn),
	})
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	return output.TagList, nil
}

// domainToResource converts an OpenSearch DomainStatus to a domain.Resource.
func (r *OpenSearchRepository) domainToResource(status *types.DomainStatus, tags []types.Tag) domain.Resource {
	resource := domain.Resource{
		ID:        aws.ToString(status.DomainName),
		Type:      domain.ResourceTypeOpenSearch,
		Region:    r.region,
		AccountID: r.accountID,
		Tags:      make(map[string]string),
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
			if value == NeverExpiresValue {
				resource.NeverExpires = true
			} else {
				if t, err := time.Parse(ExpirationDateFormat, value); err == nil {
					resource.ExpirationDate = &t
				}
			}
		}
	}

	return resource
}
