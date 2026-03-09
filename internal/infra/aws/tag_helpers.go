package aws

import (
	"log"
	"strings"
	"time"
)

// ParseExpirationDate parses an expiration date tag value and returns the parsed time.
// It handles whitespace trimming and logs warnings for invalid formats.
// Returns nil if the value cannot be parsed or is empty.
func ParseExpirationDate(value, resourceID, resourceType string) *time.Time {
	// Trim whitespace that may have been accidentally added
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		log.Printf("warning: empty expiration-date tag value for %s %s", resourceType, resourceID)
		return nil
	}

	t, err := time.Parse(ExpirationDateFormat, trimmed)
	if err != nil {
		log.Printf("warning: invalid expiration-date format for %s %s: %q (expected YYYY-MM-DD, got parse error: %v)",
			resourceType, resourceID, value, err)
		return nil
	}

	return &t
}

// IsNeverExpires checks if the tag value indicates the resource should never expire.
func IsNeverExpires(value string) bool {
	return strings.TrimSpace(value) == NeverExpiresValue
}
