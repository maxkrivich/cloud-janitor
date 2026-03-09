// Package output provides formatters for displaying results.
package output

import (
	"io"

	"github.com/maxkrivich/cloud-janitor/internal/app/service"
	"github.com/maxkrivich/cloud-janitor/internal/app/usecase"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Formatter formats output for display.
type Formatter interface {
	// FormatResources formats a list of resources.
	FormatResources(w io.Writer, resources []domain.Resource) error

	// FormatListResult formats a list result with summary.
	FormatListResult(w io.Writer, result *usecase.ListResult) error

	// FormatRunResult formats the results of a run operation.
	FormatRunResult(w io.Writer, result *service.RunResult, dryRun bool) error
}

// New creates a new Formatter based on the format name.
func New(format string) Formatter {
	switch format {
	case "json":
		return &JSONFormatter{}
	default:
		return &TableFormatter{}
	}
}
