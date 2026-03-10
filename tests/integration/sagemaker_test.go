//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
)

func TestSageMakerRepository(t *testing.T) {
	skipIfMissingSageMakerRole(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	smClient := getSageMakerClient(t)

	t.Run("ListTagDelete", func(t *testing.T) {
		notebookName := fmt.Sprintf("cj-test-sm-%d", time.Now().Unix())

		// Create SageMaker notebook instance
		_, err := smClient.CreateNotebookInstance(ctx, &sagemaker.CreateNotebookInstanceInput{
			NotebookInstanceName: aws.String(notebookName),
			InstanceType:         types.InstanceTypeMlT3Medium,
			RoleArn:              aws.String(testConfig.SageMakerRoleARN),
			SubnetId:             aws.String(testInfra.PrivateSubnetIDs[0]),
			DirectInternetAccess: types.DirectInternetAccessDisabled,
			Tags: []types.Tag{
				{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
				{Key: aws.String("Name"), Value: aws.String("cloud-janitor-test-sagemaker")},
			},
		})
		requireNoError(t, err, "creating SageMaker notebook")

		globalCleanup.Register("SageMaker "+notebookName, PrioritySageMaker, func(ctx context.Context) error {
			// Stop the notebook first if it's running
			_, stopErr := smClient.StopNotebookInstance(ctx, &sagemaker.StopNotebookInstanceInput{
				NotebookInstanceName: aws.String(notebookName),
			})
			if stopErr != nil {
				// Ignore errors - notebook might already be stopped
				t.Logf("Stop notebook warning: %v", stopErr)
			}

			// Wait for notebook to stop
			waitErr := waitFor(ctx, func() (bool, error) {
				output, descErr := smClient.DescribeNotebookInstance(ctx, &sagemaker.DescribeNotebookInstanceInput{
					NotebookInstanceName: aws.String(notebookName),
				})
				if descErr != nil {
					return true, nil // Not found = already deleted
				}
				status := output.NotebookInstanceStatus
				return status == types.NotebookInstanceStatusStopped ||
					status == types.NotebookInstanceStatusFailed, nil
			}, 30*time.Second, 10*time.Minute)
			if waitErr != nil {
				return waitErr
			}

			// Delete the notebook
			_, delErr := smClient.DeleteNotebookInstance(ctx, &sagemaker.DeleteNotebookInstanceInput{
				NotebookInstanceName: aws.String(notebookName),
			})
			return delErr
		})

		// Wait for SageMaker notebook to be InService (this takes 5-10 minutes)
		t.Log("Waiting for SageMaker notebook to be InService (this may take 5-10 minutes)...")
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := smClient.DescribeNotebookInstance(ctx, &sagemaker.DescribeNotebookInstanceInput{
				NotebookInstanceName: aws.String(notebookName),
			})
			if descErr != nil {
				return false, descErr
			}
			status := output.NotebookInstanceStatus
			t.Logf("SageMaker notebook status: %s", status)
			return status == types.NotebookInstanceStatusInService, nil
		}, 30*time.Second, 15*time.Minute)
		requireNoError(t, err, "waiting for SageMaker notebook")

		repo := awsinfra.NewSageMakerRepository(smClient, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing SageMaker notebooks")

		found := findResource(resources, notebookName)
		if found == nil {
			t.Fatalf("SageMaker notebook %s not found", notebookName)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, notebookName, expDate)
		requireNoError(t, err, "tagging SageMaker notebook")

		// Verify tag was applied
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after tag")
		found = findResource(resources, notebookName)
		if found == nil {
			t.Fatalf("SageMaker notebook %s not found after tagging", notebookName)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive after tagging, got %v", found.Status())
		}

		// Test Delete (this will stop then delete the notebook)
		t.Log("Deleting SageMaker notebook...")
		err = repo.Delete(ctx, notebookName)
		requireNoError(t, err, "deleting SageMaker notebook")

		// Wait for deletion
		err = waitFor(ctx, func() (bool, error) {
			_, descErr := smClient.DescribeNotebookInstance(ctx, &sagemaker.DescribeNotebookInstanceInput{
				NotebookInstanceName: aws.String(notebookName),
			})
			if descErr != nil {
				// Notebook not found = deleted
				return true, nil
			}
			return false, nil
		}, 30*time.Second, 15*time.Minute)
		requireNoError(t, err, "waiting for SageMaker deletion")
	})

	t.Run("NeverExpires", func(t *testing.T) {
		notebookName := fmt.Sprintf("cj-test-sm-never-%d", time.Now().Unix())

		// Create SageMaker notebook with never-expires tag
		_, err := smClient.CreateNotebookInstance(ctx, &sagemaker.CreateNotebookInstanceInput{
			NotebookInstanceName: aws.String(notebookName),
			InstanceType:         types.InstanceTypeMlT3Medium,
			RoleArn:              aws.String(testConfig.SageMakerRoleARN),
			SubnetId:             aws.String(testInfra.PrivateSubnetIDs[0]),
			DirectInternetAccess: types.DirectInternetAccessDisabled,
			Tags: []types.Tag{
				{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
				{Key: aws.String("expiration-date"), Value: aws.String("never")},
			},
		})
		requireNoError(t, err, "creating SageMaker notebook with never tag")

		globalCleanup.Register("SageMaker "+notebookName, PrioritySageMaker, func(cleanupCtx context.Context) error {
			// Stop the notebook first
			//nolint:errcheck // Best effort cleanup
			smClient.StopNotebookInstance(cleanupCtx, &sagemaker.StopNotebookInstanceInput{
				NotebookInstanceName: aws.String(notebookName),
			})

			// Wait for notebook to stop
			//nolint:errcheck // Best effort cleanup
			waitFor(cleanupCtx, func() (bool, error) {
				output, descErr := smClient.DescribeNotebookInstance(cleanupCtx, &sagemaker.DescribeNotebookInstanceInput{
					NotebookInstanceName: aws.String(notebookName),
				})
				if descErr != nil {
					return true, nil
				}
				status := output.NotebookInstanceStatus
				return status == types.NotebookInstanceStatusStopped ||
					status == types.NotebookInstanceStatusFailed, nil
			}, 30*time.Second, 10*time.Minute)

			_, delErr := smClient.DeleteNotebookInstance(cleanupCtx, &sagemaker.DeleteNotebookInstanceInput{
				NotebookInstanceName: aws.String(notebookName),
			})
			return delErr
		})

		// Wait for SageMaker notebook to be InService
		t.Log("Waiting for SageMaker notebook to be InService...")
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := smClient.DescribeNotebookInstance(ctx, &sagemaker.DescribeNotebookInstanceInput{
				NotebookInstanceName: aws.String(notebookName),
			})
			if descErr != nil {
				return false, descErr
			}
			status := output.NotebookInstanceStatus
			return status == types.NotebookInstanceStatusInService, nil
		}, 30*time.Second, 15*time.Minute)
		requireNoError(t, err, "waiting for SageMaker notebook")

		repo := awsinfra.NewSageMakerRepository(smClient, testConfig.AccountID, testConfig.Region)

		// Test List - should find with StatusNeverExpires
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing SageMaker notebooks")

		found := findResource(resources, notebookName)
		if found == nil {
			t.Fatalf("SageMaker notebook %s not found", notebookName)
		}
		if found.Status() != domain.StatusNeverExpires {
			t.Errorf("expected StatusNeverExpires, got %v", found.Status())
		}

		// Clean up - stop then delete
		//nolint:errcheck // Best effort cleanup before explicit delete
		smClient.StopNotebookInstance(ctx, &sagemaker.StopNotebookInstanceInput{
			NotebookInstanceName: aws.String(notebookName),
		})

		// Wait for notebook to stop before deleting
		//nolint:errcheck // Best effort wait
		waitFor(ctx, func() (bool, error) {
			output, descErr := smClient.DescribeNotebookInstance(ctx, &sagemaker.DescribeNotebookInstanceInput{
				NotebookInstanceName: aws.String(notebookName),
			})
			if descErr != nil {
				return true, nil
			}
			status := output.NotebookInstanceStatus
			return status == types.NotebookInstanceStatusStopped ||
				status == types.NotebookInstanceStatusFailed, nil
		}, 30*time.Second, 10*time.Minute)

		_, err = smClient.DeleteNotebookInstance(ctx, &sagemaker.DeleteNotebookInstanceInput{
			NotebookInstanceName: aws.String(notebookName),
		})
		requireNoError(t, err, "deleting SageMaker notebook")
	})
}
