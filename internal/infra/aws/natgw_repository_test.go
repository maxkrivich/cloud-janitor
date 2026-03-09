package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check for mockNATGatewayClient.
var _ natGatewayClient = (*mockNATGatewayClient)(nil)

// mockNATGatewayClient implements natGatewayClient for testing.
type mockNATGatewayClient struct {
	describeNatGatewaysOutput *ec2.DescribeNatGatewaysOutput
	describeNatGatewaysError  error
	describeNatGatewaysPages  []*ec2.DescribeNatGatewaysOutput
	pageIndex                 int

	createTagsOutput *ec2.CreateTagsOutput
	createTagsError  error
	createTagsInput  *ec2.CreateTagsInput

	deleteNatGatewayOutput *ec2.DeleteNatGatewayOutput
	deleteNatGatewayError  error
	deleteNatGatewayInput  *ec2.DeleteNatGatewayInput
	deleteNatGatewayCalled bool
}

func (m *mockNATGatewayClient) DescribeNatGateways(_ context.Context, _ *ec2.DescribeNatGatewaysInput, _ ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error) {
	if m.describeNatGatewaysError != nil {
		return nil, m.describeNatGatewaysError
	}
	if len(m.describeNatGatewaysPages) > 0 {
		if m.pageIndex >= len(m.describeNatGatewaysPages) {
			return &ec2.DescribeNatGatewaysOutput{}, nil
		}
		result := m.describeNatGatewaysPages[m.pageIndex]
		m.pageIndex++
		return result, nil
	}
	return m.describeNatGatewaysOutput, nil
}

func (m *mockNATGatewayClient) CreateTags(_ context.Context, params *ec2.CreateTagsInput, _ ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error) {
	m.createTagsInput = params
	if m.createTagsError != nil {
		return nil, m.createTagsError
	}
	return m.createTagsOutput, nil
}

func (m *mockNATGatewayClient) DeleteNatGateway(_ context.Context, params *ec2.DeleteNatGatewayInput, _ ...func(*ec2.Options)) (*ec2.DeleteNatGatewayOutput, error) {
	m.deleteNatGatewayInput = params
	m.deleteNatGatewayCalled = true
	if m.deleteNatGatewayError != nil {
		return nil, m.deleteNatGatewayError
	}
	return m.deleteNatGatewayOutput, nil
}

func TestNATGatewayRepository_Type(t *testing.T) {
	repo := &NATGatewayRepository{
		client:    &mockNATGatewayClient{},
		accountID: "123456789012",
		region:    "us-east-1",
	}
	got := repo.Type()
	want := domain.ResourceTypeNATGateway

	if got != want {
		t.Errorf("Type() = %v, want %v", got, want)
	}
}

