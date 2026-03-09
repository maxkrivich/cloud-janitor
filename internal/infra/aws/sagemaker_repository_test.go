package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check for mockSageMakerClient.
var _ sageMakerClient = (*mockSageMakerClient)(nil)

// mockSageMakerClient implements sageMakerClient for testing.
type mockSageMakerClient struct {
	listNotebookInstancesOutput *sagemaker.ListNotebookInstancesOutput
	listNotebookInstancesError  error
	listNotebookInstancesPages  []*sagemaker.ListNotebookInstancesOutput
	listNotebookInstancesIndex  int

	listTagsOutput *sagemaker.ListTagsOutput
	listTagsError  error
	listTagsInputs []*sagemaker.ListTagsInput
	// Map ARN to tags output for more granular control
	listTagsByARN map[string]*sagemaker.ListTagsOutput

	addTagsOutput *sagemaker.AddTagsOutput
	addTagsError  error
	addTagsInput  *sagemaker.AddTagsInput

	describeNotebookInstanceOutput *sagemaker.DescribeNotebookInstanceOutput
	describeNotebookInstanceError  error
	describeNotebookInstanceInput  *sagemaker.DescribeNotebookInstanceInput

	stopNotebookInstanceOutput *sagemaker.StopNotebookInstanceOutput
	stopNotebookInstanceError  error
	stopNotebookInstanceInput  *sagemaker.StopNotebookInstanceInput
	stopNotebookInstanceCalled bool

	deleteNotebookInstanceOutput *sagemaker.DeleteNotebookInstanceOutput
	deleteNotebookInstanceError  error
	deleteNotebookInstanceInput  *sagemaker.DeleteNotebookInstanceInput
	deleteNotebookInstanceCalled bool
}

func (m *mockSageMakerClient) ListNotebookInstances(_ context.Context, _ *sagemaker.ListNotebookInstancesInput, _ ...func(*sagemaker.Options)) (*sagemaker.ListNotebookInstancesOutput, error) {
	if m.listNotebookInstancesError != nil {
		return nil, m.listNotebookInstancesError
	}
	if len(m.listNotebookInstancesPages) > 0 {
		if m.listNotebookInstancesIndex >= len(m.listNotebookInstancesPages) {
			return &sagemaker.ListNotebookInstancesOutput{}, nil
		}
		result := m.listNotebookInstancesPages[m.listNotebookInstancesIndex]
		m.listNotebookInstancesIndex++
		return result, nil
	}
	return m.listNotebookInstancesOutput, nil
}

func (m *mockSageMakerClient) ListTags(_ context.Context, params *sagemaker.ListTagsInput, _ ...func(*sagemaker.Options)) (*sagemaker.ListTagsOutput, error) {
	m.listTagsInputs = append(m.listTagsInputs, params)
	if m.listTagsError != nil {
		return nil, m.listTagsError
	}
	// Check if we have ARN-specific tags
	if m.listTagsByARN != nil {
		if output, ok := m.listTagsByARN[aws.ToString(params.ResourceArn)]; ok {
			return output, nil
		}
	}
	if m.listTagsOutput != nil {
		return m.listTagsOutput, nil
	}
	return &sagemaker.ListTagsOutput{}, nil
}

func (m *mockSageMakerClient) AddTags(_ context.Context, params *sagemaker.AddTagsInput, _ ...func(*sagemaker.Options)) (*sagemaker.AddTagsOutput, error) {
	m.addTagsInput = params
	if m.addTagsError != nil {
		return nil, m.addTagsError
	}
	return m.addTagsOutput, nil
}

func (m *mockSageMakerClient) DescribeNotebookInstance(_ context.Context, params *sagemaker.DescribeNotebookInstanceInput, _ ...func(*sagemaker.Options)) (*sagemaker.DescribeNotebookInstanceOutput, error) {
	m.describeNotebookInstanceInput = params
	if m.describeNotebookInstanceError != nil {
		return nil, m.describeNotebookInstanceError
	}
	return m.describeNotebookInstanceOutput, nil
}

func (m *mockSageMakerClient) StopNotebookInstance(_ context.Context, params *sagemaker.StopNotebookInstanceInput, _ ...func(*sagemaker.Options)) (*sagemaker.StopNotebookInstanceOutput, error) {
	m.stopNotebookInstanceInput = params
	m.stopNotebookInstanceCalled = true
	if m.stopNotebookInstanceError != nil {
		return nil, m.stopNotebookInstanceError
	}
	return m.stopNotebookInstanceOutput, nil
}

