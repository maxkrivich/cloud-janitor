// Package aws provides AWS infrastructure adapters.
package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
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

// GetAccountID returns the current AWS account ID.
func (f *ClientFactory) GetAccountID(ctx context.Context) (string, error) {
	stsClient := sts.NewFromConfig(f.baseConfig)
	output, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("getting caller identity: %w", err)
	}
	return *output.Account, nil
}
