//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
)

func TestRDSRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	rdsClient := getRDSClient(t)

	// Create DB subnet group for RDS instances
	subnetGroupName := fmt.Sprintf("cj-test-rds-subnet-%d", time.Now().Unix())
	_, err := rdsClient.CreateDBSubnetGroup(ctx, &rds.CreateDBSubnetGroupInput{
		DBSubnetGroupName:        aws.String(subnetGroupName),
		DBSubnetGroupDescription: aws.String("Cloud Janitor integration test subnet group"),
		SubnetIds:                testInfra.PrivateSubnetIDs,
		Tags: []types.Tag{
			{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
		},
	})
	requireNoError(t, err, "creating DB subnet group")

	globalCleanup.Register("DBSubnetGroup "+subnetGroupName, PrioritySubnetGroup, func(ctx context.Context) error {
		_, cleanupErr := rdsClient.DeleteDBSubnetGroup(ctx, &rds.DeleteDBSubnetGroupInput{
			DBSubnetGroupName: aws.String(subnetGroupName),
		})
		return cleanupErr
	})

	t.Run("ListTagDelete", func(t *testing.T) {
		dbInstanceID := fmt.Sprintf("cj-test-rds-%d", time.Now().Unix())

		// Create RDS instance
		_, err := rdsClient.CreateDBInstance(ctx, &rds.CreateDBInstanceInput{
			DBInstanceIdentifier: aws.String(dbInstanceID),
			DBInstanceClass:      aws.String("db.t3.micro"),
			Engine:               aws.String("mysql"),
			MasterUsername:       aws.String("admin"),
			MasterUserPassword:   aws.String("TestPassword123!"),
			AllocatedStorage:     aws.Int32(20),
			DBSubnetGroupName:    aws.String(subnetGroupName),
			PubliclyAccessible:   aws.Bool(false),
			Tags: []types.Tag{
				{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
				{Key: aws.String("Name"), Value: aws.String("cloud-janitor-test-rds")},
			},
			// Skip final snapshot for test instances
			BackupRetentionPeriod: aws.Int32(0),
		})
		requireNoError(t, err, "creating RDS instance")

		globalCleanup.Register("RDS "+dbInstanceID, PriorityRDS, func(ctx context.Context) error {
			_, delErr := rdsClient.DeleteDBInstance(ctx, &rds.DeleteDBInstanceInput{
				DBInstanceIdentifier:   aws.String(dbInstanceID),
				SkipFinalSnapshot:      aws.Bool(true),
				DeleteAutomatedBackups: aws.Bool(true),
			})
			if delErr != nil {
				return delErr
			}
			// Wait for deletion
			return waitFor(ctx, func() (bool, error) {
				_, descErr := rdsClient.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
					DBInstanceIdentifier: aws.String(dbInstanceID),
				})
				if descErr != nil {
					// Instance not found = deleted
					return true, nil
				}
				return false, nil
			}, 30*time.Second, 20*time.Minute)
		})

		// Wait for RDS instance to be available (this takes 10-15 minutes)
		t.Log("Waiting for RDS instance to be available (this may take 10-15 minutes)...")
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := rdsClient.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
				DBInstanceIdentifier: aws.String(dbInstanceID),
			})
			if descErr != nil {
				return false, descErr
			}
			if len(output.DBInstances) == 0 {
				return false, nil
			}
			status := aws.ToString(output.DBInstances[0].DBInstanceStatus)
			t.Logf("RDS instance status: %s", status)
			return status == "available", nil
		}, 30*time.Second, 20*time.Minute)
		requireNoError(t, err, "waiting for RDS instance")

		repo := awsinfra.NewRDSRepository(rdsClient, testConfig.AccountID, testConfig.Region, false)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing RDS instances")

		found := findResource(resources, dbInstanceID)
		if found == nil {
			t.Fatalf("RDS instance %s not found", dbInstanceID)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, dbInstanceID, expDate)
		requireNoError(t, err, "tagging RDS instance")

		// Verify tag was applied
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after tag")
		found = findResource(resources, dbInstanceID)
		if found == nil {
			t.Fatalf("RDS instance %s not found after tagging", dbInstanceID)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive after tagging, got %v", found.Status())
		}

		// Test Delete
		t.Log("Deleting RDS instance...")
		err = repo.Delete(ctx, dbInstanceID)
		requireNoError(t, err, "deleting RDS instance")

		// Wait for deletion
		err = waitFor(ctx, func() (bool, error) {
			_, descErr := rdsClient.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
				DBInstanceIdentifier: aws.String(dbInstanceID),
			})
			if descErr != nil {
				// Instance not found = deleted
				return true, nil
			}
			return false, nil
		}, 30*time.Second, 20*time.Minute)
		requireNoError(t, err, "waiting for RDS deletion")
	})

	t.Run("NeverExpires", func(t *testing.T) {
		dbInstanceID := fmt.Sprintf("cj-test-rds-never-%d", time.Now().Unix())

		// Create RDS instance with never-expires tag
		_, err := rdsClient.CreateDBInstance(ctx, &rds.CreateDBInstanceInput{
			DBInstanceIdentifier: aws.String(dbInstanceID),
			DBInstanceClass:      aws.String("db.t3.micro"),
			Engine:               aws.String("mysql"),
			MasterUsername:       aws.String("admin"),
			MasterUserPassword:   aws.String("TestPassword123!"),
			AllocatedStorage:     aws.Int32(20),
			DBSubnetGroupName:    aws.String(subnetGroupName),
			PubliclyAccessible:   aws.Bool(false),
			Tags: []types.Tag{
				{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
				{Key: aws.String("expiration-date"), Value: aws.String("never")},
			},
			BackupRetentionPeriod: aws.Int32(0),
		})
		requireNoError(t, err, "creating RDS instance with never tag")

		globalCleanup.Register("RDS "+dbInstanceID, PriorityRDS, func(ctx context.Context) error {
			_, delErr := rdsClient.DeleteDBInstance(ctx, &rds.DeleteDBInstanceInput{
				DBInstanceIdentifier:   aws.String(dbInstanceID),
				SkipFinalSnapshot:      aws.Bool(true),
				DeleteAutomatedBackups: aws.Bool(true),
			})
			if delErr != nil {
				return delErr
			}
			return waitFor(ctx, func() (bool, error) {
				_, descErr := rdsClient.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
					DBInstanceIdentifier: aws.String(dbInstanceID),
				})
				if descErr != nil {
					return true, nil
				}
				return false, nil
			}, 30*time.Second, 20*time.Minute)
		})

		// Wait for RDS instance to be available
		t.Log("Waiting for RDS instance to be available...")
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := rdsClient.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
				DBInstanceIdentifier: aws.String(dbInstanceID),
			})
			if descErr != nil {
				return false, descErr
			}
			if len(output.DBInstances) == 0 {
				return false, nil
			}
			status := aws.ToString(output.DBInstances[0].DBInstanceStatus)
			return status == "available", nil
		}, 30*time.Second, 20*time.Minute)
		requireNoError(t, err, "waiting for RDS instance")

		repo := awsinfra.NewRDSRepository(rdsClient, testConfig.AccountID, testConfig.Region, false)

		// Test List - should find with StatusNeverExpires
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing RDS instances")

		found := findResource(resources, dbInstanceID)
		if found == nil {
			t.Fatalf("RDS instance %s not found", dbInstanceID)
		}
		if found.Status() != domain.StatusNeverExpires {
			t.Errorf("expected StatusNeverExpires, got %v", found.Status())
		}

		// Clean up
		_, err = rdsClient.DeleteDBInstance(ctx, &rds.DeleteDBInstanceInput{
			DBInstanceIdentifier:   aws.String(dbInstanceID),
			SkipFinalSnapshot:      aws.Bool(true),
			DeleteAutomatedBackups: aws.Bool(true),
		})
		requireNoError(t, err, "deleting RDS instance")
	})
}
