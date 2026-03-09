package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	"github.com/maxkrivich/cloud-janitor/internal/domain"
)

// Compile-time interface check for mockELBClient.
var _ elbClient = (*mockELBClient)(nil)

// mockELBClient implements elbClient for testing.
type mockELBClient struct {
	describeLoadBalancersOutput *elasticloadbalancingv2.DescribeLoadBalancersOutput
	describeLoadBalancersError  error
	describeLoadBalancersPages  []*elasticloadbalancingv2.DescribeLoadBalancersOutput
	pageIndex                   int

	describeTagsOutput *elasticloadbalancingv2.DescribeTagsOutput
	describeTagsError  error
	describeTagsInputs []*elasticloadbalancingv2.DescribeTagsInput

	addTagsOutput *elasticloadbalancingv2.AddTagsOutput
	addTagsError  error
	addTagsInput  *elasticloadbalancingv2.AddTagsInput

	deleteLoadBalancerOutput *elasticloadbalancingv2.DeleteLoadBalancerOutput
	deleteLoadBalancerError  error
	deleteLoadBalancerInput  *elasticloadbalancingv2.DeleteLoadBalancerInput
	deleteLoadBalancerCalled bool
}

func (m *mockELBClient) DescribeLoadBalancers(_ context.Context, _ *elasticloadbalancingv2.DescribeLoadBalancersInput, _ ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error) {
	if m.describeLoadBalancersError != nil {
		return nil, m.describeLoadBalancersError
	}
	if len(m.describeLoadBalancersPages) > 0 {
		if m.pageIndex >= len(m.describeLoadBalancersPages) {
			return &elasticloadbalancingv2.DescribeLoadBalancersOutput{}, nil
		}
		result := m.describeLoadBalancersPages[m.pageIndex]
		m.pageIndex++
		return result, nil
	}
	return m.describeLoadBalancersOutput, nil
}

func (m *mockELBClient) DescribeTags(_ context.Context, params *elasticloadbalancingv2.DescribeTagsInput, _ ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeTagsOutput, error) {
	m.describeTagsInputs = append(m.describeTagsInputs, params)
	if m.describeTagsError != nil {
		return nil, m.describeTagsError
	}
	return m.describeTagsOutput, nil
}

func (m *mockELBClient) AddTags(_ context.Context, params *elasticloadbalancingv2.AddTagsInput, _ ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.AddTagsOutput, error) {
	m.addTagsInput = params
	if m.addTagsError != nil {
		return nil, m.addTagsError
	}
	return m.addTagsOutput, nil
}

func (m *mockELBClient) DeleteLoadBalancer(_ context.Context, params *elasticloadbalancingv2.DeleteLoadBalancerInput, _ ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DeleteLoadBalancerOutput, error) {
	m.deleteLoadBalancerInput = params
	m.deleteLoadBalancerCalled = true
	if m.deleteLoadBalancerError != nil {
		return nil, m.deleteLoadBalancerError
	}
	return m.deleteLoadBalancerOutput, nil
}

func TestELBRepository_Type(t *testing.T) {
	repo := &ELBRepository{
		client:    &mockELBClient{},
		accountID: "123456789012",
		region:    "us-east-1",
	}
	got := repo.Type()
	want := domain.ResourceTypeELB

	if got != want {
		t.Errorf("Type() = %v, want %v", got, want)
	}
}

