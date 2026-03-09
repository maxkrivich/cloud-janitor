package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check for mockRDSClient.
var _ rdsClient = (*mockRDSClient)(nil)

// mockRDSClient implements rdsClient for testing.
type mockRDSClient struct {
	describeDBInstancesOutput *rds.DescribeDBInstancesOutput
	describeDBInstancesError  error
	describeDBInstancesPages  []*rds.DescribeDBInstancesOutput
	pageIndex                 int

	addTagsToResourceOutput *rds.AddTagsToResourceOutput
	addTagsToResourceError  error
	addTagsToResourceInput  *rds.AddTagsToResourceInput

	modifyDBInstanceOutput *rds.ModifyDBInstanceOutput
	modifyDBInstanceError  error
	modifyDBInstanceInput  *rds.ModifyDBInstanceInput
	modifyDBInstanceCalled bool

	deleteDBInstanceOutput *rds.DeleteDBInstanceOutput
	deleteDBInstanceError  error
	deleteDBInstanceInput  *rds.DeleteDBInstanceInput
	deleteDBInstanceCalled bool
}

func (m *mockRDSClient) DescribeDBInstances(_ context.Context, _ *rds.DescribeDBInstancesInput, _ ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	if m.describeDBInstancesError != nil {
		return nil, m.describeDBInstancesError
	}
	if len(m.describeDBInstancesPages) > 0 {
		if m.pageIndex >= len(m.describeDBInstancesPages) {
			return &rds.DescribeDBInstancesOutput{}, nil
		}
		result := m.describeDBInstancesPages[m.pageIndex]
		m.pageIndex++
		return result, nil
	}
	return m.describeDBInstancesOutput, nil
}

func (m *mockRDSClient) AddTagsToResource(_ context.Context, params *rds.AddTagsToResourceInput, _ ...func(*rds.Options)) (*rds.AddTagsToResourceOutput, error) {
	m.addTagsToResourceInput = params
	if m.addTagsToResourceError != nil {
		return nil, m.addTagsToResourceError
	}
	return m.addTagsToResourceOutput, nil
}

func (m *mockRDSClient) ModifyDBInstance(_ context.Context, params *rds.ModifyDBInstanceInput, _ ...func(*rds.Options)) (*rds.ModifyDBInstanceOutput, error) {
	m.modifyDBInstanceInput = params
	m.modifyDBInstanceCalled = true
	if m.modifyDBInstanceError != nil {
		return nil, m.modifyDBInstanceError
	}
	return m.modifyDBInstanceOutput, nil
}

func (m *mockRDSClient) DeleteDBInstance(_ context.Context, params *rds.DeleteDBInstanceInput, _ ...func(*rds.Options)) (*rds.DeleteDBInstanceOutput, error) {
	m.deleteDBInstanceInput = params
	m.deleteDBInstanceCalled = true
	if m.deleteDBInstanceError != nil {
		return nil, m.deleteDBInstanceError
	}
	return m.deleteDBInstanceOutput, nil
}

func TestRDSRepository_Type(t *testing.T) {
	repo := &RDSRepository{
		client:    &mockRDSClient{},
		accountID: "123456789012",
		region:    "us-east-1",
	}
	got := repo.Type()
	want := domain.ResourceTypeRDS

	if got != want {
		t.Errorf("Type() = %v, want %v", got, want)
	}
}

