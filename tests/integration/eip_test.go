//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
)

func TestEIPRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ec2Client := getEC2Client(t)

	t.Run("ListTagDelete", func(t *testing.T) {
		// Create test EIP
		allocOutput, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
			Domain: types.DomainTypeVpc,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeElasticIp,
					Tags:         toEC2Tags(testTags()),
				},
			},
		})
		requireNoError(t, err, "allocating EIP")
		eipID := *allocOutput.AllocationId

		// Register cleanup
		globalCleanup.Register("EIP "+eipID, PriorityElasticIP, func(ctx context.Context) error {
			_, cleanupErr := ec2Client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(eipID)})
			return cleanupErr
		})

		// Create repository
		repo := awsinfra.NewElasticIPRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		// Test List - should find untagged resource
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs")

		found := findResource(resources, eipID)
		if found == nil {
			t.Fatalf("EIP %s not found in list", eipID)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, eipID, expDate)
		requireNoError(t, err, "tagging EIP")

		// Verify tag applied
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs after tag")

		found = findResource(resources, eipID)
		if found == nil {
			t.Fatalf("EIP %s not found after tagging", eipID)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive, got %v", found.Status())
		}

		// Test Delete
		err = repo.Delete(ctx, eipID)
		requireNoError(t, err, "deleting EIP")

		// Verify deleted
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs after delete")

		if findResource(resources, eipID) != nil {
			t.Error("EIP should be deleted")
		}
	})

	t.Run("ExcludedResourceNotTagged", func(t *testing.T) {
		// Create EIP with DoNotDelete tag
		allocOutput, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
			Domain: types.DomainTypeVpc,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeElasticIp,
					Tags: toEC2Tags(mergeTags(testTags(), map[string]string{
						"DoNotDelete": "true",
					})),
				},
			},
		})
		requireNoError(t, err, "allocating excluded EIP")
		eipID := *allocOutput.AllocationId

		globalCleanup.Register("EIP "+eipID, PriorityElasticIP, func(ctx context.Context) error {
			_, cleanupErr := ec2Client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(eipID)})
			return cleanupErr
		})

		repo := awsinfra.NewElasticIPRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		// List and verify excluded status
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs")

		found := findResource(resources, eipID)
		if found == nil {
			t.Fatalf("excluded EIP %s not found", eipID)
		}

		// Verify IsExcluded works
		excludeTags := map[string]string{"DoNotDelete": "true"}
		if !found.IsExcluded(excludeTags) {
			t.Error("resource should be excluded with DoNotDelete=true tag")
		}
	})

	t.Run("NeverExpiresResource", func(t *testing.T) {
		// Create EIP with expiration-date=never
		allocOutput, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
			Domain: types.DomainTypeVpc,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeElasticIp,
					Tags: toEC2Tags(mergeTags(testTags(), map[string]string{
						"expiration-date": "never",
					})),
				},
			},
		})
		requireNoError(t, err, "allocating never-expires EIP")
		eipID := *allocOutput.AllocationId

		globalCleanup.Register("EIP "+eipID, PriorityElasticIP, func(ctx context.Context) error {
			_, cleanupErr := ec2Client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(eipID)})
			return cleanupErr
		})

		repo := awsinfra.NewElasticIPRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs")

		found := findResource(resources, eipID)
		if found == nil {
			t.Fatalf("never-expires EIP %s not found", eipID)
		}
		if found.Status() != domain.StatusNeverExpires {
			t.Errorf("expected StatusNeverExpires, got %v", found.Status())
		}
	})
}
