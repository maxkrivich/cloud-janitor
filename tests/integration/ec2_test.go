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

func TestEC2Repository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	ec2Client := getEC2Client(t)

	// Find a suitable AMI (Amazon Linux 2)
	amiOutput, err := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Owners: []string{"amazon"},
		Filters: []types.Filter{
			{Name: aws.String("name"), Values: []string{"amzn2-ami-hvm-*-x86_64-gp2"}},
			{Name: aws.String("state"), Values: []string{"available"}},
		},
	})
	requireNoError(t, err, "finding AMI")
	if len(amiOutput.Images) == 0 {
		t.Fatal("no suitable AMI found")
	}
	amiID := *amiOutput.Images[0].ImageId

	t.Run("ListTagDelete", func(t *testing.T) {
		// Launch instance
		runOutput, err := ec2Client.RunInstances(ctx, &ec2.RunInstancesInput{
			ImageId:      aws.String(amiID),
			InstanceType: types.InstanceTypeT3Micro,
			MinCount:     aws.Int32(1),
			MaxCount:     aws.Int32(1),
			SubnetId:     aws.String(testInfra.PublicSubnetIDs[0]),
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeInstance,
					Tags:         toEC2Tags(testTags()),
				},
			},
		})
		requireNoError(t, err, "launching instance")
		instanceID := *runOutput.Instances[0].InstanceId

		globalCleanup.Register("EC2 "+instanceID, PriorityEC2, func(ctx context.Context) error {
			_, cleanupErr := ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
				InstanceIds: []string{instanceID},
			})
			return cleanupErr
		})

		// Wait for instance to be running
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
				InstanceIds: []string{instanceID},
			})
			if descErr != nil {
				return false, descErr
			}
			state := output.Reservations[0].Instances[0].State.Name
			return state == types.InstanceStateNameRunning, nil
		}, 5*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for instance")

		repo := awsinfra.NewEC2Repository(ec2Client, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing instances")

		found := findResource(resources, instanceID)
		if found == nil {
			t.Fatalf("instance %s not found", instanceID)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, instanceID, expDate)
		requireNoError(t, err, "tagging instance")

		// Verify tag was applied
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after tag")
		found = findResource(resources, instanceID)
		if found == nil {
			t.Fatalf("instance %s not found after tagging", instanceID)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive after tagging, got %v", found.Status())
		}

		// Test Delete (terminate)
		err = repo.Delete(ctx, instanceID)
		requireNoError(t, err, "terminating instance")

		// Wait for termination
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
				InstanceIds: []string{instanceID},
			})
			if descErr != nil {
				return false, descErr
			}
			state := output.Reservations[0].Instances[0].State.Name
			return state == types.InstanceStateNameTerminated, nil
		}, 5*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for termination")
	})

	t.Run("NeverExpires", func(t *testing.T) {
		// Launch instance with expiration-date=never tag
		tags := testTags()
		tags["expiration-date"] = "never"

		runOutput, err := ec2Client.RunInstances(ctx, &ec2.RunInstancesInput{
			ImageId:      aws.String(amiID),
			InstanceType: types.InstanceTypeT3Micro,
			MinCount:     aws.Int32(1),
			MaxCount:     aws.Int32(1),
			SubnetId:     aws.String(testInfra.PublicSubnetIDs[0]),
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeInstance,
					Tags:         toEC2Tags(tags),
				},
			},
		})
		requireNoError(t, err, "launching instance with never tag")
		instanceID := *runOutput.Instances[0].InstanceId

		globalCleanup.Register("EC2 "+instanceID, PriorityEC2, func(ctx context.Context) error {
			_, cleanupErr := ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
				InstanceIds: []string{instanceID},
			})
			return cleanupErr
		})

		// Wait for instance to be running
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
				InstanceIds: []string{instanceID},
			})
			if descErr != nil {
				return false, descErr
			}
			state := output.Reservations[0].Instances[0].State.Name
			return state == types.InstanceStateNameRunning, nil
		}, 5*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for instance")

		repo := awsinfra.NewEC2Repository(ec2Client, testConfig.AccountID, testConfig.Region)

		// Test List - should find with StatusNeverExpires
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing instances")

		found := findResource(resources, instanceID)
		if found == nil {
			t.Fatalf("instance %s not found", instanceID)
		}
		if found.Status() != domain.StatusNeverExpires {
			t.Errorf("expected StatusNeverExpires, got %v", found.Status())
		}

		// Clean up - terminate the instance
		_, err = ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
			InstanceIds: []string{instanceID},
		})
		requireNoError(t, err, "terminating instance")
	})

	t.Run("ExcludedResource", func(t *testing.T) {
		// Launch instance with DoNotDelete=true tag
		tags := testTags()
		tags["DoNotDelete"] = "true"

		runOutput, err := ec2Client.RunInstances(ctx, &ec2.RunInstancesInput{
			ImageId:      aws.String(amiID),
			InstanceType: types.InstanceTypeT3Micro,
			MinCount:     aws.Int32(1),
			MaxCount:     aws.Int32(1),
			SubnetId:     aws.String(testInfra.PublicSubnetIDs[0]),
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeInstance,
					Tags:         toEC2Tags(tags),
				},
			},
		})
		requireNoError(t, err, "launching instance with DoNotDelete tag")
		instanceID := *runOutput.Instances[0].InstanceId

		globalCleanup.Register("EC2 "+instanceID, PriorityEC2, func(ctx context.Context) error {
			_, cleanupErr := ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
				InstanceIds: []string{instanceID},
			})
			return cleanupErr
		})

		// Wait for instance to be running
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
				InstanceIds: []string{instanceID},
			})
			if descErr != nil {
				return false, descErr
			}
			state := output.Reservations[0].Instances[0].State.Name
			return state == types.InstanceStateNameRunning, nil
		}, 5*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for instance")

		repo := awsinfra.NewEC2Repository(ec2Client, testConfig.AccountID, testConfig.Region)

		// Test List - resource should be found (exclusion filtering is done at app layer)
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing instances")

		found := findResource(resources, instanceID)
		if found == nil {
			t.Fatalf("instance %s not found", instanceID)
		}

		// Verify the DoNotDelete tag is present
		if found.Tags["DoNotDelete"] != "true" {
			t.Errorf("expected DoNotDelete=true tag, got %v", found.Tags["DoNotDelete"])
		}

		// Clean up - terminate the instance
		_, err = ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
			InstanceIds: []string{instanceID},
		})
		requireNoError(t, err, "terminating instance")
	})
}
