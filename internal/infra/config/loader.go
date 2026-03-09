// Package config provides configuration loading and management.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Config represents the application configuration.
type Config struct {
	AWS           AWSConfig           `mapstructure:"aws"`
	Expiration    ExpirationConfig    `mapstructure:"expiration"`
	Scanners      ScannersConfig      `mapstructure:"scanners"`
	Notifications NotificationsConfig `mapstructure:"notifications"`
	Output        OutputConfig        `mapstructure:"output"`
	DryRun        bool                `mapstructure:"dry_run"`
}

// AWSConfig holds AWS-specific configuration.
type AWSConfig struct {
	Accounts []AccountConfig `mapstructure:"accounts"`
	Regions  []string        `mapstructure:"regions"`
}

// AccountConfig represents an AWS account configuration.
type AccountConfig struct {
	ID   string `mapstructure:"id"`
	Name string `mapstructure:"name"`
	Role string `mapstructure:"role"`
}

// ToDomain converts AccountConfig to domain.Account.
func (a AccountConfig) ToDomain() domain.Account {
	return domain.Account{
		ID:      a.ID,
		Name:    a.Name,
		RoleARN: a.Role,
	}
}

// ExpirationConfig holds expiration-related settings.
type ExpirationConfig struct {
	TagName              string       `mapstructure:"tag_name"`
	DateFormat           string       `mapstructure:"date_format"`
	DefaultDays          int          `mapstructure:"default_days"`
	ExcludeTags          []ExcludeTag `mapstructure:"exclude_tags"`
	ForceDeleteProtected bool         `mapstructure:"force_delete_protected"` // Force delete resources with deletion protection
	EKSCascadeDelete     bool         `mapstructure:"eks_cascade_delete"`     // Delete EKS node groups before cluster
	LogsSkipPatterns     []string     `mapstructure:"logs_skip_patterns"`     // Glob patterns to skip CloudWatch Log Groups
}

// ExcludeTag represents a tag that excludes resources from cleanup.
type ExcludeTag struct {
	Key   string `mapstructure:"key"`
	Value string `mapstructure:"value"`
}

// ToMap converts ExcludeTags to a map.
func (c ExpirationConfig) ToMap() map[string]string {
	result := make(map[string]string)
	for _, tag := range c.ExcludeTags {
		result[tag.Key] = tag.Value
	}
	return result
}

// ScannersConfig enables/disables specific scanners.
type ScannersConfig struct {
	// Phase 1 scanners (existing)
	EC2          bool `mapstructure:"ec2"`
	EBS          bool `mapstructure:"ebs"`
	EBSSnapshots bool `mapstructure:"ebs_snapshots"`
	ElasticIP    bool `mapstructure:"elastic_ip"`

	// Phase 2 scanners - P1 (highest ROI)
	RDS        bool `mapstructure:"rds"`
	ELB        bool `mapstructure:"elb"`
	NATGateway bool `mapstructure:"nat_gateway"`

	// Phase 2 scanners - P2
	ElastiCache bool `mapstructure:"elasticache"`
	OpenSearch  bool `mapstructure:"opensearch"`
	EKS         bool `mapstructure:"eks"`

	// Phase 2 scanners - P3
	Redshift  bool `mapstructure:"redshift"`
	SageMaker bool `mapstructure:"sagemaker"`
	AMI       bool `mapstructure:"ami"`
	Logs      bool `mapstructure:"logs"`
}

// ToEnabledTypes converts ScannersConfig to a map of enabled resource types.
func (c ScannersConfig) ToEnabledTypes() map[domain.ResourceType]bool {
	return map[domain.ResourceType]bool{
		// Phase 1 scanners
		domain.ResourceTypeEC2:         c.EC2,
		domain.ResourceTypeEBS:         c.EBS,
		domain.ResourceTypeEBSSnapshot: c.EBSSnapshots,
		domain.ResourceTypeElasticIP:   c.ElasticIP,
		// Phase 2 scanners - P1
		domain.ResourceTypeRDS:        c.RDS,
		domain.ResourceTypeELB:        c.ELB,
		domain.ResourceTypeNATGateway: c.NATGateway,
		// Phase 2 scanners - P2
		domain.ResourceTypeElastiCache: c.ElastiCache,
		domain.ResourceTypeOpenSearch:  c.OpenSearch,
		domain.ResourceTypeEKS:         c.EKS,
		// Phase 2 scanners - P3
		domain.ResourceTypeRedshift:  c.Redshift,
		domain.ResourceTypeSageMaker: c.SageMaker,
		domain.ResourceTypeAMI:       c.AMI,
		domain.ResourceTypeLogs:      c.Logs,
	}
}

// NotificationsConfig holds notification settings.
type NotificationsConfig struct {
	Enabled bool          `mapstructure:"enabled"`
	Slack   SlackConfig   `mapstructure:"slack"`
	Discord DiscordConfig `mapstructure:"discord"`
	Teams   TeamsConfig   `mapstructure:"teams"`
	Webhook WebhookConfig `mapstructure:"webhook"`
}

// SlackConfig holds Slack notification settings.
// Supports three authentication modes:
// 1. Webhook URL (simple): Just provide webhook_url
// 2. Bot token (standard): Provide bot_token and channel_id
// 3. App token (Socket Mode): Provide app_token, bot_token, and channel_id
type SlackConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	WebhookURL string `mapstructure:"webhook_url"`
	BotToken   string `mapstructure:"bot_token"`
	AppToken   string `mapstructure:"app_token"`
	ChannelID  string `mapstructure:"channel_id"`
	Channel    string `mapstructure:"channel"` // Optional channel override for webhook mode
}

