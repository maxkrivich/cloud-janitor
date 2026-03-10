//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
)

func TestLogsRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	logsClient := getLogsClient(t)

	t.Run("ListTagDelete", func(t *testing.T) {
		logGroupName := fmt.Sprintf("/cloud-janitor-test/%d", time.Now().UnixNano())

		// Create log group
		_, err := logsClient.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(logGroupName),
			Tags: map[string]string{
				testTagKey: testTagValue,
			},
		})
		requireNoError(t, err, "creating log group")

		globalCleanup.Register("LogGroup "+logGroupName, PriorityLogGroup, func(ctx context.Context) error {
			_, cleanupErr := logsClient.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{
				LogGroupName: aws.String(logGroupName),
			})
			return cleanupErr
		})

		// Create repository with empty skip patterns to not skip test log groups
		repo := awsinfra.NewLogsRepository(logsClient, testConfig.AccountID, testConfig.Region, []string{})

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing log groups")

		found := findResource(resources, logGroupName)
		if found == nil {
			t.Fatalf("log group %s not found", logGroupName)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, logGroupName, expDate)
		requireNoError(t, err, "tagging log group")

		// Verify tag applied
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after tag")

		found = findResource(resources, logGroupName)
		if found == nil {
			t.Fatalf("log group %s not found after tagging", logGroupName)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive, got %v", found.Status())
		}

		// Test Delete
		err = repo.Delete(ctx, logGroupName)
		requireNoError(t, err, "deleting log group")

		// Verify deleted
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after delete")

		if findResource(resources, logGroupName) != nil {
			t.Error("log group should be deleted")
		}
	})

	t.Run("SkipPatternsWork", func(t *testing.T) {
		// Create log group matching skip pattern
		logGroupName := fmt.Sprintf("/aws/lambda/test-%d", time.Now().UnixNano())

		_, err := logsClient.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(logGroupName),
			Tags: map[string]string{
				testTagKey: testTagValue,
			},
		})
		requireNoError(t, err, "creating lambda log group")

		globalCleanup.Register("LogGroup "+logGroupName, PriorityLogGroup, func(ctx context.Context) error {
			_, cleanupErr := logsClient.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{
				LogGroupName: aws.String(logGroupName),
			})
			return cleanupErr
		})

		// Create repo with skip patterns
		skipPatterns := []string{"/aws/lambda/*"}
		repo := awsinfra.NewLogsRepository(logsClient, testConfig.AccountID, testConfig.Region, skipPatterns)

		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing with skip patterns")

		// Should NOT find the log group (skipped)
		if findResource(resources, logGroupName) != nil {
			t.Error("log group should be skipped by pattern /aws/lambda/*")
		}
	})

	t.Run("NeverExpiresLogGroup", func(t *testing.T) {
		logGroupName := fmt.Sprintf("/cloud-janitor-test/never-%d", time.Now().UnixNano())

		// Create log group with expiration-date=never tag
		_, err := logsClient.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(logGroupName),
			Tags: map[string]string{
				testTagKey:        testTagValue,
				"expiration-date": "never",
			},
		})
		requireNoError(t, err, "creating never-expires log group")

		globalCleanup.Register("LogGroup "+logGroupName, PriorityLogGroup, func(ctx context.Context) error {
			_, cleanupErr := logsClient.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{
				LogGroupName: aws.String(logGroupName),
			})
			return cleanupErr
		})

		// Create repository with empty skip patterns
		repo := awsinfra.NewLogsRepository(logsClient, testConfig.AccountID, testConfig.Region, []string{})

		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing log groups")

		found := findResource(resources, logGroupName)
		if found == nil {
			t.Fatalf("never-expires log group %s not found", logGroupName)
		}
		if found.Status() != domain.StatusNeverExpires {
			t.Errorf("expected StatusNeverExpires, got %v", found.Status())
		}
	})
}
