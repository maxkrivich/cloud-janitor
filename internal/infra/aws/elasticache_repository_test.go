package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticache/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check for mockElastiCacheClient.
var _ elastiCacheClient = (*mockElastiCacheClient)(nil)

// mockElastiCacheClient implements elastiCacheClient for testing.
type mockElastiCacheClient struct {
	// DescribeCacheClusters
	describeCacheClustersOutput *elasticache.DescribeCacheClustersOutput
	describeCacheClustersError  error
	describeCacheClustersPages  []*elasticache.DescribeCacheClustersOutput
	clusterPageIndex            int

	// DescribeReplicationGroups
	describeReplicationGroupsOutput *elasticache.DescribeReplicationGroupsOutput
	describeReplicationGroupsError  error
	describeReplicationGroupsPages  []*elasticache.DescribeReplicationGroupsOutput
	replicationGroupPageIndex       int

	// ListTagsForResource
	listTagsForResourceOutput  *elasticache.ListTagsForResourceOutput
	listTagsForResourceError   error
	listTagsForResourceOutputs map[string]*elasticache.ListTagsForResourceOutput // keyed by ARN
	listTagsForResourceInputs  []*elasticache.ListTagsForResourceInput

	// AddTagsToResource
	addTagsToResourceOutput *elasticache.AddTagsToResourceOutput
	addTagsToResourceError  error
	addTagsToResourceInput  *elasticache.AddTagsToResourceInput

	// DeleteCacheCluster
	deleteCacheClusterOutput *elasticache.DeleteCacheClusterOutput
	deleteCacheClusterError  error
	deleteCacheClusterInput  *elasticache.DeleteCacheClusterInput
	deleteCacheClusterCalled bool

	// DeleteReplicationGroup
	deleteReplicationGroupOutput *elasticache.DeleteReplicationGroupOutput
	deleteReplicationGroupError  error
	deleteReplicationGroupInput  *elasticache.DeleteReplicationGroupInput
	deleteReplicationGroupCalled bool
}

func (m *mockElastiCacheClient) DescribeCacheClusters(_ context.Context, _ *elasticache.DescribeCacheClustersInput, _ ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error) {
	if m.describeCacheClustersError != nil {
		return nil, m.describeCacheClustersError
	}
	if len(m.describeCacheClustersPages) > 0 {
		if m.clusterPageIndex >= len(m.describeCacheClustersPages) {
			return &elasticache.DescribeCacheClustersOutput{}, nil
		}
		result := m.describeCacheClustersPages[m.clusterPageIndex]
		m.clusterPageIndex++
		return result, nil
	}
	return m.describeCacheClustersOutput, nil
}

func (m *mockElastiCacheClient) DescribeReplicationGroups(_ context.Context, _ *elasticache.DescribeReplicationGroupsInput, _ ...func(*elasticache.Options)) (*elasticache.DescribeReplicationGroupsOutput, error) {
	if m.describeReplicationGroupsError != nil {
		return nil, m.describeReplicationGroupsError
	}
	if len(m.describeReplicationGroupsPages) > 0 {
		if m.replicationGroupPageIndex >= len(m.describeReplicationGroupsPages) {
			return &elasticache.DescribeReplicationGroupsOutput{}, nil
		}
		result := m.describeReplicationGroupsPages[m.replicationGroupPageIndex]
		m.replicationGroupPageIndex++
		return result, nil
	}
	return m.describeReplicationGroupsOutput, nil
}

func (m *mockElastiCacheClient) ListTagsForResource(_ context.Context, params *elasticache.ListTagsForResourceInput, _ ...func(*elasticache.Options)) (*elasticache.ListTagsForResourceOutput, error) {
	m.listTagsForResourceInputs = append(m.listTagsForResourceInputs, params)
	if m.listTagsForResourceError != nil {
		return nil, m.listTagsForResourceError
	}
	// Return specific output based on ARN if available
	if m.listTagsForResourceOutputs != nil {
		arn := aws.ToString(params.ResourceName)
		if output, ok := m.listTagsForResourceOutputs[arn]; ok {
			return output, nil
		}
	}
	return m.listTagsForResourceOutput, nil
}

func (m *mockElastiCacheClient) AddTagsToResource(_ context.Context, params *elasticache.AddTagsToResourceInput, _ ...func(*elasticache.Options)) (*elasticache.AddTagsToResourceOutput, error) {
	m.addTagsToResourceInput = params
	if m.addTagsToResourceError != nil {
		return nil, m.addTagsToResourceError
	}
	return m.addTagsToResourceOutput, nil
}

