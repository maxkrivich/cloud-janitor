package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Delete expired resources",
	Long: `Delete resources that have passed their expiration date.

This command only performs the cleanup step of the process.
Only resources with an expiration-date tag in the past will be deleted.

Use this command when you want to:
  - Delete expired resources without tagging new ones
  - Preview which resources would be deleted (with --dry-run)
  - Run cleanup separately from tagging

Examples:
  # Delete expired resources
  cloud-janitor cleanup

  # Preview what would be deleted
  cloud-janitor cleanup --dry-run

  # Cleanup resources in specific regions
  cloud-janitor cleanup --regions us-east-1,eu-west-1`,
	RunE: runCleanup,
}

func init() {
	rootCmd.AddCommand(cleanupCmd)
}

func runCleanup(_ *cobra.Command, _ []string) error {
	ctx := context.Background()

	cfg, err := loadConfig()
	if err != nil {
		printError("%v", err)
		return err
	}

	janitor, err := buildJanitor(ctx, cfg)
	if err != nil {
		printError("%v", err)
		return err
	}

	result, err := janitor.Cleanup(ctx)
	if err != nil {
		printError("%v", err)
		return fmt.Errorf("cleaning up resources: %w", err)
	}

	formatter := getFormatter(cfg)
	if err := formatter.FormatRunResult(os.Stdout, result, cfg.DryRun); err != nil {
		printError("formatting output: %v", err)
		return fmt.Errorf("formatting output: %w", err)
	}

	return nil
}
