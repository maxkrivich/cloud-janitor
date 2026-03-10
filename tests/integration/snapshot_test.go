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

func TestSnapshotRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	ec2Client := getEC2Client(t)

	t.Run("ListTagDelete", func(t *testing.T) {
		// First create a volume to snapshot
		volumeOutput, err := ec2Client.CreateVolume(ctx, &ec2.CreateVolumeInput{
			AvailabilityZone: aws.String(testInfra.AvailabilityZones[0]),
			Size:             aws.Int32(1), // 1 GB minimum
			VolumeType:       types.VolumeTypeGp3,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeVolume,
					Tags:         toEC2Tags(testTags()),
				},
			},
		})
		requireNoError(t, err, "creating volume")
		volumeID := *volumeOutput.VolumeId

		globalCleanup.Register("Volume "+volumeID, PriorityEBS, func(ctx context.Context) error {
			_, delErr := ec2Client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: aws.String(volumeID)})
			return delErr
		})

		// Wait for volume to be available
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := ec2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
				VolumeIds: []string{volumeID},
			})
			if descErr != nil {
				return false, descErr
			}
			return output.Volumes[0].State == types.VolumeStateAvailable, nil
		}, 2*time.Second, 60*time.Second)
		requireNoError(t, err, "waiting for volume")

		// Create snapshot
		snapshotOutput, err := ec2Client.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
			VolumeId: aws.String(volumeID),
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeSnapshot,
					Tags:         toEC2Tags(testTags()),
				},
			},
		})
		requireNoError(t, err, "creating snapshot")
		snapshotID := *snapshotOutput.SnapshotId

		globalCleanup.Register("Snapshot "+snapshotID, PrioritySnapshot, func(ctx context.Context) error {
			_, delErr := ec2Client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{SnapshotId: aws.String(snapshotID)})
			return delErr
		})

		// Wait for snapshot to complete
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := ec2Client.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{
				SnapshotIds: []string{snapshotID},
			})
			if descErr != nil {
				return false, descErr
			}
			return output.Snapshots[0].State == types.SnapshotStateCompleted, nil
		}, 5*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for snapshot")

		repo := awsinfra.NewSnapshotRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing snapshots")

		found := findResource(resources, snapshotID)
		if found == nil {
			t.Fatalf("snapshot %s not found", snapshotID)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, snapshotID, expDate)
		requireNoError(t, err, "tagging snapshot")

		// Verify tag applied
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after tag")

		found = findResource(resources, snapshotID)
		if found == nil {
			t.Fatalf("snapshot %s not found after tagging", snapshotID)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive, got %v", found.Status())
		}

		// Test Delete
		err = repo.Delete(ctx, snapshotID)
		requireNoError(t, err, "deleting snapshot")

		// Verify deleted
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after delete")

		if findResource(resources, snapshotID) != nil {
			t.Error("snapshot should be deleted")
		}
	})

	t.Run("NeverExpiresSnapshot", func(t *testing.T) {
		// Create a volume for snapshot
		volumeOutput, err := ec2Client.CreateVolume(ctx, &ec2.CreateVolumeInput{
			AvailabilityZone: aws.String(testInfra.AvailabilityZones[0]),
			Size:             aws.Int32(1),
			VolumeType:       types.VolumeTypeGp3,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeVolume,
					Tags:         toEC2Tags(testTags()),
				},
			},
		})
		requireNoError(t, err, "creating volume for never-expires test")
		volumeID := *volumeOutput.VolumeId

		globalCleanup.Register("Volume "+volumeID, PriorityEBS, func(ctx context.Context) error {
			_, delErr := ec2Client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: aws.String(volumeID)})
			return delErr
		})

		// Wait for volume
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := ec2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{VolumeIds: []string{volumeID}})
			if descErr != nil {
				return false, descErr
			}
			return output.Volumes[0].State == types.VolumeStateAvailable, nil
		}, 2*time.Second, 60*time.Second)
		requireNoError(t, err, "waiting for volume")

		// Create snapshot with never-expires tag
		snapshotOutput, err := ec2Client.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
			VolumeId: aws.String(volumeID),
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeSnapshot,
					Tags: toEC2Tags(mergeTags(testTags(), map[string]string{
						"expiration-date": "never",
					})),
				},
			},
		})
		requireNoError(t, err, "creating never-expires snapshot")
		snapshotID := *snapshotOutput.SnapshotId

		globalCleanup.Register("Snapshot "+snapshotID, PrioritySnapshot, func(ctx context.Context) error {
			_, delErr := ec2Client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{SnapshotId: aws.String(snapshotID)})
			return delErr
		})

		// Wait for snapshot
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := ec2Client.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{SnapshotIds: []string{snapshotID}})
			if descErr != nil {
				return false, descErr
			}
			return output.Snapshots[0].State == types.SnapshotStateCompleted, nil
		}, 5*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for snapshot")

		repo := awsinfra.NewSnapshotRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing snapshots")

		found := findResource(resources, snapshotID)
		if found == nil {
			t.Fatalf("never-expires snapshot %s not found", snapshotID)
		}
		if found.Status() != domain.StatusNeverExpires {
			t.Errorf("expected StatusNeverExpires, got %v", found.Status())
		}
	})
}
