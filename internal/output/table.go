package output

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/maxkrivich/cloud-janitor/internal/app/service"
	"github.com/maxkrivich/cloud-janitor/internal/app/usecase"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// TableFormatter formats output as ASCII tables.
type TableFormatter struct{}

// FormatResources formats a list of resources as a table.
func (f *TableFormatter) FormatResources(w io.Writer, resources []domain.Resource) error {
	if len(resources) == 0 {
		fmt.Fprintln(w, "No resources found.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	defer tw.Flush()

	// Header
	fmt.Fprintln(tw, "TYPE\tID\tNAME\tREGION\tSTATUS\tEXPIRATION")
	fmt.Fprintln(tw, "----\t--\t----\t------\t------\t----------")

	// Rows
	for _, r := range resources {
		expiration := "-"
		if r.NeverExpires {
			expiration = "never"
		} else if r.ExpirationDate != nil {
			expiration = r.ExpirationDate.Format("2006-01-02")
		}

		name := r.Name
		if len(name) > 30 {
			name = name[:27] + "..."
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Type, r.ID, name, r.Region, r.Status(), expiration)
	}

	return nil
}

// FormatListResult formats a list result with summary.
func (f *TableFormatter) FormatListResult(w io.Writer, result *usecase.ListResult) error {
	if err := f.FormatResources(w, result.Resources); err != nil {
		return err
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Summary: %d total, %d untagged, %d active, %d expired, %d never expires\n",
		result.Summary.Total,
		result.Summary.Untagged,
		result.Summary.Active,
		result.Summary.Expired,
		result.Summary.NeverExpires,
	)

	return nil
}

// FormatRunResult formats the results of a run operation.
func (f *TableFormatter) FormatRunResult(w io.Writer, result *service.RunResult, dryRun bool) error {
	prefix := ""
	if dryRun {
		prefix = "[DRY RUN] "
	}

	// Tagged resources
	taggedCount := result.TotalTagged()
	if taggedCount > 0 {
		fmt.Fprintf(w, "\n%sTagged Resources (%d):\n", prefix, taggedCount)
		fmt.Fprintln(w, strings.Repeat("-", 50))

		for key, tagResult := range result.TagResults {
			if len(tagResult.Tagged) == 0 {
				continue
			}
			fmt.Fprintf(w, "\n%s:\n", key)
			if err := f.FormatResources(w, tagResult.Tagged); err != nil {
				return err
			}
		}
	} else {
		fmt.Fprintf(w, "\n%sNo resources tagged.\n", prefix)
	}

	// Deleted resources
	deletedCount := result.TotalDeleted()
	if deletedCount > 0 {
		fmt.Fprintf(w, "\n%sDeleted Resources (%d):\n", prefix, deletedCount)
		fmt.Fprintln(w, strings.Repeat("-", 50))

		for key, cleanupResult := range result.CleanupResults {
			if len(cleanupResult.Deleted) == 0 {
				continue
			}
			fmt.Fprintf(w, "\n%s:\n", key)
			if err := f.FormatResources(w, cleanupResult.Deleted); err != nil {
				return err
			}
		}
	} else {
		fmt.Fprintf(w, "\n%sNo resources deleted.\n", prefix)
	}

	// Errors
	errorCount := result.TotalErrors()
	if errorCount > 0 {
		fmt.Fprintf(w, "\nErrors (%d):\n", errorCount)
		fmt.Fprintln(w, strings.Repeat("-", 50))

		for _, err := range result.Errors {
			fmt.Fprintf(w, "  - %v\n", err)
		}

		for key, tagResult := range result.TagResults {
			for _, err := range tagResult.Errors {
				fmt.Fprintf(w, "  - [%s] %v\n", key, err)
			}
		}

		for key, cleanupResult := range result.CleanupResults {
			for _, err := range cleanupResult.Errors {
				fmt.Fprintf(w, "  - [%s] %v\n", key, err)
			}
		}
	}

	// Summary
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%sSummary: %d tagged, %d deleted, %d errors\n",
		prefix, taggedCount, deletedCount, errorCount)

	return nil
}
