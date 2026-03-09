package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/maxkrivich/cloud-janitor/internal/app/usecase"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
	"github.com/maxkrivich/cloud-janitor/internal/infra/aws"
)

var (
	listStatus []string
	listTypes  []string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List resources and their expiration status",
	Long: `List all resources and their current expiration status.

This command shows all resources across configured accounts and regions,
along with their expiration dates and current status.

Resource statuses:
  - untagged:      No expiration-date tag (will be tagged on next run)
  - active:        Has expiration date in the future
  - expired:       Expiration date has passed (will be deleted on next cleanup)
  - never_expires: Marked with expiration-date=never

Examples:
  # List all resources
  cloud-janitor list

  # List only expired resources
  cloud-janitor list --status expired

  # List only EC2 instances
  cloud-janitor list --types ec2

  # List as JSON
  cloud-janitor list --output json`,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().StringSliceVar(&listStatus, "status", nil, "filter by status: untagged, active, expired, never_expires")
	listCmd.Flags().StringSliceVar(&listTypes, "types", nil, "filter by type: ec2, ebs, ebs_snapshot, elastic_ip")
}

func runList(_ *cobra.Command, _ []string) error {
	ctx := context.Background()

	cfg, err := loadConfig()
	if err != nil {
		printError("%v", err)
		return err
	}

	// Create AWS client factory
	clientFactory, err := aws.NewClientFactory(ctx)
	if err != nil {
		printError("creating AWS client factory: %v", err)
		return fmt.Errorf("creating AWS client factory: %w", err)
	}

	// Create repository factory
	repoFactory := aws.NewRepositoryFactory(clientFactory)
	repoFactory.WithEnabledTypes(cfg.Scanners.ToEnabledTypes())

	// Get accounts
	domainAccounts := cfg.GetAccounts()
	if len(domainAccounts) == 0 {
		accountID, err := clientFactory.GetAccountID(ctx)
		if err != nil {
			printError("getting current account ID: %v", err)
			return fmt.Errorf("getting current account ID: %w", err)
		}
		domainAccounts = []domain.Account{{ID: accountID, Name: "current"}}
	}

	// Build list config with filters
	listConfig := usecase.ListConfig{}

	if len(listStatus) > 0 {
		for _, s := range listStatus {
			switch s {
			case "untagged":
				listConfig.FilterStatus = append(listConfig.FilterStatus, domain.StatusUntagged)
			case "active":
				listConfig.FilterStatus = append(listConfig.FilterStatus, domain.StatusActive)
			case "expired":
				listConfig.FilterStatus = append(listConfig.FilterStatus, domain.StatusExpired)
			case "never_expires":
				listConfig.FilterStatus = append(listConfig.FilterStatus, domain.StatusNeverExpires)
			}
		}
	}

	if len(listTypes) > 0 {
		for _, t := range listTypes {
			listConfig.FilterType = append(listConfig.FilterType, domain.ResourceType(t))
		}
	}

	// Collect all resources
	var allResources []domain.Resource
	var totalSummary usecase.ListSummary

	for _, account := range domainAccounts {
		for _, region := range cfg.AWS.Regions {
			repos, err := repoFactory.CreateRepositories(ctx, account, region)
			if err != nil {
				printError("creating repositories for %s/%s: %v", account.ID, region, err)
				continue
			}

			listUC := usecase.NewListResourcesUseCase(repos, listConfig)
			result, err := listUC.Execute(ctx, region)
			if err != nil {
				printError("listing resources in %s/%s: %v", account.ID, region, err)
				continue
			}

			allResources = append(allResources, result.Resources...)
			totalSummary.Total += result.Summary.Total
			totalSummary.Untagged += result.Summary.Untagged
			totalSummary.Active += result.Summary.Active
			totalSummary.Expired += result.Summary.Expired
			totalSummary.NeverExpires += result.Summary.NeverExpires
		}
	}

	// Format output
	formatter := getFormatter(cfg)
	combinedResult := &usecase.ListResult{
		Resources: allResources,
		Summary:   totalSummary,
	}

	if err := formatter.FormatListResult(os.Stdout, combinedResult); err != nil {
		printError("formatting output: %v", err)
		return fmt.Errorf("formatting output: %w", err)
	}

	return nil
}
