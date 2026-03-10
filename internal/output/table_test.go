package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/maxkrivich/cloud-janitor/internal/app/service"
	"github.com/maxkrivich/cloud-janitor/internal/app/usecase"
	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

func TestTableFormatter_FormatRunResult_ErrorDisplay(t *testing.T) {
	tests := []struct {
		name           string
		result         *service.RunResult
		dryRun         bool
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "displays error header with natural language",
			result: &service.RunResult{
				Errors: []error{errors.New("test error")},
			},
			dryRun: false,
			wantContains: []string{
				"Encountered 1 error(s):",
			},
			wantNotContain: []string{
				"Errors (1):",
			},
		},
		{
			name: "uses bullet point character",
			result: &service.RunResult{
				Errors: []error{errors.New("test error")},
			},
			dryRun: false,
			wantContains: []string{
				"• test error",
			},
			wantNotContain: []string{
				"- test error",
			},
		},
		{
			name: "displays helpful tip after errors",
			result: &service.RunResult{
				Errors: []error{errors.New("test error")},
			},
			dryRun: false,
			wantContains: []string{
				"Tip: Use --dry-run to preview operations, or check IAM permissions if access denied.",
			},
		},
		{
			name: "displays multiple errors with bullets",
			result: &service.RunResult{
				Errors: []error{
					errors.New("first error"),
					errors.New("second error"),
				},
			},
			dryRun: false,
			wantContains: []string{
				"Encountered 2 error(s):",
				"• first error",
				"• second error",
				"Tip:",
			},
		},
		{
			name: "displays tag result errors with key prefix",
			result: &service.RunResult{
				TagResults: map[string]*usecase.TagResult{
					"us-east-1/ec2": {
						Errors: []*domain.ResourceError{
							domain.NewResourceError("i-123", domain.ResourceTypeEC2, "tagging", errors.New("access denied")),
						},
					},
				},
			},
			dryRun: false,
			wantContains: []string{
				"Encountered 1 error(s):",
				"• [us-east-1/ec2]",
				"Tip:",
			},
		},
		{
			name: "displays cleanup result errors with key prefix",
			result: &service.RunResult{
				CleanupResults: map[string]*usecase.CleanupResult{
					"us-west-2/ebs": {
						Errors: []*domain.ResourceError{
							domain.NewResourceError("vol-123", domain.ResourceTypeEBS, "deleting", errors.New("volume in use")),
						},
					},
				},
			},
			dryRun: false,
			wantContains: []string{
				"Encountered 1 error(s):",
				"• [us-west-2/ebs]",
				"Tip:",
			},
		},
		{
			name: "no error section when no errors",
			result: &service.RunResult{
				Errors:         []error{},
				TagResults:     map[string]*usecase.TagResult{},
				CleanupResults: map[string]*usecase.CleanupResult{},
			},
			dryRun: false,
			wantNotContain: []string{
				"Encountered",
				"error(s):",
				"Tip:",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			f := &TableFormatter{}

			err := f.FormatRunResult(&buf, tt.result, tt.dryRun)
			if err != nil {
				t.Fatalf("FormatRunResult() error = %v", err)
			}

			output := buf.String()

			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Errorf("output should contain %q, got:\n%s", want, output)
				}
			}

			for _, notWant := range tt.wantNotContain {
				if strings.Contains(output, notWant) {
					t.Errorf("output should not contain %q, got:\n%s", notWant, output)
				}
			}
		})
	}
}
