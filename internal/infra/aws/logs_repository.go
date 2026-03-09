package aws

import (
	"context"
	"fmt"
	"log"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// DefaultLogsSkipPatterns contains the default patterns for log groups to skip.
// These protect AWS-managed log groups from being tagged/deleted.
var DefaultLogsSkipPatterns = []string{
	"/aws/lambda/*",
	"/aws/eks/*",
	"/aws/rds/*",
	"/aws/elasticbeanstalk/*",
}

// Compile-time interface check.
var _ domain.ResourceRepository = (*LogsRepository)(nil)

// logsClient defines the interface for CloudWatch Logs client operations used by LogsRepository.
// This allows for mocking in tests.
type logsClient interface {
	DescribeLogGroups(ctx context.Context, params *cloudwatchlogs.DescribeLogGroupsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error)
	ListTagsForResource(ctx context.Context, params *cloudwatchlogs.ListTagsForResourceInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.ListTagsForResourceOutput, error)
	TagResource(ctx context.Context, params *cloudwatchlogs.TagResourceInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.TagResourceOutput, error)
	DeleteLogGroup(ctx context.Context, params *cloudwatchlogs.DeleteLogGroupInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DeleteLogGroupOutput, error)
}

// LogsRepository implements ResourceRepository for CloudWatch Log Groups.
type LogsRepository struct {
	client       logsClient
	accountID    string
	region       string
	skipPatterns []string
}

// NewLogsRepository creates a new LogsRepository.
// If skipPatterns is nil, DefaultLogsSkipPatterns will be used.
// Pass an empty slice to skip no patterns.
func NewLogsRepository(client *cloudwatchlogs.Client, accountID, region string, skipPatterns []string) *LogsRepository {
	patterns := skipPatterns
	if patterns == nil {
		patterns = DefaultLogsSkipPatterns
	}
	return &LogsRepository{
		client:       client,
		accountID:    accountID,
		region:       region,
		skipPatterns: patterns,
	}
}

// Type returns the resource type.
func (r *LogsRepository) Type() domain.ResourceType {
	return domain.ResourceTypeLogs
}

// List returns all CloudWatch Log Groups in the region, excluding those matching skip patterns.
func (r *LogsRepository) List(ctx context.Context, _ string) ([]domain.Resource, error) {
	var resources []domain.Resource

	var nextToken *string
	for {
		output, err := r.client.DescribeLogGroups(ctx, &cloudwatchlogs.DescribeLogGroupsInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("listing CloudWatch log groups: %w", err)
		}

		for _, logGroup := range output.LogGroups {
			logGroupName := aws.ToString(logGroup.LogGroupName)

			// Skip log groups matching skip patterns
			if r.shouldSkip(logGroupName) {
				continue
			}

			// Fetch tags for this log group
			tags := r.fetchTags(ctx, aws.ToString(logGroup.Arn))

			resource := r.logGroupToResource(logGroup, tags)
			resources = append(resources, resource)
		}

		nextToken = output.NextToken
		if nextToken == nil {
			break
		}
	}

	return resources, nil
}

// Tag adds the expiration-date tag to a CloudWatch Log Group.
// CloudWatch Logs uses ARN-based tagging: arn:aws:logs:{region}:{account}:log-group:{log-group-name}
func (r *LogsRepository) Tag(ctx context.Context, resourceID string, expirationDate time.Time) error {
	arn := r.buildARN(resourceID)

	_, err := r.client.TagResource(ctx, &cloudwatchlogs.TagResourceInput{
		ResourceArn: aws.String(arn),
		Tags: map[string]string{
			ExpirationTagName: expirationDate.Format(ExpirationDateFormat),
		},
	})
	if err != nil {
		return fmt.Errorf("tagging CloudWatch log group %s: %w", resourceID, err)
	}
	return nil
}

// Delete removes a CloudWatch Log Group.
func (r *LogsRepository) Delete(ctx context.Context, resourceID string) error {
	_, err := r.client.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{
		LogGroupName: aws.String(resourceID),
	})
	if err != nil {
		return fmt.Errorf("deleting CloudWatch log group %s: %w", resourceID, err)
	}
	return nil
}

// buildARN constructs the ARN for a CloudWatch Log Group.
// Format: arn:aws:logs:{region}:{account}:log-group:{log-group-name}
func (r *LogsRepository) buildARN(logGroupName string) string {
	return fmt.Sprintf("arn:aws:logs:%s:%s:log-group:%s", r.region, r.accountID, logGroupName)
}

// shouldSkip checks if a log group name matches any of the skip patterns.
// Patterns ending with /* match any nested paths (prefix matching).
// Other patterns use standard glob matching with * and ? wildcards.
func (r *LogsRepository) shouldSkip(logGroupName string) bool {
	for _, pattern := range r.skipPatterns {
		// For patterns ending with /*, use prefix matching to support nested paths
		// e.g., "/aws/lambda/*" should match "/aws/lambda/func" and "/aws/lambda/func/nested"
		if strings.HasSuffix(pattern, "/*") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(logGroupName, prefix) {
				return true
			}
			continue
		}

		// For other patterns, use standard glob matching
		// It supports * (any sequence in single component) and ? (any single character)
		matched, err := path.Match(pattern, logGroupName)
		if err != nil {
			// Invalid pattern - skip it
			continue
		}
		if matched {
			return true
		}
	}
	return false
}

// fetchTags retrieves tags for a log group ARN.
// Errors are handled gracefully - returns empty tags on error.
// Note: DescribeLogGroups returns ARNs with :* suffix, but ListTagsForResource
// requires ARNs without the suffix, so we strip it here.
func (r *LogsRepository) fetchTags(ctx context.Context, arn string) map[string]string {
	// Strip the :* suffix if present (DescribeLogGroups returns ARNs with :* suffix)
	cleanARN := strings.TrimSuffix(arn, ":*")

	output, err := r.client.ListTagsForResource(ctx, &cloudwatchlogs.ListTagsForResourceInput{
		ResourceArn: aws.String(cleanARN),
	})
	if err != nil {
		// Log the error for debugging but don't fail the operation
		log.Printf("warning: failed to fetch tags for log group %s: %v", cleanARN, err)
		return map[string]string{}
	}
	if output.Tags == nil {
		return map[string]string{}
	}
	return output.Tags
}

// logGroupToResource converts a CloudWatch LogGroup to a domain.Resource.
func (r *LogsRepository) logGroupToResource(logGroup types.LogGroup, tags map[string]string) domain.Resource {
	resource := domain.Resource{
		ID:        aws.ToString(logGroup.LogGroupName),
		Type:      domain.ResourceTypeLogs,
		Region:    r.region,
		AccountID: r.accountID,
		Tags:      make(map[string]string),
	}

	// Copy tags
	for key, value := range tags {
		resource.Tags[key] = value

		switch key {
		case "Name":
			resource.Name = value
		case ExpirationTagName:
			if IsNeverExpires(value) {
				resource.NeverExpires = true
			} else {
				resource.ExpirationDate = ParseExpirationDate(value, resource.ID, "CloudWatchLogs")
			}
		}
	}

	// Set creation time
	if logGroup.CreationTime != nil {
		resource.CreatedAt = time.UnixMilli(*logGroup.CreationTime)
	}

	return resource
}