func TestRDSRepository_List(t *testing.T) {
	now := time.Now()
	createdAt := now.AddDate(0, -1, 0)
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name      string
		mockSetup func() *mockRDSClient
		accountID string
		region    string
		want      []domain.Resource
		wantErr   bool
		errMsg    string
	}{
		{
			name: "lists instances successfully",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					describeDBInstancesOutput: &rds.DescribeDBInstancesOutput{
						DBInstances: []types.DBInstance{
							{
								DBInstanceIdentifier: aws.String("my-database"),
								DBInstanceStatus:     aws.String("available"),
								InstanceCreateTime:   &createdAt,
								TagList: []types.Tag{
									{Key: aws.String("Name"), Value: aws.String("Production DB")},
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
					ID:             "my-database",
					Type:           domain.ResourceTypeRDS,
					Region:         "us-east-1",
					AccountID:      "123456789012",
					Name:           "Production DB",
					ExpirationDate: nil,
					NeverExpires:   false,
					Tags: map[string]string{
						"Name":        "Production DB",
						"Environment": "prod",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "parses expiration date tag",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					describeDBInstancesOutput: &rds.DescribeDBInstancesOutput{
						DBInstances: []types.DBInstance{
							{
								DBInstanceIdentifier: aws.String("expiring-db"),
								DBInstanceStatus:     aws.String("available"),
								TagList: []types.Tag{
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
					ID:        "expiring-db",
					Type:      domain.ResourceTypeRDS,
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
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					describeDBInstancesOutput: &rds.DescribeDBInstancesOutput{
						DBInstances: []types.DBInstance{
							{
								DBInstanceIdentifier: aws.String("permanent-db"),
								DBInstanceStatus:     aws.String("available"),
								TagList: []types.Tag{
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
					ID:           "permanent-db",
					Type:         domain.ResourceTypeRDS,
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
			name: "skips deleting instances",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					describeDBInstancesOutput: &rds.DescribeDBInstancesOutput{
						DBInstances: []types.DBInstance{
							{
								DBInstanceIdentifier: aws.String("active-db"),
								DBInstanceStatus:     aws.String("available"),
							},
							{
								DBInstanceIdentifier: aws.String("deleting-db"),
								DBInstanceStatus:     aws.String("deleting"),
							},
							{
								DBInstanceIdentifier: aws.String("deleted-db"),
								DBInstanceStatus:     aws.String("deleted"),
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "active-db",
					Type:      domain.ResourceTypeRDS,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					describeDBInstancesError: errors.New("API error"),
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   true,
			errMsg:    "listing RDS instances",
		},
		{
			name: "handles empty result",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					describeDBInstancesOutput: &rds.DescribeDBInstancesOutput{
						DBInstances: []types.DBInstance{},
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
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					describeDBInstancesPages: []*rds.DescribeDBInstancesOutput{
						{
							DBInstances: []types.DBInstance{
								{
									DBInstanceIdentifier: aws.String("db-page-1"),
									DBInstanceStatus:     aws.String("available"),
								},
							},
							Marker: aws.String("next-page-token"),
						},
						{
							DBInstances: []types.DBInstance{
								{
									DBInstanceIdentifier: aws.String("db-page-2"),
									DBInstanceStatus:     aws.String("available"),
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
					ID:        "db-page-1",
					Type:      domain.ResourceTypeRDS,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
				{
					ID:        "db-page-2",
					Type:      domain.ResourceTypeRDS,
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
			repo := &RDSRepository{
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

func TestRDSRepository_Tag(t *testing.T) {
	expDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		mockSetup      func() *mockRDSClient
		accountID      string
		region         string
		resourceID     string
		expirationDate time.Time
		wantARN        string
		wantErr        bool
		errMsg         string
	}{
		{
			name: "tags instance successfully",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					addTagsToResourceOutput: &rds.AddTagsToResourceOutput{},
				}
			},
			accountID:      "123456789012",
			region:         "us-east-1",
			resourceID:     "my-database",
			expirationDate: expDate,
			wantARN:        "arn:aws:rds:us-east-1:123456789012:db:my-database",
			wantErr:        false,
		},
		{
			name: "constructs correct ARN for different region",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					addTagsToResourceOutput: &rds.AddTagsToResourceOutput{},
				}
			},
			accountID:      "987654321098",
			region:         "eu-west-1",
			resourceID:     "prod-db",
			expirationDate: expDate,
			wantARN:        "arn:aws:rds:eu-west-1:987654321098:db:prod-db",
			wantErr:        false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					addTagsToResourceError: errors.New("access denied"),
				}
			},
			accountID:      "123456789012",
			region:         "us-east-1",
			resourceID:     "my-database",
			expirationDate: expDate,
			wantErr:        true,
			errMsg:         "tagging RDS instance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &RDSRepository{
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

func TestRDSRepository_Delete(t *testing.T) {
	tests := []struct {
		name       string
		mockSetup  func() *mockRDSClient
		resourceID string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "deletes instance successfully",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					describeDBInstancesOutput: &rds.DescribeDBInstancesOutput{
						DBInstances: []types.DBInstance{
							{
								DBInstanceIdentifier: aws.String("my-database"),
								DBInstanceStatus:     aws.String("available"),
								DeletionProtection:   aws.Bool(false),
							},
						},
					},
					deleteDBInstanceOutput: &rds.DeleteDBInstanceOutput{},
				}
			},
			resourceID: "my-database",
			wantErr:    false,
		},
		{
			name: "uses SkipFinalSnapshot",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					describeDBInstancesOutput: &rds.DescribeDBInstancesOutput{
						DBInstances: []types.DBInstance{
							{
								DBInstanceIdentifier: aws.String("test-db"),
								DBInstanceStatus:     aws.String("available"),
								DeletionProtection:   aws.Bool(false),
							},
						},
					},
					deleteDBInstanceOutput: &rds.DeleteDBInstanceOutput{},
				}
			},
			resourceID: "test-db",
			wantErr:    false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					describeDBInstancesOutput: &rds.DescribeDBInstancesOutput{
						DBInstances: []types.DBInstance{
							{
								DBInstanceIdentifier: aws.String("my-database"),
								DBInstanceStatus:     aws.String("available"),
								DeletionProtection:   aws.Bool(false),
							},
						},
					},
					deleteDBInstanceError: errors.New("cannot delete"),
				}
			},
			resourceID: "my-database",
			wantErr:    true,
			errMsg:     "deleting RDS instance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &RDSRepository{
				client:               mock,
				accountID:            "123456789012",
				region:               "us-east-1",
				forceDeleteProtected: false,
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
				// Verify DeleteDBInstance was called
				if !mock.deleteDBInstanceCalled {
					t.Fatal("DeleteDBInstance was not called")
				}
				if mock.deleteDBInstanceInput == nil {
					t.Fatal("DeleteDBInstance input is nil")
				}
				if aws.ToString(mock.deleteDBInstanceInput.DBInstanceIdentifier) != tt.resourceID {
					t.Errorf("Delete() DBInstanceIdentifier = %v, want %v",
						aws.ToString(mock.deleteDBInstanceInput.DBInstanceIdentifier), tt.resourceID)
				}
				// Verify SkipFinalSnapshot is true
				if !aws.ToBool(mock.deleteDBInstanceInput.SkipFinalSnapshot) {
					t.Error("Delete() should set SkipFinalSnapshot to true")
				}
			}
		})
	}
}

func TestRDSRepository_Delete_WithDeletionProtection(t *testing.T) {
	tests := []struct {
		name                 string
		mockSetup            func() *mockRDSClient
		forceDeleteProtected bool
		resourceID           string
		wantModifyCalled     bool
		wantDeleteCalled     bool
		wantErr              bool
		errMsg               string
	}{
		{
			name: "disables protection when forceDeleteProtected is true",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					// First describe call returns instance with deletion protection
					describeDBInstancesOutput: &rds.DescribeDBInstancesOutput{
						DBInstances: []types.DBInstance{
							{
								DBInstanceIdentifier: aws.String("protected-db"),
								DBInstanceStatus:     aws.String("available"),
								DeletionProtection:   aws.Bool(true),
							},
						},
					},
					modifyDBInstanceOutput: &rds.ModifyDBInstanceOutput{},
					deleteDBInstanceOutput: &rds.DeleteDBInstanceOutput{},
				}
			},
			forceDeleteProtected: true,
			resourceID:           "protected-db",
			wantModifyCalled:     true,
			wantDeleteCalled:     true,
			wantErr:              false,
		},
		{
			name: "skips deletion when forceDeleteProtected is false and instance is protected",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					describeDBInstancesOutput: &rds.DescribeDBInstancesOutput{
						DBInstances: []types.DBInstance{
							{
								DBInstanceIdentifier: aws.String("protected-db"),
								DBInstanceStatus:     aws.String("available"),
								DeletionProtection:   aws.Bool(true),
							},
						},
					},
				}
			},
			forceDeleteProtected: false,
			resourceID:           "protected-db",
			wantModifyCalled:     false,
			wantDeleteCalled:     false,
			wantErr:              true,
			errMsg:               "deletion protection enabled",
		},
		{
			name: "deletes normally when instance has no protection",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					describeDBInstancesOutput: &rds.DescribeDBInstancesOutput{
						DBInstances: []types.DBInstance{
							{
								DBInstanceIdentifier: aws.String("unprotected-db"),
								DBInstanceStatus:     aws.String("available"),
								DeletionProtection:   aws.Bool(false),
							},
						},
					},
					deleteDBInstanceOutput: &rds.DeleteDBInstanceOutput{},
				}
			},
			forceDeleteProtected: false,
			resourceID:           "unprotected-db",
			wantModifyCalled:     false,
			wantDeleteCalled:     true,
			wantErr:              false,
		},
		{
			name: "handles error when disabling protection fails",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					describeDBInstancesOutput: &rds.DescribeDBInstancesOutput{
						DBInstances: []types.DBInstance{
							{
								DBInstanceIdentifier: aws.String("protected-db"),
								DBInstanceStatus:     aws.String("available"),
								DeletionProtection:   aws.Bool(true),
							},
						},
					},
					modifyDBInstanceError: errors.New("modify failed"),
				}
			},
			forceDeleteProtected: true,
			resourceID:           "protected-db",
			wantModifyCalled:     true,
			wantDeleteCalled:     false,
			wantErr:              true,
			errMsg:               "disabling deletion protection",
		},
		{
			name: "handles error when describe fails",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					describeDBInstancesError: errors.New("describe failed"),
				}
			},
			forceDeleteProtected: false,
			resourceID:           "any-db",
			wantModifyCalled:     false,
			wantDeleteCalled:     false,
			wantErr:              true,
			errMsg:               "checking deletion protection",
		},
		{
			name: "handles instance not found during describe",
			mockSetup: func() *mockRDSClient {
				return &mockRDSClient{
					describeDBInstancesOutput: &rds.DescribeDBInstancesOutput{
						DBInstances: []types.DBInstance{},
					},
				}
			},
			forceDeleteProtected: false,
			resourceID:           "nonexistent-db",
			wantModifyCalled:     false,
			wantDeleteCalled:     false,
			wantErr:              true,
			errMsg:               "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &RDSRepository{
				client:               mock,
				accountID:            "123456789012",
				region:               "us-east-1",
				forceDeleteProtected: tt.forceDeleteProtected,
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

			if mock.modifyDBInstanceCalled != tt.wantModifyCalled {
				t.Errorf("ModifyDBInstance called = %v, want %v", mock.modifyDBInstanceCalled, tt.wantModifyCalled)
			}

			if mock.deleteDBInstanceCalled != tt.wantDeleteCalled {
				t.Errorf("DeleteDBInstance called = %v, want %v", mock.deleteDBInstanceCalled, tt.wantDeleteCalled)
			}

			// Verify modify input when called
			if tt.wantModifyCalled && mock.modifyDBInstanceInput != nil {
				if aws.ToString(mock.modifyDBInstanceInput.DBInstanceIdentifier) != tt.resourceID {
					t.Errorf("ModifyDBInstance ID = %v, want %v",
						aws.ToString(mock.modifyDBInstanceInput.DBInstanceIdentifier), tt.resourceID)
				}
				if mock.modifyDBInstanceInput.DeletionProtection == nil || *mock.modifyDBInstanceInput.DeletionProtection {
					t.Error("ModifyDBInstance should set DeletionProtection to false")
				}
			}
		})
	}
}

func TestRDSRepository_InterfaceCompliance(_ *testing.T) {
	// Verify RDSRepository implements domain.ResourceRepository
	var _ domain.ResourceRepository = (*RDSRepository)(nil)
}

func TestRDSRepository_buildARN(t *testing.T) {
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
			resourceID: "my-database",
			want:       "arn:aws:rds:us-east-1:123456789012:db:my-database",
		},
		{
			name:       "different region",
			accountID:  "987654321098",
			region:     "eu-central-1",
			resourceID: "prod-db-1",
			want:       "arn:aws:rds:eu-central-1:987654321098:db:prod-db-1",
		},
		{
			name:       "database with hyphens",
			accountID:  "111222333444",
			region:     "ap-southeast-1",
			resourceID: "my-complex-database-name",
			want:       "arn:aws:rds:ap-southeast-1:111222333444:db:my-complex-database-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &RDSRepository{
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

// containsString checks if s contains substr
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStringHelper(s, substr))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestNewRDSRepository(t *testing.T) {
	tests := []struct {
		name                 string
		accountID            string
		region               string
		forceDeleteProtected bool
	}{
		{
			name:                 "creates repository with all fields",
			accountID:            "123456789012",
			region:               "us-east-1",
			forceDeleteProtected: true,
		},
		{
			name:                 "creates repository without force delete",
			accountID:            "987654321098",
			region:               "eu-west-1",
			forceDeleteProtected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a nil client since we're just testing field assignment
			repo := NewRDSRepository(nil, tt.accountID, tt.region, tt.forceDeleteProtected)

			if repo == nil {
				t.Fatal("NewRDSRepository() returned nil")
			}
			if repo.accountID != tt.accountID {
				t.Errorf("accountID = %v, want %v", repo.accountID, tt.accountID)
			}
			if repo.region != tt.region {
				t.Errorf("region = %v, want %v", repo.region, tt.region)
			}
			if repo.forceDeleteProtected != tt.forceDeleteProtected {
				t.Errorf("forceDeleteProtected = %v, want %v", repo.forceDeleteProtected, tt.forceDeleteProtected)
			}
		})
	}
}

