package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check for mockRedshiftClient.
var _ redshiftClient = (*mockRedshiftClient)(nil)

// mockRedshiftClient implements redshiftClient for testing.
type mockRedshiftClient struct {
	describeClustersOutput *redshift.DescribeClustersOutput
	describeClustersError  error
	describeClustersPages  []*redshift.DescribeClustersOutput
	pageIndex              int

	createTagsOutput *redshift.CreateTagsOutput
	createTagsError  error
	createTagsInput  *redshift.CreateTagsInput

	deleteClusterOutput *redshift.DeleteClusterOutput
	deleteClusterError  error
	deleteClusterInput  *redshift.DeleteClusterInput
	deleteClusterCalled bool
}

func (m *mockRedshiftClient) DescribeClusters(_ context.Context, _ *redshift.DescribeClustersInput, _ ...func(*redshift.Options)) (*redshift.DescribeClustersOutput, error) {
	if m.describeClustersError != nil {
		return nil, m.describeClustersError
	}
	if len(m.describeClustersPages) > 0 {
		if m.pageIndex >= len(m.describeClustersPages) {
			return &redshift.DescribeClustersOutput{}, nil
		}
		result := m.describeClustersPages[m.pageIndex]
		m.pageIndex++
		return result, nil
	}
	return m.describeClustersOutput, nil
}

func (m *mockRedshiftClient) CreateTags(_ context.Context, params *redshift.CreateTagsInput, _ ...func(*redshift.Options)) (*redshift.CreateTagsOutput, error) {
	m.createTagsInput = params
	if m.createTagsError != nil {
		return nil, m.createTagsError
	}
	return m.createTagsOutput, nil
}

func (m *mockRedshiftClient) DeleteCluster(_ context.Context, params *redshift.DeleteClusterInput, _ ...func(*redshift.Options)) (*redshift.DeleteClusterOutput, error) {
	m.deleteClusterInput = params
	m.deleteClusterCalled = true
	if m.deleteClusterError != nil {
		return nil, m.deleteClusterError
	}
	return m.deleteClusterOutput, nil
}

func TestRedshiftRepository_Type(t *testing.T) {
	repo := &RedshiftRepository{
		client:    &mockRedshiftClient{},
		accountID: "123456789012",
		region:    "us-east-1",
	}
	got := repo.Type()
	want := domain.ResourceTypeRedshift

	if got != want {
		t.Errorf("Type() = %v, want %v", got, want)
	}
}