func (m *mockSageMakerClient) DeleteNotebookInstance(_ context.Context, params *sagemaker.DeleteNotebookInstanceInput, _ ...func(*sagemaker.Options)) (*sagemaker.DeleteNotebookInstanceOutput, error) {
	m.deleteNotebookInstanceInput = params
	m.deleteNotebookInstanceCalled = true
	if m.deleteNotebookInstanceError != nil {
		return nil, m.deleteNotebookInstanceError
	}
	return m.deleteNotebookInstanceOutput, nil
}

func TestSageMakerRepository_Type(t *testing.T) {
	repo := &SageMakerRepository{
		client:    &mockSageMakerClient{},
		accountID: "123456789012",
		region:    "us-east-1",
	}
	got := repo.Type()
	want := domain.ResourceTypeSageMaker

	if got != want {
		t.Errorf("Type() = %v, want %v", got, want)
	}
}

func TestSageMakerRepository_List(t *testing.T) {
	now := time.Now()
	createdAt := now.AddDate(0, -1, 0)
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name      string
		mockSetup func() *mockSageMakerClient
		accountID string
		region    string
		want      []domain.Resource
		wantErr   bool
		errMsg    string
	}{
		{
			name: "lists notebook instances successfully",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					listNotebookInstancesOutput: &sagemaker.ListNotebookInstancesOutput{
						NotebookInstances: []types.NotebookInstanceSummary{
							{
								NotebookInstanceName:   aws.String("my-notebook"),
								NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/my-notebook"),
								NotebookInstanceStatus: types.NotebookInstanceStatusInService,
								CreationTime:           &createdAt,
							},
						},
					},
					listTagsByARN: map[string]*sagemaker.ListTagsOutput{
						"arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/my-notebook": {
							Tags: []types.Tag{
								{Key: aws.String("Name"), Value: aws.String("My Notebook")},
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
					ID:             "my-notebook",
					Type:           domain.ResourceTypeSageMaker,
					Region:         "us-east-1",
					AccountID:      "123456789012",
					Name:           "My Notebook",
					ExpirationDate: nil,
					NeverExpires:   false,
					Tags: map[string]string{
						"Name":        "My Notebook",
						"Environment": "dev",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "parses expiration date tag from ListTags",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					listNotebookInstancesOutput: &sagemaker.ListNotebookInstancesOutput{
						NotebookInstances: []types.NotebookInstanceSummary{
							{
								NotebookInstanceName:   aws.String("expiring-notebook"),
								NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/expiring-notebook"),
								NotebookInstanceStatus: types.NotebookInstanceStatusStopped,
							},
						},
					},
					listTagsByARN: map[string]*sagemaker.ListTagsOutput{
						"arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/expiring-notebook": {
							Tags: []types.Tag{
								{Key: aws.String(ExpirationTagName), Value: aws.String(expDateStr)},
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "expiring-notebook",
					Type:      domain.ResourceTypeSageMaker,
					Region:    "us-east-1",
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
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					listNotebookInstancesOutput: &sagemaker.ListNotebookInstancesOutput{
						NotebookInstances: []types.NotebookInstanceSummary{
							{
								NotebookInstanceName:   aws.String("permanent-notebook"),
								NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/permanent-notebook"),
								NotebookInstanceStatus: types.NotebookInstanceStatusInService,
							},
						},
					},
					listTagsByARN: map[string]*sagemaker.ListTagsOutput{
						"arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/permanent-notebook": {
							Tags: []types.Tag{
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
					ID:           "permanent-notebook",
					Type:         domain.ResourceTypeSageMaker,
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
			name: "filters Deleting instances",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					listNotebookInstancesOutput: &sagemaker.ListNotebookInstancesOutput{
						NotebookInstances: []types.NotebookInstanceSummary{
							{
								NotebookInstanceName:   aws.String("active-notebook"),
								NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/active-notebook"),
								NotebookInstanceStatus: types.NotebookInstanceStatusInService,
							},
							{
								NotebookInstanceName:   aws.String("deleting-notebook"),
								NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/deleting-notebook"),
								NotebookInstanceStatus: types.NotebookInstanceStatusDeleting,
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "active-notebook",
					Type:      domain.ResourceTypeSageMaker,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "filters Failed instances",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					listNotebookInstancesOutput: &sagemaker.ListNotebookInstancesOutput{
						NotebookInstances: []types.NotebookInstanceSummary{
							{
								NotebookInstanceName:   aws.String("running-notebook"),
								NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/running-notebook"),
								NotebookInstanceStatus: types.NotebookInstanceStatusInService,
							},
							{
								NotebookInstanceName:   aws.String("failed-notebook"),
								NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/failed-notebook"),
								NotebookInstanceStatus: types.NotebookInstanceStatusFailed,
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "running-notebook",
					Type:      domain.ResourceTypeSageMaker,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "includes InService, Stopped, Pending, Stopping, Updating instances",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					listNotebookInstancesOutput: &sagemaker.ListNotebookInstancesOutput{
						NotebookInstances: []types.NotebookInstanceSummary{
							{
								NotebookInstanceName:   aws.String("in-service-notebook"),
								NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/in-service-notebook"),
								NotebookInstanceStatus: types.NotebookInstanceStatusInService,
							},
							{
								NotebookInstanceName:   aws.String("stopped-notebook"),
								NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/stopped-notebook"),
								NotebookInstanceStatus: types.NotebookInstanceStatusStopped,
							},
							{
								NotebookInstanceName:   aws.String("pending-notebook"),
								NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/pending-notebook"),
								NotebookInstanceStatus: types.NotebookInstanceStatusPending,
							},
							{
								NotebookInstanceName:   aws.String("stopping-notebook"),
								NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/stopping-notebook"),
								NotebookInstanceStatus: types.NotebookInstanceStatusStopping,
							},
							{
								NotebookInstanceName:   aws.String("updating-notebook"),
								NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/updating-notebook"),
								NotebookInstanceStatus: types.NotebookInstanceStatusUpdating,
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{ID: "in-service-notebook", Type: domain.ResourceTypeSageMaker, Region: "us-east-1", AccountID: "123456789012", Tags: map[string]string{}},
				{ID: "stopped-notebook", Type: domain.ResourceTypeSageMaker, Region: "us-east-1", AccountID: "123456789012", Tags: map[string]string{}},
				{ID: "pending-notebook", Type: domain.ResourceTypeSageMaker, Region: "us-east-1", AccountID: "123456789012", Tags: map[string]string{}},
				{ID: "stopping-notebook", Type: domain.ResourceTypeSageMaker, Region: "us-east-1", AccountID: "123456789012", Tags: map[string]string{}},
				{ID: "updating-notebook", Type: domain.ResourceTypeSageMaker, Region: "us-east-1", AccountID: "123456789012", Tags: map[string]string{}},
			},
			wantErr: false,
		},
		{
			name: "handles API error for ListNotebookInstances",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					listNotebookInstancesError: errors.New("API error"),
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   true,
			errMsg:    "listing SageMaker notebook instances",
		},
		{
			name: "handles API error for ListTags gracefully",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					listNotebookInstancesOutput: &sagemaker.ListNotebookInstancesOutput{
						NotebookInstances: []types.NotebookInstanceSummary{
							{
								NotebookInstanceName:   aws.String("notebook-with-tag-error"),
								NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/notebook-with-tag-error"),
								NotebookInstanceStatus: types.NotebookInstanceStatusInService,
							},
						},
					},
					listTagsError: errors.New("access denied"),
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			// ListTags errors should be handled gracefully - resource still returned with empty tags
			want: []domain.Resource{
				{
					ID:        "notebook-with-tag-error",
					Type:      domain.ResourceTypeSageMaker,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "handles empty results",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					listNotebookInstancesOutput: &sagemaker.ListNotebookInstancesOutput{
						NotebookInstances: []types.NotebookInstanceSummary{},
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
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					listNotebookInstancesPages: []*sagemaker.ListNotebookInstancesOutput{
						{
							NotebookInstances: []types.NotebookInstanceSummary{
								{
									NotebookInstanceName:   aws.String("notebook-page-1"),
									NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/notebook-page-1"),
									NotebookInstanceStatus: types.NotebookInstanceStatusStopped,
								},
							},
							NextToken: aws.String("next-page-token"),
						},
						{
							NotebookInstances: []types.NotebookInstanceSummary{
								{
									NotebookInstanceName:   aws.String("notebook-page-2"),
									NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/notebook-page-2"),
									NotebookInstanceStatus: types.NotebookInstanceStatusStopped,
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
					ID:        "notebook-page-1",
					Type:      domain.ResourceTypeSageMaker,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
				{
					ID:        "notebook-page-2",
					Type:      domain.ResourceTypeSageMaker,
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
			repo := &SageMakerRepository{
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

func TestSageMakerRepository_Tag(t *testing.T) {
	expDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		mockSetup      func() *mockSageMakerClient
		accountID      string
		region         string
		resourceID     string
		expirationDate time.Time
		wantARN        string
		wantErr        bool
		errMsg         string
	}{
		{
			name: "tags notebook with correct ARN",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					addTagsOutput: &sagemaker.AddTagsOutput{},
				}
			},
			accountID:      "123456789012",
			region:         "us-east-1",
			resourceID:     "my-notebook",
			expirationDate: expDate,
			wantARN:        "arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/my-notebook",
			wantErr:        false,
		},
		{
			name: "constructs correct ARN for different region",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					addTagsOutput: &sagemaker.AddTagsOutput{},
				}
			},
			accountID:      "987654321098",
			region:         "eu-west-1",
			resourceID:     "prod-notebook",
			expirationDate: expDate,
			wantARN:        "arn:aws:sagemaker:eu-west-1:987654321098:notebook-instance/prod-notebook",
			wantErr:        false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					addTagsError: errors.New("access denied"),
				}
			},
			accountID:      "123456789012",
			region:         "us-east-1",
			resourceID:     "my-notebook",
			expirationDate: expDate,
			wantErr:        true,
			errMsg:         "tagging SageMaker notebook",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &SageMakerRepository{
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
				if aws.ToString(mock.addTagsInput.ResourceArn) != tt.wantARN {
					t.Errorf("Tag() ARN = %v, want %v", aws.ToString(mock.addTagsInput.ResourceArn), tt.wantARN)
				}

				// Verify the tag was set correctly
				if len(mock.addTagsInput.Tags) != 1 {
					t.Errorf("Tag() set %d tags, want 1", len(mock.addTagsInput.Tags))
				}
				if len(mock.addTagsInput.Tags) > 0 {
					tag := mock.addTagsInput.Tags[0]
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

func TestSageMakerRepository_Delete(t *testing.T) {
	tests := []struct {
		name             string
		mockSetup        func() *mockSageMakerClient
		resourceID       string
		wantDeleteCalled bool
		wantErr          bool
		errMsg           string
	}{
		{
			name: "deletes stopped notebook successfully",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					describeNotebookInstanceOutput: &sagemaker.DescribeNotebookInstanceOutput{
						NotebookInstanceName:   aws.String("stopped-notebook"),
						NotebookInstanceStatus: types.NotebookInstanceStatusStopped,
					},
					deleteNotebookInstanceOutput: &sagemaker.DeleteNotebookInstanceOutput{},
				}
			},
			resourceID:       "stopped-notebook",
			wantDeleteCalled: true,
			wantErr:          false,
		},
		{
			name: "returns error for InService notebook",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					describeNotebookInstanceOutput: &sagemaker.DescribeNotebookInstanceOutput{
						NotebookInstanceName:   aws.String("running-notebook"),
						NotebookInstanceStatus: types.NotebookInstanceStatusInService,
					},
				}
			},
			resourceID:       "running-notebook",
			wantDeleteCalled: false,
			wantErr:          true,
			errMsg:           "cannot delete notebook",
		},
		{
			name: "handles API error for DescribeNotebookInstance",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					describeNotebookInstanceError: errors.New("notebook not found"),
				}
			},
			resourceID:       "nonexistent-notebook",
			wantDeleteCalled: false,
			wantErr:          true,
			errMsg:           "describing SageMaker notebook",
		},
		{
			name: "handles API error for DeleteNotebookInstance",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					describeNotebookInstanceOutput: &sagemaker.DescribeNotebookInstanceOutput{
						NotebookInstanceName:   aws.String("stopped-notebook"),
						NotebookInstanceStatus: types.NotebookInstanceStatusStopped,
					},
					deleteNotebookInstanceError: errors.New("deletion failed"),
				}
			},
			resourceID:       "stopped-notebook",
			wantDeleteCalled: true,
			wantErr:          true,
			errMsg:           "deleting SageMaker notebook",
		},
		{
			name: "returns error for Pending notebook",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					describeNotebookInstanceOutput: &sagemaker.DescribeNotebookInstanceOutput{
						NotebookInstanceName:   aws.String("pending-notebook"),
						NotebookInstanceStatus: types.NotebookInstanceStatusPending,
					},
				}
			},
			resourceID:       "pending-notebook",
			wantDeleteCalled: false,
			wantErr:          true,
			errMsg:           "cannot delete notebook",
		},
		{
			name: "returns error for Stopping notebook",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					describeNotebookInstanceOutput: &sagemaker.DescribeNotebookInstanceOutput{
						NotebookInstanceName:   aws.String("stopping-notebook"),
						NotebookInstanceStatus: types.NotebookInstanceStatusStopping,
					},
				}
			},
			resourceID:       "stopping-notebook",
			wantDeleteCalled: false,
			wantErr:          true,
			errMsg:           "cannot delete notebook",
		},
		{
			name: "returns error for Updating notebook",
			mockSetup: func() *mockSageMakerClient {
				return &mockSageMakerClient{
					describeNotebookInstanceOutput: &sagemaker.DescribeNotebookInstanceOutput{
						NotebookInstanceName:   aws.String("updating-notebook"),
						NotebookInstanceStatus: types.NotebookInstanceStatusUpdating,
					},
				}
			},
			resourceID:       "updating-notebook",
			wantDeleteCalled: false,
			wantErr:          true,
			errMsg:           "cannot delete notebook",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &SageMakerRepository{
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
			}

			if mock.deleteNotebookInstanceCalled != tt.wantDeleteCalled {
				t.Errorf("DeleteNotebookInstance called = %v, want %v", mock.deleteNotebookInstanceCalled, tt.wantDeleteCalled)
			}

			if tt.wantDeleteCalled && !tt.wantErr {
				// Verify DeleteNotebookInstance was called with correct input
				if mock.deleteNotebookInstanceInput == nil {
					t.Fatal("DeleteNotebookInstance input is nil")
				}
				if aws.ToString(mock.deleteNotebookInstanceInput.NotebookInstanceName) != tt.resourceID {
					t.Errorf("Delete() NotebookInstanceName = %v, want %v",
						aws.ToString(mock.deleteNotebookInstanceInput.NotebookInstanceName), tt.resourceID)
				}
			}
		})
	}
}

func TestSageMakerRepository_InterfaceCompliance(t *testing.T) {
	// Verify SageMakerRepository implements domain.ResourceRepository
	var _ domain.ResourceRepository = (*SageMakerRepository)(nil)
}

func TestNewSageMakerRepository(t *testing.T) {
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
			repo := NewSageMakerRepository(nil, tt.accountID, tt.region)

			if repo == nil {
				t.Fatal("NewSageMakerRepository() returned nil")
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

func TestSageMakerRepository_buildARN(t *testing.T) {
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
			resourceID: "my-notebook",
			want:       "arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/my-notebook",
		},
		{
			name:       "different region",
			accountID:  "987654321098",
			region:     "eu-central-1",
			resourceID: "prod-notebook-1",
			want:       "arn:aws:sagemaker:eu-central-1:987654321098:notebook-instance/prod-notebook-1",
		},
		{
			name:       "notebook with hyphens",
			accountID:  "111222333444",
			region:     "ap-southeast-1",
			resourceID: "my-complex-notebook-name",
			want:       "arn:aws:sagemaker:ap-southeast-1:111222333444:notebook-instance/my-complex-notebook-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &SageMakerRepository{
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

func TestSageMakerRepository_shouldSkipStatus(t *testing.T) {
	tests := []struct {
		name   string
		status types.NotebookInstanceStatus
		want   bool
	}{
		{
			name:   "skip Deleting status",
			status: types.NotebookInstanceStatusDeleting,
			want:   true,
		},
		{
			name:   "skip Failed status",
			status: types.NotebookInstanceStatusFailed,
			want:   true,
		},
		{
			name:   "include InService status",
			status: types.NotebookInstanceStatusInService,
			want:   false,
		},
		{
			name:   "include Stopped status",
			status: types.NotebookInstanceStatusStopped,
			want:   false,
		},
		{
			name:   "include Pending status",
			status: types.NotebookInstanceStatusPending,
			want:   false,
		},
		{
			name:   "include Stopping status",
			status: types.NotebookInstanceStatusStopping,
			want:   false,
		},
		{
			name:   "include Updating status",
			status: types.NotebookInstanceStatusUpdating,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipNotebookStatus(tt.status)
			if got != tt.want {
				t.Errorf("shouldSkipNotebookStatus(%v) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestSageMakerRepository_canDeleteStatus(t *testing.T) {
	tests := []struct {
		name   string
		status types.NotebookInstanceStatus
		want   bool
	}{
		{
			name:   "can delete Stopped status",
			status: types.NotebookInstanceStatusStopped,
			want:   true,
		},
		{
			name:   "can delete Failed status",
			status: types.NotebookInstanceStatusFailed,
			want:   true,
		},
		{
			name:   "cannot delete InService status",
			status: types.NotebookInstanceStatusInService,
			want:   false,
		},
		{
			name:   "cannot delete Pending status",
			status: types.NotebookInstanceStatusPending,
			want:   false,
		},
		{
			name:   "cannot delete Stopping status",
			status: types.NotebookInstanceStatusStopping,
			want:   false,
		},
		{
			name:   "cannot delete Updating status",
			status: types.NotebookInstanceStatusUpdating,
			want:   false,
		},
		{
			name:   "cannot delete Deleting status",
			status: types.NotebookInstanceStatusDeleting,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canDeleteNotebook(tt.status)
			if got != tt.want {
				t.Errorf("canDeleteNotebook(%v) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestSageMakerRepository_instanceToResource(t *testing.T) {
	now := time.Now()
	createdAt := now.AddDate(0, -1, 0)
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name     string
		instance types.NotebookInstanceSummary
		tags     []types.Tag
		want     domain.Resource
	}{
		{
			name: "basic instance",
			instance: types.NotebookInstanceSummary{
				NotebookInstanceName:   aws.String("test-notebook"),
				NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/test-notebook"),
				NotebookInstanceStatus: types.NotebookInstanceStatusInService,
				CreationTime:           &createdAt,
			},
			tags: nil,
			want: domain.Resource{
				ID:        "test-notebook",
				Type:      domain.ResourceTypeSageMaker,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Tags:      map[string]string{},
			},
		},
		{
			name: "instance with Name tag",
			instance: types.NotebookInstanceSummary{
				NotebookInstanceName:   aws.String("prod-notebook"),
				NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/prod-notebook"),
				NotebookInstanceStatus: types.NotebookInstanceStatusStopped,
			},
			tags: []types.Tag{
				{Key: aws.String("Name"), Value: aws.String("Production Notebook")},
			},
			want: domain.Resource{
				ID:        "prod-notebook",
				Type:      domain.ResourceTypeSageMaker,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Name:      "Production Notebook",
				Tags: map[string]string{
					"Name": "Production Notebook",
				},
			},
		},
		{
			name: "instance with expiration date",
			instance: types.NotebookInstanceSummary{
				NotebookInstanceName:   aws.String("expiring-notebook"),
				NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/expiring-notebook"),
				NotebookInstanceStatus: types.NotebookInstanceStatusStopped,
			},
			tags: []types.Tag{
				{Key: aws.String(ExpirationTagName), Value: aws.String(expDateStr)},
			},
			want: domain.Resource{
				ID:        "expiring-notebook",
				Type:      domain.ResourceTypeSageMaker,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Tags: map[string]string{
					ExpirationTagName: expDateStr,
				},
			},
		},
		{
			name: "instance that never expires",
			instance: types.NotebookInstanceSummary{
				NotebookInstanceName:   aws.String("permanent-notebook"),
				NotebookInstanceArn:    aws.String("arn:aws:sagemaker:us-east-1:123456789012:notebook-instance/permanent-notebook"),
				NotebookInstanceStatus: types.NotebookInstanceStatusInService,
			},
			tags: []types.Tag{
				{Key: aws.String(ExpirationTagName), Value: aws.String(NeverExpiresValue)},
			},
			want: domain.Resource{
				ID:           "permanent-notebook",
				Type:         domain.ResourceTypeSageMaker,
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
			repo := &SageMakerRepository{
				accountID: "123456789012",
				region:    "us-east-1",
			}

			got := repo.instanceToResource(tt.instance, tt.tags)

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
