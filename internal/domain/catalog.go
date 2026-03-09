package domain

// CostCategory indicates how the resource is billed.
type CostCategory string

const (
	// CostCategoryCompute represents resources charged hourly, even when idle.
	CostCategoryCompute CostCategory = "compute"
	// CostCategoryStorage represents resources charged monthly per GB.
	CostCategoryStorage CostCategory = "storage"
)

// String returns the string representation of the cost category.
func (c CostCategory) String() string {
	return string(c)
}

// ResourceDefinition describes a cloud resource type with its cost characteristics.
type ResourceDefinition struct {
	// Type is the resource type identifier.
	Type ResourceType
	// Provider is the cloud provider (aws, gcp, azure).
	Provider CloudProvider
	// CostCategory indicates how the resource is billed.
	CostCategory CostCategory
	// Priority indicates cleanup importance (1 = highest priority, most expensive).
	Priority int
	// Description is a human-readable description.
	Description string
}

// ResourceCatalog provides the prioritized list of supported resources.
// Resources are ordered by cost impact to maximize savings with 80/20 effort.
type ResourceCatalog struct{}

// NewResourceCatalog creates a new resource catalog.
func NewResourceCatalog() *ResourceCatalog {
	return &ResourceCatalog{}
}

// AWSResources returns AWS resources ordered by cost priority (most expensive first).
func (c *ResourceCatalog) AWSResources() []ResourceDefinition {
	return []ResourceDefinition{
		{
			Type:         ResourceTypeAWSEC2,
			Provider:     ProviderAWS,
			CostCategory: CostCategoryCompute,
			Priority:     1,
			Description:  "EC2 Instances",
		},
		{
			Type:         ResourceTypeAWSRDS,
			Provider:     ProviderAWS,
			CostCategory: CostCategoryCompute,
			Priority:     2,
			Description:  "RDS Instances",
		},
		{
			Type:         ResourceTypeAWSElasticIP,
			Provider:     ProviderAWS,
			CostCategory: CostCategoryCompute,
			Priority:     3,
			Description:  "Elastic IPs (unattached)",
		},
		{
			Type:         ResourceTypeAWSEBS,
			Provider:     ProviderAWS,
			CostCategory: CostCategoryStorage,
			Priority:     4,
			Description:  "EBS Volumes",
		},
		{
			Type:         ResourceTypeAWSELB,
			Provider:     ProviderAWS,
			CostCategory: CostCategoryCompute,
			Priority:     5,
			Description:  "Load Balancers (ALB/NLB)",
		},
		{
			Type:         ResourceTypeAWSSnapshot,
			Provider:     ProviderAWS,
			CostCategory: CostCategoryStorage,
			Priority:     6,
			Description:  "EBS Snapshots",
		},
		{
			Type:         ResourceTypeAWSECR,
			Provider:     ProviderAWS,
			CostCategory: CostCategoryStorage,
			Priority:     7,
			Description:  "ECR Images",
		},
		{
			Type:         ResourceTypeAWSAMI,
			Provider:     ProviderAWS,
			CostCategory: CostCategoryStorage,
			Priority:     8,
			Description:  "AMIs",
		},
	}
}

// GCPResources returns GCP resources ordered by cost priority (most expensive first).
func (c *ResourceCatalog) GCPResources() []ResourceDefinition {
	return []ResourceDefinition{
		{
			Type:         ResourceTypeGCPInstance,
			Provider:     ProviderGCP,
			CostCategory: CostCategoryCompute,
			Priority:     1,
			Description:  "Compute Instances",
		},
		{
			Type:         ResourceTypeGCPCloudSQL,
			Provider:     ProviderGCP,
			CostCategory: CostCategoryCompute,
			Priority:     2,
			Description:  "Cloud SQL Instances",
		},
		{
			Type:         ResourceTypeGCPStaticIP,
			Provider:     ProviderGCP,
			CostCategory: CostCategoryCompute,
			Priority:     3,
			Description:  "Static IPs (unattached)",
		},
		{
			Type:         ResourceTypeGCPDisk,
			Provider:     ProviderGCP,
			CostCategory: CostCategoryStorage,
			Priority:     4,
			Description:  "Persistent Disks",
		},
		{
			Type:         ResourceTypeGCPSnapshot,
			Provider:     ProviderGCP,
			CostCategory: CostCategoryStorage,
			Priority:     5,
			Description:  "Snapshots",
		},
	}
}

// AzureResources returns Azure resources ordered by cost priority (most expensive first).
func (c *ResourceCatalog) AzureResources() []ResourceDefinition {
	return []ResourceDefinition{
		{
			Type:         ResourceTypeAzureVM,
			Provider:     ProviderAzure,
			CostCategory: CostCategoryCompute,
			Priority:     1,
			Description:  "Virtual Machines",
		},
		{
			Type:         ResourceTypeAzureSQL,
			Provider:     ProviderAzure,
			CostCategory: CostCategoryCompute,
			Priority:     2,
			Description:  "Azure SQL",
		},
		{
			Type:         ResourceTypeAzurePublicIP,
			Provider:     ProviderAzure,
			CostCategory: CostCategoryCompute,
			Priority:     3,
			Description:  "Public IPs (unassociated)",
		},
		{
			Type:         ResourceTypeAzureDisk,
			Provider:     ProviderAzure,
			CostCategory: CostCategoryStorage,
			Priority:     4,
			Description:  "Managed Disks",
		},
		{
			Type:         ResourceTypeAzureSnapshot,
			Provider:     ProviderAzure,
			CostCategory: CostCategoryStorage,
			Priority:     5,
			Description:  "Snapshots",
		},
	}
}

// AllResources returns all resources across all providers.
func (c *ResourceCatalog) AllResources() []ResourceDefinition {
	var all []ResourceDefinition
	all = append(all, c.AWSResources()...)
	all = append(all, c.GCPResources()...)
	all = append(all, c.AzureResources()...)
	return all
}

// ResourcesByProvider returns resources for a specific provider.
func (c *ResourceCatalog) ResourcesByProvider(provider CloudProvider) []ResourceDefinition {
	switch provider {
	case ProviderAWS:
		return c.AWSResources()
	case ProviderGCP:
		return c.GCPResources()
	case ProviderAzure:
		return c.AzureResources()
	default:
		return nil
	}
}

// GetDefinition returns the definition for a specific resource type.
func (c *ResourceCatalog) GetDefinition(resourceType ResourceType) (ResourceDefinition, bool) {
	for _, def := range c.AllResources() {
		if def.Type == resourceType {
			return def, true
		}
	}
	return ResourceDefinition{}, false
}
