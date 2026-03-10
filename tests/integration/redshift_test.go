//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
)

func TestRedshiftRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	rsClient := getRedshiftClient(t)

	// Create cluster subnet group for Redshift clusters
	subnetGroupName := fmt.Sprintf("cj-test-redshift-subnet-%d", time.Now().Unix())
	_, err := rsClient.CreateClusterSubnetGroup(ctx, &redshift.CreateClusterSubnetGroupInput{
		ClusterSubnetGroupName: aws.String(subnetGroupName),
		Description:            aws.String("Cloud Janitor integration test Redshift subnet group"),
		SubnetIds:              testInfra.PrivateSubnetIDs,
		Tags: []types.Tag{
			{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
		},
	})
	requireNoError(t, err, "creating cluster subnet group")

	globalCleanup.Register("ClusterSubnetGroup "+subnetGroupName, PrioritySubnetGroup, func(ctx context.Context) error {
		_, cleanupErr := rsClient.DeleteClusterSubnetGroup(ctx, &redshift.DeleteClusterSubnetGroupInput{
			ClusterSubnetGroupName: aws.String(subnetGroupName),
		})
		return cleanupErr
	})

	t.Run("ListTagDelete", func(t *testing.T) {
		clusterID := fmt.Sprintf("cj-test-rs-%d", time.Now().Unix())

		// Create Redshift cluster
		_, err := rsClient.CreateCluster(ctx, &redshift.CreateClusterInput{
			ClusterIdentifier:      aws.String(clusterID),
			NodeType:               aws.String("dc2.large"),
			MasterUsername:         aws.String("admin"),
			MasterUserPassword:     aws.String("TestPassword123!"),
			ClusterSubnetGroupName: aws.String(subnetGroupName),
			NumberOfNodes:          aws.Int32(1),
			PubliclyAccessible:     aws.Bool(false),
			Tags: []types.Tag{
				{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
				{Key: aws.String("Name"), Value: aws.String("cloud-janitor-test-redshift")},
			},
		})
		requireNoError(t, err, "creating Redshift cluster")

		globalCleanup.Register("Redshift "+clusterID, PriorityRedshift, func(ctx context.Context) error {
			_, delErr := rsClient.DeleteCluster(ctx, &redshift.DeleteClusterInput{
				ClusterIdentifier:        aws.String(clusterID),
				SkipFinalClusterSnapshot: aws.Bool(true),
			})
			if delErr != nil {
				return delErr
			}
			// Wait for deletion
			return waitFor(ctx, func() (bool, error) {
				_, descErr := rsClient.DescribeClusters(ctx, &redshift.DescribeClustersInput{
					ClusterIdentifier: aws.String(clusterID),
				})
				if descErr != nil {
					// Cluster not found = deleted
					return true, nil
				}
				return false, nil
			}, 30*time.Second, 20*time.Minute)
		})

		// Wait for Redshift cluster to be available (this takes 10-15 minutes)
		t.Log("Waiting for Redshift cluster to be available (this may take 10-15 minutes)...")
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := rsClient.DescribeClusters(ctx, &redshift.DescribeClustersInput{
				ClusterIdentifier: aws.String(clusterID),
			})
			if descErr != nil {
				return false, descErr
			}
			if len(output.Clusters) == 0 {
				return false, nil
			}
			status := aws.ToString(output.Clusters[0].ClusterStatus)
			t.Logf("Redshift cluster status: %s", status)
			return status == "available", nil
		}, 30*time.Second, 20*time.Minute)
		requireNoError(t, err, "waiting for Redshift cluster")

		repo := awsinfra.NewRedshiftRepository(rsClient, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing Redshift clusters")

		found := findResource(resources, clusterID)
		if found == nil {
			t.Fatalf("Redshift cluster %s not found", clusterID)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, clusterID, expDate)
		requireNoError(t, err, "tagging Redshift cluster")

		// Verify tag was applied
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after tag")
		found = findResource(resources, clusterID)
		if found == nil {
			t.Fatalf("Redshift cluster %s not found after tagging", clusterID)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive after tagging, got %v", found.Status())
		}

		// Test Delete
		t.Log("Deleting Redshift cluster...")
		err = repo.Delete(ctx, clusterID)
		requireNoError(t, err, "deleting Redshift cluster")

		// Wait for deletion
		err = waitFor(ctx, func() (bool, error) {
			_, descErr := rsClient.DescribeClusters(ctx, &redshift.DescribeClustersInput{
				ClusterIdentifier: aws.String(clusterID),
			})
			if descErr != nil {
				// Cluster not found = deleted
				return true, nil
			}
			return false, nil
		}, 30*time.Second, 20*time.Minute)
		requireNoError(t, err, "waiting for Redshift deletion")
	})

	t.Run("NeverExpires", func(t *testing.T) {
		clusterID := fmt.Sprintf("cj-test-rs-never-%d", time.Now().Unix())

		// Create Redshift cluster with never-expires tag
		_, err := rsClient.CreateCluster(ctx, &redshift.CreateClusterInput{
			ClusterIdentifier:      aws.String(clusterID),
			NodeType:               aws.String("dc2.large"),
			MasterUsername:         aws.String("admin"),
			MasterUserPassword:     aws.String("TestPassword123!"),
			ClusterSubnetGroupName: aws.String(subnetGroupName),
			NumberOfNodes:          aws.Int32(1),
			PubliclyAccessible:     aws.Bool(false),
			Tags: []types.Tag{
				{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
				{Key: aws.String("expiration-date"), Value: aws.String("never")},
			},
		})
		requireNoError(t, err, "creating Redshift cluster with never tag")

		globalCleanup.Register("Redshift "+clusterID, PriorityRedshift, func(ctx context.Context) error {
			_, delErr := rsClient.DeleteCluster(ctx, &redshift.DeleteClusterInput{
				ClusterIdentifier:        aws.String(clusterID),
				SkipFinalClusterSnapshot: aws.Bool(true),
			})
			if delErr != nil {
				return delErr
			}
			return waitFor(ctx, func() (bool, error) {
				_, descErr := rsClient.DescribeClusters(ctx, &redshift.DescribeClustersInput{
					ClusterIdentifier: aws.String(clusterID),
				})
				if descErr != nil {
					return true, nil
				}
				return false, nil
			}, 30*time.Second, 20*time.Minute)
		})

		// Wait for Redshift cluster to be available
		t.Log("Waiting for Redshift cluster to be available...")
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := rsClient.DescribeClusters(ctx, &redshift.DescribeClustersInput{
				ClusterIdentifier: aws.String(clusterID),
			})
			if descErr != nil {
				return false, descErr
			}
			if len(output.Clusters) == 0 {
				return false, nil
			}
			status := aws.ToString(output.Clusters[0].ClusterStatus)
			return status == "available", nil
		}, 30*time.Second, 20*time.Minute)
		requireNoError(t, err, "waiting for Redshift cluster")

		repo := awsinfra.NewRedshiftRepository(rsClient, testConfig.AccountID, testConfig.Region)

		// Test List - should find with StatusNeverExpires
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing Redshift clusters")

		found := findResource(resources, clusterID)
		if found == nil {
			t.Fatalf("Redshift cluster %s not found", clusterID)
		}
		if found.Status() != domain.StatusNeverExpires {
			t.Errorf("expected StatusNeverExpires, got %v", found.Status())
		}

		// Clean up
		_, err = rsClient.DeleteCluster(ctx, &redshift.DeleteClusterInput{
			ClusterIdentifier:        aws.String(clusterID),
			SkipFinalClusterSnapshot: aws.Bool(true),
		})
		requireNoError(t, err, "deleting Redshift cluster")
	})
}
