//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/maxkrivich/cloud-janitor/internal/app/service"
	"github.com/maxkrivich/cloud-janitor/internal/app/usecase"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
	"github.com/maxkrivich/cloud-janitor/internal/infra/notify"
)

// TestCompleteWorkflow validates the full Cloud Janitor lifecycle:
// 1. Create resources (normal, excluded, never-expires)
// 2. Tag untagged resources (Janitor.Tag)
// 3. Verify exclusion filters work
// 4. Simulate expiration by setting past date
// 5. Cleanup expired resources (Janitor.Cleanup)
// 6. Verify final state
//
// Uses Elastic IPs because they are the fastest to create/delete (~1 second).
func TestCompleteWorkflow(t *testing.T) {
	skipIfMissingConfig(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	ec2Client := getEC2Client(t)

	// === SETUP: Create three EIPs with different configurations ===
	var (
		normalID       string
		excludedID     string
		neverExpiresID string
	)

	t.Run("Setup_CreateResources", func(t *testing.T) {
		// 1. Normal EIP (no special tags) - should be tagged and deleted
		normalOutput, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
			Domain: types.DomainTypeVpc,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeElasticIp,
					Tags: toEC2Tags(mergeTags(testTags(), map[string]string{
						"Name": "workflow-test-normal",
					})),
				},
			},
		})
		requireNoError(t, err, "creating normal EIP")
		normalID = *normalOutput.AllocationId
		globalCleanup.Register("EIP-Normal "+normalID, PriorityElasticIP, func(cleanupCtx context.Context) error {
			_, releaseErr := ec2Client.ReleaseAddress(cleanupCtx, &ec2.ReleaseAddressInput{AllocationId: aws.String(normalID)})
			return releaseErr
		})
		t.Logf("Created normal EIP: %s", normalID)

		// 2. Excluded EIP (DoNotDelete=true) - should NOT be tagged or deleted
		excludedOutput, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
			Domain: types.DomainTypeVpc,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeElasticIp,
					Tags: toEC2Tags(mergeTags(testTags(), map[string]string{
						"Name":        "workflow-test-excluded",
						"DoNotDelete": "true",
					})),
				},
			},
		})
		requireNoError(t, err, "creating excluded EIP")
		excludedID = *excludedOutput.AllocationId
		globalCleanup.Register("EIP-Excluded "+excludedID, PriorityElasticIP, func(cleanupCtx context.Context) error {
			_, releaseErr := ec2Client.ReleaseAddress(cleanupCtx, &ec2.ReleaseAddressInput{AllocationId: aws.String(excludedID)})
			return releaseErr
		})
		t.Logf("Created excluded EIP: %s", excludedID)

		// 3. Never-expires EIP (expiration-date=never) - should NOT be tagged or deleted
		neverExpiresOutput, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
			Domain: types.DomainTypeVpc,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeElasticIp,
					Tags: toEC2Tags(mergeTags(testTags(), map[string]string{
						"Name":            "workflow-test-never-expires",
						"expiration-date": "never",
					})),
				},
			},
		})
		requireNoError(t, err, "creating never-expires EIP")
		neverExpiresID = *neverExpiresOutput.AllocationId
		globalCleanup.Register("EIP-NeverExpires "+neverExpiresID, PriorityElasticIP, func(cleanupCtx context.Context) error {
			_, releaseErr := ec2Client.ReleaseAddress(cleanupCtx, &ec2.ReleaseAddressInput{AllocationId: aws.String(neverExpiresID)})
			return releaseErr
		})
		t.Logf("Created never-expires EIP: %s", neverExpiresID)
	})

	// Verify setup succeeded
	if normalID == "" || excludedID == "" || neverExpiresID == "" {
		t.Fatal("Setup failed - missing resource IDs")
	}

	// === PHASE 1: TAG UNTAGGED RESOURCES ===
	t.Run("Phase1_TagUntaggedResources", func(t *testing.T) {
		janitor := createEIPOnlyJanitor(t, usecase.TagConfig{
			DefaultDays: 30,
			TagName:     "expiration-date",
			ExcludeTags: map[string]string{"DoNotDelete": "true"},
		}, usecase.CleanupConfig{
			ExcludeTags: map[string]string{"DoNotDelete": "true"},
		})

		result, err := janitor.Tag(ctx)
		requireNoError(t, err, "tagging resources")

		taggedIDs := extractTaggedIDs(result)
		t.Logf("Tagged %d resources: %v", len(taggedIDs), taggedIDs)

		// Normal EIP should be tagged
		if !contains(taggedIDs, normalID) {
			t.Errorf("normal EIP %s should be tagged", normalID)
		}

		// Excluded EIP should NOT be tagged
		if contains(taggedIDs, excludedID) {
			t.Errorf("excluded EIP %s should NOT be tagged", excludedID)
		}

		// Never-expires EIP should NOT be tagged (already has expiration-date)
		if contains(taggedIDs, neverExpiresID) {
			t.Errorf("never-expires EIP %s should NOT be tagged", neverExpiresID)
		}
	})

	// === PHASE 2: VERIFY TAGGING APPLIED ===
	t.Run("Phase2_VerifyTagsApplied", func(t *testing.T) {
		repo := awsinfra.NewElasticIPRepository(ec2Client, testConfig.AccountID, testConfig.Region)
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs")

		// Normal EIP should now be Active (has future expiration date)
		normal := findResource(resources, normalID)
		if normal == nil {
			t.Fatalf("normal EIP %s not found", normalID)
		}
		if normal.Status() != domain.StatusActive {
			t.Errorf("normal EIP: expected StatusActive, got %v", normal.Status())
		}
		t.Logf("Normal EIP status: %v, expiration: %v", normal.Status(), normal.ExpirationDate)

		// Excluded EIP should still be Untagged
		excluded := findResource(resources, excludedID)
		if excluded == nil {
			t.Fatalf("excluded EIP %s not found", excludedID)
		}
		if excluded.Status() != domain.StatusUntagged {
			t.Errorf("excluded EIP: expected StatusUntagged, got %v", excluded.Status())
		}
		t.Logf("Excluded EIP status: %v", excluded.Status())

		// Never-expires EIP should have StatusNeverExpires
		neverExpires := findResource(resources, neverExpiresID)
		if neverExpires == nil {
			t.Fatalf("never-expires EIP %s not found", neverExpiresID)
		}
		if neverExpires.Status() != domain.StatusNeverExpires {
			t.Errorf("never-expires EIP: expected StatusNeverExpires, got %v", neverExpires.Status())
		}
		t.Logf("Never-expires EIP status: %v", neverExpires.Status())
	})

	// === PHASE 3: SIMULATE EXPIRATION ===
	t.Run("Phase3_SimulateExpiration", func(t *testing.T) {
		// Set normal EIP's expiration to yesterday (make it expired)
		repo := awsinfra.NewElasticIPRepository(ec2Client, testConfig.AccountID, testConfig.Region)
		yesterday := time.Now().AddDate(0, 0, -1)
		err := repo.Tag(ctx, normalID, yesterday)
		requireNoError(t, err, "setting past expiration date")
		t.Logf("Set expiration date to %v for EIP %s", yesterday.Format("2006-01-02"), normalID)

		// Verify it's now expired
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs")

		normal := findResource(resources, normalID)
		if normal == nil {
			t.Fatalf("normal EIP %s not found", normalID)
		}
		if normal.Status() != domain.StatusExpired {
			t.Errorf("normal EIP: expected StatusExpired, got %v", normal.Status())
		}
		t.Logf("Normal EIP status after expiration: %v", normal.Status())
	})

	// === PHASE 4: CLEANUP EXPIRED RESOURCES ===
	t.Run("Phase4_CleanupExpired", func(t *testing.T) {
		janitor := createEIPOnlyJanitor(t, usecase.TagConfig{
			DefaultDays: 30,
			TagName:     "expiration-date",
			ExcludeTags: map[string]string{"DoNotDelete": "true"},
		}, usecase.CleanupConfig{
			ExcludeTags: map[string]string{"DoNotDelete": "true"},
		})

		result, err := janitor.Cleanup(ctx)
		requireNoError(t, err, "cleaning up resources")

		deletedIDs := extractDeletedIDs(result)
		t.Logf("Deleted %d resources: %v", len(deletedIDs), deletedIDs)

		// Normal (now expired) EIP should be deleted
		if !contains(deletedIDs, normalID) {
			t.Errorf("expired EIP %s should be deleted", normalID)
		}

		// Excluded EIP should NOT be deleted
		if contains(deletedIDs, excludedID) {
			t.Errorf("excluded EIP %s should NOT be deleted", excludedID)
		}

		// Never-expires EIP should NOT be deleted
		if contains(deletedIDs, neverExpiresID) {
			t.Errorf("never-expires EIP %s should NOT be deleted", neverExpiresID)
		}
	})

	// === PHASE 5: VERIFY FINAL STATE ===
	t.Run("Phase5_VerifyFinalState", func(t *testing.T) {
		repo := awsinfra.NewElasticIPRepository(ec2Client, testConfig.AccountID, testConfig.Region)
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing EIPs")

		// Normal EIP should be gone (deleted)
		if findResource(resources, normalID) != nil {
			t.Errorf("expired EIP %s should be deleted", normalID)
		}
		t.Log("Normal EIP correctly deleted")

		// Excluded EIP should still exist
		if findResource(resources, excludedID) == nil {
			t.Errorf("excluded EIP %s should still exist", excludedID)
		}
		t.Log("Excluded EIP correctly preserved")

		// Never-expires EIP should still exist
		if findResource(resources, neverExpiresID) == nil {
			t.Errorf("never-expires EIP %s should still exist", neverExpiresID)
		}
		t.Log("Never-expires EIP correctly preserved")
	})
}

