//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// Global cleanup registry
var globalCleanup *CleanupRegistry

func TestMain(m *testing.M) {
	// Initialize cleanup registry FIRST (before any resources are created)
	globalCleanup = NewCleanupRegistry()

	// Setup context with timeout for entire test suite
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)

	// Track exit code
	code := 1

	// ALWAYS run cleanup, even on panic
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("\nPANIC during tests: %v\n", r)
		}

		// Run cleanup with extended timeout
		fmt.Printf("\n=== Running cleanup (%d resources) ===\n", globalCleanup.Count())
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cleanupCancel()

		if errs := globalCleanup.RunAll(cleanupCtx); len(errs) > 0 {
			fmt.Println("\nCleanup errors:")
			for _, err := range errs {
				fmt.Printf("  - %v\n", err)
			}
		} else if globalCleanup.Count() > 0 {
			fmt.Println("Cleanup completed successfully")
		}

		cancel()
		os.Exit(code)
	}()

	// Load configuration from environment
	loadTestConfig()

	// Skip if no account configured
	if testConfig.AccountID == "" {
		fmt.Println("Skipping integration tests: TEST_AWS_ACCOUNT_ID not set")
		fmt.Println("Usage: TEST_AWS_ACCOUNT_ID=123456789012 make test-integration")
		code = 0
		return
	}

	fmt.Printf("=== Integration Test Configuration ===\n")
	fmt.Printf("  Account ID: %s\n", testConfig.AccountID)
	fmt.Printf("  Region: %s\n", testConfig.Region)
	if testConfig.EKSRoleARN != "" {
		fmt.Printf("  EKS Role: %s\n", testConfig.EKSRoleARN)
	}
	if testConfig.SageMakerRoleARN != "" {
		fmt.Printf("  SageMaker Role: %s\n", testConfig.SageMakerRoleARN)
	}
	fmt.Println()

	// Initialize AWS clients
	fmt.Println("Initializing AWS clients...")
	if err := initClients(ctx); err != nil {
		fmt.Printf("Failed to initialize AWS clients: %v\n", err)
		return
	}

	// Verify AWS credentials
	output, err := clients.STS.GetCallerIdentity(ctx, nil)
	if err != nil {
		fmt.Printf("Failed to verify AWS credentials: %v\n", err)
		return
	}
	fmt.Printf("Authenticated as: %s\n", *output.Arn)

	// Verify account ID matches
	if *output.Account != testConfig.AccountID {
		fmt.Printf("Account mismatch: expected %s, got %s\n", testConfig.AccountID, *output.Account)
		return
	}

	// Setup test infrastructure (VPC, subnets)
	fmt.Println("\nSetting up test infrastructure (VPC, subnets)...")
	testInfra, err = setupTestInfrastructure(ctx, globalCleanup)
	if err != nil {
		fmt.Printf("Failed to setup test infrastructure: %v\n", err)
		return
	}
	fmt.Printf("  VPC: %s\n", testInfra.VPCID)
	fmt.Printf("  Public Subnets: %v\n", testInfra.PublicSubnetIDs)
	fmt.Printf("  Private Subnets: %v\n", testInfra.PrivateSubnetIDs)
	fmt.Printf("  Availability Zones: %v\n", testInfra.AvailabilityZones)
	fmt.Println()

	// Run tests
	fmt.Println("=== Running Tests ===")
	code = m.Run()
}
