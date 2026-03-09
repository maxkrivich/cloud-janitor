package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check for mockLogsClient.
var _ logsClient = (*mockLogsClient)(nil)

// mockLogsClient implements logsClient for testing.
type mockLogsClient struct {
	describeLogGroupsOutput *cloudwatchlogs.DescribeLogGroupsOutput
	describeLogGroupsError  error
	describeLogGroupsPages  []*cloudwatchlogs.DescribeLogGroupsOutput
	pageIndex               int

	listTagsForResourceOutputs map[string]*cloudwatchlogs.ListTagsForResourceOutput
	listTagsForResourceError   error
	listTagsForResourceCalls   []string

	tagResourceOutput *cloudwatchlogs.TagResourceOutput
	tagResourceError  error
	tagResourceInput  *cloudwatchlogs.TagResourceInput
	tagResourceCalled bool

	deleteLogGroupOutput *cloudwatchlogs.DeleteLogGroupOutput
	deleteLogGroupError  error
	deleteLogGroupInput  *cloudwatchlogs.DeleteLogGroupInput
	deleteLogGroupCalled bool
}

func (m *mockLogsClient) DescribeLogGroups(_ context.Context, _ *cloudwatchlogs.DescribeLogGroupsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	if m.describeLogGroupsError != nil {
		return nil, m.describeLogGroupsError
	}
	if len(m.describeLogGroupsPages) > 0 {
		if m.pageIndex >= len(m.describeLogGroupsPages) {
			return &cloudwatchlogs.DescribeLogGroupsOutput{}, nil
		}
		result := m.describeLogGroupsPages[m.pageIndex]
		m.pageIndex++
		return result, nil
	}
	return m.describeLogGroupsOutput, nil
}

func (m *mockLogsClient) ListTagsForResource(_ context.Context, input *cloudwatchlogs.ListTagsForResourceInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.ListTagsForResourceOutput, error) {
	arn := aws.ToString(input.ResourceArn)
	m.listTagsForResourceCalls = append(m.listTagsForResourceCalls, arn)

	if m.listTagsForResourceError != nil {
		return nil, m.listTagsForResourceError
	}

	if m.listTagsForResourceOutputs != nil {
		if output, ok := m.listTagsForResourceOutputs[arn]; ok {
			return output, nil
		}
	}

	return &cloudwatchlogs.ListTagsForResourceOutput{
		Tags: map[string]string{},
	}, nil
}

func (m *mockLogsClient) TagResource(_ context.Context, input *cloudwatchlogs.TagResourceInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.TagResourceOutput, error) {
	m.tagResourceInput = input
	m.tagResourceCalled = true
	if m.tagResourceError != nil {
		return nil, m.tagResourceError
	}
	return m.tagResourceOutput, nil
}

func (m *mockLogsClient) DeleteLogGroup(_ context.Context, input *cloudwatchlogs.DeleteLogGroupInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DeleteLogGroupOutput, error) {
	m.deleteLogGroupInput = input
	m.deleteLogGroupCalled = true
	if m.deleteLogGroupError != nil {
		return nil, m.deleteLogGroupError
	}
	return m.deleteLogGroupOutput, nil
}

func TestLogsRepository_Type(t *testing.T) {
	repo := NewLogsRepository(nil, "123456789012", "us-east-1", nil)
	got := repo.Type()
	want := domain.ResourceTypeLogs

	if got != want {
		t.Errorf("Type() = %v, want %v", got, want)
	}
}

