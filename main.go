// Package main is the entry point for the cloud-janitor CLI tool.
// Cloud Janitor helps identify and eliminate wasteful cloud spending
// by scanning AWS infrastructure for unused or underutilized resources.
package main

import (
	"os"

	"github.com/maxkrivich/cloud-janitor/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
