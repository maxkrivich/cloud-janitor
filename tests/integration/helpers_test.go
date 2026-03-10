//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

const (
	// testTagKey is the tag key used to identify test resources.
	testTagKey = "cloud-janitor-test"
	// testTagValue is the tag value used to identify test resources.
	testTagValue = "true"
)

// testTags returns the standard tags for test resources.
func testTags() map[string]string {
	return map[string]string{
		testTagKey: testTagValue,
		"Name":     fmt.Sprintf("cloud-janitor-test-%d", time.Now().Unix()),
	}
}

// mergeTags merges additional tags with test tags.
func mergeTags(base, additional map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range additional {
		result[k] = v
	}
	return result
}

// toEC2Tags converts a map to EC2 tag slice.
func toEC2Tags(tags map[string]string) []types.Tag {
	result := make([]types.Tag, 0, len(tags))
	for k, v := range tags {
		result = append(result, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return result
}

// findResource finds a resource by ID in a slice.
func findResource(resources []domain.Resource, id string) *domain.Resource {
	for i := range resources {
		if resources[i].ID == id {
			return &resources[i]
		}
	}
	return nil
}

// containsResourceID checks if a resource ID is in the slice.
//
//nolint:unused // Used in Phase 2-4 scanner tests
func containsResourceID(resources []domain.Resource, id string) bool {
	return findResource(resources, id) != nil
}

// waitFor polls a condition until it returns true or timeout.
func waitFor(ctx context.Context, condition func() (bool, error), interval, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for condition: %w", ctx.Err())
		case <-ticker.C:
			done, err := condition()
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		}
	}
}

// requireNoError fails the test if err is not nil.
func requireNoError(t *testing.T, err error, msg string, args ...any) {
	t.Helper()
	if err != nil {
		t.Fatalf(msg+": %v", append(args, err)...)
	}
}

// assertContains checks if needle is in haystack.
//
//nolint:unused // Used in Phase 2-4 scanner tests
func assertContains(t *testing.T, haystack []string, needle string, msg string) {
	t.Helper()
	for _, s := range haystack {
		if s == needle {
			return
		}
	}
	t.Errorf("%s: %q not found in %v", msg, needle, haystack)
}

// assertNotContains checks if needle is NOT in haystack.
//
//nolint:unused // Used in Phase 2-4 scanner tests
func assertNotContains(t *testing.T, haystack []string, needle string, msg string) {
	t.Helper()
	for _, s := range haystack {
		if s == needle {
			t.Errorf("%s: %q should not be in %v", msg, needle, haystack)
			return
		}
	}
}

// getResourceIDs extracts IDs from a slice of resources.
//
//nolint:unused // Used in Phase 2-4 scanner tests
func getResourceIDs(resources []domain.Resource) []string {
	ids := make([]string, len(resources))
	for i, r := range resources {
		ids[i] = r.ID
	}
	return ids
}
