//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
	awsinfra "github.com/maxkrivich/cloud-janitor/internal/infra/aws"
)

func TestELBRepository(t *testing.T) {
	skipIfMissingConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	elbClient := getELBv2Client(t)

	t.Run("ListTagDelete_ALB", func(t *testing.T) {
		// Create ALB
		createOutput, err := elbClient.CreateLoadBalancer(ctx, &elasticloadbalancingv2.CreateLoadBalancerInput{
			Name:    aws.String("cj-test-alb"),
			Type:    types.LoadBalancerTypeEnumApplication,
			Scheme:  types.LoadBalancerSchemeEnumInternal,
			Subnets: testInfra.PrivateSubnetIDs,
			Tags: []types.Tag{
				{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
				{Key: aws.String("Name"), Value: aws.String("cloud-janitor-test-alb")},
			},
		})
		requireNoError(t, err, "creating ALB")
		albARN := *createOutput.LoadBalancers[0].LoadBalancerArn

		globalCleanup.Register("ALB "+albARN, PriorityLoadBalancer, func(ctx context.Context) error {
			_, cleanupErr := elbClient.DeleteLoadBalancer(ctx, &elasticloadbalancingv2.DeleteLoadBalancerInput{
				LoadBalancerArn: aws.String(albARN),
			})
			return cleanupErr
		})

		// Wait for ALB to be active
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := elbClient.DescribeLoadBalancers(ctx, &elasticloadbalancingv2.DescribeLoadBalancersInput{
				LoadBalancerArns: []string{albARN},
			})
			if descErr != nil {
				return false, descErr
			}
			state := output.LoadBalancers[0].State.Code
			return state == types.LoadBalancerStateEnumActive, nil
		}, 10*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for ALB")

		repo := awsinfra.NewELBRepository(elbClient, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing load balancers")

		found := findResource(resources, albARN)
		if found == nil {
			t.Fatalf("ALB %s not found", albARN)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, albARN, expDate)
		requireNoError(t, err, "tagging ALB")

		// Verify tag was applied
		resources, err = repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing after tag")
		found = findResource(resources, albARN)
		if found == nil {
			t.Fatalf("ALB %s not found after tagging", albARN)
		}
		if found.Status() != domain.StatusActive {
			t.Errorf("expected StatusActive after tagging, got %v", found.Status())
		}

		// Test Delete
		err = repo.Delete(ctx, albARN)
		requireNoError(t, err, "deleting ALB")

		// Wait for deletion
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := elbClient.DescribeLoadBalancers(ctx, &elasticloadbalancingv2.DescribeLoadBalancersInput{
				LoadBalancerArns: []string{albARN},
			})
			if descErr != nil {
				// LoadBalancer not found = deleted
				return true, nil
			}
			return len(output.LoadBalancers) == 0, nil
		}, 10*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for ALB deletion")
	})

	t.Run("ListTagDelete_NLB", func(t *testing.T) {
		// Create NLB
		createOutput, err := elbClient.CreateLoadBalancer(ctx, &elasticloadbalancingv2.CreateLoadBalancerInput{
			Name:    aws.String("cj-test-nlb"),
			Type:    types.LoadBalancerTypeEnumNetwork,
			Scheme:  types.LoadBalancerSchemeEnumInternal,
			Subnets: testInfra.PrivateSubnetIDs,
			Tags: []types.Tag{
				{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
				{Key: aws.String("Name"), Value: aws.String("cloud-janitor-test-nlb")},
			},
		})
		requireNoError(t, err, "creating NLB")
		nlbARN := *createOutput.LoadBalancers[0].LoadBalancerArn

		globalCleanup.Register("NLB "+nlbARN, PriorityLoadBalancer, func(ctx context.Context) error {
			_, cleanupErr := elbClient.DeleteLoadBalancer(ctx, &elasticloadbalancingv2.DeleteLoadBalancerInput{
				LoadBalancerArn: aws.String(nlbARN),
			})
			return cleanupErr
		})

		// Wait for NLB to be active
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := elbClient.DescribeLoadBalancers(ctx, &elasticloadbalancingv2.DescribeLoadBalancersInput{
				LoadBalancerArns: []string{nlbARN},
			})
			if descErr != nil {
				return false, descErr
			}
			state := output.LoadBalancers[0].State.Code
			return state == types.LoadBalancerStateEnumActive, nil
		}, 10*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for NLB")

		repo := awsinfra.NewELBRepository(elbClient, testConfig.AccountID, testConfig.Region)

		// Test List
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing load balancers")

		found := findResource(resources, nlbARN)
		if found == nil {
			t.Fatalf("NLB %s not found", nlbARN)
		}
		if found.Status() != domain.StatusUntagged {
			t.Errorf("expected StatusUntagged, got %v", found.Status())
		}

		// Test Tag
		expDate := time.Now().AddDate(0, 0, 30)
		err = repo.Tag(ctx, nlbARN, expDate)
		requireNoError(t, err, "tagging NLB")

		// Test Delete
		err = repo.Delete(ctx, nlbARN)
		requireNoError(t, err, "deleting NLB")

		// Wait for deletion
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := elbClient.DescribeLoadBalancers(ctx, &elasticloadbalancingv2.DescribeLoadBalancersInput{
				LoadBalancerArns: []string{nlbARN},
			})
			if descErr != nil {
				// LoadBalancer not found = deleted
				return true, nil
			}
			return len(output.LoadBalancers) == 0, nil
		}, 10*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for NLB deletion")
	})

	t.Run("NeverExpires", func(t *testing.T) {
		// Create ALB with expiration-date=never tag
		createOutput, err := elbClient.CreateLoadBalancer(ctx, &elasticloadbalancingv2.CreateLoadBalancerInput{
			Name:    aws.String("cj-test-alb-never"),
			Type:    types.LoadBalancerTypeEnumApplication,
			Scheme:  types.LoadBalancerSchemeEnumInternal,
			Subnets: testInfra.PrivateSubnetIDs,
			Tags: []types.Tag{
				{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
				{Key: aws.String("Name"), Value: aws.String("cloud-janitor-test-alb-never")},
				{Key: aws.String("expiration-date"), Value: aws.String("never")},
			},
		})
		requireNoError(t, err, "creating ALB with never tag")
		albARN := *createOutput.LoadBalancers[0].LoadBalancerArn

		globalCleanup.Register("ALB "+albARN, PriorityLoadBalancer, func(ctx context.Context) error {
			_, cleanupErr := elbClient.DeleteLoadBalancer(ctx, &elasticloadbalancingv2.DeleteLoadBalancerInput{
				LoadBalancerArn: aws.String(albARN),
			})
			return cleanupErr
		})

		// Wait for ALB to be active
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := elbClient.DescribeLoadBalancers(ctx, &elasticloadbalancingv2.DescribeLoadBalancersInput{
				LoadBalancerArns: []string{albARN},
			})
			if descErr != nil {
				return false, descErr
			}
			state := output.LoadBalancers[0].State.Code
			return state == types.LoadBalancerStateEnumActive, nil
		}, 10*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for ALB")

		repo := awsinfra.NewELBRepository(elbClient, testConfig.AccountID, testConfig.Region)

		// Test List - should find with StatusNeverExpires
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing load balancers")

		found := findResource(resources, albARN)
		if found == nil {
			t.Fatalf("ALB %s not found", albARN)
		}
		if found.Status() != domain.StatusNeverExpires {
			t.Errorf("expected StatusNeverExpires, got %v", found.Status())
		}

		// Clean up - delete the ALB
		_, err = elbClient.DeleteLoadBalancer(ctx, &elasticloadbalancingv2.DeleteLoadBalancerInput{
			LoadBalancerArn: aws.String(albARN),
		})
		requireNoError(t, err, "deleting ALB")
	})

	t.Run("ExcludedResource", func(t *testing.T) {
		// Create ALB with DoNotDelete=true tag
		createOutput, err := elbClient.CreateLoadBalancer(ctx, &elasticloadbalancingv2.CreateLoadBalancerInput{
			Name:    aws.String("cj-test-alb-excl"),
			Type:    types.LoadBalancerTypeEnumApplication,
			Scheme:  types.LoadBalancerSchemeEnumInternal,
			Subnets: testInfra.PrivateSubnetIDs,
			Tags: []types.Tag{
				{Key: aws.String(testTagKey), Value: aws.String(testTagValue)},
				{Key: aws.String("Name"), Value: aws.String("cloud-janitor-test-alb-excluded")},
				{Key: aws.String("DoNotDelete"), Value: aws.String("true")},
			},
		})
		requireNoError(t, err, "creating ALB with DoNotDelete tag")
		albARN := *createOutput.LoadBalancers[0].LoadBalancerArn

		globalCleanup.Register("ALB "+albARN, PriorityLoadBalancer, func(ctx context.Context) error {
			_, cleanupErr := elbClient.DeleteLoadBalancer(ctx, &elasticloadbalancingv2.DeleteLoadBalancerInput{
				LoadBalancerArn: aws.String(albARN),
			})
			return cleanupErr
		})

		// Wait for ALB to be active
		err = waitFor(ctx, func() (bool, error) {
			output, descErr := elbClient.DescribeLoadBalancers(ctx, &elasticloadbalancingv2.DescribeLoadBalancersInput{
				LoadBalancerArns: []string{albARN},
			})
			if descErr != nil {
				return false, descErr
			}
			state := output.LoadBalancers[0].State.Code
			return state == types.LoadBalancerStateEnumActive, nil
		}, 10*time.Second, 5*time.Minute)
		requireNoError(t, err, "waiting for ALB")

		repo := awsinfra.NewELBRepository(elbClient, testConfig.AccountID, testConfig.Region)

		// Test List - resource should be found (exclusion filtering is done at app layer)
		resources, err := repo.List(ctx, testConfig.Region)
		requireNoError(t, err, "listing load balancers")

		found := findResource(resources, albARN)
		if found == nil {
			t.Fatalf("ALB %s not found", albARN)
		}

		// Verify the DoNotDelete tag is present
		if found.Tags["DoNotDelete"] != "true" {
			t.Errorf("expected DoNotDelete=true tag, got %v", found.Tags["DoNotDelete"])
		}

		// Clean up - delete the ALB
		_, err = elbClient.DeleteLoadBalancer(ctx, &elasticloadbalancingv2.DeleteLoadBalancerInput{
			LoadBalancerArn: aws.String(albARN),
		})
		requireNoError(t, err, "deleting ALB")
	})
}
