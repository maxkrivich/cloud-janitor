//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/aws/smithy-go"
)

// Cleanup priority levels (lower = runs first)
const (
	PriorityEKSNodeGroup = 1
	PriorityEKSCluster   = 2
	PriorityNATGateway   = 3
	PriorityLoadBalancer = 4
	PriorityRDS          = 5
	PriorityElastiCache  = 6
	PriorityOpenSearch   = 7
	PriorityRedshift     = 8
	PrioritySubnetGroup  = 9
	PriorityEC2          = 10
	PriorityEBS          = 11
	PrioritySnapshot     = 12
	PriorityAMI          = 13
	PriorityElasticIP    = 14
	PriorityLogGroup     = 15
	PrioritySageMaker    = 16
	PrioritySubnet       = 100
	PriorityRouteTable   = 101
	PriorityIGW          = 102
	PriorityVPC          = 103
)

// CleanupFunc represents a cleanup function with metadata.
type CleanupFunc struct {
	Name     string
	Priority int
	Fn       func(ctx context.Context) error
}

// CleanupRegistry tracks all created resources for guaranteed cleanup.
type CleanupRegistry struct {
	mu       sync.Mutex
	cleanups []CleanupFunc
}

// NewCleanupRegistry creates a new CleanupRegistry.
func NewCleanupRegistry() *CleanupRegistry {
	return &CleanupRegistry{}
}

// Register adds a cleanup function to the registry.
func (r *CleanupRegistry) Register(name string, priority int, fn func(ctx context.Context) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanups = append(r.cleanups, CleanupFunc{
		Name:     name,
		Priority: priority,
		Fn:       fn,
	})
}

// isNotFoundError checks if the error is an AWS "resource not found" error.
// These errors are expected during cleanup when tests explicitly delete resources.
func isNotFoundError(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		// AWS "not found" error patterns
		return strings.HasSuffix(code, ".NotFound") ||
			strings.HasSuffix(code, "NotFound") ||
			code == "ResourceNotFoundException"
	}
	return false
}

// RunAll executes all cleanups in priority order (lowest first), then LIFO within same priority.
// Returns all errors encountered (does not stop on first error).
func (r *CleanupRegistry) RunAll(ctx context.Context) []error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.cleanups) == 0 {
		return nil
	}

	// Sort by priority (ascending)
	sort.SliceStable(r.cleanups, func(i, j int) bool {
		return r.cleanups[i].Priority < r.cleanups[j].Priority
	})

	// Group by priority and reverse within each group (LIFO)
	var sorted []CleanupFunc
	currentPriority := r.cleanups[0].Priority
	currentGroup := make([]CleanupFunc, 0, len(r.cleanups))

	for _, c := range r.cleanups {
		if c.Priority != currentPriority {
			// Reverse current group and add to sorted
			for i := len(currentGroup) - 1; i >= 0; i-- {
				sorted = append(sorted, currentGroup[i])
			}
			currentGroup = nil
			currentPriority = c.Priority
		}
		currentGroup = append(currentGroup, c)
	}
	// Add last group
	for i := len(currentGroup) - 1; i >= 0; i-- {
		sorted = append(sorted, currentGroup[i])
	}

	var errs []error
	var succeeded, alreadyDeleted, failed int

	for _, c := range sorted {
		fmt.Printf("  Cleaning up: %s", c.Name)
		if err := c.Fn(ctx); err != nil {
			if isNotFoundError(err) {
				fmt.Printf(" (already deleted)\n")
				alreadyDeleted++
			} else {
				fmt.Printf(" FAILED\n")
				errs = append(errs, fmt.Errorf("cleanup %s: %w", c.Name, err))
				failed++
			}
		} else {
			fmt.Printf(" done\n")
			succeeded++
		}
	}

	fmt.Printf("\nCleanup complete: %d succeeded, %d already deleted, %d failed\n", succeeded, alreadyDeleted, failed)

	return errs
}

// Count returns the number of registered cleanup functions.
func (r *CleanupRegistry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.cleanups)
}
