package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check for mockOpenSearchClient.
var _ openSearchClient = (*mockOpenSearchClient)(nil)

// mockOpenSearchClient implements openSearchClient for testing.
type mockOpenSearchClient struct {
	listDomainNamesOutput *opensearch.ListDomainNamesOutput
	listDomainNamesError  error

	describeDomainOutputs map[string]*opensearch.DescribeDomainOutput
	describeDomainError   error

	listTagsOutputs map[string]*opensearch.ListTagsOutput
	listTagsError   error

	addTagsOutput *opensearch.AddTagsOutput
	addTagsError  error
	addTagsInput  *opensearch.AddTagsInput

	deleteDomainOutput *opensearch.DeleteDomainOutput
	deleteDomainError  error
	deleteDomainInput  *opensearch.DeleteDomainInput
	deleteDomainCalled bool
}

func (m *mockOpenSearchClient) ListDomainNames(_ context.Context, _ *opensearch.ListDomainNamesInput, _ ...func(*opensearch.Options)) (*opensearch.ListDomainNamesOutput, error) {
	if m.listDomainNamesError != nil {
		return nil, m.listDomainNamesError
	}
	return m.listDomainNamesOutput, nil
}

func (m *mockOpenSearchClient) DescribeDomain(_ context.Context, params *opensearch.DescribeDomainInput, _ ...func(*opensearch.Options)) (*opensearch.DescribeDomainOutput, error) {
	if m.describeDomainError != nil {
		return nil, m.describeDomainError
	}
	domainName := aws.ToString(params.DomainName)
	if output, ok := m.describeDomainOutputs[domainName]; ok {
		return output, nil
	}
	return nil, errors.New("domain not found")
}

func (m *mockOpenSearchClient) ListTags(_ context.Context, params *opensearch.ListTagsInput, _ ...func(*opensearch.Options)) (*opensearch.ListTagsOutput, error) {
	if m.listTagsError != nil {
		return nil, m.listTagsError
	}
	arn := aws.ToString(params.ARN)
	if output, ok := m.listTagsOutputs[arn]; ok {
		return output, nil
	}
	return &opensearch.ListTagsOutput{TagList: []types.Tag{}}, nil
}

func (m *mockOpenSearchClient) AddTags(_ context.Context, params *opensearch.AddTagsInput, _ ...func(*opensearch.Options)) (*opensearch.AddTagsOutput, error) {
	m.addTagsInput = params
	if m.addTagsError != nil {
		return nil, m.addTagsError
	}
	return m.addTagsOutput, nil
}

func (m *mockOpenSearchClient) DeleteDomain(_ context.Context, params *opensearch.DeleteDomainInput, _ ...func(*opensearch.Options)) (*opensearch.DeleteDomainOutput, error) {
	m.deleteDomainInput = params
	m.deleteDomainCalled = true
	if m.deleteDomainError != nil {
		return nil, m.deleteDomainError
	}
	return m.deleteDomainOutput, nil
}

func TestOpenSearchRepository_Type(t *testing.T) {
	repo := &OpenSearchRepository{
		client:    &mockOpenSearchClient{},
		accountID: "123456789012",
		region:    "us-east-1",
	}
	got := repo.Type()
	want := domain.ResourceTypeOpenSearch

	if got != want {
		t.Errorf("Type() = %v, want %v", got, want)
	}
}

