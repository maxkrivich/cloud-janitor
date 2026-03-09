package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/maxkrivich/cloud-janitor/internal/app/service"
	"github.com/maxkrivich/cloud-janitor/internal/app/usecase"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// JSONFormatter formats output as JSON.
type JSONFormatter struct{}

// FormatResources formats a list of resources as JSON.
func (f *JSONFormatter) FormatResources(w io.Writer, resources []domain.Resource) error {
	output := make([]resourceJSON, 0, len(resources))
	for _, r := range resources {
		output = append(output, toResourceJSON(r))
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	return nil
}

// FormatListResult formats a list result with summary as JSON.
func (f *JSONFormatter) FormatListResult(w io.Writer, result *usecase.ListResult) error {
	resources := make([]resourceJSON, 0, len(result.Resources))
	for _, r := range result.Resources {
		resources = append(resources, toResourceJSON(r))
	}

	output := listResultJSON{
		Resources: resources,
		Summary: summaryJSON{
			Total:        result.Summary.Total,
			Untagged:     result.Summary.Untagged,
			Active:       result.Summary.Active,
			Expired:      result.Summary.Expired,
			NeverExpires: result.Summary.NeverExpires,
		},
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	return nil
}

// FormatRunResult formats the results of a run operation as JSON.
func (f *JSONFormatter) FormatRunResult(w io.Writer, result *service.RunResult, dryRun bool) error {
	output := runResultJSON{
		DryRun:  dryRun,
		Tagged:  make(map[string][]resourceJSON),
		Deleted: make(map[string][]resourceJSON),
		Errors:  make([]string, 0),
		Summary: runSummaryJSON{},
	}

	// Tagged resources
	for key, tagResult := range result.TagResults {
		if len(tagResult.Tagged) > 0 {
			resources := make([]resourceJSON, 0, len(tagResult.Tagged))
			for _, r := range tagResult.Tagged {
				resources = append(resources, toResourceJSON(r))
			}
			output.Tagged[key] = resources
		}

		for _, err := range tagResult.Errors {
			output.Errors = append(output.Errors, err.Error())
		}
	}

	// Deleted resources
	for key, cleanupResult := range result.CleanupResults {
		if len(cleanupResult.Deleted) > 0 {
			resources := make([]resourceJSON, 0, len(cleanupResult.Deleted))
			for _, r := range cleanupResult.Deleted {
				resources = append(resources, toResourceJSON(r))
			}
			output.Deleted[key] = resources
		}

		for _, err := range cleanupResult.Errors {
			output.Errors = append(output.Errors, err.Error())
		}
	}

	// Top-level errors
	for _, err := range result.Errors {
		output.Errors = append(output.Errors, err.Error())
	}

	// Summary
	output.Summary.Tagged = result.TotalTagged()
	output.Summary.Deleted = result.TotalDeleted()
	output.Summary.Errors = result.TotalErrors()

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	return nil
}

type resourceJSON struct {
	ID             string            `json:"id"`
	Type           string            `json:"type"`
	Name           string            `json:"name,omitempty"`
	Region         string            `json:"region"`
	AccountID      string            `json:"account_id"`
	Status         string            `json:"status"`
	ExpirationDate *string           `json:"expiration_date,omitempty"`
	NeverExpires   bool              `json:"never_expires,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
}

func toResourceJSON(r domain.Resource) resourceJSON {
	rj := resourceJSON{
		ID:           r.ID,
		Type:         string(r.Type),
		Name:         r.Name,
		Region:       r.Region,
		AccountID:    r.AccountID,
		Status:       r.Status().String(),
		NeverExpires: r.NeverExpires,
		Tags:         r.Tags,
	}

	if r.ExpirationDate != nil {
		expDate := r.ExpirationDate.Format("2006-01-02")
		rj.ExpirationDate = &expDate
	}

	return rj
}

type listResultJSON struct {
	Resources []resourceJSON `json:"resources"`
	Summary   summaryJSON    `json:"summary"`
}

type summaryJSON struct {
	Total        int `json:"total"`
	Untagged     int `json:"untagged"`
	Active       int `json:"active"`
	Expired      int `json:"expired"`
	NeverExpires int `json:"never_expires"`
}

type runResultJSON struct {
	DryRun  bool                      `json:"dry_run"`
	Tagged  map[string][]resourceJSON `json:"tagged"`
	Deleted map[string][]resourceJSON `json:"deleted"`
	Errors  []string                  `json:"errors"`
	Summary runSummaryJSON            `json:"summary"`
}

type runSummaryJSON struct {
	Tagged  int `json:"tagged"`
	Deleted int `json:"deleted"`
	Errors  int `json:"errors"`
}
