//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
)

func TestAMIRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	ec2Client := getEC2Client(t)

	t.Run("ListTagDelete", func(t *testing.T) {
		// First create a snapshot to register as AMI
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
		requireNoError(t, err, "creating volume for AMI")
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
		requireNoError(t, err, "creating snapshot for AMI")
		snapshotID := *snapshotOutput.SnapshotId

		globalCleanup.Register("Snapshot "+snapshotID, PrioritySnapshot, func(ctx context.Context) error {
			_, delErr := ec2Client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{SnapshotId: aws.String(snapshotID)})
			return delErr
		})

		// Wait for snapshot to complete
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := ec2Client.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{SnapshotIds: []string{snapshotID}})
			if descErr != nil {
				return false, descErr
			}
			return output.Snapshots[0].State == types.SnapshotStateCompleted, nil
		}, 5*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for snapshot")

		// Register AMI
		amiName := fmt.Sprintf("cloud-janitor-test-%d", time.Now().UnixNano())
		amiOutput, err := ec2Client.RegisterImage(ctx, &ec2.RegisterImageInput{
			Name:               aws.String(amiName),
			Architecture:       types.ArchitectureValuesX8664,
			RootDeviceName:     aws.String("/dev/xvda"),
			VirtualizationType: aws.String("hvm"),
			BlockDeviceMappings: []types.BlockDeviceMapping{
				{
					DeviceName: aws.String("/dev/xvda"),
					Ebs: &types.EbsBlockDevice{
						SnapshotId:          aws.String(snapshotID),
						DeleteOnTermination: aws.Bool(true),
					},
				},
			},
		})
		requireNoError(t, err, "registering AMI")
		amiID := *amiOutput.ImageId

		// Tag the AMI
		_, err = ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []string{amiID},
			Tags:      toEC2Tags(testTags()),
		})
		requireNoError(t, err, "tagging AMI")

		globalCleanup.Register("AMI "+amiID, PriorityAMI, func(ctx context.Context) error {
			_, deregErr := ec2Client.DeregisterImage(ctx, &ec2.DeregisterImageInput{ImageId: aws.String(amiID)})
			return deregErr
		})

		// Wait for AMI to be available
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{ImageIds: []string{amiID}})
			if descErr != nil {
				return false, descErr
			}
			if len(output.Images) == 0 {
				return false, nil
			}
			return output.Images[0].State == types.ImageStateAvailable, nil
		}, 5*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for AMI")

		repo := awsinfra.NewAMIRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing AMIs")

		found := findResource(resources, amiID)
		if found == nil {
			t.Fatalf("AMI %s not found", amiID)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, amiID, expDate)
		requireNoError(t, err, "tagging AMI")

		// Verify tag applied
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after tag")

		found = findResource(resources, amiID)
		if found == nil {
			t.Fatalf("AMI %s not found after tagging", amiID)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive, got %v", found.Status())
		}

		// Test Delete (deregister)
		err = repo.Delete(ctx, amiID)
		requireNoError(t, err, "deleting AMI")

		// Verify deregistered
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after delete")

		if findResource(resources, amiID) != nil {
			t.Error("AMI should be deregistered")
		}
	})

	t.Run("NeverExpiresAMI", func(t *testing.T) {
		// Create volume for snapshot
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
		requireNoError(t, err, "creating volume for never-expires AMI")
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
		requireNoError(t, err, "creating snapshot for never-expires AMI")
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

		// Register AMI with never-expires tag
		amiName := fmt.Sprintf("cloud-janitor-test-never-%d", time.Now().UnixNano())
		amiOutput, err := ec2Client.RegisterImage(ctx, &ec2.RegisterImageInput{
			Name:               aws.String(amiName),
			Architecture:       types.ArchitectureValuesX8664,
			RootDeviceName:     aws.String("/dev/xvda"),
			VirtualizationType: aws.String("hvm"),
			BlockDeviceMappings: []types.BlockDeviceMapping{
				{
					DeviceName: aws.String("/dev/xvda"),
					Ebs: &types.EbsBlockDevice{
						SnapshotId:          aws.String(snapshotID),
						DeleteOnTermination: aws.Bool(true),
					},
				},
			},
		})
		requireNoError(t, err, "registering never-expires AMI")
		amiID := *amiOutput.ImageId

		// Tag with never-expires
		_, err = ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []string{amiID},
			Tags: toEC2Tags(mergeTags(testTags(), map[string]string{
				"expiration-date": "never",
			})),
		})
		requireNoError(t, err, "tagging never-expires AMI")

		globalCleanup.Register("AMI "+amiID, PriorityAMI, func(ctx context.Context) error {
			_, deregErr := ec2Client.DeregisterImage(ctx, &ec2.DeregisterImageInput{ImageId: aws.String(amiID)})
			return deregErr
		})

		// Wait for AMI
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{ImageIds: []string{amiID}})
			if descErr != nil {
				return false, descErr
			}
			if len(output.Images) == 0 {
				return false, nil
			}
			return output.Images[0].State == types.ImageStateAvailable, nil
		}, 5*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for AMI")

		repo := awsinfra.NewAMIRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing AMIs")

		found := findResource(resources, amiID)
		if found == nil {
			t.Fatalf("never-expires AMI %s not found", amiID)
		}
		if found.Status() != domain.StatusNeverExpires {
			t.Errorf("expected StatusNeverExpires, got %v", found.Status())
		}
	})
}
