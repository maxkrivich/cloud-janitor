package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version information (set via ldflags during build)
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of cloud-janitor",
	Long:  `Display the version, commit hash, and build date of cloud-janitor.`,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("cloud-janitor %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built:  %s\n", buildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