func (m *mockElastiCacheClient) DeleteCacheCluster(_ context.Context, params *elasticache.DeleteCacheClusterInput, _ ...func(*elasticache.Options)) (*elasticache.DeleteCacheClusterOutput, error) {
	m.deleteCacheClusterInput = params
	m.deleteCacheClusterCalled = true
	if m.deleteCacheClusterError != nil {
		return nil, m.deleteCacheClusterError
	}
	return m.deleteCacheClusterOutput, nil
}

func (m *mockElastiCacheClient) DeleteReplicationGroup(_ context.Context, params *elasticache.DeleteReplicationGroupInput, _ ...func(*elasticache.Options)) (*elasticache.DeleteReplicationGroupOutput, error) {
	m.deleteReplicationGroupInput = params
	m.deleteReplicationGroupCalled = true
	if m.deleteReplicationGroupError != nil {
		return nil, m.deleteReplicationGroupError
	}
	return m.deleteReplicationGroupOutput, nil
}

func TestElastiCacheRepository_Type(t *testing.T) {
	repo := &ElastiCacheRepository{
		client:    &mockElastiCacheClient{},
		accountID: "123456789012",
		region:    "us-east-1",
	}
	got := repo.Type()
	want := domain.ResourceTypeElastiCache

	if got != want {
		t.Errorf("Type() = %v, want %v", got, want)
	}
}