func TestNATGatewayRepository_List(t *testing.T) {
	now := time.Now()
	createdAt := now.AddDate(0, -1, 0)
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name      string
		mockSetup func() *mockNATGatewayClient
		accountID string
		region    string
		want      []domain.Resource
		wantErr   bool
		errMsg    string
	}{
		{
			name: "lists NAT Gateways successfully",
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					describeNatGatewaysOutput: &ec2.DescribeNatGatewaysOutput{
						NatGateways: []types.NatGateway{
							{
								NatGatewayId: aws.String("nat-0abc123def456789"),
								State:        types.NatGatewayStateAvailable,
								CreateTime:   &createdAt,
								Tags: []types.Tag{
									{Key: aws.String("Name"), Value: aws.String("my-nat-gateway")},
									{Key: aws.String("Environment"), Value: aws.String("dev")},
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
					ID:             "nat-0abc123def456789",
					Type:           domain.ResourceTypeNATGateway,
					Region:         "us-east-1",
					AccountID:      "123456789012",
					Name:           "my-nat-gateway",
					ExpirationDate: nil,
					NeverExpires:   false,
					Tags: map[string]string{
						"Name":        "my-nat-gateway",
						"Environment": "dev",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "parses expiration date tag",
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					describeNatGatewaysOutput: &ec2.DescribeNatGatewaysOutput{
						NatGateways: []types.NatGateway{
							{
								NatGatewayId: aws.String("nat-expiring123"),
								State:        types.NatGatewayStateAvailable,
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
					ID:        "nat-expiring123",
					Type:      domain.ResourceTypeNATGateway,
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
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					describeNatGatewaysOutput: &ec2.DescribeNatGatewaysOutput{
						NatGateways: []types.NatGateway{
							{
								NatGatewayId: aws.String("nat-permanent123"),
								State:        types.NatGatewayStateAvailable,
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
					ID:           "nat-permanent123",
					Type:         domain.ResourceTypeNATGateway,
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
			name: "handles API error",
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					describeNatGatewaysError: errors.New("API error"),
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   true,
			errMsg:    "listing NAT Gateways",
		},
		{
			name: "handles empty result",
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					describeNatGatewaysOutput: &ec2.DescribeNatGatewaysOutput{
						NatGateways: []types.NatGateway{},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   false,
		},
		{
			name: "includes pending NAT Gateways",
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					describeNatGatewaysOutput: &ec2.DescribeNatGatewaysOutput{
						NatGateways: []types.NatGateway{
							{
								NatGatewayId: aws.String("nat-pending123"),
								State:        types.NatGatewayStatePending,
								Tags: []types.Tag{
									{Key: aws.String("Name"), Value: aws.String("pending-nat")},
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
					ID:        "nat-pending123",
					Type:      domain.ResourceTypeNATGateway,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Name:      "pending-nat",
					Tags: map[string]string{
						"Name": "pending-nat",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &NATGatewayRepository{
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
				if resource.Name != tt.want[i].Name {
					t.Errorf("List()[%d].Name = %v, want %v", i, resource.Name, tt.want[i].Name)
				}
			}
		})
	}
}

func TestNATGatewayRepository_List_FiltersInvalidStates(t *testing.T) {
	tests := []struct {
		name          string
		mockSetup     func() *mockNATGatewayClient
		wantCount     int
		wantResources []string // Expected resource IDs
	}{
		{
			name: "skips deleted NAT Gateways",
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					describeNatGatewaysOutput: &ec2.DescribeNatGatewaysOutput{
						NatGateways: []types.NatGateway{
							{
								NatGatewayId: aws.String("nat-available123"),
								State:        types.NatGatewayStateAvailable,
							},
							{
								NatGatewayId: aws.String("nat-deleted123"),
								State:        types.NatGatewayStateDeleted,
							},
						},
					},
				}
			},
			wantCount:     1,
			wantResources: []string{"nat-available123"},
		},
		{
			name: "skips deleting NAT Gateways",
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					describeNatGatewaysOutput: &ec2.DescribeNatGatewaysOutput{
						NatGateways: []types.NatGateway{
							{
								NatGatewayId: aws.String("nat-available123"),
								State:        types.NatGatewayStateAvailable,
							},
							{
								NatGatewayId: aws.String("nat-deleting123"),
								State:        types.NatGatewayStateDeleting,
							},
						},
					},
				}
			},
			wantCount:     1,
			wantResources: []string{"nat-available123"},
		},
		{
			name: "skips failed NAT Gateways",
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					describeNatGatewaysOutput: &ec2.DescribeNatGatewaysOutput{
						NatGateways: []types.NatGateway{
							{
								NatGatewayId: aws.String("nat-available123"),
								State:        types.NatGatewayStateAvailable,
							},
							{
								NatGatewayId: aws.String("nat-failed123"),
								State:        types.NatGatewayStateFailed,
							},
						},
					},
				}
			},
			wantCount:     1,
			wantResources: []string{"nat-available123"},
		},
		{
			name: "includes both available and pending",
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					describeNatGatewaysOutput: &ec2.DescribeNatGatewaysOutput{
						NatGateways: []types.NatGateway{
							{
								NatGatewayId: aws.String("nat-available123"),
								State:        types.NatGatewayStateAvailable,
							},
							{
								NatGatewayId: aws.String("nat-pending123"),
								State:        types.NatGatewayStatePending,
							},
						},
					},
				}
			},
			wantCount:     2,
			wantResources: []string{"nat-available123", "nat-pending123"},
		},
		{
			name: "filters out all invalid states",
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					describeNatGatewaysOutput: &ec2.DescribeNatGatewaysOutput{
						NatGateways: []types.NatGateway{
							{
								NatGatewayId: aws.String("nat-available123"),
								State:        types.NatGatewayStateAvailable,
							},
							{
								NatGatewayId: aws.String("nat-pending123"),
								State:        types.NatGatewayStatePending,
							},
							{
								NatGatewayId: aws.String("nat-deleting123"),
								State:        types.NatGatewayStateDeleting,
							},
							{
								NatGatewayId: aws.String("nat-deleted123"),
								State:        types.NatGatewayStateDeleted,
							},
							{
								NatGatewayId: aws.String("nat-failed123"),
								State:        types.NatGatewayStateFailed,
							},
						},
					},
				}
			},
			wantCount:     2,
			wantResources: []string{"nat-available123", "nat-pending123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &NATGatewayRepository{
				client:    mock,
				accountID: "123456789012",
				region:    "us-east-1",
			}

			got, err := repo.List(context.Background(), "")
			if err != nil {
				t.Errorf("List() unexpected error = %v", err)
				return
			}

			if len(got) != tt.wantCount {
				t.Errorf("List() returned %d resources, want %d", len(got), tt.wantCount)
				return
			}

			for i, resource := range got {
				if i >= len(tt.wantResources) {
					t.Errorf("List() returned more resources than expected")
					return
				}
				if resource.ID != tt.wantResources[i] {
					t.Errorf("List()[%d].ID = %v, want %v", i, resource.ID, tt.wantResources[i])
				}
			}
		})
	}
}

func TestNATGatewayRepository_List_Pagination(t *testing.T) {
	tests := []struct {
		name      string
		mockSetup func() *mockNATGatewayClient
		wantCount int
		wantIDs   []string
	}{
		{
			name: "handles pagination across multiple pages",
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					describeNatGatewaysPages: []*ec2.DescribeNatGatewaysOutput{
						{
							NatGateways: []types.NatGateway{
								{
									NatGatewayId: aws.String("nat-page1-a"),
									State:        types.NatGatewayStateAvailable,
								},
								{
									NatGatewayId: aws.String("nat-page1-b"),
									State:        types.NatGatewayStateAvailable,
								},
							},
							NextToken: aws.String("next-page-token"),
						},
						{
							NatGateways: []types.NatGateway{
								{
									NatGatewayId: aws.String("nat-page2-a"),
									State:        types.NatGatewayStateAvailable,
								},
							},
						},
					},
				}
			},
			wantCount: 3,
			wantIDs:   []string{"nat-page1-a", "nat-page1-b", "nat-page2-a"},
		},
		{
			name: "filters invalid states across pages",
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					describeNatGatewaysPages: []*ec2.DescribeNatGatewaysOutput{
						{
							NatGateways: []types.NatGateway{
								{
									NatGatewayId: aws.String("nat-available1"),
									State:        types.NatGatewayStateAvailable,
								},
								{
									NatGatewayId: aws.String("nat-deleted1"),
									State:        types.NatGatewayStateDeleted,
								},
							},
							NextToken: aws.String("next-page-token"),
						},
						{
							NatGateways: []types.NatGateway{
								{
									NatGatewayId: aws.String("nat-pending1"),
									State:        types.NatGatewayStatePending,
								},
								{
									NatGatewayId: aws.String("nat-failed1"),
									State:        types.NatGatewayStateFailed,
								},
							},
						},
					},
				}
			},
			wantCount: 2,
			wantIDs:   []string{"nat-available1", "nat-pending1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &NATGatewayRepository{
				client:    mock,
				accountID: "123456789012",
				region:    "us-east-1",
			}

			got, err := repo.List(context.Background(), "")
			if err != nil {
				t.Errorf("List() unexpected error = %v", err)
				return
			}

			if len(got) != tt.wantCount {
				t.Errorf("List() returned %d resources, want %d", len(got), tt.wantCount)
				return
			}

			for i, resource := range got {
				if i >= len(tt.wantIDs) {
					t.Errorf("List() returned more resources than expected")
					return
				}
				if resource.ID != tt.wantIDs[i] {
					t.Errorf("List()[%d].ID = %v, want %v", i, resource.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestNATGatewayRepository_Tag(t *testing.T) {
	expDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		mockSetup      func() *mockNATGatewayClient
		resourceID     string
		expirationDate time.Time
		wantErr        bool
		errMsg         string
	}{
		{
			name: "tags NAT Gateway successfully",
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					createTagsOutput: &ec2.CreateTagsOutput{},
				}
			},
			resourceID:     "nat-0abc123def456789",
			expirationDate: expDate,
			wantErr:        false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					createTagsError: errors.New("access denied"),
				}
			},
			resourceID:     "nat-0abc123def456789",
			expirationDate: expDate,
			wantErr:        true,
			errMsg:         "tagging NAT Gateway",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &NATGatewayRepository{
				client:    mock,
				accountID: "123456789012",
				region:    "us-east-1",
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
				// Verify the resource ID was passed correctly
				if mock.createTagsInput == nil {
					t.Fatal("CreateTags was not called")
				}
				if len(mock.createTagsInput.Resources) != 1 {
					t.Errorf("Tag() passed %d resources, want 1", len(mock.createTagsInput.Resources))
				}
				if mock.createTagsInput.Resources[0] != tt.resourceID {
					t.Errorf("Tag() resource ID = %v, want %v", mock.createTagsInput.Resources[0], tt.resourceID)
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

func TestNATGatewayRepository_Delete(t *testing.T) {
	tests := []struct {
		name       string
		mockSetup  func() *mockNATGatewayClient
		resourceID string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "deletes NAT Gateway successfully",
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					deleteNatGatewayOutput: &ec2.DeleteNatGatewayOutput{},
				}
			},
			resourceID: "nat-0abc123def456789",
			wantErr:    false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockNATGatewayClient {
				return &mockNATGatewayClient{
					deleteNatGatewayError: errors.New("NAT Gateway not found"),
				}
			},
			resourceID: "nat-nonexistent123",
			wantErr:    true,
			errMsg:     "deleting NAT Gateway",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &NATGatewayRepository{
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
				// Verify DeleteNatGateway was called
				if !mock.deleteNatGatewayCalled {
					t.Fatal("DeleteNatGateway was not called")
				}
				if mock.deleteNatGatewayInput == nil {
					t.Fatal("DeleteNatGateway input is nil")
				}
				if aws.ToString(mock.deleteNatGatewayInput.NatGatewayId) != tt.resourceID {
					t.Errorf("Delete() NatGatewayId = %v, want %v",
						aws.ToString(mock.deleteNatGatewayInput.NatGatewayId), tt.resourceID)
				}
			}
		})
	}
}

func TestNATGatewayRepository_InterfaceCompliance(_ *testing.T) {
	// Verify NATGatewayRepository implements domain.ResourceRepository
	var _ domain.ResourceRepository = (*NATGatewayRepository)(nil)
}

func TestNewNATGatewayRepository(t *testing.T) {
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
			repo := NewNATGatewayRepository(nil, tt.accountID, tt.region)

			if repo == nil {
				t.Fatal("NewNATGatewayRepository() returned nil")
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

func TestNATGatewayRepository_natGatewayToResource(t *testing.T) {
	now := time.Now()
	createdAt := now.AddDate(0, -1, 0)
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name       string
		natGateway types.NatGateway
		want       domain.Resource
	}{
		{
			name: "basic NAT Gateway",
			natGateway: types.NatGateway{
				NatGatewayId: aws.String("nat-basic123"),
				State:        types.NatGatewayStateAvailable,
				CreateTime:   &createdAt,
			},
			want: domain.Resource{
				ID:        "nat-basic123",
				Type:      domain.ResourceTypeNATGateway,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Tags:      map[string]string{},
			},
		},
		{
			name: "NAT Gateway with Name tag",
			natGateway: types.NatGateway{
				NatGatewayId: aws.String("nat-named123"),
				State:        types.NatGatewayStateAvailable,
				Tags: []types.Tag{
					{Key: aws.String("Name"), Value: aws.String("Production NAT")},
				},
			},
			want: domain.Resource{
				ID:        "nat-named123",
				Type:      domain.ResourceTypeNATGateway,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Name:      "Production NAT",
				Tags: map[string]string{
					"Name": "Production NAT",
				},
			},
		},
		{
			name: "NAT Gateway with expiration date",
			natGateway: types.NatGateway{
				NatGatewayId: aws.String("nat-expiring123"),
				State:        types.NatGatewayStateAvailable,
				Tags: []types.Tag{
					{Key: aws.String(ExpirationTagName), Value: aws.String(expDateStr)},
				},
			},
			want: domain.Resource{
				ID:        "nat-expiring123",
				Type:      domain.ResourceTypeNATGateway,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Tags: map[string]string{
					ExpirationTagName: expDateStr,
				},
			},
		},
		{
			name: "NAT Gateway that never expires",
			natGateway: types.NatGateway{
				NatGatewayId: aws.String("nat-permanent123"),
				State:        types.NatGatewayStateAvailable,
				Tags: []types.Tag{
					{Key: aws.String(ExpirationTagName), Value: aws.String(NeverExpiresValue)},
				},
			},
			want: domain.Resource{
				ID:           "nat-permanent123",
				Type:         domain.ResourceTypeNATGateway,
				Region:       "us-east-1",
				AccountID:    "123456789012",
				NeverExpires: true,
				Tags: map[string]string{
					ExpirationTagName: NeverExpiresValue,
				},
			},
		},
		{
			name: "NAT Gateway with multiple tags",
			natGateway: types.NatGateway{
				NatGatewayId: aws.String("nat-multitag123"),
				State:        types.NatGatewayStateAvailable,
				Tags: []types.Tag{
					{Key: aws.String("Name"), Value: aws.String("Multi-Tag NAT")},
					{Key: aws.String("Environment"), Value: aws.String("dev")},
					{Key: aws.String("Team"), Value: aws.String("platform")},
				},
			},
			want: domain.Resource{
				ID:        "nat-multitag123",
				Type:      domain.ResourceTypeNATGateway,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Name:      "Multi-Tag NAT",
				Tags: map[string]string{
					"Name":        "Multi-Tag NAT",
					"Environment": "dev",
					"Team":        "platform",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &NATGatewayRepository{
				accountID: "123456789012",
				region:    "us-east-1",
			}

			got := repo.natGatewayToResource(tt.natGateway)

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

func TestNATGatewayRepository_isValidState(t *testing.T) {
	tests := []struct {
		name  string
		state types.NatGatewayState
		want  bool
	}{
		{
			name:  "available state is valid",
			state: types.NatGatewayStateAvailable,
			want:  true,
		},
		{
			name:  "pending state is valid",
			state: types.NatGatewayStatePending,
			want:  true,
		},
		{
			name:  "deleting state is invalid",
			state: types.NatGatewayStateDeleting,
			want:  false,
		},
		{
			name:  "deleted state is invalid",
			state: types.NatGatewayStateDeleted,
			want:  false,
		},
		{
			name:  "failed state is invalid",
			state: types.NatGatewayStateFailed,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &NATGatewayRepository{}
			got := repo.isValidState(tt.state)
			if got != tt.want {
				t.Errorf("isValidState(%v) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}
