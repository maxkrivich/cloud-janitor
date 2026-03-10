//go:build integration

package integration

import (
	"os"
	"testing"
)

// TestConfig holds configuration for integration tests.
type TestConfig struct {
	AccountID        string
	Region           string
	RoleARN          string // Optional: IAM role to assume for cross-account access
	EKSRoleARN       string // Optional: IAM role for EKS clusters
	SageMakerRoleARN string // Optional: IAM role for SageMaker notebooks
}

// Global test configuration
var testConfig TestConfig

// loadTestConfig loads configuration from environment variables.
func loadTestConfig() {
	testConfig = TestConfig{
		AccountID:        os.Getenv("TEST_AWS_ACCOUNT_ID"),
		Region:           os.Getenv("TEST_AWS_REGION"),
		RoleARN:          os.Getenv("TEST_ROLE_ARN"),
		EKSRoleARN:       os.Getenv("TEST_EKS_ROLE_ARN"),
		SageMakerRoleARN: os.Getenv("TEST_SAGEMAKER_ROLE_ARN"),
	}

	if testConfig.Region == "" {
		testConfig.Region = "us-west-2"
	}
}

// skipIfMissingConfig skips the test if required config is missing.
func skipIfMissingConfig(t *testing.T) {
	t.Helper()
	if testConfig.AccountID == "" {
		t.Skip("Skipping: TEST_AWS_ACCOUNT_ID not set")
	}
}

// skipIfMissingEKSRole skips EKS tests if role is not configured.
func skipIfMissingEKSRole(t *testing.T) {
	t.Helper()
	skipIfMissingConfig(t)
	if testConfig.EKSRoleARN == "" {
		t.Skip("Skipping: TEST_EKS_ROLE_ARN not set")
	}
}

// skipIfMissingSageMakerRole skips SageMaker tests if role is not configured.
func skipIfMissingSageMakerRole(t *testing.T) {
	t.Helper()
	skipIfMissingConfig(t)
	if testConfig.SageMakerRoleARN == "" {
		t.Skip("Skipping: TEST_SAGEMAKER_ROLE_ARN not set")
	}
}
