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

func TestNATGatewayRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	ec2Client := getEC2Client(t)

	t.Run("ListTagDelete", func(t *testing.T) {
		// First allocate an EIP for NAT Gateway
		allocOutput, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
			Domain: types.DomainTypeVpc,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeElasticIp,
					Tags:         toEC2Tags(testTags()),
				},
			},
		})
		requireNoError(t, err, "allocating EIP for NAT")
		eipAllocID := *allocOutput.AllocationId

		globalCleanup.Register("EIP "+eipAllocID, PriorityElasticIP, func(ctx context.Context) error {
			_, cleanupErr := ec2Client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(eipAllocID)})
			return cleanupErr
		})

		// Create NAT Gateway
		natOutput, err := ec2Client.CreateNatGateway(ctx, &ec2.CreateNatGatewayInput{
			AllocationId: aws.String(eipAllocID),
			SubnetId:     aws.String(testInfra.PublicSubnetIDs[0]),
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeNatgateway,
					Tags:         toEC2Tags(testTags()),
				},
			},
		})
		requireNoError(t, err, "creating NAT Gateway")
		natID := *natOutput.NatGateway.NatGatewayId

		globalCleanup.Register("NAT "+natID, PriorityNATGateway, func(ctx context.Context) error {
			_, delErr := ec2Client.DeleteNatGateway(ctx, &ec2.DeleteNatGatewayInput{NatGatewayId: aws.String(natID)})
			if delErr != nil {
				return delErr
			}
			// Wait for deletion
			return waitFor(ctx, func() (bool, error) {
				output, descErr := ec2Client.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{
					NatGatewayIds: []string{natID},
				})
				if descErr != nil {
					return false, descErr
				}
				if len(output.NatGateways) == 0 {
					return true, nil
				}
				state := output.NatGateways[0].State
				return state == types.NatGatewayStateDeleted, nil
			}, 5*time.Second, 5*time.Minute)
		})

		// Wait for NAT Gateway to be available
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := ec2Client.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{
				NatGatewayIds: []string{natID},
			})
			if descErr != nil {
				return false, descErr
			}
			state := output.NatGateways[0].State
			return state == types.NatGatewayStateAvailable, nil
		}, 10*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for NAT Gateway")

		repo := awsinfra.NewNATGatewayRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing NAT Gateways")

		found := findResource(resources, natID)
		if found == nil {
			t.Fatalf("NAT Gateway %s not found", natID)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, natID, expDate)
		requireNoError(t, err, "tagging NAT Gateway")

		// Verify tag was applied
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after tag")
		found = findResource(resources, natID)
		if found == nil {
			t.Fatalf("NAT Gateway %s not found after tagging", natID)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive after tagging, got %v", found.Status())
		}

		// Test Delete
		err = repo.Delete(ctx, natID)
		requireNoError(t, err, "deleting NAT Gateway")

		// Wait for deletion
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := ec2Client.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{
				NatGatewayIds: []string{natID},
			})
			if descErr != nil {
				return false, descErr
			}
			state := output.NatGateways[0].State
			return state == types.NatGatewayStateDeleted, nil
		}, 10*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for NAT deletion")
	})

	t.Run("NeverExpires", func(t *testing.T) {
		// Allocate EIP for NAT Gateway
		tags := testTags()
		tags["expiration-date"] = "never"

		allocOutput, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
			Domain: types.DomainTypeVpc,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeElasticIp,
					Tags:         toEC2Tags(tags),
				},
			},
		})
		requireNoError(t, err, "allocating EIP for NAT")
		eipAllocID := *allocOutput.AllocationId

		globalCleanup.Register("EIP "+eipAllocID, PriorityElasticIP, func(ctx context.Context) error {
			_, cleanupErr := ec2Client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(eipAllocID)})
			return cleanupErr
		})

		// Create NAT Gateway with never expires tag
		natOutput, err := ec2Client.CreateNatGateway(ctx, &ec2.CreateNatGatewayInput{
			AllocationId: aws.String(eipAllocID),
			SubnetId:     aws.String(testInfra.PublicSubnetIDs[0]),
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeNatgateway,
					Tags:         toEC2Tags(tags),
				},
			},
		})
		requireNoError(t, err, "creating NAT Gateway with never tag")
		natID := *natOutput.NatGateway.NatGatewayId

		globalCleanup.Register("NAT "+natID, PriorityNATGateway, func(ctx context.Context) error {
			_, delErr := ec2Client.DeleteNatGateway(ctx, &ec2.DeleteNatGatewayInput{NatGatewayId: aws.String(natID)})
			if delErr != nil {
				return delErr
			}
			return waitFor(ctx, func() (bool, error) {
				output, descErr := ec2Client.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{
					NatGatewayIds: []string{natID},
				})
				if descErr != nil {
					return false, descErr
				}
				if len(output.NatGateways) == 0 {
					return true, nil
				}
				state := output.NatGateways[0].State
				return state == types.NatGatewayStateDeleted, nil
			}, 5*time.Second, 5*time.Minute)
		})

		// Wait for NAT Gateway to be available
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := ec2Client.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{
				NatGatewayIds: []string{natID},
			})
			if descErr != nil {
				return false, descErr
			}
			state := output.NatGateways[0].State
			return state == types.NatGatewayStateAvailable, nil
		}, 10*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for NAT Gateway")

		repo := awsinfra.NewNATGatewayRepository(ec2Client, testConfig.AccountID, testConfig.Region)

		// Test List - should find with StatusNeverExpires
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing NAT Gateways")

		found := findResource(resources, natID)
		if found == nil {
			t.Fatalf("NAT Gateway %s not found", natID)
		}
		if found.Status() != domain.StatusNeverExpires {
			t.Errorf("expected StatusNeverExpires, got %v", found.Status())
		}

		// Clean up - delete the NAT Gateway
		_, err = ec2Client.DeleteNatGateway(ctx, &ec2.DeleteNatGatewayInput{
			NatGatewayId: aws.String(natID),
		})
		requireNoError(t, err, "deleting NAT Gateway")

		// Wait for deletion before releasing EIP
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := ec2Client.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{
				NatGatewayIds: []string{natID},
			})
			if descErr != nil {
				return false, descErr
			}
			state := output.NatGateways[0].State
			return state == types.NatGatewayStateDeleted, nil
		}, 10*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for NAT deletion in cleanup")
	})
}