func TestRDSRepository_instanceToResource(t *testing.T) {
	now := time.Now()
	createdAt := now.AddDate(0, -1, 0)
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name     string
		instance types.DBInstance
		want     domain.Resource
	}{
		{
			name: "basic instance",
			instance: types.DBInstance{
				DBInstanceIdentifier: aws.String("test-db"),
				DBInstanceStatus:     aws.String("available"),
				InstanceCreateTime:   &createdAt,
			},
			want: domain.Resource{
				ID:        "test-db",
				Type:      domain.ResourceTypeRDS,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Tags:      map[string]string{},
			},
		},
		{
			name: "instance with Name tag",
			instance: types.DBInstance{
				DBInstanceIdentifier: aws.String("prod-db"),
				DBInstanceStatus:     aws.String("available"),
				TagList: []types.Tag{
					{Key: aws.String("Name"), Value: aws.String("Production Database")},
				},
			},
			want: domain.Resource{
				ID:        "prod-db",
				Type:      domain.ResourceTypeRDS,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Name:      "Production Database",
				Tags: map[string]string{
					"Name": "Production Database",
				},
			},
		},
		{
			name: "instance with expiration date",
			instance: types.DBInstance{
				DBInstanceIdentifier: aws.String("expiring-db"),
				DBInstanceStatus:     aws.String("available"),
				TagList: []types.Tag{
					{Key: aws.String(ExpirationTagName), Value: aws.String(expDateStr)},
				},
			},
			want: domain.Resource{
				ID:        "expiring-db",
				Type:      domain.ResourceTypeRDS,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Tags: map[string]string{
					ExpirationTagName: expDateStr,
				},
			},
		},
		{
			name: "instance that never expires",
			instance: types.DBInstance{
				DBInstanceIdentifier: aws.String("permanent-db"),
				DBInstanceStatus:     aws.String("available"),
				TagList: []types.Tag{
					{Key: aws.String(ExpirationTagName), Value: aws.String(NeverExpiresValue)},
				},
			},
			want: domain.Resource{
				ID:           "permanent-db",
				Type:         domain.ResourceTypeRDS,
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
			repo := &RDSRepository{
				accountID: "123456789012",
				region:    "us-east-1",
			}

			got := repo.instanceToResource(tt.instance)

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
