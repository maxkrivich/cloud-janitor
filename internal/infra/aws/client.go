// Package aws provides AWS infrastructure adapters.
package aws

import (
	"context"
	"fmt"

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

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// ClientFactory creates AWS clients for different accounts and regions.
type ClientFactory struct {
	baseConfig aws.Config
}

// NewClientFactory creates a new ClientFactory with the default AWS config.
func NewClientFactory(ctx context.Context) (*ClientFactory, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	return &ClientFactory{
		baseConfig: cfg,
	}, nil
}

// NewClientFactoryWithConfig creates a new ClientFactory with a custom AWS config.
func NewClientFactoryWithConfig(cfg aws.Config) *ClientFactory {
	return &ClientFactory{
		baseConfig: cfg,
	}
}

// GetConfig returns the AWS config for the specified account and region.
// If the account has a role ARN, it assumes that role.
func (f *ClientFactory) GetConfig(_ context.Context, account domain.Account, region string) (aws.Config, error) {
	cfg := f.baseConfig.Copy()
	cfg.Region = region

	if account.RoleARN != "" {
		stsClient := sts.NewFromConfig(cfg)
		creds := stscreds.NewAssumeRoleProvider(stsClient, account.RoleARN)
		cfg.Credentials = aws.NewCredentialsCache(creds)
	}

	return cfg, nil
}

// EC2Client returns an EC2 client for the specified account and region.
func (f *ClientFactory) EC2Client(ctx context.Context, account domain.Account, region string) (*ec2.Client, error) {
	cfg, err := f.GetConfig(ctx, account, region)
	if err != nil {
		return nil, err
	}
	return ec2.NewFromConfig(cfg), nil
}

// RDSClient returns an RDS client for the specified account and region.
func (f *ClientFactory) RDSClient(ctx context.Context, account domain.Account, region string) (*rds.Client, error) {
	cfg, err := f.GetConfig(ctx, account, region)
	if err != nil {
		return nil, err
	}
	return rds.NewFromConfig(cfg), nil
}

// ELBv2Client returns an Elastic Load Balancing v2 client for the specified account and region.
// This client handles both Application Load Balancers (ALB) and Network Load Balancers (NLB).
func (f *ClientFactory) ELBv2Client(ctx context.Context, account domain.Account, region string) (*elasticloadbalancingv2.Client, error) {
	cfg, err := f.GetConfig(ctx, account, region)
	if err != nil {
		return nil, err
	}
	return elasticloadbalancingv2.NewFromConfig(cfg), nil
}

// ElastiCacheClient returns an ElastiCache client for the specified account and region.
func (f *ClientFactory) ElastiCacheClient(ctx context.Context, account domain.Account, region string) (*elasticache.Client, error) {
	cfg, err := f.GetConfig(ctx, account, region)
	if err != nil {
		return nil, err
	}
	return elasticache.NewFromConfig(cfg), nil
}

// OpenSearchClient returns an OpenSearch client for the specified account and region.
func (f *ClientFactory) OpenSearchClient(ctx context.Context, account domain.Account, region string) (*opensearch.Client, error) {
	cfg, err := f.GetConfig(ctx, account, region)
	if err != nil {
		return nil, err
	}
	return opensearch.NewFromConfig(cfg), nil
}

// EKSClient returns an EKS client for the specified account and region.
func (f *ClientFactory) EKSClient(ctx context.Context, account domain.Account, region string) (*eks.Client, error) {
	cfg, err := f.GetConfig(ctx, account, region)
	if err != nil {
		return nil, err
	}
	return eks.NewFromConfig(cfg), nil
}

// RedshiftClient returns a Redshift client for the specified account and region.
func (f *ClientFactory) RedshiftClient(ctx context.Context, account domain.Account, region string) (*redshift.Client, error) {
	cfg, err := f.GetConfig(ctx, account, region)
	if err != nil {
		return nil, err
	}
	return redshift.NewFromConfig(cfg), nil
}

// SageMakerClient returns a SageMaker client for the specified account and region.
func (f *ClientFactory) SageMakerClient(ctx context.Context, account domain.Account, region string) (*sagemaker.Client, error) {
	cfg, err := f.GetConfig(ctx, account, region)
	if err != nil {
		return nil, err
	}
	return sagemaker.NewFromConfig(cfg), nil
}

// CloudWatchLogsClient returns a CloudWatch Logs client for the specified account and region.
func (f *ClientFactory) CloudWatchLogsClient(ctx context.Context, account domain.Account, region string) (*cloudwatchlogs.Client, error) {
	cfg, err := f.GetConfig(ctx, account, region)
	if err != nil {
		return nil, err
	}
	return cloudwatchlogs.NewFromConfig(cfg), nil
}

// GetAccountID returns the current AWS account ID.
func (f *ClientFactory) GetAccountID(ctx context.Context) (string, error) {
	stsClient := sts.NewFromConfig(f.baseConfig)
	output, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("getting caller identity: %w", err)
	}
	return *output.Account, nil
}
