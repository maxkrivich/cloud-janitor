package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check for mockEKSClient.
var _ eksClient = (*mockEKSClient)(nil)

// mockEKSClient implements eksClient for testing.
type mockEKSClient struct {
	// ListClusters
	listClustersOutput *eks.ListClustersOutput
	listClustersError  error
	listClustersPages  []*eks.ListClustersOutput
	listClustersIndex  int

	// DescribeCluster
	describeClusterOutputs map[string]*eks.DescribeClusterOutput
	describeClusterError   error
	describeClusterCalls   []string

	// TagResource
	tagResourceOutput *eks.TagResourceOutput
	tagResourceError  error
	tagResourceInput  *eks.TagResourceInput

	// ListNodegroups
	listNodegroupsOutputs map[string]*eks.ListNodegroupsOutput
	listNodegroupsError   error

	// DeleteNodegroup
	deleteNodegroupOutput *eks.DeleteNodegroupOutput
	deleteNodegroupError  error
	deleteNodegroupCalls  []string

	// DeleteCluster
	deleteClusterOutput *eks.DeleteClusterOutput
	deleteClusterError  error
	deleteClusterInput  *eks.DeleteClusterInput
	deleteClusterCalled bool
}

func (m *mockEKSClient) ListClusters(_ context.Context, _ *eks.ListClustersInput, _ ...func(*eks.Options)) (*eks.ListClustersOutput, error) {
	if m.listClustersError != nil {
		return nil, m.listClustersError
	}
	if len(m.listClustersPages) > 0 {
		if m.listClustersIndex >= len(m.listClustersPages) {
			return &eks.ListClustersOutput{}, nil
		}
		result := m.listClustersPages[m.listClustersIndex]
		m.listClustersIndex++
		return result, nil
	}
	return m.listClustersOutput, nil
}

func (m *mockEKSClient) DescribeCluster(_ context.Context, params *eks.DescribeClusterInput, _ ...func(*eks.Options)) (*eks.DescribeClusterOutput, error) {
	m.describeClusterCalls = append(m.describeClusterCalls, aws.ToString(params.Name))
	if m.describeClusterError != nil {
		return nil, m.describeClusterError
	}
	if m.describeClusterOutputs != nil {
		if output, ok := m.describeClusterOutputs[aws.ToString(params.Name)]; ok {
			return output, nil
		}
	}
	return nil, errors.New("cluster not found")
}

func (m *mockEKSClient) TagResource(_ context.Context, params *eks.TagResourceInput, _ ...func(*eks.Options)) (*eks.TagResourceOutput, error) {
	m.tagResourceInput = params
	if m.tagResourceError != nil {
		return nil, m.tagResourceError
	}
	return m.tagResourceOutput, nil
}

func (m *mockEKSClient) ListNodegroups(_ context.Context, params *eks.ListNodegroupsInput, _ ...func(*eks.Options)) (*eks.ListNodegroupsOutput, error) {
	if m.listNodegroupsError != nil {
		return nil, m.listNodegroupsError
	}
	if m.listNodegroupsOutputs != nil {
		if output, ok := m.listNodegroupsOutputs[aws.ToString(params.ClusterName)]; ok {
			return output, nil
		}
	}
	return &eks.ListNodegroupsOutput{Nodegroups: []string{}}, nil
}

func (m *mockEKSClient) DeleteNodegroup(_ context.Context, params *eks.DeleteNodegroupInput, _ ...func(*eks.Options)) (*eks.DeleteNodegroupOutput, error) {
	m.deleteNodegroupCalls = append(m.deleteNodegroupCalls, aws.ToString(params.NodegroupName))
	if m.deleteNodegroupError != nil {
		return nil, m.deleteNodegroupError
	}
	return m.deleteNodegroupOutput, nil
}

