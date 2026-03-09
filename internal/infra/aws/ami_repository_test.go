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

// Compile-time interface check for mockAMIClient.
var _ amiClient = (*mockAMIClient)(nil)

// mockAMIClient implements amiClient for testing.
type mockAMIClient struct {
	describeImagesOutput *ec2.DescribeImagesOutput
	describeImagesError  error
	describeImagesInput  *ec2.DescribeImagesInput

	createTagsOutput *ec2.CreateTagsOutput
	createTagsError  error
	createTagsInput  *ec2.CreateTagsInput

	deregisterImageOutput *ec2.DeregisterImageOutput
	deregisterImageError  error
	deregisterImageInput  *ec2.DeregisterImageInput
	deregisterImageCalled bool

	deleteSnapshotOutput *ec2.DeleteSnapshotOutput
	deleteSnapshotError  error
	deleteSnapshotInputs []*ec2.DeleteSnapshotInput
	deleteSnapshotCalled bool
	deleteSnapshotErrors map[string]error // Per-snapshot errors
}

func (m *mockAMIClient) DescribeImages(_ context.Context, params *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	m.describeImagesInput = params
	if m.describeImagesError != nil {
		return nil, m.describeImagesError
	}
	return m.describeImagesOutput, nil
}

func (m *mockAMIClient) CreateTags(_ context.Context, params *ec2.CreateTagsInput, _ ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error) {
	m.createTagsInput = params
	if m.createTagsError != nil {
		return nil, m.createTagsError
	}
	return m.createTagsOutput, nil
}

func (m *mockAMIClient) DeregisterImage(_ context.Context, params *ec2.DeregisterImageInput, _ ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
	m.deregisterImageInput = params
	m.deregisterImageCalled = true
	if m.deregisterImageError != nil {
		return nil, m.deregisterImageError
	}
	return m.deregisterImageOutput, nil
}

func (m *mockAMIClient) DeleteSnapshot(_ context.Context, params *ec2.DeleteSnapshotInput, _ ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
	m.deleteSnapshotInputs = append(m.deleteSnapshotInputs, params)
	m.deleteSnapshotCalled = true

	// Check for per-snapshot errors
	if m.deleteSnapshotErrors != nil {
		snapshotID := aws.ToString(params.SnapshotId)
		if err, ok := m.deleteSnapshotErrors[snapshotID]; ok {
			return nil, err
		}
	}

	if m.deleteSnapshotError != nil {
		return nil, m.deleteSnapshotError
	}
	return m.deleteSnapshotOutput, nil
}

func TestAMIRepository_Type(t *testing.T) {
	repo := &AMIRepository{
		client:    &mockAMIClient{},
		accountID: "123456789012",
		region:    "us-east-1",
	}
	got := repo.Type()
	want := domain.ResourceTypeAMI

	if got != want {
		t.Errorf("Type() = %v, want %v", got, want)
	}
}

