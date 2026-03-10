//go:build integration

package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

// CreateTestResourceResult contains the created resource ID and its cleanup function.
type CreateTestResourceResult struct {
	ID      string
	Cleanup func(ctx context.Context) error
}

// --- Elastic IP ---

// CreateTestEIP allocates an Elastic IP for testing.
// Returns the allocation ID and registers cleanup automatically.
func CreateTestEIP(ctx context.Context, cleanup *CleanupRegistry, extraTags map[string]string) (*CreateTestResourceResult, error) {
	tags := testTags()
	for k, v := range extraTags {
		tags[k] = v
	}

	output, err := clients.EC2.AllocateAddress(ctx, &ec2.AllocateAddressInput{
		Domain: types.DomainTypeVpc,
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeElasticIp,
				Tags:         toEC2Tags(tags),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("allocating EIP: %w", err)
	}

	eipID := *output.AllocationId
	cleanupFn := func(ctx context.Context) error {
		_, err := clients.EC2.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{
			AllocationId: aws.String(eipID),
		})
		return err
	}

	cleanup.Register("EIP "+eipID, PriorityElasticIP, cleanupFn)

	return &CreateTestResourceResult{ID: eipID, Cleanup: cleanupFn}, nil
}

// --- CloudWatch Log Group ---

// CreateTestLogGroup creates a CloudWatch Log Group for testing.
func CreateTestLogGroup(ctx context.Context, cleanup *CleanupRegistry, namePrefix string, extraTags map[string]string) (*CreateTestResourceResult, error) {
	tags := map[string]string{testTagKey: testTagValue}
	for k, v := range extraTags {
		tags[k] = v
	}

	logGroupName := fmt.Sprintf("%s/%d", namePrefix, time.Now().UnixNano())

	_, err := clients.Logs.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(logGroupName),
		Tags:         tags,
	})
	if err != nil {
		return nil, fmt.Errorf("creating log group: %w", err)
	}

	cleanupFn := func(ctx context.Context) error {
		_, err := clients.Logs.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{
			LogGroupName: aws.String(logGroupName),
		})
		return err
	}

	cleanup.Register("LogGroup "+logGroupName, PriorityLogGroup, cleanupFn)

	return &CreateTestResourceResult{ID: logGroupName, Cleanup: cleanupFn}, nil
}

// --- EBS Volume ---

// CreateTestEBSVolume creates an EBS volume for testing.
// Waits for the volume to be available before returning.
func CreateTestEBSVolume(ctx context.Context, cleanup *CleanupRegistry, az string, extraTags map[string]string) (*CreateTestResourceResult, error) {
	tags := testTags()
	for k, v := range extraTags {
		tags[k] = v
	}

	output, err := clients.EC2.CreateVolume(ctx, &ec2.CreateVolumeInput{
		AvailabilityZone: aws.String(az),
		Size:             aws.Int32(1), // 1 GB minimum
		VolumeType:       types.VolumeTypeGp3,
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeVolume,
				Tags:         toEC2Tags(tags),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating volume: %w", err)
	}

	volumeID := *output.VolumeId
	cleanupFn := func(ctx context.Context) error {
		_, delErr := clients.EC2.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
			VolumeId: aws.String(volumeID),
		})
		return delErr
	}

	cleanup.Register("Volume "+volumeID, PriorityEBS, cleanupFn)

	// Wait for volume to be available
	err = waitFor(ctx, func() (bool, error) {
		desc, descErr := clients.EC2.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
			VolumeIds: []string{volumeID},
		})
		if descErr != nil {
			return false, descErr
		}
		return desc.Volumes[0].State == types.VolumeStateAvailable, nil
	}, 2*time.Second, 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("waiting for volume %s: %w", volumeID, err)
	}

	return &CreateTestResourceResult{ID: volumeID, Cleanup: cleanupFn}, nil
}

// --- EBS Snapshot ---

