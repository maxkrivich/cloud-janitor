// Package cmd contains all CLI commands for cloud-janitor.
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/maxkrivich/cloud-janitor/internal/app/service"
	"github.com/maxkrivich/cloud-janitor/internal/app/usecase"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
	"github.com/maxkrivich/cloud-janitor/internal/infra/aws"
	"github.com/maxkrivich/cloud-janitor/internal/infra/config"
	"github.com/maxkrivich/cloud-janitor/internal/infra/notify"
	"github.com/maxkrivich/cloud-janitor/internal/output"
)

var (
	cfgFile   string
	dryRun    bool
	verbose   bool
	outFormat string
	regions   []string
	accounts  []string
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "cloud-janitor",
	Short: "Automated AWS resource cleanup tool",
	Long: `Cloud Janitor is an automated cleanup tool for AWS development accounts.

It uses a tag-based expiration system to automatically remove unused resources
after a grace period, reducing cloud costs with minimal manual intervention.

The cleanup process has two steps:
  1. TAG: Resources without an expiration-date tag get tagged (30-day grace period)
  2. CLEANUP: Resources past their expiration date are automatically deleted

Examples:
  # Run full scan and cleanup cycle
  cloud-janitor run

  # Preview what would happen (no changes)
  cloud-janitor run --dry-run

  # Only tag resources (no deletion)
  cloud-janitor tag

  # Only delete expired resources (no tagging)
  cloud-janitor cleanup

  # Show resources and their expiration status
  cloud-janitor list`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		return fmt.Errorf("executing command: %w", err)
	}
	return nil
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./cloud-janitor.yaml)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "preview changes without making them")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVarP(&outFormat, "output", "o", "table", "output format: table, json")
	rootCmd.PersistentFlags().StringSliceVarP(&regions, "regions", "r", nil, "AWS regions to scan (overrides config)")
	rootCmd.PersistentFlags().StringSliceVarP(&accounts, "accounts", "a", nil, "AWS account IDs to scan (overrides config)")
}

// loadConfig loads the configuration file.
func loadConfig() (*config.Config, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	// Override with CLI flags
	if dryRun {
		cfg.DryRun = true
	}
	if verbose {
		cfg.Output.Verbose = true
	}
	if outFormat != "" {
		cfg.Output.Format = outFormat
	}
	if len(regions) > 0 {
		cfg.AWS.Regions = regions
	}

	return cfg, nil
}

// buildNotifier creates a notifier based on configuration.
func buildNotifier(cfg *config.Config) domain.Notifier {
	if !cfg.Notifications.Enabled || cfg.DryRun {
		return notify.NewNoopNotifier()
	}

	var notifiers []domain.Notifier

	if cfg.Notifications.Slack.Enabled {
		var slackNotifier domain.Notifier
		var err error

		// Priority: App token > Bot token > Webhook URL
		switch {
		case cfg.Notifications.Slack.AppToken != "" &&
			cfg.Notifications.Slack.BotToken != "" &&
			cfg.Notifications.Slack.ChannelID != "":
			slackNotifier, err = notify.NewSlackNotifierApp(
				cfg.Notifications.Slack.AppToken,
				cfg.Notifications.Slack.BotToken,
				cfg.Notifications.Slack.ChannelID,
			)
		case cfg.Notifications.Slack.BotToken != "" && cfg.Notifications.Slack.ChannelID != "":
			slackNotifier, err = notify.NewSlackNotifierBot(
				cfg.Notifications.Slack.BotToken,
				cfg.Notifications.Slack.ChannelID,
			)
		case cfg.Notifications.Slack.WebhookURL != "":
			opts := []notify.SlackOption{}
			if cfg.Notifications.Slack.Channel != "" {
				opts = append(opts, notify.WithSlackChannel(cfg.Notifications.Slack.Channel))
			}
			slackNotifier = notify.NewSlackNotifierWebhook(cfg.Notifications.Slack.WebhookURL, opts...)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to create Slack notifier: %v\n", err)
		} else if slackNotifier != nil {
			notifiers = append(notifiers, slackNotifier)
		}
	}

	if cfg.Notifications.Discord.Enabled {
		var discordNotifier domain.Notifier
		var err error

		// Prefer bot token over webhook URL if both are provided
		if cfg.Notifications.Discord.BotToken != "" && cfg.Notifications.Discord.ChannelID != "" {
			discordNotifier, err = notify.NewDiscordNotifierBot(
				cfg.Notifications.Discord.BotToken,
				cfg.Notifications.Discord.ChannelID,
			)
		} else if cfg.Notifications.Discord.WebhookURL != "" {
			discordNotifier, err = notify.NewDiscordNotifierWebhook(cfg.Notifications.Discord.WebhookURL)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to create Discord notifier: %v\n", err)
		} else if discordNotifier != nil {
			notifiers = append(notifiers, discordNotifier)
		}
	}

	if cfg.Notifications.Webhook.Enabled && cfg.Notifications.Webhook.URL != "" {
		opts := []notify.WebhookOption{}
		if len(cfg.Notifications.Webhook.Headers) > 0 {
			opts = append(opts, notify.WithWebhookHeaders(cfg.Notifications.Webhook.Headers))
		}
		notifiers = append(notifiers, notify.NewWebhookNotifier(cfg.Notifications.Webhook.URL, opts...))
	}

	if len(notifiers) == 0 {
		return notify.NewNoopNotifier()
	}

	if len(notifiers) == 1 {
		return notifiers[0]
	}

	return notify.NewMultiNotifier(notifiers...)
}

