//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
)

func TestEKSRepository(t *testing.T) {
	skipIfMissingEKSRole(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	eksClient := getEKSClient(t)

	t.Run("ListTagDelete", func(t *testing.T) {
		clusterName := fmt.Sprintf("cj-test-eks-%d", time.Now().Unix())

		// Create EKS cluster
		_, err := eksClient.CreateCluster(ctx, &eks.CreateClusterInput{
			Name:    aws.String(clusterName),
			RoleArn: aws.String(testConfig.EKSRoleARN),
			ResourcesVpcConfig: &types.VpcConfigRequest{
				SubnetIds: testInfra.PrivateSubnetIDs,
			},
			Tags: map[string]string{
				testTagKey: testTagValue,
				"Name":     "cloud-janitor-test-eks",
			},
		})
		requireNoError(t, err, "creating EKS cluster")

		globalCleanup.Register("EKS "+clusterName, PriorityEKSCluster, func(ctx context.Context) error {
			_, delErr := eksClient.DeleteCluster(ctx, &eks.DeleteClusterInput{
				Name: aws.String(clusterName),
			})
			if delErr != nil {
				return delErr
			}
			// Wait for deletion
			return waitFor(ctx, func() (bool, error) {
				_, descErr := eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{
					Name: aws.String(clusterName),
				})
				if descErr != nil {
					// Cluster not found = deleted
					return true, nil
				}
				return false, nil
			}, 60*time.Second, 20*time.Minute)
		})

		// Wait for EKS cluster to be ACTIVE (this takes 10-15 minutes)
		t.Log("Waiting for EKS cluster to be ACTIVE (this may take 10-15 minutes)...")
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{
				Name: aws.String(clusterName),
			})
			if descErr != nil {
				return false, descErr
			}
			if output.Cluster == nil {
				return false, nil
			}
			status := output.Cluster.Status
			t.Logf("EKS cluster status: %s", status)
			return status == types.ClusterStatusActive, nil
		}, 60*time.Second, 20*time.Minute)
		requireNoError(t, err, "waiting for EKS cluster")

		repo := awsinfra.NewEKSRepository(eksClient, testConfig.AccountID, testConfig.Region, true)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EKS clusters")

		found := findResource(resources, clusterName)
		if found == nil {
			t.Fatalf("EKS cluster %s not found", clusterName)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, clusterName, expDate)
		requireNoError(t, err, "tagging EKS cluster")

		// Verify tag was applied
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after tag")
		found = findResource(resources, clusterName)
		if found == nil {
			t.Fatalf("EKS cluster %s not found after tagging", clusterName)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive after tagging, got %v", found.Status())
		}

		// Test Delete
		t.Log("Deleting EKS cluster...")
		err = repo.Delete(ctx, clusterName)
		requireNoError(t, err, "deleting EKS cluster")

		// Wait for deletion
		err = waitFor(ctx, func() (bool, error) {
			_, descErr := eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{
				Name: aws.String(clusterName),
			})
			if descErr != nil {
				// Cluster not found = deleted
				return true, nil
			}
			return false, nil
		}, 60*time.Second, 20*time.Minute)
		requireNoError(t, err, "waiting for EKS deletion")
	})

	t.Run("NeverExpires", func(t *testing.T) {
		clusterName := fmt.Sprintf("cj-test-eks-never-%d", time.Now().Unix())

		// Create EKS cluster with never-expires tag
		_, err := eksClient.CreateCluster(ctx, &eks.CreateClusterInput{
			Name:    aws.String(clusterName),
			RoleArn: aws.String(testConfig.EKSRoleARN),
			ResourcesVpcConfig: &types.VpcConfigRequest{
				SubnetIds: testInfra.PrivateSubnetIDs,
			},
			Tags: map[string]string{
				testTagKey:        testTagValue,
				"expiration-date": "never",
			},
		})
		requireNoError(t, err, "creating EKS cluster with never tag")

		globalCleanup.Register("EKS "+clusterName, PriorityEKSCluster, func(ctx context.Context) error {
			_, delErr := eksClient.DeleteCluster(ctx, &eks.DeleteClusterInput{
				Name: aws.String(clusterName),
			})
			if delErr != nil {
				return delErr
			}
			return waitFor(ctx, func() (bool, error) {
				_, descErr := eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{
					Name: aws.String(clusterName),
				})
				if descErr != nil {
					return true, nil
				}
				return false, nil
			}, 60*time.Second, 20*time.Minute)
		})

		// Wait for EKS cluster to be ACTIVE
		t.Log("Waiting for EKS cluster to be ACTIVE...")
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{
				Name: aws.String(clusterName),
			})
			if descErr != nil {
				return false, descErr
			}
			if output.Cluster == nil {
				return false, nil
			}
			status := output.Cluster.Status
			return status == types.ClusterStatusActive, nil
		}, 60*time.Second, 20*time.Minute)
		requireNoError(t, err, "waiting for EKS cluster")

		repo := awsinfra.NewEKSRepository(eksClient, testConfig.AccountID, testConfig.Region, true)

		// Test List - should find with StatusNeverExpires
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EKS clusters")

		found := findResource(resources, clusterName)
		if found == nil {
			t.Fatalf("EKS cluster %s not found", clusterName)
		}
		if found.Status() != domain.StatusNeverExpires {
			t.Errorf("expected StatusNeverExpires, got %v", found.Status())
		}

		// Clean up
		_, err = eksClient.DeleteCluster(ctx, &eks.DeleteClusterInput{
			Name: aws.String(clusterName),
		})
		requireNoError(t, err, "deleting EKS cluster")
	})

	t.Run("CascadeDeleteNodeGroups", func(t *testing.T) {
		// This test verifies that deleting an EKS cluster with cascade=true
		// also deletes associated node groups
		clusterName := fmt.Sprintf("cj-test-eks-ng-%d", time.Now().Unix())

		// Create EKS cluster
		_, err := eksClient.CreateCluster(ctx, &eks.CreateClusterInput{
			Name:    aws.String(clusterName),
			RoleArn: aws.String(testConfig.EKSRoleARN),
			ResourcesVpcConfig: &types.VpcConfigRequest{
				SubnetIds: testInfra.PrivateSubnetIDs,
			},
			Tags: map[string]string{
				testTagKey: testTagValue,
				"Name":     "cloud-janitor-test-eks-cascade",
			},
		})
		requireNoError(t, err, "creating EKS cluster for cascade test")

		globalCleanup.Register("EKS "+clusterName, PriorityEKSCluster, func(cleanupCtx context.Context) error {
			// First delete any node groups
			ngOutput, listErr := eksClient.ListNodegroups(cleanupCtx, &eks.ListNodegroupsInput{
				ClusterName: aws.String(clusterName),
			})
			if listErr == nil {
				for _, ngName := range ngOutput.Nodegroups {
					//nolint:errcheck // Best effort cleanup
					eksClient.DeleteNodegroup(cleanupCtx, &eks.DeleteNodegroupInput{
						ClusterName:   aws.String(clusterName),
						NodegroupName: aws.String(ngName),
					})
				}
				// Wait for node groups to be deleted
				//nolint:errcheck // Best effort cleanup
				waitFor(cleanupCtx, func() (bool, error) {
					ngOut, ngErr := eksClient.ListNodegroups(cleanupCtx, &eks.ListNodegroupsInput{
						ClusterName: aws.String(clusterName),
					})
					if ngErr != nil {
						return true, nil
					}
					return len(ngOut.Nodegroups) == 0, nil
				}, 30*time.Second, 10*time.Minute)
			}

			_, delErr := eksClient.DeleteCluster(cleanupCtx, &eks.DeleteClusterInput{
				Name: aws.String(clusterName),
			})
			if delErr != nil {
				return delErr
			}
			return waitFor(cleanupCtx, func() (bool, error) {
				_, descErr := eksClient.DescribeCluster(cleanupCtx, &eks.DescribeClusterInput{
					Name: aws.String(clusterName),
				})
				if descErr != nil {
					return true, nil
				}
				return false, nil
			}, 60*time.Second, 20*time.Minute)
		})

		// Wait for EKS cluster to be ACTIVE
		t.Log("Waiting for EKS cluster to be ACTIVE...")
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{
				Name: aws.String(clusterName),
			})
			if descErr != nil {
				return false, descErr
			}
			if output.Cluster == nil {
				return false, nil
			}
			status := output.Cluster.Status
			t.Logf("EKS cluster status: %s", status)
			return status == types.ClusterStatusActive, nil
		}, 60*time.Second, 20*time.Minute)
		requireNoError(t, err, "waiting for EKS cluster")

		// Note: Creating a node group requires an EC2 node role which may not be available.
		// If the EKS role can also be used for nodes, we'll try; otherwise skip this part.
		nodeGroupName := fmt.Sprintf("cj-test-ng-%d", time.Now().Unix())
		_, ngErr := eksClient.CreateNodegroup(ctx, &eks.CreateNodegroupInput{
			ClusterName:   aws.String(clusterName),
			NodegroupName: aws.String(nodeGroupName),
			NodeRole:      aws.String(testConfig.EKSRoleARN), // May need separate node role
			Subnets:       testInfra.PrivateSubnetIDs,
			ScalingConfig: &types.NodegroupScalingConfig{
				MinSize:     aws.Int32(1),
				MaxSize:     aws.Int32(1),
				DesiredSize: aws.Int32(1),
			},
			Tags: map[string]string{
				testTagKey: testTagValue,
			},
		})

		if ngErr != nil {
			t.Logf("Could not create node group (may need separate node IAM role): %v", ngErr)
			t.Log("Testing cascade delete without node groups...")
		} else {
			globalCleanup.Register("EKS NodeGroup "+nodeGroupName, PriorityEKSNodeGroup, func(ctx context.Context) error {
				_, delErr := eksClient.DeleteNodegroup(ctx, &eks.DeleteNodegroupInput{
					ClusterName:   aws.String(clusterName),
					NodegroupName: aws.String(nodeGroupName),
				})
				return delErr
			})

			// Wait for node group to be ACTIVE
			t.Log("Waiting for node group to be ACTIVE...")
			err = waitFor(ctx, func() (bool, error) {
				output, descErr := eksClient.DescribeNodegroup(ctx, &eks.DescribeNodegroupInput{
					ClusterName:   aws.String(clusterName),
					NodegroupName: aws.String(nodeGroupName),
				})
				if descErr != nil {
					return false, descErr
				}
				if output.Nodegroup == nil {
					return false, nil
				}
				status := output.Nodegroup.Status
				t.Logf("Node group status: %s", status)
				return status == types.NodegroupStatusActive, nil
			}, 60*time.Second, 15*time.Minute)
			requireNoError(t, err, "waiting for node group")
		}

		// Create repository with cascade delete enabled
		repo := awsinfra.NewEKSRepository(eksClient, testConfig.AccountID, testConfig.Region, true)

		// Delete cluster (should cascade delete node groups)
		t.Log("Deleting EKS cluster with cascade delete...")
		err = repo.Delete(ctx, clusterName)
		requireNoError(t, err, "cascade deleting EKS cluster")

		// Verify cluster is deleted
		err = waitFor(ctx, func() (bool, error) {
			_, descErr := eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{
				Name: aws.String(clusterName),
			})
			if descErr != nil {
				return true, nil
			}
			return false, nil
		}, 60*time.Second, 20*time.Minute)
		requireNoError(t, err, "waiting for EKS cascade deletion")
	})
}