// CreateTestSnapshot creates an EBS snapshot from a volume.
// Waits for the snapshot to complete before returning.
func CreateTestSnapshot(ctx context.Context, cleanup *CleanupRegistry, volumeID string, extraTags map[string]string) (*CreateTestResourceResult, error) {
	tags := testTags()
	for k, v := range extraTags {
		tags[k] = v
	}

	output, err := clients.EC2.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
		VolumeId: aws.String(volumeID),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeSnapshot,
				Tags:         toEC2Tags(tags),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating snapshot: %w", err)
	}

	snapshotID := *output.SnapshotId
	cleanupFn := func(ctx context.Context) error {
		_, delErr := clients.EC2.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
			SnapshotId: aws.String(snapshotID),
		})
		return delErr
	}

	cleanup.Register("Snapshot "+snapshotID, PrioritySnapshot, cleanupFn)

	// Wait for snapshot to complete
	err = waitFor(ctx, func() (bool, error) {
		desc, descErr := clients.EC2.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{
			SnapshotIds: []string{snapshotID},
		})
		if descErr != nil {
			return false, descErr
		}
		return desc.Snapshots[0].State == types.SnapshotStateCompleted, nil
	}, 5*time.Second, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("waiting for snapshot %s: %w", snapshotID, err)
	}

	return &CreateTestResourceResult{ID: snapshotID, Cleanup: cleanupFn}, nil
}

// --- EC2 Instance ---

// CreateTestEC2Instance launches an EC2 instance for testing.
// Waits for the instance to be running before returning.
func CreateTestEC2Instance(ctx context.Context, cleanup *CleanupRegistry, subnetID string, extraTags map[string]string) (*CreateTestResourceResult, error) {
	tags := testTags()
	for k, v := range extraTags {
		tags[k] = v
	}

	// Find Amazon Linux 2 AMI
	amiOutput, err := clients.EC2.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Owners: []string{"amazon"},
		Filters: []types.Filter{
			{Name: aws.String("name"), Values: []string{"amzn2-ami-hvm-*-x86_64-gp2"}},
			{Name: aws.String("state"), Values: []string{"available"}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("finding AMI: %w", err)
	}
	if len(amiOutput.Images) == 0 {
		return nil, fmt.Errorf("no suitable AMI found")
	}
	amiID := *amiOutput.Images[0].ImageId

	output, err := clients.EC2.RunInstances(ctx, &ec2.RunInstancesInput{
		ImageId:      aws.String(amiID),
		InstanceType: types.InstanceTypeT3Micro,
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
		SubnetId:     aws.String(subnetID),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeInstance,
				Tags:         toEC2Tags(tags),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("launching instance: %w", err)
	}

	instanceID := *output.Instances[0].InstanceId
	cleanupFn := func(ctx context.Context) error {
		_, termErr := clients.EC2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
			InstanceIds: []string{instanceID},
		})
		return termErr
	}

	cleanup.Register("EC2 "+instanceID, PriorityEC2, cleanupFn)

	// Wait for instance to be running
	err = waitFor(ctx, func() (bool, error) {
		desc, descErr := clients.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: []string{instanceID},
		})
		if descErr != nil {
			return false, descErr
		}
		state := desc.Reservations[0].Instances[0].State.Name
		return state == types.InstanceStateNameRunning, nil
	}, 5*time.Second, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("waiting for instance %s: %w", instanceID, err)
	}

	return &CreateTestResourceResult{ID: instanceID, Cleanup: cleanupFn}, nil
}

// --- NAT Gateway ---