// createEIPOnlyJanitor creates a Janitor configured for EIP-only testing.
func createEIPOnlyJanitor(t *testing.T, tagConfig usecase.TagConfig, cleanupConfig usecase.CleanupConfig) *service.Janitor {
	t.Helper()

	account := domain.Account{
		ID:   testConfig.AccountID,
		Name: "integration-test",
	}

	// Create repository factory that only returns EIP repository
	repoFactory := func(_ context.Context, acc domain.Account) ([]domain.ResourceRepository, error) {
		return []domain.ResourceRepository{
			awsinfra.NewElasticIPRepository(clients.EC2, acc.ID, testConfig.Region),
		}, nil
	}

	config := service.JanitorConfig{
		Accounts:      []domain.Account{account},
		Regions:       []string{testConfig.Region},
		TagConfig:     tagConfig,
		CleanupConfig: cleanupConfig,
	}

	return service.NewJanitor(config, notify.NewNoopNotifier(), repoFactory)
}

// extractTaggedIDs extracts resource IDs from TagResults.
func extractTaggedIDs(result *service.RunResult) []string {
	var ids []string
	for _, tr := range result.TagResults {
		for _, r := range tr.Tagged {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

// extractDeletedIDs extracts resource IDs from CleanupResults.
func extractDeletedIDs(result *service.RunResult) []string {
	var ids []string
	for _, cr := range result.CleanupResults {
		for _, r := range cr.Deleted {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

// contains checks if needle is in haystack.
func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