func TestElastiCacheRepository_List(t *testing.T) {
	now := time.Now()
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name      string
		mockSetup func() *mockElastiCacheClient
		accountID string
		region    string
		want      []domain.Resource
		wantErr   bool
		errMsg    string
	}{
		{
			name: "lists cache clusters successfully",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					describeCacheClustersOutput: &elasticache.DescribeCacheClustersOutput{
						CacheClusters: []types.CacheCluster{
							{
								CacheClusterId:     aws.String("my-redis-cluster"),
								CacheClusterStatus: aws.String("available"),
								ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:cluster:my-redis-cluster"),
							},
						},
					},
					describeReplicationGroupsOutput: &elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []types.ReplicationGroup{},
					},
					listTagsForResourceOutputs: map[string]*elasticache.ListTagsForResourceOutput{
						"arn:aws:elasticache:us-east-1:123456789012:cluster:my-redis-cluster": {
							TagList: []types.Tag{
								{Key: aws.String("Name"), Value: aws.String("My Redis")},
								{Key: aws.String("Environment"), Value: aws.String("dev")},
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:             "cluster:my-redis-cluster",
					Type:           domain.ResourceTypeElastiCache,
					Region:         "us-east-1",
					AccountID:      "123456789012",
					Name:           "My Redis",
					ExpirationDate: nil,
					NeverExpires:   false,
					Tags: map[string]string{
						"Name":        "My Redis",
						"Environment": "dev",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "lists replication groups successfully",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					describeCacheClustersOutput: &elasticache.DescribeCacheClustersOutput{
						CacheClusters: []types.CacheCluster{},
					},
					describeReplicationGroupsOutput: &elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []types.ReplicationGroup{
							{
								ReplicationGroupId: aws.String("my-redis-repl"),
								Status:             aws.String("available"),
								ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:replicationgroup:my-redis-repl"),
								Description:        aws.String("My Redis Replication Group"),
							},
						},
					},
					listTagsForResourceOutputs: map[string]*elasticache.ListTagsForResourceOutput{
						"arn:aws:elasticache:us-east-1:123456789012:replicationgroup:my-redis-repl": {
							TagList: []types.Tag{
								{Key: aws.String("Name"), Value: aws.String("My Redis Repl")},
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:             "replication-group:my-redis-repl",
					Type:           domain.ResourceTypeElastiCache,
					Region:         "us-east-1",
					AccountID:      "123456789012",
					Name:           "My Redis Repl",
					ExpirationDate: nil,
					NeverExpires:   false,
					Tags: map[string]string{
						"Name": "My Redis Repl",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "combines cache clusters and replication groups",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					describeCacheClustersOutput: &elasticache.DescribeCacheClustersOutput{
						CacheClusters: []types.CacheCluster{
							{
								CacheClusterId:     aws.String("standalone-cluster"),
								CacheClusterStatus: aws.String("available"),
								ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:cluster:standalone-cluster"),
							},
						},
					},
					describeReplicationGroupsOutput: &elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []types.ReplicationGroup{
							{
								ReplicationGroupId: aws.String("repl-group-1"),
								Status:             aws.String("available"),
								ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:replicationgroup:repl-group-1"),
							},
						},
					},
					listTagsForResourceOutputs: map[string]*elasticache.ListTagsForResourceOutput{
						"arn:aws:elasticache:us-east-1:123456789012:cluster:standalone-cluster": {
							TagList: []types.Tag{},
						},
						"arn:aws:elasticache:us-east-1:123456789012:replicationgroup:repl-group-1": {
							TagList: []types.Tag{},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "cluster:standalone-cluster",
					Type:      domain.ResourceTypeElastiCache,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
				{
					ID:        "replication-group:repl-group-1",
					Type:      domain.ResourceTypeElastiCache,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "parses expiration date tag",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					describeCacheClustersOutput: &elasticache.DescribeCacheClustersOutput{
						CacheClusters: []types.CacheCluster{
							{
								CacheClusterId:     aws.String("expiring-cluster"),
								CacheClusterStatus: aws.String("available"),
								ARN:                aws.String("arn:aws:elasticache:us-west-2:123456789012:cluster:expiring-cluster"),
							},
						},
					},
					describeReplicationGroupsOutput: &elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []types.ReplicationGroup{},
					},
					listTagsForResourceOutputs: map[string]*elasticache.ListTagsForResourceOutput{
						"arn:aws:elasticache:us-west-2:123456789012:cluster:expiring-cluster": {
							TagList: []types.Tag{
								{Key: aws.String(ExpirationTagName), Value: aws.String(expDateStr)},
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-west-2",
			want: []domain.Resource{
				{
					ID:        "cluster:expiring-cluster",
					Type:      domain.ResourceTypeElastiCache,
					Region:    "us-west-2",
					AccountID: "123456789012",
					Tags: map[string]string{
						ExpirationTagName: expDateStr,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "handles never expires tag",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					describeCacheClustersOutput: &elasticache.DescribeCacheClustersOutput{
						CacheClusters: []types.CacheCluster{
							{
								CacheClusterId:     aws.String("permanent-cluster"),
								CacheClusterStatus: aws.String("available"),
								ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:cluster:permanent-cluster"),
							},
						},
					},
					describeReplicationGroupsOutput: &elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []types.ReplicationGroup{},
					},
					listTagsForResourceOutputs: map[string]*elasticache.ListTagsForResourceOutput{
						"arn:aws:elasticache:us-east-1:123456789012:cluster:permanent-cluster": {
							TagList: []types.Tag{
								{Key: aws.String(ExpirationTagName), Value: aws.String(NeverExpiresValue)},
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:           "cluster:permanent-cluster",
					Type:         domain.ResourceTypeElastiCache,
					Region:       "us-east-1",
					AccountID:    "123456789012",
					NeverExpires: true,
					Tags: map[string]string{
						ExpirationTagName: NeverExpiresValue,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "filters invalid states for cache clusters",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					describeCacheClustersOutput: &elasticache.DescribeCacheClustersOutput{
						CacheClusters: []types.CacheCluster{
							{
								CacheClusterId:     aws.String("active-cluster"),
								CacheClusterStatus: aws.String("available"),
								ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:cluster:active-cluster"),
							},
							{
								CacheClusterId:     aws.String("deleting-cluster"),
								CacheClusterStatus: aws.String("deleting"),
								ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:cluster:deleting-cluster"),
							},
							{
								CacheClusterId:     aws.String("deleted-cluster"),
								CacheClusterStatus: aws.String("deleted"),
								ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:cluster:deleted-cluster"),
							},
							{
								CacheClusterId:     aws.String("failed-cluster"),
								CacheClusterStatus: aws.String("create-failed"),
								ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:cluster:failed-cluster"),
							},
						},
					},
					describeReplicationGroupsOutput: &elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []types.ReplicationGroup{},
					},
					listTagsForResourceOutputs: map[string]*elasticache.ListTagsForResourceOutput{
						"arn:aws:elasticache:us-east-1:123456789012:cluster:active-cluster": {
							TagList: []types.Tag{},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "cluster:active-cluster",
					Type:      domain.ResourceTypeElastiCache,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "filters invalid states for replication groups",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					describeCacheClustersOutput: &elasticache.DescribeCacheClustersOutput{
						CacheClusters: []types.CacheCluster{},
					},
					describeReplicationGroupsOutput: &elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []types.ReplicationGroup{
							{
								ReplicationGroupId: aws.String("active-repl"),
								Status:             aws.String("available"),
								ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:replicationgroup:active-repl"),
							},
							{
								ReplicationGroupId: aws.String("deleting-repl"),
								Status:             aws.String("deleting"),
								ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:replicationgroup:deleting-repl"),
							},
							{
								ReplicationGroupId: aws.String("failed-repl"),
								Status:             aws.String("create-failed"),
								ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:replicationgroup:failed-repl"),
							},
						},
					},
					listTagsForResourceOutputs: map[string]*elasticache.ListTagsForResourceOutput{
						"arn:aws:elasticache:us-east-1:123456789012:replicationgroup:active-repl": {
							TagList: []types.Tag{},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "replication-group:active-repl",
					Type:      domain.ResourceTypeElastiCache,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "handles API error for describe cache clusters",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					describeCacheClustersError: errors.New("API error"),
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   true,
			errMsg:    "listing ElastiCache clusters",
		},
		{
			name: "handles API error for describe replication groups",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					describeCacheClustersOutput: &elasticache.DescribeCacheClustersOutput{
						CacheClusters: []types.CacheCluster{},
					},
					describeReplicationGroupsError: errors.New("API error"),
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   true,
			errMsg:    "listing ElastiCache replication groups",
		},
		{
			name: "handles API error for list tags",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					describeCacheClustersOutput: &elasticache.DescribeCacheClustersOutput{
						CacheClusters: []types.CacheCluster{
							{
								CacheClusterId:     aws.String("my-cluster"),
								CacheClusterStatus: aws.String("available"),
								ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:cluster:my-cluster"),
							},
						},
					},
					describeReplicationGroupsOutput: &elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []types.ReplicationGroup{},
					},
					listTagsForResourceError: errors.New("tags API error"),
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   true,
			errMsg:    "listing tags for ElastiCache",
		},
		{
			name: "handles empty result",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					describeCacheClustersOutput: &elasticache.DescribeCacheClustersOutput{
						CacheClusters: []types.CacheCluster{},
					},
					describeReplicationGroupsOutput: &elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []types.ReplicationGroup{},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   false,
		},
		{
			name: "handles pagination for cache clusters",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					describeCacheClustersPages: []*elasticache.DescribeCacheClustersOutput{
						{
							CacheClusters: []types.CacheCluster{
								{
									CacheClusterId:     aws.String("cluster-page-1"),
									CacheClusterStatus: aws.String("available"),
									ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:cluster:cluster-page-1"),
								},
							},
							Marker: aws.String("next-page-token"),
						},
						{
							CacheClusters: []types.CacheCluster{
								{
									CacheClusterId:     aws.String("cluster-page-2"),
									CacheClusterStatus: aws.String("available"),
									ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:cluster:cluster-page-2"),
								},
							},
						},
					},
					describeReplicationGroupsOutput: &elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []types.ReplicationGroup{},
					},
					listTagsForResourceOutputs: map[string]*elasticache.ListTagsForResourceOutput{
						"arn:aws:elasticache:us-east-1:123456789012:cluster:cluster-page-1": {
							TagList: []types.Tag{},
						},
						"arn:aws:elasticache:us-east-1:123456789012:cluster:cluster-page-2": {
							TagList: []types.Tag{},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "cluster:cluster-page-1",
					Type:      domain.ResourceTypeElastiCache,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
				{
					ID:        "cluster:cluster-page-2",
					Type:      domain.ResourceTypeElastiCache,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "handles pagination for replication groups",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					describeCacheClustersOutput: &elasticache.DescribeCacheClustersOutput{
						CacheClusters: []types.CacheCluster{},
					},
					describeReplicationGroupsPages: []*elasticache.DescribeReplicationGroupsOutput{
						{
							ReplicationGroups: []types.ReplicationGroup{
								{
									ReplicationGroupId: aws.String("repl-page-1"),
									Status:             aws.String("available"),
									ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:replicationgroup:repl-page-1"),
								},
							},
							Marker: aws.String("next-page-token"),
						},
						{
							ReplicationGroups: []types.ReplicationGroup{
								{
									ReplicationGroupId: aws.String("repl-page-2"),
									Status:             aws.String("available"),
									ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:replicationgroup:repl-page-2"),
								},
							},
						},
					},
					listTagsForResourceOutputs: map[string]*elasticache.ListTagsForResourceOutput{
						"arn:aws:elasticache:us-east-1:123456789012:replicationgroup:repl-page-1": {
							TagList: []types.Tag{},
						},
						"arn:aws:elasticache:us-east-1:123456789012:replicationgroup:repl-page-2": {
							TagList: []types.Tag{},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "replication-group:repl-page-1",
					Type:      domain.ResourceTypeElastiCache,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
				{
					ID:        "replication-group:repl-page-2",
					Type:      domain.ResourceTypeElastiCache,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "skips clusters that are part of replication groups",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					describeCacheClustersOutput: &elasticache.DescribeCacheClustersOutput{
						CacheClusters: []types.CacheCluster{
							{
								CacheClusterId:     aws.String("standalone-cluster"),
								CacheClusterStatus: aws.String("available"),
								ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:cluster:standalone-cluster"),
								ReplicationGroupId: nil, // standalone
							},
							{
								CacheClusterId:     aws.String("repl-member-cluster"),
								CacheClusterStatus: aws.String("available"),
								ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:cluster:repl-member-cluster"),
								ReplicationGroupId: aws.String("my-repl-group"), // part of replication group
							},
						},
					},
					describeReplicationGroupsOutput: &elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []types.ReplicationGroup{
							{
								ReplicationGroupId: aws.String("my-repl-group"),
								Status:             aws.String("available"),
								ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:replicationgroup:my-repl-group"),
							},
						},
					},
					listTagsForResourceOutputs: map[string]*elasticache.ListTagsForResourceOutput{
						"arn:aws:elasticache:us-east-1:123456789012:cluster:standalone-cluster": {
							TagList: []types.Tag{},
						},
						"arn:aws:elasticache:us-east-1:123456789012:replicationgroup:my-repl-group": {
							TagList: []types.Tag{},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "cluster:standalone-cluster",
					Type:      domain.ResourceTypeElastiCache,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
				{
					ID:        "replication-group:my-repl-group",
					Type:      domain.ResourceTypeElastiCache,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &ElastiCacheRepository{
				client:    mock,
				accountID: tt.accountID,
				region:    tt.region,
			}

			got, err := repo.List(context.Background(), "")
			if (err != nil) != tt.wantErr {
				t.Errorf("List() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !containsString(err.Error(), tt.errMsg) {
					t.Errorf("List() error = %v, should contain %q", err, tt.errMsg)
				}
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("List() returned %d resources, want %d", len(got), len(tt.want))
				return
			}

			for i, resource := range got {
				if resource.ID != tt.want[i].ID {
					t.Errorf("List()[%d].ID = %v, want %v", i, resource.ID, tt.want[i].ID)
				}
				if resource.Type != tt.want[i].Type {
					t.Errorf("List()[%d].Type = %v, want %v", i, resource.Type, tt.want[i].Type)
				}
				if resource.Region != tt.want[i].Region {
					t.Errorf("List()[%d].Region = %v, want %v", i, resource.Region, tt.want[i].Region)
				}
				if resource.AccountID != tt.want[i].AccountID {
					t.Errorf("List()[%d].AccountID = %v, want %v", i, resource.AccountID, tt.want[i].AccountID)
				}
				if resource.NeverExpires != tt.want[i].NeverExpires {
					t.Errorf("List()[%d].NeverExpires = %v, want %v", i, resource.NeverExpires, tt.want[i].NeverExpires)
				}
				// Check Name only if expected
				if tt.want[i].Name != "" && resource.Name != tt.want[i].Name {
					t.Errorf("List()[%d].Name = %v, want %v", i, resource.Name, tt.want[i].Name)
				}
			}
		})
	}
}

func TestElastiCacheRepository_Tag(t *testing.T) {
	expDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		mockSetup      func() *mockElastiCacheClient
		accountID      string
		region         string
		resourceID     string
		expirationDate time.Time
		wantARN        string
		wantErr        bool
		errMsg         string
	}{
		{
			name: "tags cache cluster successfully",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					addTagsToResourceOutput: &elasticache.AddTagsToResourceOutput{},
				}
			},
			accountID:      "123456789012",
			region:         "us-east-1",
			resourceID:     "cluster:my-redis-cluster",
			expirationDate: expDate,
			wantARN:        "arn:aws:elasticache:us-east-1:123456789012:cluster:my-redis-cluster",
			wantErr:        false,
		},
		{
			name: "tags replication group successfully",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					addTagsToResourceOutput: &elasticache.AddTagsToResourceOutput{},
				}
			},
			accountID:      "123456789012",
			region:         "us-east-1",
			resourceID:     "replication-group:my-redis-repl",
			expirationDate: expDate,
			wantARN:        "arn:aws:elasticache:us-east-1:123456789012:replicationgroup:my-redis-repl",
			wantErr:        false,
		},
		{
			name: "constructs correct ARN for different region",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					addTagsToResourceOutput: &elasticache.AddTagsToResourceOutput{},
				}
			},
			accountID:      "987654321098",
			region:         "eu-west-1",
			resourceID:     "cluster:prod-cache",
			expirationDate: expDate,
			wantARN:        "arn:aws:elasticache:eu-west-1:987654321098:cluster:prod-cache",
			wantErr:        false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					addTagsToResourceError: errors.New("access denied"),
				}
			},
			accountID:      "123456789012",
			region:         "us-east-1",
			resourceID:     "cluster:my-cluster",
			expirationDate: expDate,
			wantErr:        true,
			errMsg:         "tagging ElastiCache",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &ElastiCacheRepository{
				client:    mock,
				accountID: tt.accountID,
				region:    tt.region,
			}

			err := repo.Tag(context.Background(), tt.resourceID, tt.expirationDate)
			if (err != nil) != tt.wantErr {
				t.Errorf("Tag() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !containsString(err.Error(), tt.errMsg) {
					t.Errorf("Tag() error = %v, should contain %q", err, tt.errMsg)
				}
				return
			}

			if !tt.wantErr {
				// Verify the ARN was constructed correctly
				if mock.addTagsToResourceInput == nil {
					t.Fatal("AddTagsToResource was not called")
				}
				if aws.ToString(mock.addTagsToResourceInput.ResourceName) != tt.wantARN {
					t.Errorf("Tag() ARN = %v, want %v", aws.ToString(mock.addTagsToResourceInput.ResourceName), tt.wantARN)
				}

				// Verify the tag was set correctly
				if len(mock.addTagsToResourceInput.Tags) != 1 {
					t.Errorf("Tag() set %d tags, want 1", len(mock.addTagsToResourceInput.Tags))
				}
				if len(mock.addTagsToResourceInput.Tags) > 0 {
					tag := mock.addTagsToResourceInput.Tags[0]
					if aws.ToString(tag.Key) != ExpirationTagName {
						t.Errorf("Tag() key = %v, want %v", aws.ToString(tag.Key), ExpirationTagName)
					}
					wantValue := tt.expirationDate.Format(ExpirationDateFormat)
					if aws.ToString(tag.Value) != wantValue {
						t.Errorf("Tag() value = %v, want %v", aws.ToString(tag.Value), wantValue)
					}
				}
			}
		})
	}
}

func TestElastiCacheRepository_Delete(t *testing.T) {
	tests := []struct {
		name                    string
		mockSetup               func() *mockElastiCacheClient
		resourceID              string
		wantDeleteClusterCalled bool
		wantDeleteReplGrpCalled bool
		wantDeletedClusterID    string
		wantDeletedReplGroupID  string
		wantErr                 bool
		errMsg                  string
	}{
		{
			name: "deletes cache cluster successfully",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					deleteCacheClusterOutput: &elasticache.DeleteCacheClusterOutput{},
				}
			},
			resourceID:              "cluster:my-redis-cluster",
			wantDeleteClusterCalled: true,
			wantDeleteReplGrpCalled: false,
			wantDeletedClusterID:    "my-redis-cluster",
			wantErr:                 false,
		},
		{
			name: "deletes replication group successfully",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					deleteReplicationGroupOutput: &elasticache.DeleteReplicationGroupOutput{},
				}
			},
			resourceID:              "replication-group:my-redis-repl",
			wantDeleteClusterCalled: false,
			wantDeleteReplGrpCalled: true,
			wantDeletedReplGroupID:  "my-redis-repl",
			wantErr:                 false,
		},
		{
			name: "handles API error for cache cluster deletion",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					deleteCacheClusterError: errors.New("cannot delete"),
				}
			},
			resourceID:              "cluster:my-cluster",
			wantDeleteClusterCalled: true,
			wantDeleteReplGrpCalled: false,
			wantErr:                 true,
			errMsg:                  "deleting ElastiCache cluster",
		},
		{
			name: "handles API error for replication group deletion",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{
					deleteReplicationGroupError: errors.New("cannot delete"),
				}
			},
			resourceID:              "replication-group:my-repl",
			wantDeleteClusterCalled: false,
			wantDeleteReplGrpCalled: true,
			wantErr:                 true,
			errMsg:                  "deleting ElastiCache replication group",
		},
		{
			name: "handles unknown resource type prefix",
			mockSetup: func() *mockElastiCacheClient {
				return &mockElastiCacheClient{}
			},
			resourceID:              "unknown:some-id",
			wantDeleteClusterCalled: false,
			wantDeleteReplGrpCalled: false,
			wantErr:                 true,
			errMsg:                  "unknown ElastiCache resource type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &ElastiCacheRepository{
				client:    mock,
				accountID: "123456789012",
				region:    "us-east-1",
			}

			err := repo.Delete(context.Background(), tt.resourceID)
			if (err != nil) != tt.wantErr {
				t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !containsString(err.Error(), tt.errMsg) {
					t.Errorf("Delete() error = %v, should contain %q", err, tt.errMsg)
				}
				return
			}

			// Verify correct API was called
			if mock.deleteCacheClusterCalled != tt.wantDeleteClusterCalled {
				t.Errorf("DeleteCacheCluster called = %v, want %v", mock.deleteCacheClusterCalled, tt.wantDeleteClusterCalled)
			}
			if mock.deleteReplicationGroupCalled != tt.wantDeleteReplGrpCalled {
				t.Errorf("DeleteReplicationGroup called = %v, want %v", mock.deleteReplicationGroupCalled, tt.wantDeleteReplGrpCalled)
			}

			// Verify cluster ID was passed correctly
			if tt.wantDeleteClusterCalled && mock.deleteCacheClusterInput != nil {
				gotID := aws.ToString(mock.deleteCacheClusterInput.CacheClusterId)
				if gotID != tt.wantDeletedClusterID {
					t.Errorf("Delete() CacheClusterId = %v, want %v", gotID, tt.wantDeletedClusterID)
				}
				// Verify FinalSnapshotIdentifier is not set (skip final snapshot)
				if mock.deleteCacheClusterInput.FinalSnapshotIdentifier != nil {
					t.Error("Delete() should not set FinalSnapshotIdentifier for cache cluster")
				}
			}

			// Verify replication group ID was passed correctly
			if tt.wantDeleteReplGrpCalled && mock.deleteReplicationGroupInput != nil {
				gotID := aws.ToString(mock.deleteReplicationGroupInput.ReplicationGroupId)
				if gotID != tt.wantDeletedReplGroupID {
					t.Errorf("Delete() ReplicationGroupId = %v, want %v", gotID, tt.wantDeletedReplGroupID)
				}
				// Verify FinalSnapshotIdentifier is not set (skip final snapshot)
				if mock.deleteReplicationGroupInput.FinalSnapshotIdentifier != nil {
					t.Error("Delete() should not set FinalSnapshotIdentifier for replication group")
				}
			}
		})
	}
}

func TestElastiCacheRepository_InterfaceCompliance(_ *testing.T) {
	// Verify ElastiCacheRepository implements domain.ResourceRepository
	var _ domain.ResourceRepository = (*ElastiCacheRepository)(nil)
}

func TestNewElastiCacheRepository(t *testing.T) {
	tests := []struct {
		name      string
		accountID string
		region    string
	}{
		{
			name:      "creates repository with all fields",
			accountID: "123456789012",
			region:    "us-east-1",
		},
		{
			name:      "creates repository with different region",
			accountID: "987654321098",
			region:    "eu-west-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a nil client since we're just testing field assignment
			repo := NewElastiCacheRepository(nil, tt.accountID, tt.region)

			if repo == nil {
				t.Fatal("NewElastiCacheRepository() returned nil")
			}
			if repo.accountID != tt.accountID {
				t.Errorf("accountID = %v, want %v", repo.accountID, tt.accountID)
			}
			if repo.region != tt.region {
				t.Errorf("region = %v, want %v", repo.region, tt.region)
			}
		})
	}
}

func TestElastiCacheRepository_buildARN(t *testing.T) {
	tests := []struct {
		name       string
		accountID  string
		region     string
		resourceID string
		want       string
	}{
		{
			name:       "builds ARN for cache cluster",
			accountID:  "123456789012",
			region:     "us-east-1",
			resourceID: "cluster:my-redis-cluster",
			want:       "arn:aws:elasticache:us-east-1:123456789012:cluster:my-redis-cluster",
		},
		{
			name:       "builds ARN for replication group",
			accountID:  "123456789012",
			region:     "us-east-1",
			resourceID: "replication-group:my-redis-repl",
			want:       "arn:aws:elasticache:us-east-1:123456789012:replicationgroup:my-redis-repl",
		},
		{
			name:       "builds ARN for different region",
			accountID:  "987654321098",
			region:     "eu-central-1",
			resourceID: "cluster:prod-cache-1",
			want:       "arn:aws:elasticache:eu-central-1:987654321098:cluster:prod-cache-1",
		},
		{
			name:       "handles cluster with hyphens",
			accountID:  "111222333444",
			region:     "ap-southeast-1",
			resourceID: "cluster:my-complex-cluster-name",
			want:       "arn:aws:elasticache:ap-southeast-1:111222333444:cluster:my-complex-cluster-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &ElastiCacheRepository{
				accountID: tt.accountID,
				region:    tt.region,
			}
			got := repo.buildARN(tt.resourceID)
			if got != tt.want {
				t.Errorf("buildARN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestElastiCacheRepository_isValidClusterState(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "available is valid", status: "available", want: true},
		{name: "creating is valid", status: "creating", want: true},
		{name: "modifying is valid", status: "modifying", want: true},
		{name: "snapshotting is valid", status: "snapshotting", want: true},
		{name: "deleting is invalid", status: "deleting", want: false},
		{name: "deleted is invalid", status: "deleted", want: false},
		{name: "create-failed is invalid", status: "create-failed", want: false},
		{name: "incompatible-network is invalid", status: "incompatible-network", want: false},
		{name: "restore-failed is invalid", status: "restore-failed", want: false},
	}

	repo := &ElastiCacheRepository{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.isValidClusterState(tt.status)
			if got != tt.want {
				t.Errorf("isValidClusterState(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestElastiCacheRepository_isValidReplicationGroupState(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "available is valid", status: "available", want: true},
		{name: "creating is valid", status: "creating", want: true},
		{name: "modifying is valid", status: "modifying", want: true},
		{name: "snapshotting is valid", status: "snapshotting", want: true},
		{name: "deleting is invalid", status: "deleting", want: false},
		{name: "create-failed is invalid", status: "create-failed", want: false},
	}

	repo := &ElastiCacheRepository{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.isValidReplicationGroupState(tt.status)
			if got != tt.want {
				t.Errorf("isValidReplicationGroupState(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestElastiCacheRepository_parseResourceID(t *testing.T) {
	tests := []struct {
		name       string
		resourceID string
		wantType   string
		wantID     string
		wantErr    bool
	}{
		{
			name:       "parses cluster ID",
			resourceID: "cluster:my-redis-cluster",
			wantType:   "cluster",
			wantID:     "my-redis-cluster",
			wantErr:    false,
		},
		{
			name:       "parses replication group ID",
			resourceID: "replication-group:my-redis-repl",
			wantType:   "replication-group",
			wantID:     "my-redis-repl",
			wantErr:    false,
		},
		{
			name:       "handles ID with multiple colons",
			resourceID: "cluster:my-cluster:with:colons",
			wantType:   "cluster",
			wantID:     "my-cluster:with:colons",
			wantErr:    false,
		},
		{
			name:       "returns error for invalid format",
			resourceID: "invalid-no-colon",
			wantType:   "",
			wantID:     "",
			wantErr:    true,
		},
	}

	repo := &ElastiCacheRepository{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotID, err := repo.parseResourceID(tt.resourceID)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseResourceID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotType != tt.wantType {
				t.Errorf("parseResourceID() type = %v, want %v", gotType, tt.wantType)
			}
			if gotID != tt.wantID {
				t.Errorf("parseResourceID() id = %v, want %v", gotID, tt.wantID)
			}
		})
	}
}
