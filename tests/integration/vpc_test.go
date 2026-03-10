//go:build integration

package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// TestInfrastructure holds VPC and networking resources for tests.
type TestInfrastructure struct {
	VPCID              string
	PublicSubnetIDs    []string // 2 subnets in different AZs (for ELB, NAT)
	PrivateSubnetIDs   []string // 2 subnets in different AZs (for RDS, ElastiCache)
	InternetGatewayID  string
	PublicRouteTableID string
	AvailabilityZones  []string
}

// Global test infrastructure
var testInfra *TestInfrastructure

// setupTestInfrastructure creates VPC, subnets, and networking for tests.
func setupTestInfrastructure(ctx context.Context, cleanup *CleanupRegistry) (*TestInfrastructure, error) {
	ec2Client := clients.EC2
	infra := &TestInfrastructure{}

	// Get availability zones
	azOutput, err := ec2Client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		Filters: []types.Filter{
			{Name: aws.String("state"), Values: []string{"available"}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("describing AZs: %w", err)
	}
	if len(azOutput.AvailabilityZones) < 2 {
		return nil, fmt.Errorf("need at least 2 AZs, found %d", len(azOutput.AvailabilityZones))
	}
	infra.AvailabilityZones = []string{
		*azOutput.AvailabilityZones[0].ZoneName,
		*azOutput.AvailabilityZones[1].ZoneName,
	}

	// Create VPC
	vpcOutput, err := ec2Client.CreateVpc(ctx, &ec2.CreateVpcInput{
		CidrBlock: aws.String("10.0.0.0/16"),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeVpc,
				Tags:         toEC2Tags(testTags()),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating VPC: %w", err)
	}
	infra.VPCID = *vpcOutput.Vpc.VpcId
	cleanup.Register("VPC "+infra.VPCID, PriorityVPC, func(ctx context.Context) error {
		_, delErr := ec2Client.DeleteVpc(ctx, &ec2.DeleteVpcInput{VpcId: aws.String(infra.VPCID)})
		return delErr
	})

	// Enable DNS hostnames (optional - some tests may work without it)
	// This requires ec2:ModifyVpcAttribute permission
	_, err = ec2Client.ModifyVpcAttribute(ctx, &ec2.ModifyVpcAttributeInput{
		VpcId:              aws.String(infra.VPCID),
		EnableDnsHostnames: &types.AttributeBooleanValue{Value: aws.Bool(true)},
	})
	if err != nil {
		// Log warning but continue - DNS hostnames are only required for some services (RDS, etc.)
		fmt.Printf("  Warning: could not enable DNS hostnames (ec2:ModifyVpcAttribute): %v\n", err)
	}

	// Create Internet Gateway
	igwOutput, err := ec2Client.CreateInternetGateway(ctx, &ec2.CreateInternetGatewayInput{
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeInternetGateway,
				Tags:         toEC2Tags(testTags()),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating IGW: %w", err)
	}
	infra.InternetGatewayID = *igwOutput.InternetGateway.InternetGatewayId
	cleanup.Register("IGW "+infra.InternetGatewayID, PriorityIGW, func(ctx context.Context) error {
		// Detach first (ignore error as it may already be detached)
		if _, detachErr := ec2Client.DetachInternetGateway(ctx, &ec2.DetachInternetGatewayInput{
			InternetGatewayId: aws.String(infra.InternetGatewayID),
			VpcId:             aws.String(infra.VPCID),
		}); detachErr != nil {
			// Log but continue - IGW may already be detached
			fmt.Printf("    Warning: detach IGW: %v\n", detachErr)
		}
		_, delErr := ec2Client.DeleteInternetGateway(ctx, &ec2.DeleteInternetGatewayInput{
			InternetGatewayId: aws.String(infra.InternetGatewayID),
		})
		return delErr
	})

	// Attach IGW to VPC
	_, err = ec2Client.AttachInternetGateway(ctx, &ec2.AttachInternetGatewayInput{
		InternetGatewayId: aws.String(infra.InternetGatewayID),
		VpcId:             aws.String(infra.VPCID),
	})
	if err != nil {
		return nil, fmt.Errorf("attaching IGW: %w", err)
	}

	// Create public subnets (2 in different AZs)
	publicCIDRs := []string{"10.0.1.0/24", "10.0.2.0/24"}
	for i, cidr := range publicCIDRs {
		subnetOutput, subnetErr := ec2Client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
			VpcId:            aws.String(infra.VPCID),
			CidrBlock:        aws.String(cidr),
			AvailabilityZone: aws.String(infra.AvailabilityZones[i]),
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeSubnet,
					Tags:         toEC2Tags(mergeTags(testTags(), map[string]string{"Type": "public"})),
				},
			},
		})
		if subnetErr != nil {
			return nil, fmt.Errorf("creating public subnet %d: %w", i, subnetErr)
		}
		subnetID := *subnetOutput.Subnet.SubnetId
		infra.PublicSubnetIDs = append(infra.PublicSubnetIDs, subnetID)
		cleanup.Register("Subnet "+subnetID, PrioritySubnet, func(ctx context.Context) error {
			_, delErr := ec2Client.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{SubnetId: aws.String(subnetID)})
			return delErr
		})
	}

	// Create private subnets (2 in different AZs)
	privateCIDRs := []string{"10.0.3.0/24", "10.0.4.0/24"}
	for i, cidr := range privateCIDRs {
		subnetOutput, subnetErr := ec2Client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
			VpcId:            aws.String(infra.VPCID),
			CidrBlock:        aws.String(cidr),
			AvailabilityZone: aws.String(infra.AvailabilityZones[i]),
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeSubnet,
					Tags:         toEC2Tags(mergeTags(testTags(), map[string]string{"Type": "private"})),
				},
			},
		})
		if subnetErr != nil {
			return nil, fmt.Errorf("creating private subnet %d: %w", i, subnetErr)
		}
		subnetID := *subnetOutput.Subnet.SubnetId
		infra.PrivateSubnetIDs = append(infra.PrivateSubnetIDs, subnetID)
		cleanup.Register("Subnet "+subnetID, PrioritySubnet, func(ctx context.Context) error {
			_, delErr := ec2Client.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{SubnetId: aws.String(subnetID)})
			return delErr
		})
	}

	// Create route table for public subnets
	rtOutput, err := ec2Client.CreateRouteTable(ctx, &ec2.CreateRouteTableInput{
		VpcId: aws.String(infra.VPCID),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeRouteTable,
				Tags:         toEC2Tags(testTags()),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating route table: %w", err)
	}
	infra.PublicRouteTableID = *rtOutput.RouteTable.RouteTableId
	cleanup.Register("RouteTable "+infra.PublicRouteTableID, PriorityRouteTable, func(ctx context.Context) error {
		_, delErr := ec2Client.DeleteRouteTable(ctx, &ec2.DeleteRouteTableInput{
			RouteTableId: aws.String(infra.PublicRouteTableID),
		})
		return delErr
	})

	// Add route to IGW
	_, err = ec2Client.CreateRoute(ctx, &ec2.CreateRouteInput{
		RouteTableId:         aws.String(infra.PublicRouteTableID),
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		GatewayId:            aws.String(infra.InternetGatewayID),
	})
	if err != nil {
		return nil, fmt.Errorf("creating route to IGW: %w", err)
	}

	// Associate public subnets with route table
	for _, subnetID := range infra.PublicSubnetIDs {
		_, err = ec2Client.AssociateRouteTable(ctx, &ec2.AssociateRouteTableInput{
			RouteTableId: aws.String(infra.PublicRouteTableID),
			SubnetId:     aws.String(subnetID),
		})
		if err != nil {
			return nil, fmt.Errorf("associating subnet %s with route table: %w", subnetID, err)
		}
	}

	// Wait for subnets to be available
	err = waitFor(ctx, func() (bool, error) {
		descOutput, descErr := ec2Client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
			SubnetIds: append(infra.PublicSubnetIDs, infra.PrivateSubnetIDs...),
		})
		if descErr != nil {
			return false, descErr
		}
		for _, subnet := range descOutput.Subnets {
			if subnet.State != types.SubnetStateAvailable {
				return false, nil
			}
		}
		return true, nil
	}, 2*time.Second, 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("waiting for subnets: %w", err)
	}

	return infra, nil
}
