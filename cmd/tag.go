package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var tagCmd = &cobra.Command{
	Use:   "tag",
	Short: "Tag untagged resources with expiration date",
	Long: `Tag resources that don't have an expiration-date tag.

This command only performs the tagging step of the cleanup process.
Resources will be tagged with an expiration date 30 days from now.

Use this command when you want to:
  - Tag resources without deleting anything
  - Preview which resources would be tagged (with --dry-run)
  - Run tagging separately from cleanup

Examples:
  # Tag untagged resources
  cloud-janitor tag

  # Preview what would be tagged
  cloud-janitor tag --dry-run

  # Tag resources in specific regions
  cloud-janitor tag --regions us-east-1,eu-west-1`,
	RunE: runTag,
}

func init() {
	rootCmd.AddCommand(tagCmd)
}

func runTag(_ *cobra.Command, _ []string) error {
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

	result, err := janitor.Tag(ctx)
	if err != nil {
		printError("%v", err)
		return fmt.Errorf("tagging resources: %w", err)
	}

	formatter := getFormatter(cfg)
	if err := formatter.FormatRunResult(os.Stdout, result, cfg.DryRun); err != nil {
		printError("formatting output: %v", err)
		return fmt.Errorf("formatting output: %w", err)
	}

	return nil
}