func (m *mockEKSClient) DeleteCluster(_ context.Context, params *eks.DeleteClusterInput, _ ...func(*eks.Options)) (*eks.DeleteClusterOutput, error) {
	m.deleteClusterInput = params
	m.deleteClusterCalled = true
	if m.deleteClusterError != nil {
		return nil, m.deleteClusterError
	}
	return m.deleteClusterOutput, nil
}

func TestEKSRepository_Type(t *testing.T) {
	repo := &EKSRepository{
		client:    &mockEKSClient{},
		accountID: "123456789012",
		region:    "us-east-1",
	}
	got := repo.Type()
	want := domain.ResourceTypeEKS

	if got != want {
		t.Errorf("Type() = %v, want %v", got, want)
	}
}

func TestEKSRepository_List(t *testing.T) {
	now := time.Now()
	createdAt := now.AddDate(0, -1, 0)
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name      string
		mockSetup func() *mockEKSClient
		accountID string
		region    string
		want      []domain.Resource
		wantErr   bool
		errMsg    string
	}{
		{
			name: "lists clusters successfully",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					listClustersOutput: &eks.ListClustersOutput{
						Clusters: []string{"my-cluster"},
					},
					describeClusterOutputs: map[string]*eks.DescribeClusterOutput{
						"my-cluster": {
							Cluster: &types.Cluster{
								Name:      aws.String("my-cluster"),
								Arn:       aws.String("arn:aws:eks:us-east-1:123456789012:cluster/my-cluster"),
								Status:    types.ClusterStatusActive,
								CreatedAt: &createdAt,
								Tags: map[string]string{
									"Name":        "Production Cluster",
									"Environment": "prod",
								},
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:             "my-cluster",
					Type:           domain.ResourceTypeEKS,
					Region:         "us-east-1",
					AccountID:      "123456789012",
					Name:           "Production Cluster",
					ExpirationDate: nil,
					NeverExpires:   false,
					Tags: map[string]string{
						"Name":        "Production Cluster",
						"Environment": "prod",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "parses expiration date tag",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					listClustersOutput: &eks.ListClustersOutput{
						Clusters: []string{"expiring-cluster"},
					},
					describeClusterOutputs: map[string]*eks.DescribeClusterOutput{
						"expiring-cluster": {
							Cluster: &types.Cluster{
								Name:   aws.String("expiring-cluster"),
								Arn:    aws.String("arn:aws:eks:us-west-2:123456789012:cluster/expiring-cluster"),
								Status: types.ClusterStatusActive,
								Tags: map[string]string{
									ExpirationTagName: expDateStr,
								},
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-west-2",
			want: []domain.Resource{
				{
					ID:        "expiring-cluster",
					Type:      domain.ResourceTypeEKS,
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
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					listClustersOutput: &eks.ListClustersOutput{
						Clusters: []string{"permanent-cluster"},
					},
					describeClusterOutputs: map[string]*eks.DescribeClusterOutput{
						"permanent-cluster": {
							Cluster: &types.Cluster{
								Name:   aws.String("permanent-cluster"),
								Arn:    aws.String("arn:aws:eks:us-east-1:123456789012:cluster/permanent-cluster"),
								Status: types.ClusterStatusActive,
								Tags: map[string]string{
									ExpirationTagName: NeverExpiresValue,
								},
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:           "permanent-cluster",
					Type:         domain.ResourceTypeEKS,
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
			name: "filters DELETING clusters",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					listClustersOutput: &eks.ListClustersOutput{
						Clusters: []string{"active-cluster", "deleting-cluster"},
					},
					describeClusterOutputs: map[string]*eks.DescribeClusterOutput{
						"active-cluster": {
							Cluster: &types.Cluster{
								Name:   aws.String("active-cluster"),
								Arn:    aws.String("arn:aws:eks:us-east-1:123456789012:cluster/active-cluster"),
								Status: types.ClusterStatusActive,
								Tags:   map[string]string{},
							},
						},
						"deleting-cluster": {
							Cluster: &types.Cluster{
								Name:   aws.String("deleting-cluster"),
								Arn:    aws.String("arn:aws:eks:us-east-1:123456789012:cluster/deleting-cluster"),
								Status: types.ClusterStatusDeleting,
								Tags:   map[string]string{},
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "active-cluster",
					Type:      domain.ResourceTypeEKS,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "filters FAILED clusters",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					listClustersOutput: &eks.ListClustersOutput{
						Clusters: []string{"active-cluster", "failed-cluster"},
					},
					describeClusterOutputs: map[string]*eks.DescribeClusterOutput{
						"active-cluster": {
							Cluster: &types.Cluster{
								Name:   aws.String("active-cluster"),
								Arn:    aws.String("arn:aws:eks:us-east-1:123456789012:cluster/active-cluster"),
								Status: types.ClusterStatusActive,
								Tags:   map[string]string{},
							},
						},
						"failed-cluster": {
							Cluster: &types.Cluster{
								Name:   aws.String("failed-cluster"),
								Arn:    aws.String("arn:aws:eks:us-east-1:123456789012:cluster/failed-cluster"),
								Status: types.ClusterStatusFailed,
								Tags:   map[string]string{},
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "active-cluster",
					Type:      domain.ResourceTypeEKS,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "includes CREATING and UPDATING clusters",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					listClustersOutput: &eks.ListClustersOutput{
						Clusters: []string{"creating-cluster", "updating-cluster"},
					},
					describeClusterOutputs: map[string]*eks.DescribeClusterOutput{
						"creating-cluster": {
							Cluster: &types.Cluster{
								Name:   aws.String("creating-cluster"),
								Arn:    aws.String("arn:aws:eks:us-east-1:123456789012:cluster/creating-cluster"),
								Status: types.ClusterStatusCreating,
								Tags:   map[string]string{},
							},
						},
						"updating-cluster": {
							Cluster: &types.Cluster{
								Name:   aws.String("updating-cluster"),
								Arn:    aws.String("arn:aws:eks:us-east-1:123456789012:cluster/updating-cluster"),
								Status: types.ClusterStatusUpdating,
								Tags:   map[string]string{},
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "creating-cluster",
					Type:      domain.ResourceTypeEKS,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
				{
					ID:        "updating-cluster",
					Type:      domain.ResourceTypeEKS,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "handles API error for ListClusters",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					listClustersError: errors.New("API error"),
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   true,
			errMsg:    "listing EKS clusters",
		},
		{
			name: "handles API error for DescribeCluster",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					listClustersOutput: &eks.ListClustersOutput{
						Clusters: []string{"my-cluster"},
					},
					describeClusterError: errors.New("describe error"),
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   true,
			errMsg:    "describing EKS cluster",
		},
		{
			name: "handles empty results",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					listClustersOutput: &eks.ListClustersOutput{
						Clusters: []string{},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   false,
		},
		{
			name: "handles pagination",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					listClustersPages: []*eks.ListClustersOutput{
						{
							Clusters:  []string{"cluster-page-1"},
							NextToken: aws.String("next-page-token"),
						},
						{
							Clusters: []string{"cluster-page-2"},
						},
					},
					describeClusterOutputs: map[string]*eks.DescribeClusterOutput{
						"cluster-page-1": {
							Cluster: &types.Cluster{
								Name:   aws.String("cluster-page-1"),
								Arn:    aws.String("arn:aws:eks:us-east-1:123456789012:cluster/cluster-page-1"),
								Status: types.ClusterStatusActive,
								Tags:   map[string]string{},
							},
						},
						"cluster-page-2": {
							Cluster: &types.Cluster{
								Name:   aws.String("cluster-page-2"),
								Arn:    aws.String("arn:aws:eks:us-east-1:123456789012:cluster/cluster-page-2"),
								Status: types.ClusterStatusActive,
								Tags:   map[string]string{},
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "cluster-page-1",
					Type:      domain.ResourceTypeEKS,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
				{
					ID:        "cluster-page-2",
					Type:      domain.ResourceTypeEKS,
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
			repo := &EKSRepository{
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

func TestEKSRepository_Tag(t *testing.T) {
	expDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		mockSetup      func() *mockEKSClient
		accountID      string
		region         string
		resourceID     string
		expirationDate time.Time
		wantARN        string
		wantErr        bool
		errMsg         string
	}{
		{
			name: "tags cluster successfully",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					describeClusterOutputs: map[string]*eks.DescribeClusterOutput{
						"my-cluster": {
							Cluster: &types.Cluster{
								Name:   aws.String("my-cluster"),
								Arn:    aws.String("arn:aws:eks:us-east-1:123456789012:cluster/my-cluster"),
								Status: types.ClusterStatusActive,
							},
						},
					},
					tagResourceOutput: &eks.TagResourceOutput{},
				}
			},
			accountID:      "123456789012",
			region:         "us-east-1",
			resourceID:     "my-cluster",
			expirationDate: expDate,
			wantARN:        "arn:aws:eks:us-east-1:123456789012:cluster/my-cluster",
			wantErr:        false,
		},
		{
			name: "constructs correct ARN for different region",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					describeClusterOutputs: map[string]*eks.DescribeClusterOutput{
						"prod-cluster": {
							Cluster: &types.Cluster{
								Name:   aws.String("prod-cluster"),
								Arn:    aws.String("arn:aws:eks:eu-west-1:987654321098:cluster/prod-cluster"),
								Status: types.ClusterStatusActive,
							},
						},
					},
					tagResourceOutput: &eks.TagResourceOutput{},
				}
			},
			accountID:      "987654321098",
			region:         "eu-west-1",
			resourceID:     "prod-cluster",
			expirationDate: expDate,
			wantARN:        "arn:aws:eks:eu-west-1:987654321098:cluster/prod-cluster",
			wantErr:        false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					describeClusterOutputs: map[string]*eks.DescribeClusterOutput{
						"my-cluster": {
							Cluster: &types.Cluster{
								Name:   aws.String("my-cluster"),
								Arn:    aws.String("arn:aws:eks:us-east-1:123456789012:cluster/my-cluster"),
								Status: types.ClusterStatusActive,
							},
						},
					},
					tagResourceError: errors.New("access denied"),
				}
			},
			accountID:      "123456789012",
			region:         "us-east-1",
			resourceID:     "my-cluster",
			expirationDate: expDate,
			wantErr:        true,
			errMsg:         "tagging EKS cluster",
		},
		{
			name: "handles DescribeCluster error",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					describeClusterError: errors.New("cluster not found"),
				}
			},
			accountID:      "123456789012",
			region:         "us-east-1",
			resourceID:     "nonexistent-cluster",
			expirationDate: expDate,
			wantErr:        true,
			errMsg:         "getting EKS cluster ARN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &EKSRepository{
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
				if mock.tagResourceInput == nil {
					t.Fatal("TagResource was not called")
				}
				if aws.ToString(mock.tagResourceInput.ResourceArn) != tt.wantARN {
					t.Errorf("Tag() ARN = %v, want %v", aws.ToString(mock.tagResourceInput.ResourceArn), tt.wantARN)
				}

				// Verify the tag was set correctly
				if len(mock.tagResourceInput.Tags) != 1 {
					t.Errorf("Tag() set %d tags, want 1", len(mock.tagResourceInput.Tags))
				}
				if val, ok := mock.tagResourceInput.Tags[ExpirationTagName]; ok {
					wantValue := tt.expirationDate.Format(ExpirationDateFormat)
					if val != wantValue {
						t.Errorf("Tag() value = %v, want %v", val, wantValue)
					}
				} else {
					t.Errorf("Tag() key %v not found in tags", ExpirationTagName)
				}
			}
		})
	}
}

func TestEKSRepository_Delete(t *testing.T) {
	tests := []struct {
		name                 string
		mockSetup            func() *mockEKSClient
		cascadeDelete        bool
		resourceID           string
		wantNodegroupDeletes []string
		wantClusterDeleted   bool
		wantErr              bool
		errMsg               string
	}{
		{
			name: "deletes cluster without node groups successfully",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					listNodegroupsOutputs: map[string]*eks.ListNodegroupsOutput{
						"my-cluster": {
							Nodegroups: []string{},
						},
					},
					deleteClusterOutput: &eks.DeleteClusterOutput{},
				}
			},
			cascadeDelete:        false,
			resourceID:           "my-cluster",
			wantNodegroupDeletes: nil,
			wantClusterDeleted:   true,
			wantErr:              false,
		},
		{
			name: "with cascadeDelete=true deletes node groups then cluster",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					listNodegroupsOutputs: map[string]*eks.ListNodegroupsOutput{
						"my-cluster": {
							Nodegroups: []string{"nodegroup-1", "nodegroup-2"},
						},
					},
					deleteNodegroupOutput: &eks.DeleteNodegroupOutput{},
					deleteClusterOutput:   &eks.DeleteClusterOutput{},
				}
			},
			cascadeDelete:        true,
			resourceID:           "my-cluster",
			wantNodegroupDeletes: []string{"nodegroup-1", "nodegroup-2"},
			wantClusterDeleted:   true,
			wantErr:              false,
		},
		{
			name: "with cascadeDelete=false skips cluster with node groups",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					listNodegroupsOutputs: map[string]*eks.ListNodegroupsOutput{
						"my-cluster": {
							Nodegroups: []string{"nodegroup-1", "nodegroup-2"},
						},
					},
				}
			},
			cascadeDelete:        false,
			resourceID:           "my-cluster",
			wantNodegroupDeletes: nil,
			wantClusterDeleted:   false,
			wantErr:              true,
			errMsg:               "has 2 node groups",
		},
		{
			name: "handles API error for ListNodegroups",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					listNodegroupsError: errors.New("access denied"),
				}
			},
			cascadeDelete:      false,
			resourceID:         "my-cluster",
			wantClusterDeleted: false,
			wantErr:            true,
			errMsg:             "listing node groups",
		},
		{
			name: "handles API error for DeleteNodegroup",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					listNodegroupsOutputs: map[string]*eks.ListNodegroupsOutput{
						"my-cluster": {
							Nodegroups: []string{"nodegroup-1"},
						},
					},
					deleteNodegroupError: errors.New("delete failed"),
				}
			},
			cascadeDelete:        true,
			resourceID:           "my-cluster",
			wantNodegroupDeletes: []string{"nodegroup-1"},
			wantClusterDeleted:   false,
			wantErr:              true,
			errMsg:               "deleting node group",
		},
		{
			name: "handles API error for DeleteCluster",
			mockSetup: func() *mockEKSClient {
				return &mockEKSClient{
					listNodegroupsOutputs: map[string]*eks.ListNodegroupsOutput{
						"my-cluster": {
							Nodegroups: []string{},
						},
					},
					deleteClusterError: errors.New("cannot delete"),
				}
			},
			cascadeDelete:      false,
			resourceID:         "my-cluster",
			wantClusterDeleted: true,
			wantErr:            true,
			errMsg:             "deleting EKS cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &EKSRepository{
				client:        mock,
				accountID:     "123456789012",
				region:        "us-east-1",
				cascadeDelete: tt.cascadeDelete,
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
			}

			// Verify node group deletes
			if len(mock.deleteNodegroupCalls) != len(tt.wantNodegroupDeletes) {
				t.Errorf("Delete() deleted %d node groups, want %d", len(mock.deleteNodegroupCalls), len(tt.wantNodegroupDeletes))
			}
			for i, ng := range tt.wantNodegroupDeletes {
				if i < len(mock.deleteNodegroupCalls) && mock.deleteNodegroupCalls[i] != ng {
					t.Errorf("Delete() deleted node group %v, want %v", mock.deleteNodegroupCalls[i], ng)
				}
			}

			// Verify cluster delete was called
			if mock.deleteClusterCalled != tt.wantClusterDeleted {
				t.Errorf("DeleteCluster called = %v, want %v", mock.deleteClusterCalled, tt.wantClusterDeleted)
			}

			// Verify correct cluster name was used
			if tt.wantClusterDeleted && mock.deleteClusterInput != nil {
				if aws.ToString(mock.deleteClusterInput.Name) != tt.resourceID {
					t.Errorf("Delete() cluster name = %v, want %v",
						aws.ToString(mock.deleteClusterInput.Name), tt.resourceID)
				}
			}
		})
	}
}

func TestEKSRepository_InterfaceCompliance(_ *testing.T) {
	// Verify EKSRepository implements domain.ResourceRepository
	var _ domain.ResourceRepository = (*EKSRepository)(nil)
}

func TestNewEKSRepository(t *testing.T) {
	tests := []struct {
		name          string
		accountID     string
		region        string
		cascadeDelete bool
	}{
		{
			name:          "creates repository with all fields",
			accountID:     "123456789012",
			region:        "us-east-1",
			cascadeDelete: true,
		},
		{
			name:          "creates repository without cascade delete",
			accountID:     "987654321098",
			region:        "eu-west-1",
			cascadeDelete: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a nil client since we're just testing field assignment
			repo := NewEKSRepository(nil, tt.accountID, tt.region, tt.cascadeDelete)

			if repo == nil {
				t.Fatal("NewEKSRepository() returned nil")
			}
			if repo.accountID != tt.accountID {
				t.Errorf("accountID = %v, want %v", repo.accountID, tt.accountID)
			}
			if repo.region != tt.region {
				t.Errorf("region = %v, want %v", repo.region, tt.region)
			}
			if repo.cascadeDelete != tt.cascadeDelete {
				t.Errorf("cascadeDelete = %v, want %v", repo.cascadeDelete, tt.cascadeDelete)
			}
		})
	}
}

func TestEKSRepository_isValidClusterState(t *testing.T) {
	tests := []struct {
		name   string
		status types.ClusterStatus
		want   bool
	}{
		{
			name:   "ACTIVE is valid",
			status: types.ClusterStatusActive,
			want:   true,
		},
		{
			name:   "CREATING is valid",
			status: types.ClusterStatusCreating,
			want:   true,
		},
		{
			name:   "UPDATING is valid",
			status: types.ClusterStatusUpdating,
			want:   true,
		},
		{
			name:   "PENDING is valid",
			status: types.ClusterStatusPending,
			want:   true,
		},
		{
			name:   "DELETING is invalid",
			status: types.ClusterStatusDeleting,
			want:   false,
		},
		{
			name:   "FAILED is invalid",
			status: types.ClusterStatusFailed,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &EKSRepository{}
			got := repo.isValidClusterState(tt.status)
			if got != tt.want {
				t.Errorf("isValidClusterState(%v) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestEKSRepository_clusterToResource(t *testing.T) {
	now := time.Now()
	createdAt := now.AddDate(0, -1, 0)
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name    string
		cluster *types.Cluster
		want    domain.Resource
	}{
		{
			name: "basic cluster",
			cluster: &types.Cluster{
				Name:      aws.String("test-cluster"),
				Arn:       aws.String("arn:aws:eks:us-east-1:123456789012:cluster/test-cluster"),
				Status:    types.ClusterStatusActive,
				CreatedAt: &createdAt,
				Tags:      map[string]string{},
			},
			want: domain.Resource{
				ID:        "test-cluster",
				Type:      domain.ResourceTypeEKS,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Tags:      map[string]string{},
			},
		},
		{
			name: "cluster with Name tag",
			cluster: &types.Cluster{
				Name:      aws.String("prod-cluster"),
				Arn:       aws.String("arn:aws:eks:us-east-1:123456789012:cluster/prod-cluster"),
				Status:    types.ClusterStatusActive,
				CreatedAt: &createdAt,
				Tags: map[string]string{
					"Name": "Production Cluster",
				},
			},
			want: domain.Resource{
				ID:        "prod-cluster",
				Type:      domain.ResourceTypeEKS,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Name:      "Production Cluster",
				Tags: map[string]string{
					"Name": "Production Cluster",
				},
			},
		},
		{
			name: "cluster with expiration date",
			cluster: &types.Cluster{
				Name:      aws.String("expiring-cluster"),
				Arn:       aws.String("arn:aws:eks:us-east-1:123456789012:cluster/expiring-cluster"),
				Status:    types.ClusterStatusActive,
				CreatedAt: &createdAt,
				Tags: map[string]string{
					ExpirationTagName: expDateStr,
				},
			},
			want: domain.Resource{
				ID:        "expiring-cluster",
				Type:      domain.ResourceTypeEKS,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Tags: map[string]string{
					ExpirationTagName: expDateStr,
				},
			},
		},
		{
			name: "cluster that never expires",
			cluster: &types.Cluster{
				Name:      aws.String("permanent-cluster"),
				Arn:       aws.String("arn:aws:eks:us-east-1:123456789012:cluster/permanent-cluster"),
				Status:    types.ClusterStatusActive,
				CreatedAt: &createdAt,
				Tags: map[string]string{
					ExpirationTagName: NeverExpiresValue,
				},
			},
			want: domain.Resource{
				ID:           "permanent-cluster",
				Type:         domain.ResourceTypeEKS,
				Region:       "us-east-1",
				AccountID:    "123456789012",
				NeverExpires: true,
				Tags: map[string]string{
					ExpirationTagName: NeverExpiresValue,
				},
			},
		},
		{
			name: "cluster with nil tags",
			cluster: &types.Cluster{
				Name:      aws.String("no-tags-cluster"),
				Arn:       aws.String("arn:aws:eks:us-east-1:123456789012:cluster/no-tags-cluster"),
				Status:    types.ClusterStatusActive,
				CreatedAt: &createdAt,
				Tags:      nil,
			},
			want: domain.Resource{
				ID:        "no-tags-cluster",
				Type:      domain.ResourceTypeEKS,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Tags:      map[string]string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &EKSRepository{
				accountID: "123456789012",
				region:    "us-east-1",
			}

			got := repo.clusterToResource(tt.cluster)

			if got.ID != tt.want.ID {
				t.Errorf("ID = %v, want %v", got.ID, tt.want.ID)
			}
			if got.Type != tt.want.Type {
				t.Errorf("Type = %v, want %v", got.Type, tt.want.Type)
			}
			if got.Region != tt.want.Region {
				t.Errorf("Region = %v, want %v", got.Region, tt.want.Region)
			}
			if got.AccountID != tt.want.AccountID {
				t.Errorf("AccountID = %v, want %v", got.AccountID, tt.want.AccountID)
			}
			if got.Name != tt.want.Name {
				t.Errorf("Name = %v, want %v", got.Name, tt.want.Name)
			}
			if got.NeverExpires != tt.want.NeverExpires {
				t.Errorf("NeverExpires = %v, want %v", got.NeverExpires, tt.want.NeverExpires)
			}
			// Check tags
			if got.Tags == nil {
				t.Error("Tags should not be nil")
			}
			for key, wantValue := range tt.want.Tags {
				if gotValue, ok := got.Tags[key]; !ok || gotValue != wantValue {
					t.Errorf("Tags[%s] = %v, want %v", key, gotValue, wantValue)
				}
			}
		})
	}
}