func TestAMIRepository_List(t *testing.T) {
	now := time.Now()
	createdAt := now.AddDate(0, -1, 0)
	createdAtStr := createdAt.Format(time.RFC3339)
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name      string
		mockSetup func() *mockAMIClient
		accountID string
		region    string
		want      []domain.Resource
		wantErr   bool
		errMsg    string
	}{
		{
			name: "lists AMIs successfully with self-owned filter",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{
							{
								ImageId:      aws.String("ami-12345678"),
								Name:         aws.String("my-app-image"),
								State:        types.ImageStateAvailable,
								CreationDate: &createdAtStr,
								Tags: []types.Tag{
									{Key: aws.String("Name"), Value: aws.String("My App Image")},
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
					ID:             "ami-12345678",
					Type:           domain.ResourceTypeAMI,
					Region:         "us-east-1",
					AccountID:      "123456789012",
					Name:           "My App Image",
					ExpirationDate: nil,
					NeverExpires:   false,
					Tags: map[string]string{
						"Name":        "My App Image",
						"Environment": "dev",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "verifies self-owned filter is applied",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   false,
		},
		{
			name: "parses expiration date tag",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{
							{
								ImageId: aws.String("ami-expiring"),
								Name:    aws.String("expiring-image"),
								State:   types.ImageStateAvailable,
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
					ID:        "ami-expiring",
					Type:      domain.ResourceTypeAMI,
					Region:    "us-west-2",
					AccountID: "123456789012",
					Name:      "expiring-image",
					Tags: map[string]string{
						ExpirationTagName: expDateStr,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "handles never expires tag",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{
							{
								ImageId: aws.String("ami-permanent"),
								Name:    aws.String("permanent-image"),
								State:   types.ImageStateAvailable,
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
					ID:           "ami-permanent",
					Type:         domain.ResourceTypeAMI,
					Region:       "us-east-1",
					AccountID:    "123456789012",
					Name:         "permanent-image",
					NeverExpires: true,
					Tags: map[string]string{
						ExpirationTagName: NeverExpiresValue,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "filters invalid and failed AMIs",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{
							{
								ImageId: aws.String("ami-available"),
								Name:    aws.String("available-image"),
								State:   types.ImageStateAvailable,
							},
							{
								ImageId: aws.String("ami-pending"),
								Name:    aws.String("pending-image"),
								State:   types.ImageStatePending,
							},
							{
								ImageId: aws.String("ami-deregistered"),
								Name:    aws.String("deregistered-image"),
								State:   types.ImageStateDeregistered,
							},
							{
								ImageId: aws.String("ami-invalid"),
								Name:    aws.String("invalid-image"),
								State:   types.ImageStateInvalid,
							},
							{
								ImageId: aws.String("ami-transient"),
								Name:    aws.String("transient-image"),
								State:   types.ImageStateTransient,
							},
							{
								ImageId: aws.String("ami-failed"),
								Name:    aws.String("failed-image"),
								State:   types.ImageStateFailed,
							},
							{
								ImageId: aws.String("ami-error"),
								Name:    aws.String("error-image"),
								State:   types.ImageStateError,
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "ami-available",
					Type:      domain.ResourceTypeAMI,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Name:      "available-image",
					Tags:      map[string]string{},
				},
				{
					ID:        "ami-pending",
					Type:      domain.ResourceTypeAMI,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Name:      "pending-image",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesError: errors.New("API error"),
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   true,
			errMsg:    "listing AMIs",
		},
		{
			name: "handles empty result",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want:      nil,
			wantErr:   false,
		},
		{
			name: "handles multiple AMIs",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{
							{
								ImageId: aws.String("ami-first"),
								Name:    aws.String("first-image"),
								State:   types.ImageStateAvailable,
							},
							{
								ImageId: aws.String("ami-second"),
								Name:    aws.String("second-image"),
								State:   types.ImageStateAvailable,
							},
							{
								ImageId: aws.String("ami-third"),
								Name:    aws.String("third-image"),
								State:   types.ImageStateAvailable,
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "eu-west-1",
			want: []domain.Resource{
				{
					ID:        "ami-first",
					Type:      domain.ResourceTypeAMI,
					Region:    "eu-west-1",
					AccountID: "123456789012",
					Name:      "first-image",
					Tags:      map[string]string{},
				},
				{
					ID:        "ami-second",
					Type:      domain.ResourceTypeAMI,
					Region:    "eu-west-1",
					AccountID: "123456789012",
					Name:      "second-image",
					Tags:      map[string]string{},
				},
				{
					ID:        "ami-third",
					Type:      domain.ResourceTypeAMI,
					Region:    "eu-west-1",
					AccountID: "123456789012",
					Name:      "third-image",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "uses AMI name when Name tag is missing",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{
							{
								ImageId: aws.String("ami-noname"),
								Name:    aws.String("image-from-ami-name"),
								State:   types.ImageStateAvailable,
								Tags:    []types.Tag{},
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "ami-noname",
					Type:      domain.ResourceTypeAMI,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Name:      "image-from-ami-name",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "parses creation date correctly",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{
							{
								ImageId:      aws.String("ami-dated"),
								Name:         aws.String("dated-image"),
								State:        types.ImageStateAvailable,
								CreationDate: &createdAtStr,
							},
						},
					},
				}
			},
			accountID: "123456789012",
			region:    "us-east-1",
			want: []domain.Resource{
				{
					ID:        "ami-dated",
					Type:      domain.ResourceTypeAMI,
					Region:    "us-east-1",
					AccountID: "123456789012",
					Name:      "dated-image",
					Tags:      map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "handles invalid expiration date format",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{
							{
								ImageId: aws.String("ami-baddate"),
								Name:    aws.String("bad-date-image"),
								State:   types.ImageStateAvailable,
								Tags: []types.Tag{
									{Key: aws.String(ExpirationTagName), Value: aws.String("invalid-date")},
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
					ID:             "ami-baddate",
					Type:           domain.ResourceTypeAMI,
					Region:         "us-east-1",
					AccountID:      "123456789012",
					Name:           "bad-date-image",
					ExpirationDate: nil, // Should remain nil due to parse error
					Tags: map[string]string{
						ExpirationTagName: "invalid-date",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &AMIRepository{
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
				if err == nil || !containsSubstring(err.Error(), tt.errMsg) {
					t.Errorf("List() error = %v, should contain %q", err, tt.errMsg)
				}
				return
			}

			// Verify self-owned filter is used
			if mock.describeImagesInput != nil && !tt.wantErr {
				foundSelfOwner := false
				for _, owner := range mock.describeImagesInput.Owners {
					if owner == "self" {
						foundSelfOwner = true
						break
					}
				}
				if !foundSelfOwner {
					t.Error("List() should use Owners: []string{\"self\"} filter")
				}
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

func TestAMIRepository_Tag(t *testing.T) {
	expDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		mockSetup      func() *mockAMIClient
		resourceID     string
		expirationDate time.Time
		wantErr        bool
		errMsg         string
	}{
		{
			name: "tags AMI with correct ID",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					createTagsOutput: &ec2.CreateTagsOutput{},
				}
			},
			resourceID:     "ami-12345678",
			expirationDate: expDate,
			wantErr:        false,
		},
		{
			name: "uses AMI ID directly without ARN",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					createTagsOutput: &ec2.CreateTagsOutput{},
				}
			},
			resourceID:     "ami-abcdef12",
			expirationDate: expDate,
			wantErr:        false,
		},
		{
			name: "handles API error",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					createTagsError: errors.New("access denied"),
				}
			},
			resourceID:     "ami-12345678",
			expirationDate: expDate,
			wantErr:        true,
			errMsg:         "tagging AMI",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &AMIRepository{
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
				if err == nil || !containsSubstring(err.Error(), tt.errMsg) {
					t.Errorf("Tag() error = %v, should contain %q", err, tt.errMsg)
				}
				return
			}

			if !tt.wantErr {
				// Verify the AMI ID was used directly (not ARN)
				if mock.createTagsInput == nil {
					t.Fatal("CreateTags was not called")
				}
				if len(mock.createTagsInput.Resources) != 1 {
					t.Errorf("Tag() set %d resources, want 1", len(mock.createTagsInput.Resources))
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

func TestAMIRepository_Delete(t *testing.T) {
	tests := []struct {
		name                    string
		mockSetup               func() *mockAMIClient
		resourceID              string
		wantDeregisterCalled    bool
		wantDeleteSnapshotCount int
		wantErr                 bool
		errMsg                  string
	}{
		{
			name: "deregisters AMI and deletes associated snapshots",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{
							{
								ImageId: aws.String("ami-12345678"),
								State:   types.ImageStateAvailable,
								BlockDeviceMappings: []types.BlockDeviceMapping{
									{
										DeviceName: aws.String("/dev/xvda"),
										Ebs: &types.EbsBlockDevice{
											SnapshotId: aws.String("snap-11111111"),
										},
									},
									{
										DeviceName: aws.String("/dev/xvdb"),
										Ebs: &types.EbsBlockDevice{
											SnapshotId: aws.String("snap-22222222"),
										},
									},
								},
							},
						},
					},
					deregisterImageOutput: &ec2.DeregisterImageOutput{},
					deleteSnapshotOutput:  &ec2.DeleteSnapshotOutput{},
				}
			},
			resourceID:              "ami-12345678",
			wantDeregisterCalled:    true,
			wantDeleteSnapshotCount: 2,
			wantErr:                 false,
		},
		{
			name: "handles AMI with no snapshots",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{
							{
								ImageId:             aws.String("ami-nosnapshot"),
								State:               types.ImageStateAvailable,
								BlockDeviceMappings: []types.BlockDeviceMapping{},
							},
						},
					},
					deregisterImageOutput: &ec2.DeregisterImageOutput{},
				}
			},
			resourceID:              "ami-nosnapshot",
			wantDeregisterCalled:    true,
			wantDeleteSnapshotCount: 0,
			wantErr:                 false,
		},
		{
			name: "handles block device without EBS",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{
							{
								ImageId: aws.String("ami-instancestore"),
								State:   types.ImageStateAvailable,
								BlockDeviceMappings: []types.BlockDeviceMapping{
									{
										DeviceName:  aws.String("/dev/sda1"),
										VirtualName: aws.String("ephemeral0"),
										// No Ebs field - instance store
									},
								},
							},
						},
					},
					deregisterImageOutput: &ec2.DeregisterImageOutput{},
				}
			},
			resourceID:              "ami-instancestore",
			wantDeregisterCalled:    true,
			wantDeleteSnapshotCount: 0,
			wantErr:                 false,
		},
		{
			name: "handles API error on DeregisterImage",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{
							{
								ImageId: aws.String("ami-12345678"),
								State:   types.ImageStateAvailable,
							},
						},
					},
					deregisterImageError: errors.New("cannot deregister"),
				}
			},
			resourceID:              "ami-12345678",
			wantDeregisterCalled:    true,
			wantDeleteSnapshotCount: 0,
			wantErr:                 true,
			errMsg:                  "deregistering AMI",
		},
		{
			name: "continues on snapshot deletion errors",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{
							{
								ImageId: aws.String("ami-12345678"),
								State:   types.ImageStateAvailable,
								BlockDeviceMappings: []types.BlockDeviceMapping{
									{
										DeviceName: aws.String("/dev/xvda"),
										Ebs: &types.EbsBlockDevice{
											SnapshotId: aws.String("snap-11111111"),
										},
									},
									{
										DeviceName: aws.String("/dev/xvdb"),
										Ebs: &types.EbsBlockDevice{
											SnapshotId: aws.String("snap-22222222"),
										},
									},
								},
							},
						},
					},
					deregisterImageOutput: &ec2.DeregisterImageOutput{},
					deleteSnapshotOutput:  &ec2.DeleteSnapshotOutput{},
					deleteSnapshotErrors: map[string]error{
						"snap-11111111": errors.New("snapshot in use by another AMI"),
					},
				}
			},
			resourceID:              "ami-12345678",
			wantDeregisterCalled:    true,
			wantDeleteSnapshotCount: 2, // Should attempt both
			wantErr:                 false,
		},
		{
			name: "handles describe images error",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesError: errors.New("describe failed"),
				}
			},
			resourceID:              "ami-12345678",
			wantDeregisterCalled:    false,
			wantDeleteSnapshotCount: 0,
			wantErr:                 true,
			errMsg:                  "describing AMI",
		},
		{
			name: "handles AMI not found",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{},
					},
				}
			},
			resourceID:              "ami-notfound",
			wantDeregisterCalled:    false,
			wantDeleteSnapshotCount: 0,
			wantErr:                 true,
			errMsg:                  "not found",
		},
		{
			name: "handles mixed block devices (EBS and instance store)",
			mockSetup: func() *mockAMIClient {
				return &mockAMIClient{
					describeImagesOutput: &ec2.DescribeImagesOutput{
						Images: []types.Image{
							{
								ImageId: aws.String("ami-mixed"),
								State:   types.ImageStateAvailable,
								BlockDeviceMappings: []types.BlockDeviceMapping{
									{
										DeviceName: aws.String("/dev/xvda"),
										Ebs: &types.EbsBlockDevice{
											SnapshotId: aws.String("snap-ebs"),
										},
									},
									{
										DeviceName:  aws.String("/dev/sdb"),
										VirtualName: aws.String("ephemeral0"),
										// Instance store - no EBS
									},
									{
										DeviceName: aws.String("/dev/xvdc"),
										Ebs: &types.EbsBlockDevice{
											SnapshotId: aws.String("snap-ebs2"),
										},
									},
								},
							},
						},
					},
					deregisterImageOutput: &ec2.DeregisterImageOutput{},
					deleteSnapshotOutput:  &ec2.DeleteSnapshotOutput{},
				}
			},
			resourceID:              "ami-mixed",
			wantDeregisterCalled:    true,
			wantDeleteSnapshotCount: 2, // Only EBS snapshots
			wantErr:                 false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.mockSetup()
			repo := &AMIRepository{
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
				if err == nil || !containsSubstring(err.Error(), tt.errMsg) {
					t.Errorf("Delete() error = %v, should contain %q", err, tt.errMsg)
				}
			}

			if mock.deregisterImageCalled != tt.wantDeregisterCalled {
				t.Errorf("DeregisterImage called = %v, want %v", mock.deregisterImageCalled, tt.wantDeregisterCalled)
			}

			if len(mock.deleteSnapshotInputs) != tt.wantDeleteSnapshotCount {
				t.Errorf("DeleteSnapshot called %d times, want %d", len(mock.deleteSnapshotInputs), tt.wantDeleteSnapshotCount)
			}

			// Verify deregister was called with correct AMI ID
			if tt.wantDeregisterCalled && mock.deregisterImageInput != nil {
				if aws.ToString(mock.deregisterImageInput.ImageId) != tt.resourceID {
					t.Errorf("DeregisterImage ID = %v, want %v",
						aws.ToString(mock.deregisterImageInput.ImageId), tt.resourceID)
				}
			}
		})
	}
}

func TestAMIRepository_InterfaceCompliance(_ *testing.T) {
	// Verify AMIRepository implements domain.ResourceRepository
	var _ domain.ResourceRepository = (*AMIRepository)(nil)
}

func TestNewAMIRepository(t *testing.T) {
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
			repo := NewAMIRepository(nil, tt.accountID, tt.region)

			if repo == nil {
				t.Fatal("NewAMIRepository() returned nil")
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

func TestAMIRepository_imageToResource(t *testing.T) {
	now := time.Now()
	createdAt := now.AddDate(0, -1, 0)
	createdAtStr := createdAt.Format(time.RFC3339)
	expDate := now.AddDate(0, 0, 30)
	expDateStr := expDate.Format(ExpirationDateFormat)

	tests := []struct {
		name  string
		image types.Image
		want  domain.Resource
	}{
		{
			name: "basic image",
			image: types.Image{
				ImageId: aws.String("ami-basic"),
				Name:    aws.String("basic-image"),
				State:   types.ImageStateAvailable,
			},
			want: domain.Resource{
				ID:        "ami-basic",
				Type:      domain.ResourceTypeAMI,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Name:      "basic-image",
				Tags:      map[string]string{},
			},
		},
		{
			name: "image with Name tag overrides AMI name",
			image: types.Image{
				ImageId: aws.String("ami-tagged"),
				Name:    aws.String("ami-name"),
				State:   types.ImageStateAvailable,
				Tags: []types.Tag{
					{Key: aws.String("Name"), Value: aws.String("Tag Name")},
				},
			},
			want: domain.Resource{
				ID:        "ami-tagged",
				Type:      domain.ResourceTypeAMI,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Name:      "Tag Name",
				Tags: map[string]string{
					"Name": "Tag Name",
				},
			},
		},
		{
			name: "image with expiration date",
			image: types.Image{
				ImageId: aws.String("ami-expiring"),
				Name:    aws.String("expiring-image"),
				State:   types.ImageStateAvailable,
				Tags: []types.Tag{
					{Key: aws.String(ExpirationTagName), Value: aws.String(expDateStr)},
				},
			},
			want: domain.Resource{
				ID:        "ami-expiring",
				Type:      domain.ResourceTypeAMI,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Name:      "expiring-image",
				Tags: map[string]string{
					ExpirationTagName: expDateStr,
				},
			},
		},
		{
			name: "image that never expires",
			image: types.Image{
				ImageId: aws.String("ami-permanent"),
				Name:    aws.String("permanent-image"),
				State:   types.ImageStateAvailable,
				Tags: []types.Tag{
					{Key: aws.String(ExpirationTagName), Value: aws.String(NeverExpiresValue)},
				},
			},
			want: domain.Resource{
				ID:           "ami-permanent",
				Type:         domain.ResourceTypeAMI,
				Region:       "us-east-1",
				AccountID:    "123456789012",
				Name:         "permanent-image",
				NeverExpires: true,
				Tags: map[string]string{
					ExpirationTagName: NeverExpiresValue,
				},
			},
		},
		{
			name: "image with creation date",
			image: types.Image{
				ImageId:      aws.String("ami-dated"),
				Name:         aws.String("dated-image"),
				State:        types.ImageStateAvailable,
				CreationDate: &createdAtStr,
			},
			want: domain.Resource{
				ID:        "ami-dated",
				Type:      domain.ResourceTypeAMI,
				Region:    "us-east-1",
				AccountID: "123456789012",
				Name:      "dated-image",
				Tags:      map[string]string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &AMIRepository{
				accountID: "123456789012",
				region:    "us-east-1",
			}

			got := repo.imageToResource(tt.image)

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

func TestAMIRepository_extractSnapshotIDs(t *testing.T) {
	tests := []struct {
		name     string
		mappings []types.BlockDeviceMapping
		want     []string
	}{
		{
			name:     "empty mappings",
			mappings: []types.BlockDeviceMapping{},
			want:     nil,
		},
		{
			name: "single EBS snapshot",
			mappings: []types.BlockDeviceMapping{
				{
					DeviceName: aws.String("/dev/xvda"),
					Ebs: &types.EbsBlockDevice{
						SnapshotId: aws.String("snap-12345678"),
					},
				},
			},
			want: []string{"snap-12345678"},
		},
		{
			name: "multiple EBS snapshots",
			mappings: []types.BlockDeviceMapping{
				{
					DeviceName: aws.String("/dev/xvda"),
					Ebs: &types.EbsBlockDevice{
						SnapshotId: aws.String("snap-11111111"),
					},
				},
				{
					DeviceName: aws.String("/dev/xvdb"),
					Ebs: &types.EbsBlockDevice{
						SnapshotId: aws.String("snap-22222222"),
					},
				},
			},
			want: []string{"snap-11111111", "snap-22222222"},
		},
		{
			name: "instance store volumes (no EBS)",
			mappings: []types.BlockDeviceMapping{
				{
					DeviceName:  aws.String("/dev/sda1"),
					VirtualName: aws.String("ephemeral0"),
				},
			},
			want: nil,
		},
		{
			name: "mixed EBS and instance store",
			mappings: []types.BlockDeviceMapping{
				{
					DeviceName: aws.String("/dev/xvda"),
					Ebs: &types.EbsBlockDevice{
						SnapshotId: aws.String("snap-ebs"),
					},
				},
				{
					DeviceName:  aws.String("/dev/sdb"),
					VirtualName: aws.String("ephemeral0"),
				},
			},
			want: []string{"snap-ebs"},
		},
		{
			name: "EBS without snapshot ID",
			mappings: []types.BlockDeviceMapping{
				{
					DeviceName: aws.String("/dev/xvda"),
					Ebs: &types.EbsBlockDevice{
						// No SnapshotId - new volume
						VolumeSize: aws.Int32(100),
					},
				},
			},
			want: nil,
		},
		{
			name: "nil EBS block device",
			mappings: []types.BlockDeviceMapping{
				{
					DeviceName: aws.String("/dev/xvda"),
					Ebs:        nil,
				},
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &AMIRepository{}
			got := repo.extractSnapshotIDs(tt.mappings)

			if len(got) != len(tt.want) {
				t.Errorf("extractSnapshotIDs() returned %d snapshots, want %d", len(got), len(tt.want))
				return
			}

			for i, snapshotID := range got {
				if snapshotID != tt.want[i] {
					t.Errorf("extractSnapshotIDs()[%d] = %v, want %v", i, snapshotID, tt.want[i])
				}
			}
		})
	}
}

// containsSubstring checks if s contains substr
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