// CreateTestNATGateway creates a NAT Gateway for testing.
// Also creates the required EIP. Waits for NAT Gateway to be available.
func CreateTestNATGateway(ctx context.Context, cleanup *CleanupRegistry, subnetID string, extraTags map[string]string) (*CreateTestResourceResult, error) {
	// First create EIP for NAT Gateway
	eipResult, err := CreateTestEIP(ctx, cleanup, nil)
	if err != nil {
		return nil, fmt.Errorf("creating EIP for NAT: %w", err)
	}

	tags := testTags()
	for k, v := range extraTags {
		tags[k] = v
	}

	output, err := clients.EC2.CreateNatGateway(ctx, &ec2.CreateNatGatewayInput{
		AllocationId: aws.String(eipResult.ID),
		SubnetId:     aws.String(subnetID),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeNatgateway,
				Tags:         toEC2Tags(tags),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating NAT Gateway: %w", err)
	}

	natID := *output.NatGateway.NatGatewayId
	cleanupFn := func(ctx context.Context) error {
		_, delErr := clients.EC2.DeleteNatGateway(ctx, &ec2.DeleteNatGatewayInput{
			NatGatewayId: aws.String(natID),
		})
		if delErr != nil {
			return delErr
		}
		// Wait for deletion before releasing EIP
		return waitFor(ctx, func() (bool, error) {
			desc, descErr := clients.EC2.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{
				NatGatewayIds: []string{natID},
			})
			if descErr != nil {
				return false, descErr
			}
			if len(desc.NatGateways) == 0 {
				return true, nil
			}
			return desc.NatGateways[0].State == types.NatGatewayStateDeleted, nil
		}, 5*time.Second, 5*time.Minute)
	}

	cleanup.Register("NAT "+natID, PriorityNATGateway, cleanupFn)

	// Wait for NAT Gateway to be available
	err = waitFor(ctx, func() (bool, error) {
		desc, descErr := clients.EC2.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{
			NatGatewayIds: []string{natID},
		})
		if descErr != nil {
			return false, descErr
		}
		return desc.NatGateways[0].State == types.NatGatewayStateAvailable, nil
	}, 10*time.Second, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("waiting for NAT Gateway %s: %w", natID, err)
	}

	return &CreateTestResourceResult{ID: natID, Cleanup: cleanupFn}, nil
}

// --- Application Load Balancer ---

// CreateTestALB creates an Application Load Balancer for testing.
// Waits for ALB to be active before returning.
func CreateTestALB(ctx context.Context, cleanup *CleanupRegistry, subnetIDs []string, extraTags map[string]string) (*CreateTestResourceResult, error) {
	tags := []elbv2types.Tag{
		{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
	}
	for k, v := range extraTags {
		tags = append(tags, elbv2types.Tag{Key: aws.String(k), Value: aws.String(v)})
	}

	albName := fmt.Sprintf("cj-test-%d", time.Now().Unix()%100000)

	output, err := clients.ELBv2.CreateLoadBalancer(ctx, &elbv2.CreateLoadBalancerInput{
		Name:    aws.String(albName),
		Type:    elbv2types.LoadBalancerTypeEnumApplication,
		Scheme:  elbv2types.LoadBalancerSchemeEnumInternal,
		Subnets: subnetIDs,
		Tags:    tags,
	})
	if err != nil {
		return nil, fmt.Errorf("creating ALB: %w", err)
	}

	albARN := *output.LoadBalancers[0].LoadBalancerArn
	cleanupFn := func(ctx context.Context) error {
		_, delErr := clients.ELBv2.DeleteLoadBalancer(ctx, &elbv2.DeleteLoadBalancerInput{
			LoadBalancerArn: aws.String(albARN),
		})
		return delErr
	}

	cleanup.Register("ALB "+albARN, PriorityLoadBalancer, cleanupFn)

	// Wait for ALB to be active
	err = waitFor(ctx, func() (bool, error) {
		desc, descErr := clients.ELBv2.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{
			LoadBalancerArns: []string{albARN},
		})
		if descErr != nil {
			return false, descErr
		}
		return desc.LoadBalancers[0].State.Code == elbv2types.LoadBalancerStateEnumActive, nil
	}, 10*time.Second, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("waiting for ALB %s: %w", albARN, err)
	}

	return &CreateTestResourceResult{ID: albARN, Cleanup: cleanupFn}, nil
}
