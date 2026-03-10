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

func TestEBSRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	ec2Client := getEC2Client(t)

	t.Run("ListTagDelete", func(t *testing.T) {
		// Create volume
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

		repo := awsinfra.NewEBSRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing volumes")

		found := findResource(resources, volumeID)
		if found == nil {
			t.Fatalf("volume %s not found", volumeID)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, volumeID, expDate)
		requireNoError(t, err, "tagging volume")

		// Verify tag
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after tag")

		found = findResource(resources, volumeID)
		if found == nil {
			t.Fatalf("volume %s not found after tag", volumeID)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive, got %v", found.Status())
		}

		// Test Delete
		err = repo.Delete(ctx, volumeID)
		requireNoError(t, err, "deleting volume")

		// Verify deleted
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after delete")

		if findResource(resources, volumeID) != nil {
			t.Error("volume should be deleted")
		}
	})

	t.Run("NeverExpiresVolume", func(t *testing.T) {
		// Create volume with never-expires tag
		volumeOutput, err := ec2Client.CreateVolume(ctx, &ec2.CreateVolumeInput{
			AvailabilityZone: aws.String(testInfra.AvailabilityZones[0]),
			Size:             aws.Int32(1),
			VolumeType:       types.VolumeTypeGp3,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeVolume,
					Tags: toEC2Tags(mergeTags(testTags(), map[string]string{
						"expiration-date": "never",
					})),
				},
			},
		})
		requireNoError(t, err, "creating never-expires volume")
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

		repo := awsinfra.NewEBSRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing volumes")

		found := findResource(resources, volumeID)
		if found == nil {
			t.Fatalf("never-expires volume %s not found", volumeID)
		}
		if found.Status() != domain.StatusNeverExpires {
			t.Errorf("expected StatusNeverExpires, got %v", found.Status())
		}
	})

	t.Run("ExcludedVolume", func(t *testing.T) {
		// Create volume with DoNotDelete tag
		volumeOutput, err := ec2Client.CreateVolume(ctx, &ec2.CreateVolumeInput{
			AvailabilityZone: aws.String(testInfra.AvailabilityZones[0]),
			Size:             aws.Int32(1),
			VolumeType:       types.VolumeTypeGp3,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeVolume,
					Tags: toEC2Tags(mergeTags(testTags(), map[string]string{
						"DoNotDelete": "true",
					})),
				},
			},
		})
		requireNoError(t, err, "creating excluded volume")
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

		repo := awsinfra.NewEBSRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing volumes")

		found := findResource(resources, volumeID)
		if found == nil {
			t.Fatalf("excluded volume %s not found", volumeID)
		}

		// Verify IsExcluded works
		excludeTags := map[string]string{"DoNotDelete": "true"}
		if !found.IsExcluded(excludeTags) {
			t.Error("volume should be excluded with DoNotDelete=true tag")
		}
	})

	t.Run("SkipsAttachedVolumes", func(t *testing.T) {
		// EBS volumes attached to instances should still be listed
		// but deleting them should fail (tested in unit tests)
		// Integration test just verifies List works with attached volumes
		t.Skip("Requires EC2 instance - covered in ec2_test.go")
	})
}
