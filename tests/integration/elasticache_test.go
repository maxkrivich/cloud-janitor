//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticache/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
)

func TestElastiCacheRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	cacheClient := getElastiCacheClient(t)

	// Create cache subnet group for ElastiCache clusters
	subnetGroupName := fmt.Sprintf("cj-test-cache-subnet-%d", time.Now().Unix())
	_, err := cacheClient.CreateCacheSubnetGroup(ctx, &elasticache.CreateCacheSubnetGroupInput{
		CacheSubnetGroupName:        aws.String(subnetGroupName),
		CacheSubnetGroupDescription: aws.String("Cloud Janitor integration test cache subnet group"),
		SubnetIds:                   testInfra.PrivateSubnetIDs,
		Tags: []types.Tag{
			{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
		},
	})
	requireNoError(t, err, "creating cache subnet group")

	globalCleanup.Register("CacheSubnetGroup "+subnetGroupName, PrioritySubnetGroup, func(ctx context.Context) error {
		_, cleanupErr := cacheClient.DeleteCacheSubnetGroup(ctx, &elasticache.DeleteCacheSubnetGroupInput{
			CacheSubnetGroupName: aws.String(subnetGroupName),
		})
		return cleanupErr
	})

	t.Run("ListTagDelete", func(t *testing.T) {
		clusterID := fmt.Sprintf("cj-test-cache-%d", time.Now().Unix())

		// Create ElastiCache cluster (Redis)
		_, err := cacheClient.CreateCacheCluster(ctx, &elasticache.CreateCacheClusterInput{
			CacheClusterId:       aws.String(clusterID),
			CacheNodeType:        aws.String("cache.t3.micro"),
			Engine:               aws.String("redis"),
			NumCacheNodes:        aws.Int32(1),
			CacheSubnetGroupName: aws.String(subnetGroupName),
			Tags: []types.Tag{
				{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
				{Key: aws.String("Name"), Value: aws.String("cloud-janitor-test-cache")},
			},
		})
		requireNoError(t, err, "creating ElastiCache cluster")

		globalCleanup.Register("ElastiCache "+clusterID, PriorityElastiCache, func(ctx context.Context) error {
			_, delErr := cacheClient.DeleteCacheCluster(ctx, &elasticache.DeleteCacheClusterInput{
				CacheClusterId: aws.String(clusterID),
			})
			if delErr != nil {
				return delErr
			}
			// Wait for deletion
			return waitFor(ctx, func() (bool, error) {
				_, descErr := cacheClient.DescribeCacheClusters(ctx, &elasticache.DescribeCacheClustersInput{
					CacheClusterId: aws.String(clusterID),
				})
				if descErr != nil {
					// Cluster not found = deleted
					return true, nil
				}
				return false, nil
			}, 30*time.Second, 15*time.Minute)
		})

		// Wait for ElastiCache cluster to be available (this takes 5-10 minutes)
		t.Log("Waiting for ElastiCache cluster to be available (this may take 5-10 minutes)...")
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := cacheClient.DescribeCacheClusters(ctx, &elasticache.DescribeCacheClustersInput{
				CacheClusterId: aws.String(clusterID),
			})
			if descErr != nil {
				return false, descErr
			}
			if len(output.CacheClusters) == 0 {
				return false, nil
			}
			status := aws.ToString(output.CacheClusters[0].CacheClusterStatus)
			t.Logf("ElastiCache cluster status: %s", status)
			return status == "available", nil
		}, 30*time.Second, 15*time.Minute)
		requireNoError(t, err, "waiting for ElastiCache cluster")

		repo := awsinfra.NewElastiCacheRepository(cacheClient, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing ElastiCache clusters")

		found := findResource(resources, clusterID)
		if found == nil {
			t.Fatalf("ElastiCache cluster %s not found", clusterID)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, clusterID, expDate)
		requireNoError(t, err, "tagging ElastiCache cluster")

		// Verify tag was applied
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after tag")
		found = findResource(resources, clusterID)
		if found == nil {
			t.Fatalf("ElastiCache cluster %s not found after tagging", clusterID)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive after tagging, got %v", found.Status())
		}

		// Test Delete
		t.Log("Deleting ElastiCache cluster...")
		err = repo.Delete(ctx, clusterID)
		requireNoError(t, err, "deleting ElastiCache cluster")

		// Wait for deletion
		err = waitFor(ctx, func() (bool, error) {
			_, descErr := cacheClient.DescribeCacheClusters(ctx, &elasticache.DescribeCacheClustersInput{
				CacheClusterId: aws.String(clusterID),
			})
			if descErr != nil {
				// Cluster not found = deleted
				return true, nil
			}
			return false, nil
		}, 30*time.Second, 15*time.Minute)
		requireNoError(t, err, "waiting for ElastiCache deletion")
	})

	t.Run("NeverExpires", func(t *testing.T) {
		clusterID := fmt.Sprintf("cj-test-cache-nev-%d", time.Now().Unix())

		// Create ElastiCache cluster with never-expires tag
		_, err := cacheClient.CreateCacheCluster(ctx, &elasticache.CreateCacheClusterInput{
			CacheClusterId:       aws.String(clusterID),
			CacheNodeType:        aws.String("cache.t3.micro"),
			Engine:               aws.String("redis"),
			NumCacheNodes:        aws.Int32(1),
			CacheSubnetGroupName: aws.String(subnetGroupName),
			Tags: []types.Tag{
				{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
				{Key: aws.String("expiration-date"), Value: aws.String("never")},
			},
		})
		requireNoError(t, err, "creating ElastiCache cluster with never tag")

		globalCleanup.Register("ElastiCache "+clusterID, PriorityElastiCache, func(ctx context.Context) error {
			_, delErr := cacheClient.DeleteCacheCluster(ctx, &elasticache.DeleteCacheClusterInput{
				CacheClusterId: aws.String(clusterID),
			})
			if delErr != nil {
				return delErr
			}
			return waitFor(ctx, func() (bool, error) {
				_, descErr := cacheClient.DescribeCacheClusters(ctx, &elasticache.DescribeCacheClustersInput{
					CacheClusterId: aws.String(clusterID),
				})
				if descErr != nil {
					return true, nil
				}
				return false, nil
			}, 30*time.Second, 15*time.Minute)
		})

		// Wait for ElastiCache cluster to be available
		t.Log("Waiting for ElastiCache cluster to be available...")
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := cacheClient.DescribeCacheClusters(ctx, &elasticache.DescribeCacheClustersInput{
				CacheClusterId: aws.String(clusterID),
			})
			if descErr != nil {
				return false, descErr
			}
			if len(output.CacheClusters) == 0 {
				return false, nil
			}
			status := aws.ToString(output.CacheClusters[0].CacheClusterStatus)
			return status == "available", nil
		}, 30*time.Second, 15*time.Minute)
		requireNoError(t, err, "waiting for ElastiCache cluster")

		repo := awsinfra.NewElastiCacheRepository(cacheClient, testConfig.AccountID, testConfig.Region)

		// Test List - should find with StatusNeverExpires
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing ElastiCache clusters")

		found := findResource(resources, clusterID)
		if found == nil {
			t.Fatalf("ElastiCache cluster %s not found", clusterID)
		}
		if found.Status() != domain.StatusNeverExpires {
			t.Errorf("expected StatusNeverExpires, got %v", found.Status())
		}

		// Clean up
		_, err = cacheClient.DeleteCacheCluster(ctx, &elasticache.DeleteCacheClusterInput{
			CacheClusterId: aws.String(clusterID),
		})
		requireNoError(t, err, "deleting ElastiCache cluster")
	})
}
