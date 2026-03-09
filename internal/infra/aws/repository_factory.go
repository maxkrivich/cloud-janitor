package aws

import (
	"context"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// RepositoryFactory creates repositories for a specific account.
type RepositoryFactory struct {
	clientFactory *ClientFactory
	enabledTypes  map[domain.ResourceType]bool
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
		},
	}
}

// WithEnabledTypes sets which resource types are enabled.
func (f *RepositoryFactory) WithEnabledTypes(types map[domain.ResourceType]bool) *RepositoryFactory {
	f.enabledTypes = types
	return f
}

// CreateRepositories creates all enabled repositories for an account.
func (f *RepositoryFactory) CreateRepositories(ctx context.Context, account domain.Account, region string) ([]domain.ResourceRepository, error) {
	ec2Client, err := f.clientFactory.EC2Client(ctx, account, region)
	if err != nil {
		return nil, err
	}

	var repos []domain.ResourceRepository

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

	return repos, nil
}
