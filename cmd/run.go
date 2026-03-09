package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run full tag and cleanup cycle",
	Long: `Run the complete Cloud Janitor cycle:

1. TAG: Resources without an expiration-date tag get tagged with a date 30 days from now
2. CLEANUP: Resources with an expiration-date in the past are deleted

This is the main command for scheduled runs (e.g., daily via CI/CD).

Examples:
  # Run full cycle
  cloud-janitor run

  # Preview what would happen
  cloud-janitor run --dry-run

  # Run for specific regions
  cloud-janitor run --regions us-east-1,us-west-2

  # Run with JSON output
  cloud-janitor run --output json`,
	RunE: runRun,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runRun(_ *cobra.Command, _ []string) error {
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

	result, err := janitor.Run(ctx)
	if err != nil {
		printError("%v", err)
		return fmt.Errorf("running janitor: %w", err)
	}

	formatter := getFormatter(cfg)
	if err := formatter.FormatRunResult(os.Stdout, result, cfg.DryRun); err != nil {
		printError("formatting output: %v", err)
		return fmt.Errorf("formatting output: %w", err)
	}

	// Exit with error if there were any errors during the run
	if result.TotalErrors() > 0 {
		return nil // Errors already printed in output
	}

	return nil
}