func TestOpenSearchRepository_List(t *testing.T) {
	now := time.Now()
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name      string
		mockSetup func() *mockOpenSearchClient
		accountID string
		region    string
		want      []domain.Resource
		wantErr   bool
		errMsg    string
	}{
		{
			name: "lists domains successfully",
			mockSetup: func() *mockOpenSearchClient {
				return &mockOpenSearchClient{
					listDomainNamesOutput: &opensearch.ListDomainNamesOutput{
						DomainNames: []types.DomainInfo{
							{DomainName: aws.String("my-domain")},
						},
					},
					describeDomainOutputs: map[string]*opensearch.DescribeDomainOutput{
						"my-domain": {
							DomainStatus: &types.DomainStatus{
								DomainName: aws.String("my-domain"),
								ARN:        aws.String("arn:aws:es:us-east-1:123456789012:domain/my-domain"),
								Created:    aws.Bool(true),
								Deleted:    aws.Bool(false),
								Processing: aws.Bool(false),
							},
						},
					},
					listTagsOutputs: map[string]*opensearch.ListTagsOutput{
						"arn:aws:es:us-east-1:123456789012:domain/my-domain": {
							TagList: []types.Tag{
								{Key: aws.String("Name"), Value: aws.String("My Domain")},
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
					ID:             "my-domain",
					Type:           domain.ResourceTypeOpenSearch,
					Region:         "us-east-1",
					AccountID:      "123456789012",
					Name:           "My Domain",
					ExpirationDate: nil,
					NeverExpires:   false,
					Tags: map[string]string{
						"Name":        "My Domain",
						"Environment": "dev",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "parses expiration date tag",
			mockSetup: func() *mockOpenSearchClient {
				return &mockOpenSearchClient{
					listDomainNamesOutput: &opensearch.ListDomainNamesOutput{
						DomainNames: []types.DomainInfo{
							{DomainName: aws.String("expiring-domain")},
						},
					},
					describeDomainOutputs: map[string]*opensearch.DescribeDomainOutput{
						"expiring-domain": {
							DomainStatus: &types.DomainStatus{
								DomainName: aws.String("expiring-domain"),
								ARN:        aws.String("arn:aws:es:us-west-2:123456789012:domain/expiring-domain"),
								Created:    aws.Bool(true),
								Deleted:    aws.Bool(false),
							},
						},
					},
					listTagsOutputs: map[string]*opensearch.ListTagsOutput{
						"arn:aws:es:us-west-2:123456789012:domain/expiring-domain": {
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
					ID:        "expiring-domain",
					Type:      domain.ResourceTypeOpenSearch,
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
			mockSetup: func() *mockOpenSearchClient {
				return &mockOpenSearchClient{
					listDomainNamesOutput: &opensearch.ListDomainNamesOutput{
						DomainNames: []types.DomainInfo{
							{DomainName: aws.String("permanent-domain")},
						},
					},
					describeDomainOutputs: map[string]*opensearch.DescribeDomainOutput{
						"permanent-domain": {
							DomainStatus: &types.DomainStatus{
								DomainName: aws.String("permanent-domain"),
								ARN:        aws.String("arn:aws:es:us-east-1:123456789012:domain/permanent-domain"),
								Created:    aws.Bool(true),
								Deleted:    aws.Bool(false),
							},
						},
					},
					listTagsOutputs: map[string]*opensearch.ListTagsOutput{
						"arn:aws:es:us-east-1:123456789012:domain/permanent-domain": {
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
					ID:           "permanent-domain",
					Type:         domain.ResourceTypeOpenSearch,
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
			name: "skips deleted domains",
			mockSetup: func() *mockOpenSearchClient {
				return &mockOpenSearchClient{
					listDomainNamesOutput: &opensearch.ListDomainNamesOutput{
						DomainNames: []types.DomainInfo{
							{DomainName: aws.String("active-domain")},
							{DomainName: aws.String("deleted-domain")},
						},
					},
					describeDomainOutputs: map[string]*opensearch.DescribeDomainOutput{
						"active-domain": {
							DomainStatus: &types.DomainStatus{
								DomainName: aws.String("active-domain"),
								ARN:        aws.String("arn:aws:es:us-east-1:123456789012:domain/active-domain"),
								Created:    aws.Bool(true),
								Deleted:    aws.Bool(false),
							},
						},
						"deleted-domain": {
							DomainStatus: &types.DomainStatus{
								DomainName: aws.String("deleted-domain"),
								ARN:        aws.String("arn:aws:es:us-east-1:123456789012:domain/deleted-domain"),
								Created:    aws.Bool(true),
								Deleted:    aws.Bool(true),
							},
						},
					},
					listTagsOutputs: map[string]*opensearch.ListTagsOutput{
						"arn:aws:es:us-east-1:123456789012:domain/active-domain": {
							TagList: []types.Tag{},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "active-domain",
					Type:      domain.ResourceTypeOpenSearch,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "handles API error for ListDomainNames",
			mockSetup: func() *mockOpenSearchClient {
				return &mockOpenSearchClient{
					listDomainNamesError: errors.New("API error"),
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   true,
			errMsg:    "listing OpenSearch domain names",
		},
		{
			name: "handles API error for DescribeDomain",
			mockSetup: func() *mockOpenSearchClient {
				return &mockOpenSearchClient{
					listDomainNamesOutput: &opensearch.ListDomainNamesOutput{
						DomainNames: []types.DomainInfo{
							{DomainName: aws.String("my-domain")},
						},
					},
					describeDomainError: errors.New("describe error"),
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   true,
			errMsg:    "describing OpenSearch domain",
		},
		{
			name: "handles API error for ListTags",
			mockSetup: func() *mockOpenSearchClient {
				return &mockOpenSearchClient{
					listDomainNamesOutput: &opensearch.ListDomainNamesOutput{
						DomainNames: []types.DomainInfo{
							{DomainName: aws.String("my-domain")},
						},
					},
					describeDomainOutputs: map[string]*opensearch.DescribeDomainOutput{
						"my-domain": {
							DomainStatus: &types.DomainStatus{
								DomainName: aws.String("my-domain"),
								ARN:        aws.String("arn:aws:es:us-east-1:123456789012:domain/my-domain"),
								Created:    aws.Bool(true),
								Deleted:    aws.Bool(false),
							},
						},
					},
					listTagsError: errors.New("list tags error"),
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   true,
			errMsg:    "listing tags for OpenSearch domain",
		},
		{
			name: "handles empty results",
			mockSetup: func() *mockOpenSearchClient {
				return &mockOpenSearchClient{
					listDomainNamesOutput: &opensearch.ListDomainNamesOutput{
						DomainNames: []types.DomainInfo{},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   false,
		},
		{
			name: "handles multiple domains",
			mockSetup: func() *mockOpenSearchClient {
				return &mockOpenSearchClient{
					listDomainNamesOutput: &opensearch.ListDomainNamesOutput{
						DomainNames: []types.DomainInfo{
							{DomainName: aws.String("domain-1")},
							{DomainName: aws.String("domain-2")},
							{DomainName: aws.String("domain-3")},
						},
					},
					describeDomainOutputs: map[string]*opensearch.DescribeDomainOutput{
						"domain-1": {
							DomainStatus: &types.DomainStatus{
								DomainName: aws.String("domain-1"),
								ARN:        aws.String("arn:aws:es:us-east-1:123456789012:domain/domain-1"),
								Created:    aws.Bool(true),
								Deleted:    aws.Bool(false),
							},
						},
						"domain-2": {
							DomainStatus: &types.DomainStatus{
								DomainName: aws.String("domain-2"),
								ARN:        aws.String("arn:aws:es:us-east-1:123456789012:domain/domain-2"),
								Created:    aws.Bool(true),
								Deleted:    aws.Bool(false),
							},
						},
						"domain-3": {
							DomainStatus: &types.DomainStatus{
								DomainName: aws.String("domain-3"),
								ARN:        aws.String("arn:aws:es:us-east-1:123456789012:domain/domain-3"),
								Created:    aws.Bool(true),
								Deleted:    aws.Bool(false),
							},
						},
					},
					listTagsOutputs: map[string]*opensearch.ListTagsOutput{
						"arn:aws:es:us-east-1:123456789012:domain/domain-1": {TagList: []types.Tag{}},
						"arn:aws:es:us-east-1:123456789012:domain/domain-2": {TagList: []types.Tag{}},
						"arn:aws:es:us-east-1:123456789012:domain/domain-3": {TagList: []types.Tag{}},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "domain-1",
					Type:      domain.ResourceTypeOpenSearch,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
				{
					ID:        "domain-2",
					Type:      domain.ResourceTypeOpenSearch,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
				{
					ID:        "domain-3",
					Type:      domain.ResourceTypeOpenSearch,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "includes processing domains",
			mockSetup: func() *mockOpenSearchClient {
				return &mockOpenSearchClient{
					listDomainNamesOutput: &opensearch.ListDomainNamesOutput{
						DomainNames: []types.DomainInfo{
							{DomainName: aws.String("processing-domain")},
						},
					},
					describeDomainOutputs: map[string]*opensearch.DescribeDomainOutput{
						"processing-domain": {
							DomainStatus: &types.DomainStatus{
								DomainName: aws.String("processing-domain"),
								ARN:        aws.String("arn:aws:es:us-east-1:123456789012:domain/processing-domain"),
								Created:    aws.Bool(true),
								Deleted:    aws.Bool(false),
								Processing: aws.Bool(true),
							},
						},
					},
					listTagsOutputs: map[string]*opensearch.ListTagsOutput{
						"arn:aws:es:us-east-1:123456789012:domain/processing-domain": {TagList: []types.Tag{}},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "processing-domain",
					Type:      domain.ResourceTypeOpenSearch,
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
			repo := &OpenSearchRepository{
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

func TestOpenSearchRepository_Tag(t *testing.T) {
	expDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		mockSetup      func() *mockOpenSearchClient
		accountID      string
		region         string
		resourceID     string
		expirationDate time.Time
		wantARN        string
		wantErr        bool
		errMsg         string
	}{
		{
			name: "tags domain successfully",
			mockSetup: func() *mockOpenSearchClient {
				return &mockOpenSearchClient{
					addTagsOutput: &opensearch.AddTagsOutput{},
				}
			},
			accountID:      "123456789012",
			region:         "us-east-1",
			resourceID:     "my-domain",
			expirationDate: expDate,
			wantARN:        "arn:aws:es:us-east-1:123456789012:domain/my-domain",
			wantErr:        false,
		},
		{
			name: "constructs correct ARN for different region",
			mockSetup: func() *mockOpenSearchClient {
				return &mockOpenSearchClient{
					addTagsOutput: &opensearch.AddTagsOutput{},
				}
			},
			accountID:      "987654321098",
			region:         "eu-west-1",
			resourceID:     "prod-domain",
			expirationDate: expDate,
			wantARN:        "arn:aws:es:eu-west-1:987654321098:domain/prod-domain",
			wantErr:        false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockOpenSearchClient {
				return &mockOpenSearchClient{
					addTagsError: errors.New("access denied"),
				}
			},
			accountID:      "123456789012",
			region:         "us-east-1",
			resourceID:     "my-domain",
			expirationDate: expDate,
			wantErr:        true,
			errMsg:         "tagging OpenSearch domain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &OpenSearchRepository{
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
				if mock.addTagsInput == nil {
					t.Fatal("AddTags was not called")
				}
				if aws.ToString(mock.addTagsInput.ARN) != tt.wantARN {
					t.Errorf("Tag() ARN = %v, want %v", aws.ToString(mock.addTagsInput.ARN), tt.wantARN)
				}

				// Verify the tag was set correctly
				if len(mock.addTagsInput.TagList) != 1 {
					t.Errorf("Tag() set %d tags, want 1", len(mock.addTagsInput.TagList))
				}
				if len(mock.addTagsInput.TagList) > 0 {
					tag := mock.addTagsInput.TagList[0]
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

func TestOpenSearchRepository_Delete(t *testing.T) {
	tests := []struct {
		name       string
		mockSetup  func() *mockOpenSearchClient
		resourceID string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "deletes domain successfully",
			mockSetup: func() *mockOpenSearchClient {
				return &mockOpenSearchClient{
					deleteDomainOutput: &opensearch.DeleteDomainOutput{},
				}
			},
			resourceID: "my-domain",
			wantErr:    false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockOpenSearchClient {
				return &mockOpenSearchClient{
					deleteDomainError: errors.New("cannot delete"),
				}
			},
			resourceID: "my-domain",
			wantErr:    true,
			errMsg:     "deleting OpenSearch domain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &OpenSearchRepository{
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
				// Verify DeleteDomain was called
				if !mock.deleteDomainCalled {
					t.Fatal("DeleteDomain was not called")
				}
				if mock.deleteDomainInput == nil {
					t.Fatal("DeleteDomain input is nil")
				}
				if aws.ToString(mock.deleteDomainInput.DomainName) != tt.resourceID {
					t.Errorf("Delete() DomainName = %v, want %v",
						aws.ToString(mock.deleteDomainInput.DomainName), tt.resourceID)
				}
			}
		})
	}
}

func TestOpenSearchRepository_InterfaceCompliance(_ *testing.T) {
	// Verify OpenSearchRepository implements domain.ResourceRepository
	var _ domain.ResourceRepository = (*OpenSearchRepository)(nil)
}

func TestOpenSearchRepository_buildARN(t *testing.T) {
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
			resourceID: "my-domain",
			want:       "arn:aws:es:us-east-1:123456789012:domain/my-domain",
		},
		{
			name:       "different region",
			accountID:  "987654321098",
			region:     "eu-central-1",
			resourceID: "prod-domain-1",
			want:       "arn:aws:es:eu-central-1:987654321098:domain/prod-domain-1",
		},
		{
			name:       "domain with hyphens",
			accountID:  "111222333444",
			region:     "ap-southeast-1",
			resourceID: "my-complex-domain-name",
			want:       "arn:aws:es:ap-southeast-1:111222333444:domain/my-complex-domain-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &OpenSearchRepository{
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

func TestNewOpenSearchRepository(t *testing.T) {
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
			repo := NewOpenSearchRepository(nil, tt.accountID, tt.region)

			if repo == nil {
				t.Fatal("NewOpenSearchRepository() returned nil")
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

func TestOpenSearchRepository_domainToResource(t *testing.T) {
	now := time.Now()
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name   string
		status *types.DomainStatus
		tags   []types.Tag
		want   domain.Resource
	}{
		{
			name: "basic domain",
			status: &types.DomainStatus{
				DomainName: aws.String("test-domain"),
				ARN:        aws.String("arn:aws:es:us-east-1:123456789012:domain/test-domain"),
				Created:    aws.Bool(true),
				Deleted:    aws.Bool(false),
			},
			tags: []types.Tag{},
			want: domain.Resource{
				ID:        "test-domain",
				Type:      domain.ResourceTypeOpenSearch,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Tags:      map[string]string{},
			},
		},
		{
			name: "domain with Name tag",
			status: &types.DomainStatus{
				DomainName: aws.String("prod-domain"),
				ARN:        aws.String("arn:aws:es:us-east-1:123456789012:domain/prod-domain"),
				Created:    aws.Bool(true),
				Deleted:    aws.Bool(false),
			},
			tags: []types.Tag{
				{Key: aws.String("Name"), Value: aws.String("Production Domain")},
			},
			want: domain.Resource{
				ID:        "prod-domain",
				Type:      domain.ResourceTypeOpenSearch,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Name:      "Production Domain",
				Tags: map[string]string{
					"Name": "Production Domain",
				},
			},
		},
		{
			name: "domain with expiration date",
			status: &types.DomainStatus{
				DomainName: aws.String("expiring-domain"),
				ARN:        aws.String("arn:aws:es:us-east-1:123456789012:domain/expiring-domain"),
				Created:    aws.Bool(true),
				Deleted:    aws.Bool(false),
			},
			tags: []types.Tag{
				{Key: aws.String(ExpirationTagName), Value: aws.String(expDateStr)},
			},
			want: domain.Resource{
				ID:        "expiring-domain",
				Type:      domain.ResourceTypeOpenSearch,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Tags: map[string]string{
					ExpirationTagName: expDateStr,
				},
			},
		},
		{
			name: "domain that never expires",
			status: &types.DomainStatus{
				DomainName: aws.String("permanent-domain"),
				ARN:        aws.String("arn:aws:es:us-east-1:123456789012:domain/permanent-domain"),
				Created:    aws.Bool(true),
				Deleted:    aws.Bool(false),
			},
			tags: []types.Tag{
				{Key: aws.String(ExpirationTagName), Value: aws.String(NeverExpiresValue)},
			},
			want: domain.Resource{
				ID:           "permanent-domain",
				Type:         domain.ResourceTypeOpenSearch,
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
			repo := &OpenSearchRepository{
				accountID: "123456789012",
				region:    "us-east-1",
			}

			got := repo.domainToResource(tt.status, tt.tags)

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
