package aws

import (
	"testing"
	"time"
)

func TestParseExpirationDate(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		resourceID   string
		resourceType string
		wantNil      bool
		wantDate     string // Expected date in YYYY-MM-DD format, empty if wantNil
	}{
		{
			name:         "valid date",
			value:        "2024-03-15",
			resourceID:   "i-123",
			resourceType: "EC2",
			wantNil:      false,
			wantDate:     "2024-03-15",
		},
		{
			name:         "date with leading whitespace",
			value:        "  2024-03-15",
			resourceID:   "i-123",
			resourceType: "EC2",
			wantNil:      false,
			wantDate:     "2024-03-15",
		},
		{
			name:         "date with trailing whitespace",
			value:        "2024-03-15  ",
			resourceID:   "i-123",
			resourceType: "EC2",
			wantNil:      false,
			wantDate:     "2024-03-15",
		},
		{
			name:         "date with both leading and trailing whitespace",
			value:        "  2024-03-15  ",
			resourceID:   "i-123",
			resourceType: "EC2",
			wantNil:      false,
			wantDate:     "2024-03-15",
		},
		{
			name:         "date with tab characters",
			value:        "\t2024-03-15\t",
			resourceID:   "i-123",
			resourceType: "EC2",
			wantNil:      false,
			wantDate:     "2024-03-15",
		},
		{
			name:         "date with newline characters",
			value:        "\n2024-03-15\n",
			resourceID:   "i-123",
			resourceType: "EC2",
			wantNil:      false,
			wantDate:     "2024-03-15",
		},
		{
			name:         "invalid date format - wrong separator",
			value:        "2024/03/15",
			resourceID:   "i-123",
			resourceType: "EC2",
			wantNil:      true,
		},
		{
			name:         "invalid date format - text",
			value:        "invalid-date",
			resourceID:   "i-123",
			resourceType: "EC2",
			wantNil:      true,
		},
		{
			name:         "invalid date format - incomplete",
			value:        "2024-03",
			resourceID:   "i-123",
			resourceType: "EC2",
			wantNil:      true,
		},
		{
			name:         "empty string",
			value:        "",
			resourceID:   "i-123",
			resourceType: "EC2",
			wantNil:      true,
		},
		{
			name:         "whitespace only",
			value:        "   ",
			resourceID:   "i-123",
			resourceType: "EC2",
			wantNil:      true,
		},
		{
			name:         "date with extra content",
			value:        "2024-03-15T00:00:00Z",
			resourceID:   "i-123",
			resourceType: "EC2",
			wantNil:      true,
		},
		{
			name:         "never value should not parse",
			value:        "never",
			resourceID:   "i-123",
			resourceType: "EC2",
			wantNil:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseExpirationDate(tt.value, tt.resourceID, tt.resourceType)

			if tt.wantNil {
				if result != nil {
					t.Errorf("ParseExpirationDate() = %v, want nil", result)
				}
			} else {
				if result == nil {
					t.Errorf("ParseExpirationDate() = nil, want date %s", tt.wantDate)
					return
				}
				gotDate := result.Format(ExpirationDateFormat)
				if gotDate != tt.wantDate {
					t.Errorf("ParseExpirationDate() = %s, want %s", gotDate, tt.wantDate)
				}
			}
		})
	}
}

func TestIsNeverExpires(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{
			name:  "exact never value",
			value: "never",
			want:  true,
		},
		{
			name:  "never with leading whitespace",
			value: "  never",
			want:  true,
		},
		{
			name:  "never with trailing whitespace",
			value: "never  ",
			want:  true,
		},
		{
			name:  "never with both leading and trailing whitespace",
			value: "  never  ",
			want:  true,
		},
		{
			name:  "never with tab characters",
			value: "\tnever\t",
			want:  true,
		},
		{
			name:  "never with newline characters",
			value: "\nnever\n",
			want:  true,
		},
		{
			name:  "NEVER uppercase",
			value: "NEVER",
			want:  false, // case sensitive
		},
		{
			name:  "Never mixed case",
			value: "Never",
			want:  false, // case sensitive
		},
		{
			name:  "valid date",
			value: "2024-03-15",
			want:  false,
		},
		{
			name:  "empty string",
			value: "",
			want:  false,
		},
		{
			name:  "whitespace only",
			value: "   ",
			want:  false,
		},
		{
			name:  "never with extra text",
			value: "never-delete",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNeverExpires(tt.value)
			if got != tt.want {
				t.Errorf("IsNeverExpires(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestParseExpirationDate_ReturnsCorrectTime(t *testing.T) {
	// Test that the returned time has the expected components
	result := ParseExpirationDate("2024-12-25", "test-id", "Test")

	if result == nil {
		t.Fatal("ParseExpirationDate() returned nil for valid date")
	}

	expectedYear := 2024
	expectedMonth := time.December
	expectedDay := 25

	if result.Year() != expectedYear {
		t.Errorf("Year = %d, want %d", result.Year(), expectedYear)
	}
	if result.Month() != expectedMonth {
		t.Errorf("Month = %v, want %v", result.Month(), expectedMonth)
	}
	if result.Day() != expectedDay {
		t.Errorf("Day = %d, want %d", result.Day(), expectedDay)
	}
}
