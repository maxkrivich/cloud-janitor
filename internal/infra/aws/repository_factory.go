package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// RepositoryFactory creates repositories for a specific account.
type RepositoryFactory struct {
	clientFactory        *ClientFactory
	enabledTypes         map[domain.ResourceType]bool
	forceDeleteProtected bool
	eksCascadeDelete     bool
	logsSkipPatterns     []string
}

// NewRepositoryFactory creates a new RepositoryFactory.
func NewRepositoryFactory(clientFactory *ClientFactory) *RepositoryFactory {
	return &RepositoryFactory{
		clientFactory: clientFactory,
		enabledTypes: map[domain.ResourceType]bool{
			domain.ResourceTypeEC2:         true,
			domain.ResourceTypeEBS:         true,
			domain.ResourceTypeEBSSnapshot: true,
			domain.ResourceTypeElasticIP:   true,
			domain.ResourceTypeRDS:         true,
			domain.ResourceTypeELB:         true,
			domain.ResourceTypeNATGateway:  true,
			domain.ResourceTypeElastiCache: true,
			domain.ResourceTypeOpenSearch:  true,
			domain.ResourceTypeEKS:         true,
			domain.ResourceTypeRedshift:    true,
			domain.ResourceTypeSageMaker:   true,
			domain.ResourceTypeAMI:         true,
			domain.ResourceTypeLogs:        true,
		},
		logsSkipPatterns: DefaultLogsSkipPatterns,
	}
}

// WithEnabledTypes sets which resource types are enabled.
func (f *RepositoryFactory) WithEnabledTypes(types map[domain.ResourceType]bool) *RepositoryFactory {
	f.enabledTypes = types
	return f
}

// WithForceDeleteProtected sets whether to force delete protected resources (e.g., RDS with deletion protection).
func (f *RepositoryFactory) WithForceDeleteProtected(force bool) *RepositoryFactory {
	f.forceDeleteProtected = force
	return f
}

// WithEKSCascadeDelete sets whether to cascade delete EKS node groups before deleting clusters.
func (f *RepositoryFactory) WithEKSCascadeDelete(cascade bool) *RepositoryFactory {
	f.eksCascadeDelete = cascade
	return f
}

// WithLogsSkipPatterns sets the patterns for log groups to skip.
func (f *RepositoryFactory) WithLogsSkipPatterns(patterns []string) *RepositoryFactory {
	f.logsSkipPatterns = patterns
	return f
}

// CreateRepositories creates all enabled repositories for an account.
func (f *RepositoryFactory) CreateRepositories(ctx context.Context, account domain.Account, region string) ([]domain.ResourceRepository, error) {
	var repos []domain.ResourceRepository

	// Create EC2 client if any EC2-based resource types are enabled
	needsEC2 := f.enabledTypes[domain.ResourceTypeEC2] ||
		f.enabledTypes[domain.ResourceTypeEBS] ||
		f.enabledTypes[domain.ResourceTypeEBSSnapshot] ||
		f.enabledTypes[domain.ResourceTypeElasticIP] ||
		f.enabledTypes[domain.ResourceTypeNATGateway] ||
		f.enabledTypes[domain.ResourceTypeAMI]

	var ec2Client *ec2.Client
	if needsEC2 {
		var err error
		ec2Client, err = f.clientFactory.EC2Client(ctx, account, region)
		if err != nil {
			return nil, err
		}
	}

	// EC2-based repositories
	if f.enabledTypes[domain.ResourceTypeEC2] {
		repos = append(repos, NewEC2Repository(ec2Client, account.ID, region))
	}

	if f.enabledTypes[domain.ResourceTypeEBS] {
		repos = append(repos, NewEBSRepository(ec2Client, account.ID, region))
	}

	if f.enabledTypes[domain.ResourceTypeEBSSnapshot] {
		repos = append(repos, NewSnapshotRepository(ec2Client, account.ID, region))
	}

	if f.enabledTypes[domain.ResourceTypeElasticIP] {
		repos = append(repos, NewElasticIPRepository(ec2Client, account.ID, region))
	}

	if f.enabledTypes[domain.ResourceTypeNATGateway] {
		repos = append(repos, NewNATGatewayRepository(ec2Client, account.ID, region))
	}

	if f.enabledTypes[domain.ResourceTypeAMI] {
		repos = append(repos, NewAMIRepository(ec2Client, account.ID, region))
	}

	// RDS repository
	if f.enabledTypes[domain.ResourceTypeRDS] {
		rdsClient, err := f.clientFactory.RDSClient(ctx, account, region)
		if err != nil {
			return nil, err
		}
		repos = append(repos, NewRDSRepository(rdsClient, account.ID, region, f.forceDeleteProtected))
	}

	// ELB repository
	if f.enabledTypes[domain.ResourceTypeELB] {
		elbClient, err := f.clientFactory.ELBv2Client(ctx, account, region)
		if err != nil {
			return nil, err
		}
		repos = append(repos, NewELBRepository(elbClient, account.ID, region))
	}

	// OpenSearch repository
	if f.enabledTypes[domain.ResourceTypeOpenSearch] {
		openSearchClient, err := f.clientFactory.OpenSearchClient(ctx, account, region)
		if err != nil {
			return nil, err
		}
		repos = append(repos, NewOpenSearchRepository(openSearchClient, account.ID, region))
	}

	// ElastiCache repository
	if f.enabledTypes[domain.ResourceTypeElastiCache] {
		elastiCacheClient, err := f.clientFactory.ElastiCacheClient(ctx, account, region)
		if err != nil {
			return nil, err
		}
		repos = append(repos, NewElastiCacheRepository(elastiCacheClient, account.ID, region))
	}

	// EKS repository
	if f.enabledTypes[domain.ResourceTypeEKS] {
		eksClient, err := f.clientFactory.EKSClient(ctx, account, region)
		if err != nil {
			return nil, err
		}
		repos = append(repos, NewEKSRepository(eksClient, account.ID, region, f.eksCascadeDelete))
	}

	// Redshift repository
	if f.enabledTypes[domain.ResourceTypeRedshift] {
		redshiftClient, err := f.clientFactory.RedshiftClient(ctx, account, region)
		if err != nil {
			return nil, err
		}
		repos = append(repos, NewRedshiftRepository(redshiftClient, account.ID, region))
	}

	// SageMaker repository
	if f.enabledTypes[domain.ResourceTypeSageMaker] {
		sagemakerClient, err := f.clientFactory.SageMakerClient(ctx, account, region)
		if err != nil {
			return nil, err
		}
		repos = append(repos, NewSageMakerRepository(sagemakerClient, account.ID, region))
	}

	// CloudWatch Logs repository
	if f.enabledTypes[domain.ResourceTypeLogs] {
		logsClient, err := f.clientFactory.CloudWatchLogsClient(ctx, account, region)
		if err != nil {
			return nil, err
		}
		repos = append(repos, NewLogsRepository(logsClient, account.ID, region, f.logsSkipPatterns))
	}

	return repos, nil
}