func TestLogsRepository_List(t *testing.T) {
	now := time.Now()
	createdAt := now.AddDate(0, -1, 0).UnixMilli()
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name         string
		mockSetup    func() *mockLogsClient
		accountID    string
		region       string
		skipPatterns []string
		want         []domain.Resource
		wantErr      bool
		errMsg       string
	}{
		{
			name: "lists log groups successfully",
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					describeLogGroupsOutput: &cloudwatchlogs.DescribeLogGroupsOutput{
						LogGroups: []types.LogGroup{
							{
								LogGroupName: aws.String("/my-app/logs"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/my-app/logs"),
								CreationTime: aws.Int64(createdAt),
								StoredBytes:  aws.Int64(1024),
							},
						},
					},
					listTagsForResourceOutputs: map[string]*cloudwatchlogs.ListTagsForResourceOutput{
						"arn:aws:logs:us-east-1:123456789012:log-group:/my-app/logs": {
							Tags: map[string]string{
								"Name":        "My App Logs",
								"Environment": "prod",
							},
						},
					},
				}
			},
			accountID:    "123456789012",
			region:       "us-east-1",
			skipPatterns: nil,
			want: []domain.Resource{
				{
					ID:             "/my-app/logs",
					Type:           domain.ResourceTypeLogs,
					Region:         "us-east-1",
					AccountID:      "123456789012",
					Name:           "My App Logs",
					ExpirationDate: nil,
					NeverExpires:   false,
					Tags: map[string]string{
						"Name":        "My App Logs",
						"Environment": "prod",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "parses expiration date tag",
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					describeLogGroupsOutput: &cloudwatchlogs.DescribeLogGroupsOutput{
						LogGroups: []types.LogGroup{
							{
								LogGroupName: aws.String("/expiring-app/logs"),
								Arn:          aws.String("arn:aws:logs:us-west-2:123456789012:log-group:/expiring-app/logs"),
							},
						},
					},
					listTagsForResourceOutputs: map[string]*cloudwatchlogs.ListTagsForResourceOutput{
						"arn:aws:logs:us-west-2:123456789012:log-group:/expiring-app/logs": {
							Tags: map[string]string{
								ExpirationTagName: expDateStr,
							},
						},
					},
				}
			},
			accountID:    "123456789012",
			region:       "us-west-2",
			skipPatterns: nil,
			want: []domain.Resource{
				{
					ID:        "/expiring-app/logs",
					Type:      domain.ResourceTypeLogs,
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
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					describeLogGroupsOutput: &cloudwatchlogs.DescribeLogGroupsOutput{
						LogGroups: []types.LogGroup{
							{
								LogGroupName: aws.String("/permanent/logs"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/permanent/logs"),
							},
						},
					},
					listTagsForResourceOutputs: map[string]*cloudwatchlogs.ListTagsForResourceOutput{
						"arn:aws:logs:us-east-1:123456789012:log-group:/permanent/logs": {
							Tags: map[string]string{
								ExpirationTagName: NeverExpiresValue,
							},
						},
					},
				}
			},
			accountID:    "123456789012",
			region:       "us-east-1",
			skipPatterns: nil,
			want: []domain.Resource{
				{
					ID:           "/permanent/logs",
					Type:         domain.ResourceTypeLogs,
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
			name: "filters log groups matching skip patterns",
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					describeLogGroupsOutput: &cloudwatchlogs.DescribeLogGroupsOutput{
						LogGroups: []types.LogGroup{
							{
								LogGroupName: aws.String("/aws/lambda/my-function"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/aws/lambda/my-function"),
							},
							{
								LogGroupName: aws.String("/my-app/logs"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/my-app/logs"),
							},
							{
								LogGroupName: aws.String("/aws/eks/my-cluster/cluster"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/aws/eks/my-cluster/cluster"),
							},
						},
					},
				}
			},
			accountID:    "123456789012",
			region:       "us-east-1",
			skipPatterns: []string{"/aws/lambda/*", "/aws/eks/*"},
			want: []domain.Resource{
				{
					ID:        "/my-app/logs",
					Type:      domain.ResourceTypeLogs,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "handles multiple skip patterns",
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					describeLogGroupsOutput: &cloudwatchlogs.DescribeLogGroupsOutput{
						LogGroups: []types.LogGroup{
							{
								LogGroupName: aws.String("/aws/lambda/func1"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/aws/lambda/func1"),
							},
							{
								LogGroupName: aws.String("/aws/rds/instance/postgres/error"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/aws/rds/instance/postgres/error"),
							},
							{
								LogGroupName: aws.String("/aws/elasticbeanstalk/env/var/log"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/aws/elasticbeanstalk/env/var/log"),
							},
							{
								LogGroupName: aws.String("/custom/app"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/custom/app"),
							},
						},
					},
				}
			},
			accountID:    "123456789012",
			region:       "us-east-1",
			skipPatterns: []string{"/aws/lambda/*", "/aws/rds/*", "/aws/elasticbeanstalk/*"},
			want: []domain.Resource{
				{
					ID:        "/custom/app",
					Type:      domain.ResourceTypeLogs,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "handles no skip patterns - lists all",
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					describeLogGroupsOutput: &cloudwatchlogs.DescribeLogGroupsOutput{
						LogGroups: []types.LogGroup{
							{
								LogGroupName: aws.String("/aws/lambda/my-function"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/aws/lambda/my-function"),
							},
							{
								LogGroupName: aws.String("/my-app/logs"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/my-app/logs"),
							},
						},
					},
				}
			},
			accountID:    "123456789012",
			region:       "us-east-1",
			skipPatterns: []string{},
			want: []domain.Resource{
				{
					ID:        "/aws/lambda/my-function",
					Type:      domain.ResourceTypeLogs,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
				{
					ID:        "/my-app/logs",
					Type:      domain.ResourceTypeLogs,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "handles API error for DescribeLogGroups",
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					describeLogGroupsError: errors.New("API error"),
				}
			},
			accountID:    "123456789012",
			region:       "us-east-1",
			skipPatterns: nil,
			want:         nil,
			wantErr:      true,
			errMsg:       "listing CloudWatch log groups",
		},
		{
			name: "handles API error for ListTagsForResource gracefully",
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					describeLogGroupsOutput: &cloudwatchlogs.DescribeLogGroupsOutput{
						LogGroups: []types.LogGroup{
							{
								LogGroupName: aws.String("/my-app/logs"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/my-app/logs"),
							},
						},
					},
					listTagsForResourceError: errors.New("access denied"),
				}
			},
			accountID:    "123456789012",
			region:       "us-east-1",
			skipPatterns: nil,
			want: []domain.Resource{
				{
					ID:        "/my-app/logs",
					Type:      domain.ResourceTypeLogs,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "handles empty result",
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					describeLogGroupsOutput: &cloudwatchlogs.DescribeLogGroupsOutput{
						LogGroups: []types.LogGroup{},
					},
				}
			},
			accountID:    "123456789012",
			region:       "us-east-1",
			skipPatterns: nil,
			want:         nil,
			wantErr:      false,
		},
		{
			name: "handles pagination",
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					describeLogGroupsPages: []*cloudwatchlogs.DescribeLogGroupsOutput{
						{
							LogGroups: []types.LogGroup{
								{
									LogGroupName: aws.String("/app/page1"),
									Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/app/page1"),
								},
							},
							NextToken: aws.String("next-page-token"),
						},
						{
							LogGroups: []types.LogGroup{
								{
									LogGroupName: aws.String("/app/page2"),
									Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/app/page2"),
								},
							},
						},
					},
				}
			},
			accountID:    "123456789012",
			region:       "us-east-1",
			skipPatterns: nil,
			want: []domain.Resource{
				{
					ID:        "/app/page1",
					Type:      domain.ResourceTypeLogs,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
				{
					ID:        "/app/page2",
					Type:      domain.ResourceTypeLogs,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "uses default skip patterns",
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					describeLogGroupsOutput: &cloudwatchlogs.DescribeLogGroupsOutput{
						LogGroups: []types.LogGroup{
							{
								LogGroupName: aws.String("/aws/lambda/function"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/aws/lambda/function"),
							},
							{
								LogGroupName: aws.String("/aws/eks/cluster/logs"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/aws/eks/cluster/logs"),
							},
							{
								LogGroupName: aws.String("/aws/rds/instance/error"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/aws/rds/instance/error"),
							},
							{
								LogGroupName: aws.String("/aws/elasticbeanstalk/env/var"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/aws/elasticbeanstalk/env/var"),
							},
							{
								LogGroupName: aws.String("/custom/logs"),
								Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/custom/logs"),
							},
						},
					},
				}
			},
			accountID:    "123456789012",
			region:       "us-east-1",
			skipPatterns: nil, // nil means use defaults
			want: []domain.Resource{
				{
					ID:        "/custom/logs",
					Type:      domain.ResourceTypeLogs,
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
			repo := &LogsRepository{
				client:       mock,
				accountID:    tt.accountID,
				region:       tt.region,
				skipPatterns: tt.skipPatterns,
			}

			// Use default patterns if nil (not empty slice)
			if tt.skipPatterns == nil {
				repo.skipPatterns = DefaultLogsSkipPatterns
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

func TestLogsRepository_Tag(t *testing.T) {
	expDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		mockSetup      func() *mockLogsClient
		accountID      string
		region         string
		resourceID     string
		expirationDate time.Time
		wantARN        string
		wantErr        bool
		errMsg         string
	}{
		{
			name: "tags log group successfully",
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					tagResourceOutput: &cloudwatchlogs.TagResourceOutput{},
				}
			},
			accountID:      "123456789012",
			region:         "us-east-1",
			resourceID:     "/my-app/logs",
			expirationDate: expDate,
			wantARN:        "arn:aws:logs:us-east-1:123456789012:log-group:/my-app/logs",
			wantErr:        false,
		},
		{
			name: "constructs correct ARN for different region",
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					tagResourceOutput: &cloudwatchlogs.TagResourceOutput{},
				}
			},
			accountID:      "987654321098",
			region:         "eu-west-1",
			resourceID:     "/prod/application",
			expirationDate: expDate,
			wantARN:        "arn:aws:logs:eu-west-1:987654321098:log-group:/prod/application",
			wantErr:        false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					tagResourceError: errors.New("access denied"),
				}
			},
			accountID:      "123456789012",
			region:         "us-east-1",
			resourceID:     "/my-app/logs",
			expirationDate: expDate,
			wantErr:        true,
			errMsg:         "tagging CloudWatch log group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &LogsRepository{
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
				if len(mock.tagResourceInput.Tags) > 0 {
					if value, ok := mock.tagResourceInput.Tags[ExpirationTagName]; !ok {
						t.Errorf("Tag() did not set %s tag", ExpirationTagName)
					} else {
						wantValue := tt.expirationDate.Format(ExpirationDateFormat)
						if value != wantValue {
							t.Errorf("Tag() value = %v, want %v", value, wantValue)
						}
					}
				}
			}
		})
	}
}

func TestLogsRepository_Delete(t *testing.T) {
	tests := []struct {
		name       string
		mockSetup  func() *mockLogsClient
		resourceID string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "deletes log group successfully",
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					deleteLogGroupOutput: &cloudwatchlogs.DeleteLogGroupOutput{},
				}
			},
			resourceID: "/my-app/logs",
			wantErr:    false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					deleteLogGroupError: errors.New("cannot delete"),
				}
			},
			resourceID: "/my-app/logs",
			wantErr:    true,
			errMsg:     "deleting CloudWatch log group",
		},
		{
			name: "handles log group with special characters in name",
			mockSetup: func() *mockLogsClient {
				return &mockLogsClient{
					deleteLogGroupOutput: &cloudwatchlogs.DeleteLogGroupOutput{},
				}
			},
			resourceID: "/aws/lambda/my-function-name_v1.2.3",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &LogsRepository{
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
				// Verify DeleteLogGroup was called
				if !mock.deleteLogGroupCalled {
					t.Fatal("DeleteLogGroup was not called")
				}
				if mock.deleteLogGroupInput == nil {
					t.Fatal("DeleteLogGroup input is nil")
				}
				if aws.ToString(mock.deleteLogGroupInput.LogGroupName) != tt.resourceID {
					t.Errorf("Delete() LogGroupName = %v, want %v",
						aws.ToString(mock.deleteLogGroupInput.LogGroupName), tt.resourceID)
				}
			}
		})
	}
}

func TestLogsRepository_shouldSkip(t *testing.T) {
	tests := []struct {
		name         string
		logGroupName string
		skipPatterns []string
		want         bool
	}{
		{
			name:         "matches lambda pattern",
			logGroupName: "/aws/lambda/my-function",
			skipPatterns: []string{"/aws/lambda/*"},
			want:         true,
		},
		{
			name:         "matches eks pattern",
			logGroupName: "/aws/eks/my-cluster/cluster",
			skipPatterns: []string{"/aws/eks/*"},
			want:         true,
		},
		{
			name:         "matches rds pattern",
			logGroupName: "/aws/rds/instance/postgres/error",
			skipPatterns: []string{"/aws/rds/*"},
			want:         true,
		},
		{
			name:         "matches elasticbeanstalk pattern",
			logGroupName: "/aws/elasticbeanstalk/env/var/log",
			skipPatterns: []string{"/aws/elasticbeanstalk/*"},
			want:         true,
		},
		{
			name:         "does not match pattern",
			logGroupName: "/my-app/logs",
			skipPatterns: []string{"/aws/lambda/*", "/aws/eks/*"},
			want:         false,
		},
		{
			name:         "exact match",
			logGroupName: "/exact/match",
			skipPatterns: []string{"/exact/match"},
			want:         true,
		},
		{
			name:         "wildcard at end only",
			logGroupName: "/aws/lambda/function",
			skipPatterns: []string{"/aws/lambda/*"},
			want:         true,
		},
		{
			name:         "no patterns means no skip",
			logGroupName: "/aws/lambda/my-function",
			skipPatterns: []string{},
			want:         false,
		},
		{
			name:         "pattern does not match partial path",
			logGroupName: "/aws/lambda-custom/function",
			skipPatterns: []string{"/aws/lambda/*"},
			want:         false,
		},
		{
			name:         "matches nested path",
			logGroupName: "/aws/eks/cluster/control-plane/audit",
			skipPatterns: []string{"/aws/eks/*"},
			want:         true,
		},
		{
			name:         "question mark wildcard",
			logGroupName: "/app/log1",
			skipPatterns: []string{"/app/log?"},
			want:         true,
		},
		{
			name:         "question mark does not match multiple chars",
			logGroupName: "/app/log123",
			skipPatterns: []string{"/app/log?"},
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &LogsRepository{
				skipPatterns: tt.skipPatterns,
			}
			got := repo.shouldSkip(tt.logGroupName)
			if got != tt.want {
				t.Errorf("shouldSkip(%q) = %v, want %v", tt.logGroupName, got, tt.want)
			}
		})
	}
}

func TestLogsRepository_buildARN(t *testing.T) {
	tests := []struct {
		name         string
		accountID    string
		region       string
		logGroupName string
		want         string
	}{
		{
			name:         "standard ARN",
			accountID:    "123456789012",
			region:       "us-east-1",
			logGroupName: "/my-app/logs",
			want:         "arn:aws:logs:us-east-1:123456789012:log-group:/my-app/logs",
		},
		{
			name:         "different region",
			accountID:    "987654321098",
			region:       "eu-central-1",
			logGroupName: "/prod/application",
			want:         "arn:aws:logs:eu-central-1:987654321098:log-group:/prod/application",
		},
		{
			name:         "lambda log group",
			accountID:    "111222333444",
			region:       "ap-southeast-1",
			logGroupName: "/aws/lambda/my-function",
			want:         "arn:aws:logs:ap-southeast-1:111222333444:log-group:/aws/lambda/my-function",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &LogsRepository{
				accountID: tt.accountID,
				region:    tt.region,
			}
			got := repo.buildARN(tt.logGroupName)
			if got != tt.want {
				t.Errorf("buildARN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLogsRepository_InterfaceCompliance(_ *testing.T) {
	// Verify LogsRepository implements domain.ResourceRepository
	var _ domain.ResourceRepository = (*LogsRepository)(nil)
}

func TestNewLogsRepository(t *testing.T) {
	tests := []struct {
		name             string
		accountID        string
		region           string
		skipPatterns     []string
		wantSkipPatterns []string
	}{
		{
			name:             "creates repository with default skip patterns",
			accountID:        "123456789012",
			region:           "us-east-1",
			skipPatterns:     nil,
			wantSkipPatterns: DefaultLogsSkipPatterns,
		},
		{
			name:             "creates repository with custom skip patterns",
			accountID:        "987654321098",
			region:           "eu-west-1",
			skipPatterns:     []string{"/custom/*"},
			wantSkipPatterns: []string{"/custom/*"},
		},
		{
			name:             "creates repository with empty skip patterns",
			accountID:        "111222333444",
			region:           "ap-northeast-1",
			skipPatterns:     []string{},
			wantSkipPatterns: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewLogsRepository(nil, tt.accountID, tt.region, tt.skipPatterns)

			if repo == nil {
				t.Fatal("NewLogsRepository() returned nil")
			}
			if repo.accountID != tt.accountID {
				t.Errorf("accountID = %v, want %v", repo.accountID, tt.accountID)
			}
			if repo.region != tt.region {
				t.Errorf("region = %v, want %v", repo.region, tt.region)
			}
			if len(repo.skipPatterns) != len(tt.wantSkipPatterns) {
				t.Errorf("skipPatterns length = %v, want %v", len(repo.skipPatterns), len(tt.wantSkipPatterns))
			}
			for i, pattern := range repo.skipPatterns {
				if pattern != tt.wantSkipPatterns[i] {
					t.Errorf("skipPatterns[%d] = %v, want %v", i, pattern, tt.wantSkipPatterns[i])
				}
			}
		})
	}
}

func TestLogsRepository_logGroupToResource(t *testing.T) {
	now := time.Now()
	createdAt := now.AddDate(0, -1, 0).UnixMilli()
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name     string
		logGroup types.LogGroup
		tags     map[string]string
		want     domain.Resource
	}{
		{
			name: "basic log group",
			logGroup: types.LogGroup{
				LogGroupName: aws.String("/test/logs"),
				Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/test/logs"),
				CreationTime: aws.Int64(createdAt),
			},
			tags: map[string]string{},
			want: domain.Resource{
				ID:        "/test/logs",
				Type:      domain.ResourceTypeLogs,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Tags:      map[string]string{},
			},
		},
		{
			name: "log group with Name tag",
			logGroup: types.LogGroup{
				LogGroupName: aws.String("/prod/app"),
				Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/prod/app"),
			},
			tags: map[string]string{
				"Name": "Production Application Logs",
			},
			want: domain.Resource{
				ID:        "/prod/app",
				Type:      domain.ResourceTypeLogs,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Name:      "Production Application Logs",
				Tags: map[string]string{
					"Name": "Production Application Logs",
				},
			},
		},
		{
			name: "log group with expiration date",
			logGroup: types.LogGroup{
				LogGroupName: aws.String("/expiring/logs"),
				Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/expiring/logs"),
			},
			tags: map[string]string{
				ExpirationTagName: expDateStr,
			},
			want: domain.Resource{
				ID:        "/expiring/logs",
				Type:      domain.ResourceTypeLogs,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Tags: map[string]string{
					ExpirationTagName: expDateStr,
				},
			},
		},
		{
			name: "log group that never expires",
			logGroup: types.LogGroup{
				LogGroupName: aws.String("/permanent/logs"),
				Arn:          aws.String("arn:aws:logs:us-east-1:123456789012:log-group:/permanent/logs"),
			},
			tags: map[string]string{
				ExpirationTagName: NeverExpiresValue,
			},
			want: domain.Resource{
				ID:           "/permanent/logs",
				Type:         domain.ResourceTypeLogs,
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
			repo := &LogsRepository{
				accountID: "123456789012",
				region:    "us-east-1",
			}

			got := repo.logGroupToResource(tt.logGroup, tt.tags)

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

func TestDefaultLogsSkipPatterns(t *testing.T) {
	// Verify default patterns are set correctly
	expectedPatterns := []string{
		"/aws/lambda/*",
		"/aws/eks/*",
		"/aws/rds/*",
		"/aws/elasticbeanstalk/*",
	}

	if len(DefaultLogsSkipPatterns) != len(expectedPatterns) {
		t.Errorf("DefaultLogsSkipPatterns length = %d, want %d", len(DefaultLogsSkipPatterns), len(expectedPatterns))
	}

	for i, pattern := range DefaultLogsSkipPatterns {
		if pattern != expectedPatterns[i] {
			t.Errorf("DefaultLogsSkipPatterns[%d] = %v, want %v", i, pattern, expectedPatterns[i])
		}
	}
}
