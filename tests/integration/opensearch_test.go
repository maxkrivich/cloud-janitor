//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
)

func TestOpenSearchRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	osClient := getOpenSearchClient(t)

	t.Run("ListTagDelete", func(t *testing.T) {
		// OpenSearch domain names must be lowercase and between 3-28 characters
		domainName := fmt.Sprintf("cj-test-os-%d", time.Now().Unix()%1000000)

		// Create OpenSearch domain (using minimal configuration for cost)
		_, err := osClient.CreateDomain(ctx, &opensearch.CreateDomainInput{
			DomainName:    aws.String(domainName),
			EngineVersion: aws.String("OpenSearch_2.5"),
			ClusterConfig: &types.ClusterConfig{
				InstanceType:  types.OpenSearchPartitionInstanceTypeT3SmallSearch,
				InstanceCount: aws.Int32(1),
			},
			EBSOptions: &types.EBSOptions{
				EBSEnabled: aws.Bool(true),
				VolumeSize: aws.Int32(10),
				VolumeType: types.VolumeTypeGp2,
			},
			// Use VPC config for security
			VPCOptions: &types.VPCOptions{
				SubnetIds: []string{testInfra.PrivateSubnetIDs[0]},
			},
			TagList: []types.Tag{
				{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
				{Key: aws.String("Name"), Value: aws.String("cloud-janitor-test-opensearch")},
			},
		})
		requireNoError(t, err, "creating OpenSearch domain")

		globalCleanup.Register("OpenSearch "+domainName, PriorityOpenSearch, func(ctx context.Context) error {
			_, delErr := osClient.DeleteDomain(ctx, &opensearch.DeleteDomainInput{
				DomainName: aws.String(domainName),
			})
			if delErr != nil {
				return delErr
			}
			// Wait for deletion
			return waitFor(ctx, func() (bool, error) {
				_, descErr := osClient.DescribeDomain(ctx, &opensearch.DescribeDomainInput{
					DomainName: aws.String(domainName),
				})
				if descErr != nil {
					// Domain not found = deleted
					return true, nil
				}
				return false, nil
			}, 60*time.Second, 30*time.Minute)
		})

		// Wait for OpenSearch domain to be active (this takes 15-20 minutes)
		t.Log("Waiting for OpenSearch domain to be active (this may take 15-20 minutes)...")
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := osClient.DescribeDomain(ctx, &opensearch.DescribeDomainInput{
				DomainName: aws.String(domainName),
			})
			if descErr != nil {
				return false, descErr
			}
			if output.DomainStatus == nil {
				return false, nil
			}
			processing := aws.ToBool(output.DomainStatus.Processing)
			created := aws.ToBool(output.DomainStatus.Created)
			t.Logf("OpenSearch domain: created=%v, processing=%v", created, processing)
			return created && !processing, nil
		}, 60*time.Second, 30*time.Minute)
		requireNoError(t, err, "waiting for OpenSearch domain")

		repo := awsinfra.NewOpenSearchRepository(osClient, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing OpenSearch domains")

		found := findResource(resources, domainName)
		if found == nil {
			t.Fatalf("OpenSearch domain %s not found", domainName)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, domainName, expDate)
		requireNoError(t, err, "tagging OpenSearch domain")

		// Verify tag was applied
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after tag")
		found = findResource(resources, domainName)
		if found == nil {
			t.Fatalf("OpenSearch domain %s not found after tagging", domainName)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive after tagging, got %v", found.Status())
		}

		// Test Delete
		t.Log("Deleting OpenSearch domain...")
		err = repo.Delete(ctx, domainName)
		requireNoError(t, err, "deleting OpenSearch domain")

		// Wait for deletion
		err = waitFor(ctx, func() (bool, error) {
			_, descErr := osClient.DescribeDomain(ctx, &opensearch.DescribeDomainInput{
				DomainName: aws.String(domainName),
			})
			if descErr != nil {
				// Domain not found = deleted
				return true, nil
			}
			return false, nil
		}, 60*time.Second, 30*time.Minute)
		requireNoError(t, err, "waiting for OpenSearch deletion")
	})

	t.Run("NeverExpires", func(t *testing.T) {
		domainName := fmt.Sprintf("cj-test-os-nev-%d", time.Now().Unix()%100000)

		// Create OpenSearch domain with never-expires tag
		_, err := osClient.CreateDomain(ctx, &opensearch.CreateDomainInput{
			DomainName:    aws.String(domainName),
			EngineVersion: aws.String("OpenSearch_2.5"),
			ClusterConfig: &types.ClusterConfig{
				InstanceType:  types.OpenSearchPartitionInstanceTypeT3SmallSearch,
				InstanceCount: aws.Int32(1),
			},
			EBSOptions: &types.EBSOptions{
				EBSEnabled: aws.Bool(true),
				VolumeSize: aws.Int32(10),
				VolumeType: types.VolumeTypeGp2,
			},
			VPCOptions: &types.VPCOptions{
				SubnetIds: []string{testInfra.PrivateSubnetIDs[0]},
			},
			TagList: []types.Tag{
				{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
				{Key: aws.String("expiration-date"), Value: aws.String("never")},
			},
		})
		requireNoError(t, err, "creating OpenSearch domain with never tag")

		globalCleanup.Register("OpenSearch "+domainName, PriorityOpenSearch, func(ctx context.Context) error {
			_, delErr := osClient.DeleteDomain(ctx, &opensearch.DeleteDomainInput{
				DomainName: aws.String(domainName),
			})
			if delErr != nil {
				return delErr
			}
			return waitFor(ctx, func() (bool, error) {
				_, descErr := osClient.DescribeDomain(ctx, &opensearch.DescribeDomainInput{
					DomainName: aws.String(domainName),
				})
				if descErr != nil {
					return true, nil
				}
				return false, nil
			}, 60*time.Second, 30*time.Minute)
		})

		// Wait for OpenSearch domain to be active
		t.Log("Waiting for OpenSearch domain to be active...")
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := osClient.DescribeDomain(ctx, &opensearch.DescribeDomainInput{
				DomainName: aws.String(domainName),
			})
			if descErr != nil {
				return false, descErr
			}
			if output.DomainStatus == nil {
				return false, nil
			}
			processing := aws.ToBool(output.DomainStatus.Processing)
			created := aws.ToBool(output.DomainStatus.Created)
			return created && !processing, nil
		}, 60*time.Second, 30*time.Minute)
		requireNoError(t, err, "waiting for OpenSearch domain")

		repo := awsinfra.NewOpenSearchRepository(osClient, testConfig.AccountID, testConfig.Region)

		// Test List - should find with StatusNeverExpires
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing OpenSearch domains")

		found := findResource(resources, domainName)
		if found == nil {
			t.Fatalf("OpenSearch domain %s not found", domainName)
		}
		if found.Status() != domain.StatusNeverExpires {
			t.Errorf("expected StatusNeverExpires, got %v", found.Status())
		}

		// Clean up
		_, err = osClient.DeleteDomain(ctx, &opensearch.DeleteDomainInput{
			DomainName: aws.String(domainName),
		})
		requireNoError(t, err, "deleting OpenSearch domain")
	})
}