// buildJanitor creates a Janitor service from configuration.
func buildJanitor(ctx context.Context, cfg *config.Config) (*service.Janitor, error) {
	// Create AWS client factory
	clientFactory, err := aws.NewClientFactory(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating AWS client factory: %w", err)
	}

	// Create repository factory
	repoFactory := aws.NewRepositoryFactory(clientFactory)
	repoFactory.WithEnabledTypes(cfg.Scanners.ToEnabledTypes())

	// Build notifier
	notifier := buildNotifier(cfg)

	// Get accounts (use current account if none configured)
	domainAccounts := cfg.GetAccounts()
	if len(domainAccounts) == 0 {
		// Use current AWS credentials
		accountID, err := clientFactory.GetAccountID(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting current account ID: %w", err)
		}
		domainAccounts = []domain.Account{{ID: accountID, Name: "current"}}
	}

	// Filter accounts if specified via CLI
	if len(accounts) > 0 {
		filtered := make([]domain.Account, 0)
		for _, acc := range domainAccounts {
			for _, id := range accounts {
				if acc.ID == id {
					filtered = append(filtered, acc)
					break
				}
			}
		}
		domainAccounts = filtered
	}

	// Build janitor config
	janitorCfg := service.JanitorConfig{
		Accounts: domainAccounts,
		Regions:  cfg.AWS.Regions,
		TagConfig: usecase.TagConfig{
			DefaultDays: cfg.Expiration.DefaultDays,
			TagName:     cfg.Expiration.TagName,
			ExcludeTags: cfg.Expiration.ToMap(),
			DryRun:      cfg.DryRun,
		},
		CleanupConfig: usecase.CleanupConfig{
			ExcludeTags: cfg.Expiration.ToMap(),
			DryRun:      cfg.DryRun,
		},
		ListConfig: usecase.ListConfig{},
	}

	// Create repository factory function
	repoFactoryFn := func(ctx context.Context, account domain.Account) ([]domain.ResourceRepository, error) {
		var allRepos []domain.ResourceRepository
		for _, region := range cfg.AWS.Regions {
			repos, err := repoFactory.CreateRepositories(ctx, account, region)
			if err != nil {
				return nil, fmt.Errorf("creating repositories for region %s: %w", region, err)
			}
			allRepos = append(allRepos, repos...)
		}
		return allRepos, nil
	}

	return service.NewJanitor(janitorCfg, notifier, repoFactoryFn), nil
}

// getFormatter returns the output formatter.
func getFormatter(cfg *config.Config) output.Formatter {
	return output.New(cfg.Output.Format)
}

// printError prints an error message to stderr.
func printError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
}