func TestELBRepository_List(t *testing.T) {
	now := time.Now()
	createdAt := now.AddDate(0, -1, 0)
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name      string
		mockSetup func() *mockELBClient
		accountID string
		region    string
		want      []domain.Resource
		wantErr   bool
		errMsg    string
	}{
		{
			name: "lists load balancers successfully",
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					describeLoadBalancersOutput: &elasticloadbalancingv2.DescribeLoadBalancersOutput{
						LoadBalancers: []types.LoadBalancer{
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/my-alb/50dc6c495c0c9188"),
								LoadBalancerName: aws.String("my-alb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActive},
								Type:             types.LoadBalancerTypeEnumApplication,
								CreatedTime:      &createdAt,
							},
						},
					},
					describeTagsOutput: &elasticloadbalancingv2.DescribeTagsOutput{
						TagDescriptions: []types.TagDescription{
							{
								ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/my-alb/50dc6c495c0c9188"),
								Tags: []types.Tag{
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
					ID:             "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/my-alb/50dc6c495c0c9188",
					Type:           domain.ResourceTypeELB,
					Region:         "us-east-1",
					AccountID:      "123456789012",
					Name:           "my-alb",
					ExpirationDate: nil,
					NeverExpires:   false,
					Tags: map[string]string{
						"Environment": "prod",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "parses expiration date tag",
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					describeLoadBalancersOutput: &elasticloadbalancingv2.DescribeLoadBalancersOutput{
						LoadBalancers: []types.LoadBalancer{
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/expiring-nlb/abc123"),
								LoadBalancerName: aws.String("expiring-nlb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActive},
								Type:             types.LoadBalancerTypeEnumNetwork,
							},
						},
					},
					describeTagsOutput: &elasticloadbalancingv2.DescribeTagsOutput{
						TagDescriptions: []types.TagDescription{
							{
								ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/expiring-nlb/abc123"),
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
					ID:        "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/expiring-nlb/abc123",
					Type:      domain.ResourceTypeELB,
					Region:    "us-west-2",
					AccountID: "123456789012",
					Name:      "expiring-nlb",
					Tags: map[string]string{
						ExpirationTagName: expDateStr,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "handles never expires tag",
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					describeLoadBalancersOutput: &elasticloadbalancingv2.DescribeLoadBalancersOutput{
						LoadBalancers: []types.LoadBalancer{
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/permanent-alb/xyz789"),
								LoadBalancerName: aws.String("permanent-alb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActive},
							},
						},
					},
					describeTagsOutput: &elasticloadbalancingv2.DescribeTagsOutput{
						TagDescriptions: []types.TagDescription{
							{
								ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/permanent-alb/xyz789"),
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
					ID:           "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/permanent-alb/xyz789",
					Type:         domain.ResourceTypeELB,
					Region:       "us-east-1",
					AccountID:    "123456789012",
					Name:         "permanent-alb",
					NeverExpires: true,
					Tags: map[string]string{
						ExpirationTagName: NeverExpiresValue,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "handles API error on describe load balancers",
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					describeLoadBalancersError: errors.New("API error"),
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   true,
			errMsg:    "listing ELBs",
		},
		{
			name: "handles API error on describe tags",
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					describeLoadBalancersOutput: &elasticloadbalancingv2.DescribeLoadBalancersOutput{
						LoadBalancers: []types.LoadBalancer{
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/my-alb/123"),
								LoadBalancerName: aws.String("my-alb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActive},
							},
						},
					},
					describeTagsError: errors.New("tags API error"),
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   true,
			errMsg:    "describing tags",
		},
		{
			name: "handles empty result",
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					describeLoadBalancersOutput: &elasticloadbalancingv2.DescribeLoadBalancersOutput{
						LoadBalancers: []types.LoadBalancer{},
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
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					describeLoadBalancersPages: []*elasticloadbalancingv2.DescribeLoadBalancersOutput{
						{
							LoadBalancers: []types.LoadBalancer{
								{
									LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/lb-page-1/111"),
									LoadBalancerName: aws.String("lb-page-1"),
									State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActive},
								},
							},
							NextMarker: aws.String("next-page-token"),
						},
						{
							LoadBalancers: []types.LoadBalancer{
								{
									LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/lb-page-2/222"),
									LoadBalancerName: aws.String("lb-page-2"),
									State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActive},
								},
							},
						},
					},
					describeTagsOutput: &elasticloadbalancingv2.DescribeTagsOutput{
						TagDescriptions: []types.TagDescription{
							{
								ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/lb-page-1/111"),
								Tags:        []types.Tag{},
							},
							{
								ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/lb-page-2/222"),
								Tags:        []types.Tag{},
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/lb-page-1/111",
					Type:      domain.ResourceTypeELB,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Name:      "lb-page-1",
					Tags:      map[string]string{},
				},
				{
					ID:        "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/lb-page-2/222",
					Type:      domain.ResourceTypeELB,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Name:      "lb-page-2",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "handles multiple load balancer types",
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					describeLoadBalancersOutput: &elasticloadbalancingv2.DescribeLoadBalancersOutput{
						LoadBalancers: []types.LoadBalancer{
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/my-alb/111"),
								LoadBalancerName: aws.String("my-alb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActive},
								Type:             types.LoadBalancerTypeEnumApplication,
							},
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/net/my-nlb/222"),
								LoadBalancerName: aws.String("my-nlb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActive},
								Type:             types.LoadBalancerTypeEnumNetwork,
							},
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/gwy/my-gwlb/333"),
								LoadBalancerName: aws.String("my-gwlb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActive},
								Type:             types.LoadBalancerTypeEnumGateway,
							},
						},
					},
					describeTagsOutput: &elasticloadbalancingv2.DescribeTagsOutput{
						TagDescriptions: []types.TagDescription{
							{
								ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/my-alb/111"),
								Tags:        []types.Tag{},
							},
							{
								ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/net/my-nlb/222"),
								Tags:        []types.Tag{},
							},
							{
								ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/gwy/my-gwlb/333"),
								Tags:        []types.Tag{},
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/my-alb/111",
					Type:      domain.ResourceTypeELB,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Name:      "my-alb",
					Tags:      map[string]string{},
				},
				{
					ID:        "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/net/my-nlb/222",
					Type:      domain.ResourceTypeELB,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Name:      "my-nlb",
					Tags:      map[string]string{},
				},
				{
					ID:        "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/gwy/my-gwlb/333",
					Type:      domain.ResourceTypeELB,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Name:      "my-gwlb",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &ELBRepository{
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

func TestELBRepository_List_FiltersInvalidStates(t *testing.T) {
	tests := []struct {
		name          string
		mockSetup     func() *mockELBClient
		wantCount     int
		wantResources []string // Expected resource names
	}{
		{
			name: "skips provisioning load balancers",
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					describeLoadBalancersOutput: &elasticloadbalancingv2.DescribeLoadBalancersOutput{
						LoadBalancers: []types.LoadBalancer{
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/active-lb/111"),
								LoadBalancerName: aws.String("active-lb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActive},
							},
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/provisioning-lb/222"),
								LoadBalancerName: aws.String("provisioning-lb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumProvisioning},
							},
						},
					},
					describeTagsOutput: &elasticloadbalancingv2.DescribeTagsOutput{
						TagDescriptions: []types.TagDescription{
							{
								ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/active-lb/111"),
								Tags:        []types.Tag{},
							},
						},
					},
				}
			},
			wantCount:     1,
			wantResources: []string{"active-lb"},
		},
		{
			name: "skips failed load balancers",
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					describeLoadBalancersOutput: &elasticloadbalancingv2.DescribeLoadBalancersOutput{
						LoadBalancers: []types.LoadBalancer{
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/active-lb/111"),
								LoadBalancerName: aws.String("active-lb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActive},
							},
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/failed-lb/333"),
								LoadBalancerName: aws.String("failed-lb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumFailed},
							},
						},
					},
					describeTagsOutput: &elasticloadbalancingv2.DescribeTagsOutput{
						TagDescriptions: []types.TagDescription{
							{
								ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/active-lb/111"),
								Tags:        []types.Tag{},
							},
						},
					},
				}
			},
			wantCount:     1,
			wantResources: []string{"active-lb"},
		},
		{
			name: "includes active_impaired load balancers",
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					describeLoadBalancersOutput: &elasticloadbalancingv2.DescribeLoadBalancersOutput{
						LoadBalancers: []types.LoadBalancer{
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/active-lb/111"),
								LoadBalancerName: aws.String("active-lb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActive},
							},
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/impaired-lb/444"),
								LoadBalancerName: aws.String("impaired-lb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActiveImpaired},
							},
						},
					},
					describeTagsOutput: &elasticloadbalancingv2.DescribeTagsOutput{
						TagDescriptions: []types.TagDescription{
							{
								ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/active-lb/111"),
								Tags:        []types.Tag{},
							},
							{
								ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/impaired-lb/444"),
								Tags:        []types.Tag{},
							},
						},
					},
				}
			},
			wantCount:     2,
			wantResources: []string{"active-lb", "impaired-lb"},
		},
		{
			name: "filters out all invalid states",
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					describeLoadBalancersOutput: &elasticloadbalancingv2.DescribeLoadBalancersOutput{
						LoadBalancers: []types.LoadBalancer{
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/active-lb/111"),
								LoadBalancerName: aws.String("active-lb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActive},
							},
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/provisioning-lb/222"),
								LoadBalancerName: aws.String("provisioning-lb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumProvisioning},
							},
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/failed-lb/333"),
								LoadBalancerName: aws.String("failed-lb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumFailed},
							},
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/impaired-lb/444"),
								LoadBalancerName: aws.String("impaired-lb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActiveImpaired},
							},
						},
					},
					describeTagsOutput: &elasticloadbalancingv2.DescribeTagsOutput{
						TagDescriptions: []types.TagDescription{
							{
								ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/active-lb/111"),
								Tags:        []types.Tag{},
							},
							{
								ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/impaired-lb/444"),
								Tags:        []types.Tag{},
							},
						},
					},
				}
			},
			wantCount:     2,
			wantResources: []string{"active-lb", "impaired-lb"},
		},
		{
			name: "handles nil state gracefully",
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					describeLoadBalancersOutput: &elasticloadbalancingv2.DescribeLoadBalancersOutput{
						LoadBalancers: []types.LoadBalancer{
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/active-lb/111"),
								LoadBalancerName: aws.String("active-lb"),
								State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActive},
							},
							{
								LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/nil-state-lb/555"),
								LoadBalancerName: aws.String("nil-state-lb"),
								State:            nil,
							},
						},
					},
					describeTagsOutput: &elasticloadbalancingv2.DescribeTagsOutput{
						TagDescriptions: []types.TagDescription{
							{
								ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/active-lb/111"),
								Tags:        []types.Tag{},
							},
						},
					},
				}
			},
			wantCount:     1,
			wantResources: []string{"active-lb"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &ELBRepository{
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
				if resource.Name != tt.wantResources[i] {
					t.Errorf("List()[%d].Name = %v, want %v", i, resource.Name, tt.wantResources[i])
				}
			}
		})
	}
}

func TestELBRepository_List_BatchTagFetch(t *testing.T) {
	// Create 25 load balancers to test batching (max 20 per API call)
	loadBalancers := make([]types.LoadBalancer, 25)
	tagDescriptions := make([]types.TagDescription, 25)

	for i := 0; i < 25; i++ {
		arn := aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/lb-" + string(rune('a'+i)) + "/123")
		loadBalancers[i] = types.LoadBalancer{
			LoadBalancerArn:  arn,
			LoadBalancerName: aws.String("lb-" + string(rune('a'+i))),
			State:            &types.LoadBalancerState{Code: types.LoadBalancerStateEnumActive},
		}
		tagDescriptions[i] = types.TagDescription{
			ResourceArn: arn,
			Tags:        []types.Tag{},
		}
	}

	mock := &mockELBClient{
		describeLoadBalancersOutput: &elasticloadbalancingv2.DescribeLoadBalancersOutput{
			LoadBalancers: loadBalancers,
		},
		describeTagsOutput: &elasticloadbalancingv2.DescribeTagsOutput{
			TagDescriptions: tagDescriptions,
		},
	}

	repo := &ELBRepository{
		client:    mock,
		accountID: "123456789012",
		region:    "us-east-1",
	}

	got, err := repo.List(context.Background(), "")
	if err != nil {
		t.Errorf("List() unexpected error = %v", err)
		return
	}

	// Verify we got all resources
	if len(got) != 25 {
		t.Errorf("List() returned %d resources, want 25", len(got))
	}

	// Verify DescribeTags was called twice (batched: 20 + 5)
	if len(mock.describeTagsInputs) != 2 {
		t.Errorf("DescribeTags called %d times, want 2 (batched)", len(mock.describeTagsInputs))
		return
	}

	// Verify first batch has 20 ARNs
	if len(mock.describeTagsInputs[0].ResourceArns) != 20 {
		t.Errorf("First DescribeTags batch had %d ARNs, want 20", len(mock.describeTagsInputs[0].ResourceArns))
	}

	// Verify second batch has 5 ARNs
	if len(mock.describeTagsInputs[1].ResourceArns) != 5 {
		t.Errorf("Second DescribeTags batch had %d ARNs, want 5", len(mock.describeTagsInputs[1].ResourceArns))
	}
}

func TestELBRepository_Tag(t *testing.T) {
	expDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		mockSetup      func() *mockELBClient
		resourceID     string
		expirationDate time.Time
		wantErr        bool
		errMsg         string
	}{
		{
			name: "tags load balancer successfully",
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					addTagsOutput: &elasticloadbalancingv2.AddTagsOutput{},
				}
			},
			resourceID:     "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/my-alb/50dc6c495c0c9188",
			expirationDate: expDate,
			wantErr:        false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					addTagsError: errors.New("access denied"),
				}
			},
			resourceID:     "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/my-alb/123",
			expirationDate: expDate,
			wantErr:        true,
			errMsg:         "tagging ELB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &ELBRepository{
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
				// Verify the ARN was passed correctly
				if mock.addTagsInput == nil {
					t.Fatal("AddTags was not called")
				}
				if len(mock.addTagsInput.ResourceArns) != 1 {
					t.Errorf("Tag() passed %d ARNs, want 1", len(mock.addTagsInput.ResourceArns))
				}
				if mock.addTagsInput.ResourceArns[0] != tt.resourceID {
					t.Errorf("Tag() ARN = %v, want %v", mock.addTagsInput.ResourceArns[0], tt.resourceID)
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

func TestELBRepository_Delete(t *testing.T) {
	tests := []struct {
		name       string
		mockSetup  func() *mockELBClient
		resourceID string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "deletes load balancer successfully",
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					deleteLoadBalancerOutput: &elasticloadbalancingv2.DeleteLoadBalancerOutput{},
				}
			},
			resourceID: "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/my-alb/50dc6c495c0c9188",
			wantErr:    false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockELBClient {
				return &mockELBClient{
					deleteLoadBalancerError: errors.New("load balancer not found"),
				}
			},
			resourceID: "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/nonexistent/123",
			wantErr:    true,
			errMsg:     "deleting ELB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &ELBRepository{
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
				// Verify DeleteLoadBalancer was called
				if !mock.deleteLoadBalancerCalled {
					t.Fatal("DeleteLoadBalancer was not called")
				}
				if mock.deleteLoadBalancerInput == nil {
					t.Fatal("DeleteLoadBalancer input is nil")
				}
				if aws.ToString(mock.deleteLoadBalancerInput.LoadBalancerArn) != tt.resourceID {
					t.Errorf("Delete() LoadBalancerArn = %v, want %v",
						aws.ToString(mock.deleteLoadBalancerInput.LoadBalancerArn), tt.resourceID)
				}
			}
		})
	}
}

func TestELBRepository_InterfaceCompliance(_ *testing.T) {
	// Verify ELBRepository implements domain.ResourceRepository
	var _ domain.ResourceRepository = (*ELBRepository)(nil)
}

func TestNewELBRepository(t *testing.T) {
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
			repo := NewELBRepository(nil, tt.accountID, tt.region)

			if repo == nil {
				t.Fatal("NewELBRepository() returned nil")
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