// DiscordConfig holds Discord notification settings.
// Supports two authentication modes:
// 1. Webhook URL (simple): Just provide webhook_url
// 2. Bot token (advanced): Provide bot_token and channel_id
type DiscordConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	WebhookURL string `mapstructure:"webhook_url"`
	BotToken   string `mapstructure:"bot_token"`
	ChannelID  string `mapstructure:"channel_id"`
}

// TeamsConfig holds Microsoft Teams notification settings.
// Uses Incoming Webhook with MessageCard format.
type TeamsConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	WebhookURL string `mapstructure:"webhook_url"`
}

// WebhookConfig holds generic webhook notification settings.
type WebhookConfig struct {
	Enabled bool              `mapstructure:"enabled"`
	URL     string            `mapstructure:"url"`
	Headers map[string]string `mapstructure:"headers"`
}

// OutputConfig holds output formatting settings.
type OutputConfig struct {
	Format  string `mapstructure:"format"`
	Verbose bool   `mapstructure:"verbose"`
}

// Load loads configuration from a file and environment variables.
func Load(configFile string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Load config file
	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		// Search for config in common locations
		home, err := os.UserHomeDir()
		if err == nil {
			v.AddConfigPath(home)
		}
		v.AddConfigPath(".")
		v.SetConfigType("yaml")
		v.SetConfigName("cloud-janitor") // looks for cloud-janitor.yaml
	}

	// Read environment variables
	v.SetEnvPrefix("CLOUD_JANITOR")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file (ignore error if not found)
	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	// Unmarshal config
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	// Expand environment variables in sensitive fields
	cfg.Notifications.Slack.WebhookURL = os.ExpandEnv(cfg.Notifications.Slack.WebhookURL)
	cfg.Notifications.Slack.BotToken = os.ExpandEnv(cfg.Notifications.Slack.BotToken)
	cfg.Notifications.Slack.AppToken = os.ExpandEnv(cfg.Notifications.Slack.AppToken)
	cfg.Notifications.Slack.ChannelID = os.ExpandEnv(cfg.Notifications.Slack.ChannelID)
	cfg.Notifications.Discord.WebhookURL = os.ExpandEnv(cfg.Notifications.Discord.WebhookURL)
	cfg.Notifications.Discord.BotToken = os.ExpandEnv(cfg.Notifications.Discord.BotToken)
	cfg.Notifications.Discord.ChannelID = os.ExpandEnv(cfg.Notifications.Discord.ChannelID)
	cfg.Notifications.Teams.WebhookURL = os.ExpandEnv(cfg.Notifications.Teams.WebhookURL)
	cfg.Notifications.Webhook.URL = os.ExpandEnv(cfg.Notifications.Webhook.URL)
	for key, value := range cfg.Notifications.Webhook.Headers {
		cfg.Notifications.Webhook.Headers[key] = os.ExpandEnv(value)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// AWS defaults
	v.SetDefault("aws.regions", []string{"us-east-1"})

	// Expiration defaults
	v.SetDefault("expiration.tag_name", "expiration-date")
	v.SetDefault("expiration.date_format", "2006-01-02")
	v.SetDefault("expiration.default_days", 30)
	v.SetDefault("expiration.force_delete_protected", false)
	v.SetDefault("expiration.eks_cascade_delete", false)
	v.SetDefault("expiration.logs_skip_patterns", []string{
		"/aws/lambda/*",
		"/aws/eks/*",
		"/aws/rds/*",
		"/aws/elasticbeanstalk/*",
	})

	// Scanner defaults - Phase 1 (all enabled by default)
	v.SetDefault("scanners.ec2", true)
	v.SetDefault("scanners.ebs", true)
	v.SetDefault("scanners.ebs_snapshots", true)
	v.SetDefault("scanners.elastic_ip", true)

	// Scanner defaults - Phase 2 P1 (all enabled by default)
	v.SetDefault("scanners.rds", true)
	v.SetDefault("scanners.elb", true)
	v.SetDefault("scanners.nat_gateway", true)

	// Scanner defaults - Phase 2 P2 (all enabled by default)
	v.SetDefault("scanners.elasticache", true)
	v.SetDefault("scanners.opensearch", true)
	v.SetDefault("scanners.eks", true)

	// Scanner defaults - Phase 2 P3 (all enabled by default)
	v.SetDefault("scanners.redshift", true)
	v.SetDefault("scanners.sagemaker", true)
	v.SetDefault("scanners.ami", true)
	v.SetDefault("scanners.logs", true)

	// Notification defaults
	v.SetDefault("notifications.enabled", true)

	// Output defaults
	v.SetDefault("output.format", "table")
	v.SetDefault("output.verbose", false)

	// Dry run default
	v.SetDefault("dry_run", false)
}

// GetAccounts returns domain accounts from the config.
func (c *Config) GetAccounts() []domain.Account {
	accounts := make([]domain.Account, 0, len(c.AWS.Accounts))
	for _, acc := range c.AWS.Accounts {
		accounts = append(accounts, acc.ToDomain())
	}
	return accounts
}