func TestRedshiftRepository_List(t *testing.T) {
	now := time.Now()
	createdAt := now.AddDate(0, -1, 0)
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name      string
		mockSetup func() *mockRedshiftClient
		accountID string
		region    string
		want      []domain.Resource
		wantErr   bool
		errMsg    string
	}{
		{
			name: "lists clusters successfully",
			mockSetup: func() *mockRedshiftClient {
				return &mockRedshiftClient{
					describeClustersOutput: &redshift.DescribeClustersOutput{
						Clusters: []types.Cluster{
							{
								ClusterIdentifier: aws.String("my-cluster"),
								ClusterStatus:     aws.String("available"),
								ClusterCreateTime: &createdAt,
								Tags: []types.Tag{
									{Key: aws.String("Name"), Value: aws.String("Production Cluster")},
									{Key: aws.String("Environment"), Value: aws.String("prod")},
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
					Type:           domain.ResourceTypeRedshift,
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
			mockSetup: func() *mockRedshiftClient {
				return &mockRedshiftClient{
					describeClustersOutput: &redshift.DescribeClustersOutput{
						Clusters: []types.Cluster{
							{
								ClusterIdentifier: aws.String("expiring-cluster"),
								ClusterStatus:     aws.String("available"),
								Tags: []types.Tag{
									{Key: aws.String(ExpirationTagName), Value: aws.String(expDateStr)},
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
					Type:      domain.ResourceTypeRedshift,
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
			mockSetup: func() *mockRedshiftClient {
				return &mockRedshiftClient{
					describeClustersOutput: &redshift.DescribeClustersOutput{
						Clusters: []types.Cluster{
							{
								ClusterIdentifier: aws.String("permanent-cluster"),
								ClusterStatus:     aws.String("available"),
								Tags: []types.Tag{
									{Key: aws.String(ExpirationTagName), Value: aws.String(NeverExpiresValue)},
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
					Type:         domain.ResourceTypeRedshift,
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
			name: "filters deleting clusters",
			mockSetup: func() *mockRedshiftClient {
				return &mockRedshiftClient{
					describeClustersOutput: &redshift.DescribeClustersOutput{
						Clusters: []types.Cluster{
							{
								ClusterIdentifier: aws.String("active-cluster"),
								ClusterStatus:     aws.String("available"),
							},
							{
								ClusterIdentifier: aws.String("deleting-cluster"),
								ClusterStatus:     aws.String("deleting"),
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
					Type:      domain.ResourceTypeRedshift,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "filters final-snapshot clusters",
			mockSetup: func() *mockRedshiftClient {
				return &mockRedshiftClient{
					describeClustersOutput: &redshift.DescribeClustersOutput{
						Clusters: []types.Cluster{
							{
								ClusterIdentifier: aws.String("active-cluster"),
								ClusterStatus:     aws.String("available"),
							},
							{
								ClusterIdentifier: aws.String("snapshot-cluster"),
								ClusterStatus:     aws.String("final-snapshot"),
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
					Type:      domain.ResourceTypeRedshift,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockRedshiftClient {
				return &mockRedshiftClient{
					describeClustersError: errors.New("API error"),
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   true,
			errMsg:    "listing Redshift clusters",
		},
		{
			name: "handles empty result",
			mockSetup: func() *mockRedshiftClient {
				return &mockRedshiftClient{
					describeClustersOutput: &redshift.DescribeClustersOutput{
						Clusters: []types.Cluster{},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   false,
		},
		{
			name: "handles pagination via Marker",
			mockSetup: func() *mockRedshiftClient {
				return &mockRedshiftClient{
					describeClustersPages: []*redshift.DescribeClustersOutput{
						{
							Clusters: []types.Cluster{
								{
									ClusterIdentifier: aws.String("cluster-page-1"),
									ClusterStatus:     aws.String("available"),
								},
							},
							Marker: aws.String("next-page-marker"),
						},
						{
							Clusters: []types.Cluster{
								{
									ClusterIdentifier: aws.String("cluster-page-2"),
									ClusterStatus:     aws.String("available"),
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
					ID:        "cluster-page-1",
					Type:      domain.ResourceTypeRedshift,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
				{
					ID:        "cluster-page-2",
					Type:      domain.ResourceTypeRedshift,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "includes clusters in valid states",
			mockSetup: func() *mockRedshiftClient {
				return &mockRedshiftClient{
					describeClustersOutput: &redshift.DescribeClustersOutput{
						Clusters: []types.Cluster{
							{ClusterIdentifier: aws.String("cluster-available"), ClusterStatus: aws.String("available")},
							{ClusterIdentifier: aws.String("cluster-creating"), ClusterStatus: aws.String("creating")},
							{ClusterIdentifier: aws.String("cluster-rebooting"), ClusterStatus: aws.String("rebooting")},
							{ClusterIdentifier: aws.String("cluster-renaming"), ClusterStatus: aws.String("renaming")},
							{ClusterIdentifier: aws.String("cluster-resizing"), ClusterStatus: aws.String("resizing")},
							{ClusterIdentifier: aws.String("cluster-rotating-keys"), ClusterStatus: aws.String("rotating-keys")},
							{ClusterIdentifier: aws.String("cluster-storage-full"), ClusterStatus: aws.String("storage-full")},
							{ClusterIdentifier: aws.String("cluster-updating-hsm"), ClusterStatus: aws.String("updating-hsm")},
							{ClusterIdentifier: aws.String("cluster-paused"), ClusterStatus: aws.String("paused")},
							{ClusterIdentifier: aws.String("cluster-resuming"), ClusterStatus: aws.String("resuming")},
							{ClusterIdentifier: aws.String("cluster-modifying"), ClusterStatus: aws.String("modifying")},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{ID: "cluster-available", Type: domain.ResourceTypeRedshift, Region: "us-east-1", AccountID: "123456789012", Tags: map[string]string{}},
				{ID: "cluster-creating", Type: domain.ResourceTypeRedshift, Region: "us-east-1", AccountID: "123456789012", Tags: map[string]string{}},
				{ID: "cluster-rebooting", Type: domain.ResourceTypeRedshift, Region: "us-east-1", AccountID: "123456789012", Tags: map[string]string{}},
				{ID: "cluster-renaming", Type: domain.ResourceTypeRedshift, Region: "us-east-1", AccountID: "123456789012", Tags: map[string]string{}},
				{ID: "cluster-resizing", Type: domain.ResourceTypeRedshift, Region: "us-east-1", AccountID: "123456789012", Tags: map[string]string{}},
				{ID: "cluster-rotating-keys", Type: domain.ResourceTypeRedshift, Region: "us-east-1", AccountID: "123456789012", Tags: map[string]string{}},
				{ID: "cluster-storage-full", Type: domain.ResourceTypeRedshift, Region: "us-east-1", AccountID: "123456789012", Tags: map[string]string{}},
				{ID: "cluster-updating-hsm", Type: domain.ResourceTypeRedshift, Region: "us-east-1", AccountID: "123456789012", Tags: map[string]string{}},
				{ID: "cluster-paused", Type: domain.ResourceTypeRedshift, Region: "us-east-1", AccountID: "123456789012", Tags: map[string]string{}},
				{ID: "cluster-resuming", Type: domain.ResourceTypeRedshift, Region: "us-east-1", AccountID: "123456789012", Tags: map[string]string{}},
				{ID: "cluster-modifying", Type: domain.ResourceTypeRedshift, Region: "us-east-1", AccountID: "123456789012", Tags: map[string]string{}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &RedshiftRepository{
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

func TestRedshiftRepository_Tag(t *testing.T) {
	expDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		mockSetup      func() *mockRedshiftClient
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
			mockSetup: func() *mockRedshiftClient {
				return &mockRedshiftClient{
					createTagsOutput: &redshift.CreateTagsOutput{},
				}
			},
			accountID:      "123456789012",
			region:         "us-east-1",
			resourceID:     "my-cluster",
			expirationDate: expDate,
			wantARN:        "arn:aws:redshift:us-east-1:123456789012:cluster:my-cluster",
			wantErr:        false,
		},
		{
			name: "constructs correct ARN for different region",
			mockSetup: func() *mockRedshiftClient {
				return &mockRedshiftClient{
					createTagsOutput: &redshift.CreateTagsOutput{},
				}
			},
			accountID:      "987654321098",
			region:         "eu-west-1",
			resourceID:     "prod-cluster",
			expirationDate: expDate,
			wantARN:        "arn:aws:redshift:eu-west-1:987654321098:cluster:prod-cluster",
			wantErr:        false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockRedshiftClient {
				return &mockRedshiftClient{
					createTagsError: errors.New("access denied"),
				}
			},
			accountID:      "123456789012",
			region:         "us-east-1",
			resourceID:     "my-cluster",
			expirationDate: expDate,
			wantErr:        true,
			errMsg:         "tagging Redshift cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &RedshiftRepository{
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
				if mock.createTagsInput == nil {
					t.Fatal("CreateTags was not called")
				}
				if aws.ToString(mock.createTagsInput.ResourceName) != tt.wantARN {
					t.Errorf("Tag() ARN = %v, want %v", aws.ToString(mock.createTagsInput.ResourceName), tt.wantARN)
				}

				// Verify the tag was set correctly
				if len(mock.createTagsInput.Tags) != 1 {
					t.Errorf("Tag() set %d tags, want 1", len(mock.createTagsInput.Tags))
				}
				if len(mock.createTagsInput.Tags) > 0 {
					tag := mock.createTagsInput.Tags[0]
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

func TestRedshiftRepository_Delete(t *testing.T) {
	tests := []struct {
		name       string
		mockSetup  func() *mockRedshiftClient
		resourceID string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "deletes cluster successfully",
			mockSetup: func() *mockRedshiftClient {
				return &mockRedshiftClient{
					deleteClusterOutput: &redshift.DeleteClusterOutput{},
				}
			},
			resourceID: "my-cluster",
			wantErr:    false,
		},
		{
			name: "uses SkipFinalClusterSnapshot",
			mockSetup: func() *mockRedshiftClient {
				return &mockRedshiftClient{
					deleteClusterOutput: &redshift.DeleteClusterOutput{},
				}
			},
			resourceID: "test-cluster",
			wantErr:    false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockRedshiftClient {
				return &mockRedshiftClient{
					deleteClusterError: errors.New("cannot delete"),
				}
			},
			resourceID: "my-cluster",
			wantErr:    true,
			errMsg:     "deleting Redshift cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &RedshiftRepository{
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

			if !tt.wantErr {
				// Verify DeleteCluster was called
				if !mock.deleteClusterCalled {
					t.Fatal("DeleteCluster was not called")
				}
				if mock.deleteClusterInput == nil {
					t.Fatal("DeleteCluster input is nil")
				}
				if aws.ToString(mock.deleteClusterInput.ClusterIdentifier) != tt.resourceID {
					t.Errorf("Delete() ClusterIdentifier = %v, want %v",
						aws.ToString(mock.deleteClusterInput.ClusterIdentifier), tt.resourceID)
				}
				// Verify SkipFinalClusterSnapshot is true
				if !aws.ToBool(mock.deleteClusterInput.SkipFinalClusterSnapshot) {
					t.Error("Delete() should set SkipFinalClusterSnapshot to true")
				}
			}
		})
	}
}

func TestRedshiftRepository_InterfaceCompliance(_ *testing.T) {
	// Verify RedshiftRepository implements domain.ResourceRepository
	var _ domain.ResourceRepository = (*RedshiftRepository)(nil)
}

func TestRedshiftRepository_buildARN(t *testing.T) {
	tests := []struct {
		name       string
		accountID  string
		region     string
		resourceID string
		want       string
	}{
		{
			name:       "standard ARN",
			accountID:  "123456789012",
			region:     "us-east-1",
			resourceID: "my-cluster",
			want:       "arn:aws:redshift:us-east-1:123456789012:cluster:my-cluster",
		},
		{
			name:       "different region",
			accountID:  "987654321098",
			region:     "eu-central-1",
			resourceID: "prod-cluster-1",
			want:       "arn:aws:redshift:eu-central-1:987654321098:cluster:prod-cluster-1",
		},
		{
			name:       "cluster with hyphens",
			accountID:  "111222333444",
			region:     "ap-southeast-1",
			resourceID: "my-complex-cluster-name",
			want:       "arn:aws:redshift:ap-southeast-1:111222333444:cluster:my-complex-cluster-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &RedshiftRepository{
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

func TestNewRedshiftRepository(t *testing.T) {
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
			repo := NewRedshiftRepository(nil, tt.accountID, tt.region)

			if repo == nil {
				t.Fatal("NewRedshiftRepository() returned nil")
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

func TestRedshiftRepository_clusterToResource(t *testing.T) {
	now := time.Now()
	createdAt := now.AddDate(0, -1, 0)
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name    string
		cluster types.Cluster
		want    domain.Resource
	}{
		{
			name: "basic cluster",
			cluster: types.Cluster{
				ClusterIdentifier: aws.String("test-cluster"),
				ClusterStatus:     aws.String("available"),
				ClusterCreateTime: &createdAt,
			},
			want: domain.Resource{
				ID:        "test-cluster",
				Type:      domain.ResourceTypeRedshift,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Tags:      map[string]string{},
			},
		},
		{
			name: "cluster with Name tag",
			cluster: types.Cluster{
				ClusterIdentifier: aws.String("prod-cluster"),
				ClusterStatus:     aws.String("available"),
				Tags: []types.Tag{
					{Key: aws.String("Name"), Value: aws.String("Production Cluster")},
				},
			},
			want: domain.Resource{
				ID:        "prod-cluster",
				Type:      domain.ResourceTypeRedshift,
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
			cluster: types.Cluster{
				ClusterIdentifier: aws.String("expiring-cluster"),
				ClusterStatus:     aws.String("available"),
				Tags: []types.Tag{
					{Key: aws.String(ExpirationTagName), Value: aws.String(expDateStr)},
				},
			},
			want: domain.Resource{
				ID:        "expiring-cluster",
				Type:      domain.ResourceTypeRedshift,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Tags: map[string]string{
					ExpirationTagName: expDateStr,
				},
			},
		},
		{
			name: "cluster that never expires",
			cluster: types.Cluster{
				ClusterIdentifier: aws.String("permanent-cluster"),
				ClusterStatus:     aws.String("available"),
				Tags: []types.Tag{
					{Key: aws.String(ExpirationTagName), Value: aws.String(NeverExpiresValue)},
				},
			},
			want: domain.Resource{
				ID:           "permanent-cluster",
				Type:         domain.ResourceTypeRedshift,
				Region:       "us-east-1",
				AccountID:    "123456789012",
				NeverExpires: true,
				Tags: map[string]string{
					ExpirationTagName: NeverExpiresValue,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &RedshiftRepository{
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
			for key, wantValue := range tt.want.Tags {
				if gotValue, ok := got.Tags[key]; !ok || gotValue != wantValue {
					t.Errorf("Tags[%s] = %v, want %v", key, gotValue, wantValue)
				}
			}
		})
	}
}

func TestRedshiftRepository_isValidClusterState(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		// Valid states
		{name: "available is valid", status: "available", want: true},
		{name: "creating is valid", status: "creating", want: true},
		{name: "rebooting is valid", status: "rebooting", want: true},
		{name: "renaming is valid", status: "renaming", want: true},
		{name: "resizing is valid", status: "resizing", want: true},
		{name: "rotating-keys is valid", status: "rotating-keys", want: true},
		{name: "storage-full is valid", status: "storage-full", want: true},
		{name: "updating-hsm is valid", status: "updating-hsm", want: true},
		{name: "paused is valid", status: "paused", want: true},
		{name: "resuming is valid", status: "resuming", want: true},
		{name: "modifying is valid", status: "modifying", want: true},
		// Invalid states
		{name: "deleting is invalid", status: "deleting", want: false},
		{name: "final-snapshot is invalid", status: "final-snapshot", want: false},
	}

	repo := &RedshiftRepository{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.isValidClusterState(tt.status)
			if got != tt.want {
				t.Errorf("isValidClusterState(%s) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
