package domain_test

import (
	"testing"
	"time"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

func TestResource_Status(t *testing.T) {
	now := time.Now()
	past := now.AddDate(0, 0, -1)
	future := now.AddDate(0, 0, 1)

	tests := []struct {
		name     string
		resource domain.Resource
		want     domain.Status
	}{
		{
			name:     "untagged resource",
			resource: domain.Resource{ID: "i-123", ExpirationDate: nil},
			want:     domain.StatusUntagged,
		},
		{
			name:     "expired resource",
			resource: domain.Resource{ID: "i-123", ExpirationDate: &past},
			want:     domain.StatusExpired,
		},
		{
			name:     "active resource",
			resource: domain.Resource{ID: "i-123", ExpirationDate: &future},
			want:     domain.StatusActive,
		},
		{
			name:     "never expires resource",
			resource: domain.Resource{ID: "i-123", NeverExpires: true},
			want:     domain.StatusNeverExpires,
		},
		{
			name:     "never expires takes precedence over expiration date",
			resource: domain.Resource{ID: "i-123", ExpirationDate: &past, NeverExpires: true},
			want:     domain.StatusNeverExpires,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.resource.Status()
			if got != tt.want {
				t.Errorf("Status() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResource_DaysUntilExpiration(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		resource domain.Resource
		want     int
	}{
		{
			name:     "untagged resource returns -1",
			resource: domain.Resource{ID: "i-123", ExpirationDate: nil},
			want:     -1,
		},
		{
			name:     "never expires returns -1",
			resource: domain.Resource{ID: "i-123", NeverExpires: true},
			want:     -1,
		},
		{
			name: "expired resource returns 0",
			resource: domain.Resource{
				ID:             "i-123",
				ExpirationDate: timePtr(now.AddDate(0, 0, -5)),
			},
			want: 0,
		},
		{
			name: "expiring in 10 days",
			resource: domain.Resource{
				ID:             "i-123",
				ExpirationDate: timePtr(now.AddDate(0, 0, 10)),
			},
			want: 10,
		},
		{
			name: "expiring today",
			resource: domain.Resource{
				ID:             "i-123",
				ExpirationDate: timePtr(now),
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.resource.DaysUntilExpiration()
			// Allow +/- 1 day tolerance for timing issues
			if got != tt.want && got != tt.want+1 && got != tt.want-1 {
				t.Errorf("DaysUntilExpiration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResource_IsExcluded(t *testing.T) {
	tests := []struct {
		name        string
		resource    domain.Resource
		excludeTags map[string]string
		want        bool
	}{
		{
			name: "resource with matching exclude tag",
			resource: domain.Resource{
				ID:   "i-123",
				Tags: map[string]string{"Environment": "production"},
			},
			excludeTags: map[string]string{"Environment": "production"},
			want:        true,
		},
		{
			name: "resource without matching exclude tag",
			resource: domain.Resource{
				ID:   "i-123",
				Tags: map[string]string{"Environment": "development"},
			},
			excludeTags: map[string]string{"Environment": "production"},
			want:        false,
		},
		{
			name: "resource with no tags",
			resource: domain.Resource{
				ID:   "i-123",
				Tags: map[string]string{},
			},
			excludeTags: map[string]string{"Environment": "production"},
			want:        false,
		},
		{
			name: "resource with DoNotDelete tag",
			resource: domain.Resource{
				ID:   "i-123",
				Tags: map[string]string{"DoNotDelete": "true"},
			},
			excludeTags: map[string]string{"DoNotDelete": "true"},
			want:        true,
		},
		{
			name: "empty exclude tags",
			resource: domain.Resource{
				ID:   "i-123",
				Tags: map[string]string{"Environment": "production"},
			},
			excludeTags: map[string]string{},
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.resource.IsExcluded(tt.excludeTags)
			if got != tt.want {
				t.Errorf("IsExcluded() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResourceType_String(t *testing.T) {
	tests := []struct {
		rt   domain.ResourceType
		want string
	}{
		{domain.ResourceTypeEC2, "ec2"},
		{domain.ResourceTypeEBS, "ebs"},
		{domain.ResourceTypeEBSSnapshot, "ebs_snapshot"},
		{domain.ResourceTypeElasticIP, "elastic_ip"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.rt.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatus_String(t *testing.T) {
	tests := []struct {
		s    domain.Status
		want string
	}{
		{domain.StatusUntagged, "untagged"},
		{domain.StatusActive, "active"},
		{domain.StatusExpired, "expired"},
		{domain.StatusNeverExpires, "never_expires"},
		{domain.Status(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.s.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
