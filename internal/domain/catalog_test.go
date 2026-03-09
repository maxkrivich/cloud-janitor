package domain_test

import (
	"testing"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

func TestResourceCatalog_AWSResources(t *testing.T) {
	catalog := domain.NewResourceCatalog()
	resources := catalog.AWSResources()

	if len(resources) != 8 {
		t.Errorf("expected 8 AWS resources, got %d", len(resources))
	}

	// Verify priority order (1 = highest priority)
	for i, r := range resources {
		expectedPriority := i + 1
		if r.Priority != expectedPriority {
			t.Errorf("resource %s: expected priority %d, got %d", r.Type, expectedPriority, r.Priority)
		}
		if r.Provider != domain.ProviderAWS {
			t.Errorf("resource %s: expected provider AWS, got %s", r.Type, r.Provider)
		}
	}

	// Verify first resource is EC2 (most expensive)
	if resources[0].Type != domain.ResourceTypeAWSEC2 {
		t.Errorf("expected first resource to be EC2, got %s", resources[0].Type)
	}
}

func TestResourceCatalog_GCPResources(t *testing.T) {
	catalog := domain.NewResourceCatalog()
	resources := catalog.GCPResources()

	if len(resources) != 5 {
		t.Errorf("expected 5 GCP resources, got %d", len(resources))
	}

	for _, r := range resources {
		if r.Provider != domain.ProviderGCP {
			t.Errorf("resource %s: expected provider GCP, got %s", r.Type, r.Provider)
		}
	}
}

func TestResourceCatalog_AzureResources(t *testing.T) {
	catalog := domain.NewResourceCatalog()
	resources := catalog.AzureResources()

	if len(resources) != 5 {
		t.Errorf("expected 5 Azure resources, got %d", len(resources))
	}

	for _, r := range resources {
		if r.Provider != domain.ProviderAzure {
			t.Errorf("resource %s: expected provider Azure, got %s", r.Type, r.Provider)
		}
	}
}

func TestResourceCatalog_AllResources(t *testing.T) {
	catalog := domain.NewResourceCatalog()
	all := catalog.AllResources()

	// 8 AWS + 5 GCP + 5 Azure = 18
	if len(all) != 18 {
		t.Errorf("expected 18 total resources, got %d", len(all))
	}
}

func TestResourceCatalog_ResourcesByProvider(t *testing.T) {
	catalog := domain.NewResourceCatalog()

	tests := []struct {
		provider domain.CloudProvider
		expected int
	}{
		{domain.ProviderAWS, 8},
		{domain.ProviderGCP, 5},
		{domain.ProviderAzure, 5},
		{domain.CloudProvider("unknown"), 0},
	}

	for _, tt := range tests {
		t.Run(string(tt.provider), func(t *testing.T) {
			resources := catalog.ResourcesByProvider(tt.provider)
			if len(resources) != tt.expected {
				t.Errorf("expected %d resources for %s, got %d", tt.expected, tt.provider, len(resources))
			}
		})
	}
}

func TestResourceCatalog_GetDefinition(t *testing.T) {
	catalog := domain.NewResourceCatalog()

	tests := []struct {
		resourceType   domain.ResourceType
		expectFound    bool
		expectPriority int
	}{
		{domain.ResourceTypeAWSEC2, true, 1},
		{domain.ResourceTypeAWSRDS, true, 2},
		{domain.ResourceTypeGCPInstance, true, 1},
		{domain.ResourceTypeAzureVM, true, 1},
		{domain.ResourceType("unknown:resource"), false, 0},
	}

	for _, tt := range tests {
		t.Run(string(tt.resourceType), func(t *testing.T) {
			def, found := catalog.GetDefinition(tt.resourceType)
			if found != tt.expectFound {
				t.Errorf("expected found=%v, got found=%v", tt.expectFound, found)
			}
			if found && def.Priority != tt.expectPriority {
				t.Errorf("expected priority %d, got %d", tt.expectPriority, def.Priority)
			}
		})
	}
}

func TestCostCategory_String(t *testing.T) {
	tests := []struct {
		category domain.CostCategory
		expected string
	}{
		{domain.CostCategoryCompute, "compute"},
		{domain.CostCategoryStorage, "storage"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.category.String(); got != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestCloudProvider_String(t *testing.T) {
	tests := []struct {
		provider domain.CloudProvider
		expected string
	}{
		{domain.ProviderAWS, "aws"},
		{domain.ProviderGCP, "gcp"},
		{domain.ProviderAzure, "azure"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.provider.String(); got != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestResourceCatalog_CostCategories(t *testing.T) {
	catalog := domain.NewResourceCatalog()

	// Verify compute resources
	computeTypes := []domain.ResourceType{
		domain.ResourceTypeAWSEC2,
		domain.ResourceTypeAWSRDS,
		domain.ResourceTypeAWSElasticIP,
		domain.ResourceTypeAWSELB,
	}

	for _, rt := range computeTypes {
		def, found := catalog.GetDefinition(rt)
		if !found {
			t.Errorf("resource %s not found in catalog", rt)
			continue
		}
		if def.CostCategory != domain.CostCategoryCompute {
			t.Errorf("resource %s: expected compute category, got %s", rt, def.CostCategory)
		}
	}

	// Verify storage resources
	storageTypes := []domain.ResourceType{
		domain.ResourceTypeAWSEBS,
		domain.ResourceTypeAWSSnapshot,
		domain.ResourceTypeAWSECR,
		domain.ResourceTypeAWSAMI,
	}

	for _, rt := range storageTypes {
		def, found := catalog.GetDefinition(rt)
		if !found {
			t.Errorf("resource %s not found in catalog", rt)
			continue
		}
		if def.CostCategory != domain.CostCategoryStorage {
			t.Errorf("resource %s: expected storage category, got %s", rt, def.CostCategory)
		}
	}
}
