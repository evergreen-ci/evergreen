package cloud

import (
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

// AWSClient is a wrapper for aws-sdk-go so we can use a mock in testing.
type AWSClient interface {
	// Create a new aws-sdk-client or mock if one does not exist, otherwise no-op.
	Create(*credentials.Credentials) error

	// Close an aws-sdk-client or mock.
	Close()

	// RunInstances is a wrapper for ec2.RunInstances.
	RunInstances(*ec2.RunInstancesInput) (*ec2.Reservation, error)

	// DescribeInstances is a wrapper for ec2.DescribeInstances.
	DescribeInstances(*ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error)

	// CreateTags is a wrapper for ec2.CreateTags.
	CreateTags(*ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error)

	// TerminateInstances is a wrapper for ec2.TerminateInstances.
	TerminateInstances(*ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error)

	// RequestSpotInstances is a wrapper for ec2.RequestSpotInstances.
	RequestSpotInstances(*ec2.RequestSpotInstancesInput) (*ec2.RequestSpotInstancesOutput, error)

	// DescribeSpotInstanceRequests is a wrapper for ec2.DescribeSpotInstanceRequests.
	DescribeSpotInstanceRequests(*ec2.DescribeSpotInstanceRequestsInput) (*ec2.DescribeSpotInstanceRequestsOutput, error)

	// CancelSpotInstanceRequests is a wrapper for ec2.CancelSpotInstanceRequests.
	CancelSpotInstanceRequests(*ec2.CancelSpotInstanceRequestsInput) (*ec2.CancelSpotInstanceRequestsOutput, error)

	// DescribeVolumes is a wrapper for ec2.DescribeVolumes.
	DescribeVolumes(*ec2.DescribeVolumesInput) (*ec2.DescribeVolumesOutput, error)

	// DescribeSpotPriceHistory is a wrapper for ec2.DescribeSpotPriceHistory.
	DescribeSpotPriceHistory(*ec2.DescribeSpotPriceHistoryInput) (*ec2.DescribeSpotPriceHistoryOutput, error)
}

// awsClientImpl wraps ec2.EC2.
type awsClientImpl struct {
	session    *session.Session
	httpClient *http.Client
	*ec2.EC2
}

// Create a new aws-sdk-client if one does not exist, otherwise no-op.
func (c *awsClientImpl) Create(creds *credentials.Credentials) error {
	if creds == nil {
		return errors.New("creds must not be nil")
	}
	if c.session == nil {
		c.httpClient = util.GetHttpClient()
		s, err := session.NewSession(&aws.Config{
			HTTPClient: c.httpClient,
		})
		if err != nil {
			return errors.Wrap(err, "error creating session")
		}
		c.session = s
	}
	c.EC2 = ec2.New(c.session)
	return nil
}

func (c *awsClientImpl) Close() {
	util.PutHttpClient(c.httpClient)
	c.httpClient = nil
}

// awsClientMock mocks ec2.EC2.
type awsClientMock struct {
	*credentials.Credentials
	*ec2.RunInstancesInput
	*ec2.DescribeInstancesInput
	*ec2.CreateTagsInput
	*ec2.TerminateInstancesInput
	*ec2.RequestSpotInstancesInput
	*ec2.DescribeSpotInstanceRequestsInput
	*ec2.CancelSpotInstanceRequestsInput
	*ec2.DescribeVolumesInput
	*ec2.DescribeSpotPriceHistoryInput
}

// Create a new mock client.
func (c *awsClientMock) Create(creds *credentials.Credentials) error {
	c.Credentials = creds
	return nil
}

func (c *awsClientMock) Close() {}

// RunInstances is a mock for ec2.RunInstances.
func (c *awsClientMock) RunInstances(input *ec2.RunInstancesInput) (*ec2.Reservation, error) {
	c.RunInstancesInput = input
	return &ec2.Reservation{
		Instances: []*ec2.Instance{
			&ec2.Instance{
				InstanceId: makeStringPtr("instance_id"),
				KeyName:    makeStringPtr("keyName"),
			},
		},
	}, nil
}

// DescribeInstances is a mock for ec2.DescribeInstances
func (c *awsClientMock) DescribeInstances(input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	c.DescribeInstancesInput = input
	return &ec2.DescribeInstancesOutput{
		Reservations: []*ec2.Reservation{
			&ec2.Reservation{
				Instances: []*ec2.Instance{
					&ec2.Instance{
						InstanceId:   makeStringPtr("instance_id"),
						InstanceType: makeStringPtr("instance_type"),
						State: &ec2.InstanceState{
							Name: makeStringPtr(ec2.InstanceStateNameRunning),
						},
						PublicDnsName: makeStringPtr("public_dns_name"),
					},
				},
			},
		},
	}, nil
}

// CreateTags is a mock for ec2.CreateTags.
func (c *awsClientMock) CreateTags(input *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
	c.CreateTagsInput = input
	return nil, nil
}

// TerminateInstances is a mock for ec2.TerminateInstances.
func (c *awsClientMock) TerminateInstances(input *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
	c.TerminateInstancesInput = input
	return &ec2.TerminateInstancesOutput{}, nil
}

// RequestSpotInstances is a mock for ec2.RequestSpotInstances.
func (c *awsClientMock) RequestSpotInstances(input *ec2.RequestSpotInstancesInput) (*ec2.RequestSpotInstancesOutput, error) {
	c.RequestSpotInstancesInput = input
	return &ec2.RequestSpotInstancesOutput{
		SpotInstanceRequests: []*ec2.SpotInstanceRequest{
			&ec2.SpotInstanceRequest{
				InstanceId: makeStringPtr("instance_id"),
				State:      makeStringPtr(SpotStatusOpen),
				SpotInstanceRequestId: makeStringPtr("instance_id"),
			},
		},
	}, nil
}

// DescribeSpotInstanceRequests is a mock for ec2.DescribeSpotInstanceRequests.
func (c *awsClientMock) DescribeSpotInstanceRequests(input *ec2.DescribeSpotInstanceRequestsInput) (*ec2.DescribeSpotInstanceRequestsOutput, error) {
	c.DescribeSpotInstanceRequestsInput = input
	return &ec2.DescribeSpotInstanceRequestsOutput{
		SpotInstanceRequests: []*ec2.SpotInstanceRequest{
			&ec2.SpotInstanceRequest{
				InstanceId: makeStringPtr("instance_id"),
				State:      makeStringPtr(SpotStatusActive),
				SpotInstanceRequestId: makeStringPtr("instance_id"),
			},
		},
	}, nil
}

// CancelSpotInstanceRequests is a mock for ec2.CancelSpotInstanceRequests.
func (c *awsClientMock) CancelSpotInstanceRequests(input *ec2.CancelSpotInstanceRequestsInput) (*ec2.CancelSpotInstanceRequestsOutput, error) {
	c.CancelSpotInstanceRequestsInput = input
	return nil, nil
}

// DescribeVolumes is a mock for ec2.DescribeVolumes.
func (c *awsClientMock) DescribeVolumes(input *ec2.DescribeVolumesInput) (*ec2.DescribeVolumesOutput, error) {
	c.DescribeVolumesInput = input
	return &ec2.DescribeVolumesOutput{}, nil
}

// DescribeSpotPriceHistory is a mock for ec2.DescribeSpotPriceHistory.
func (c *awsClientMock) DescribeSpotPriceHistory(input *ec2.DescribeSpotPriceHistoryInput) (*ec2.DescribeSpotPriceHistoryOutput, error) {
	c.DescribeSpotPriceHistoryInput = input
	return &ec2.DescribeSpotPriceHistoryOutput{}, nil
}
