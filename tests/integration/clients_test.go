//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// testClients holds all AWS clients for integration tests.
type testClients struct {
	EC2         *ec2.Client
	RDS         *rds.Client
	ELBv2       *elasticloadbalancingv2.Client
	ElastiCache *elasticache.Client
	OpenSearch  *opensearch.Client
	EKS         *eks.Client
	Redshift    *redshift.Client
	SageMaker   *sagemaker.Client
	Logs        *cloudwatchlogs.Client
	STS         *sts.Client
}

// Global clients instance
var clients *testClients

// initClients initializes all AWS clients.
// If TEST_ROLE_ARN is set, it assumes that role before creating clients.
func initClients(ctx context.Context) error {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(testConfig.Region))
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	// If a role ARN is configured, assume that role
	if testConfig.RoleARN != "" {
		stsClient := sts.NewFromConfig(cfg)
		creds := stscreds.NewAssumeRoleProvider(stsClient, testConfig.RoleARN)
		cfg.Credentials = aws.NewCredentialsCache(creds)
	}

	clients = &testClients{
		EC2:         ec2.NewFromConfig(cfg),
		RDS:         rds.NewFromConfig(cfg),
		ELBv2:       elasticloadbalancingv2.NewFromConfig(cfg),
		ElastiCache: elasticache.NewFromConfig(cfg),
		OpenSearch:  opensearch.NewFromConfig(cfg),
		EKS:         eks.NewFromConfig(cfg),
		Redshift:    redshift.NewFromConfig(cfg),
		SageMaker:   sagemaker.NewFromConfig(cfg),
		Logs:        cloudwatchlogs.NewFromConfig(cfg),
		STS:         sts.NewFromConfig(cfg),
	}

	return nil
}

// getEC2Client returns the EC2 client, initializing if needed.
func getEC2Client(t *testing.T) *ec2.Client {
	t.Helper()
	if clients == nil || clients.EC2 == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.EC2
}

// getRDSClient returns the RDS client.
func getRDSClient(t *testing.T) *rds.Client {
	t.Helper()
	if clients == nil || clients.RDS == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.RDS
}

// getELBv2Client returns the ELBv2 client.
func getELBv2Client(t *testing.T) *elasticloadbalancingv2.Client {
	t.Helper()
	if clients == nil || clients.ELBv2 == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.ELBv2
}

// getElastiCacheClient returns the ElastiCache client.
func getElastiCacheClient(t *testing.T) *elasticache.Client {
	t.Helper()
	if clients == nil || clients.ElastiCache == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.ElastiCache
}

// getOpenSearchClient returns the OpenSearch client.
func getOpenSearchClient(t *testing.T) *opensearch.Client {
	t.Helper()
	if clients == nil || clients.OpenSearch == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.OpenSearch
}

// getEKSClient returns the EKS client.
func getEKSClient(t *testing.T) *eks.Client {
	t.Helper()
	if clients == nil || clients.EKS == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.EKS
}

// getRedshiftClient returns the Redshift client.
func getRedshiftClient(t *testing.T) *redshift.Client {
	t.Helper()
	if clients == nil || clients.Redshift == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.Redshift
}

// getSageMakerClient returns the SageMaker client.
func getSageMakerClient(t *testing.T) *sagemaker.Client {
	t.Helper()
	if clients == nil || clients.SageMaker == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.SageMaker
}

// getLogsClient returns the CloudWatch Logs client.
func getLogsClient(t *testing.T) *cloudwatchlogs.Client {
	t.Helper()
	if clients == nil || clients.Logs == nil {
		t.Fatal("AWS clients not initialized")
	}
	return clients.Logs
}
