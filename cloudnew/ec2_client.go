package cloudnew

import (
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/pkg/errors"
)

// AWSClient is a wrapper for aws-sdk-go so we can use a mock in testing.
type AWSClient interface {
	// Create a new aws-sdk-client or mock if one does not exist, otherwise no-op.
	Create(*credentials.Credentials) error

	// RunInstances is a wrapper for ec2.RunInstances.
	RunInstances(*ec2.RunInstancesInput) (*ec2.Reservation, error)

	// DescribeInstances is a wrapper for ec2.DescribeInstances.
	DescribeInstances(*ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error)

	// CreateTags is a wrapper for ec2.CreateTags.
	CreateTags(*ec2.CreateTagsInput) error

	// TerminateInstances is a wrapper for ec2.TerminateInstances.
	TerminateInstances(*ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error)

	// RequestSpotInstances is a wrapper for ec2.RequestSpotInstances.
	RequestSpotInstances(*ec2.RequestSpotInstancesInput) (*ec2.RequestSpotInstancesOutput, error)

	// DescribeSpotInstanceRequests is a wrapper for ec2.DescribeSpotInstanceRequests.
	DescribeSpotInstanceRequests(*ec2.DescribeSpotInstanceRequestsInput) (*ec2.DescribeSpotInstanceRequestsOutput, error)

	// CancelSpotInstanceRequests is a wrapper for ec2.CancelSpotInstanceRequests.
	CancelSpotInstanceRequests(*ec2.CancelSpotInstanceRequestsInput) error

	// DescribeVolumes is a wrapper for ec2.DescribeVolumes.
	DescribeVolumes(*ec2.DescribeVolumesInput) (*ec2.DescribeVolumesOutput, error)

	// DescribeSpotPriceHistory is a wrapper for ec2.DescribeSpotPriceHistory.
	DescribeSpotPriceHistory(*ec2.DescribeSpotPriceHistoryInput) (*ec2.DescribeSpotPriceHistoryOutput, error)
}

// AWSClientImpl wraps ec2.EC2.
type AWSClientImpl struct {
	session *session.Session
	*ec2.EC2
}

// Create a new aws-sdk-client if one does not exist, otherwise no-op.
func (c *AWSClientImpl) Create(creds *credentials.Credentials) error {
	if creds == nil {
		return errors.New("creds must not be nil")
	}
	if c.session == nil {
		s, err := session.NewSession()
		if err != nil {
			return err
		}
		c.session = s
	}
	c.EC2 = ec2.New(c.session)
	return nil
}

// RunInstances is a wrapper for ec2.RunInstances.
func (c *AWSClientImpl) RunInstances(input *ec2.RunInstancesInput) (*ec2.Reservation, error) {
	return c.EC2.RunInstances(input)
}

// DescribeInstances is a wrapper for ec2.DescribeInstances
func (c *AWSClientImpl) DescribeInstances(input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	return c.EC2.DescribeInstances(input)
}

// CreateTags is a wrapper for ec2.CreateTags.
func (c *AWSClientImpl) CreateTags(input *ec2.CreateTagsInput) error {
	_, err := c.EC2.CreateTags(input)
	return err
}

// TerminateInstances is a wrapper for ec2.TerminateInstances.
func (c *AWSClientImpl) TerminateInstances(input *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
	return c.EC2.TerminateInstances(input)
}

// RequestSpotInstances is a wrapper for ec2.RequestSpotInstances.
func (c *AWSClientImpl) RequestSpotInstances(input *ec2.RequestSpotInstancesInput) (*ec2.RequestSpotInstancesOutput, error) {
	return c.EC2.RequestSpotInstances(input)
}

// DescribeSpotInstanceRequests is a wrapper for ec2.DescribeSpotInstanceRequests.
func (c *AWSClientImpl) DescribeSpotInstanceRequests(input *ec2.DescribeSpotInstanceRequestsInput) (*ec2.DescribeSpotInstanceRequestsOutput, error) {
	return c.EC2.DescribeSpotInstanceRequests(input)
}

// CancelSpotInstanceRequests is a wrapper for ec2.CancelSpotInstanceRequests.
func (c *AWSClientImpl) CancelSpotInstanceRequests(input *ec2.CancelSpotInstanceRequestsInput) error {
	_, err := c.EC2.CancelSpotInstanceRequests(input)
	return err
}

// DescribeVolumes is a wrapper for ec2.DescribeVolumes.
func (c *AWSClientImpl) DescribeVolumes(input *ec2.DescribeVolumesInput) (*ec2.DescribeVolumesOutput, error) {
	return c.EC2.DescribeVolumes(input)
}

// DescribeSpotPriceHistory is a wrapper for ec2.DescribeSpotPriceHistory.
func (c *AWSClientImpl) DescribeSpotPriceHistory(input *ec2.DescribeSpotPriceHistoryInput) (*ec2.DescribeSpotPriceHistoryOutput, error) {
	return c.EC2.DescribeSpotPriceHistory(input)
}

// AWSClientMock mocks ec2.EC2.
type AWSClientMock struct {
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
func (c *AWSClientMock) Create(creds *credentials.Credentials) error {
	c.Credentials = creds
	return nil
}

// RunInstances is a mock for ec2.RunInstances.
func (c *AWSClientMock) RunInstances(input *ec2.RunInstancesInput) (*ec2.Reservation, error) {
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
func (c *AWSClientMock) DescribeInstances(input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
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
func (c *AWSClientMock) CreateTags(input *ec2.CreateTagsInput) error {
	c.CreateTagsInput = input
	return nil
}

// TerminateInstances is a mock for ec2.TerminateInstances.
func (c *AWSClientMock) TerminateInstances(input *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
	c.TerminateInstancesInput = input
	return &ec2.TerminateInstancesOutput{}, nil
}

// RequestSpotInstances is a mock for ec2.RequestSpotInstances.
func (c *AWSClientMock) RequestSpotInstances(input *ec2.RequestSpotInstancesInput) (*ec2.RequestSpotInstancesOutput, error) {
	c.RequestSpotInstancesInput = input
	return &ec2.RequestSpotInstancesOutput{
		SpotInstanceRequests: []*ec2.SpotInstanceRequest{
			&ec2.SpotInstanceRequest{
				InstanceId: makeStringPtr("instance_id"),
				State:      makeStringPtr(cloud.SpotStatusOpen),
				SpotInstanceRequestId: makeStringPtr("instance_id"),
			},
		},
	}, nil
}

// DescribeSpotInstanceRequests is a mock for ec2.DescribeSpotInstanceRequests.
func (c *AWSClientMock) DescribeSpotInstanceRequests(input *ec2.DescribeSpotInstanceRequestsInput) (*ec2.DescribeSpotInstanceRequestsOutput, error) {
	c.DescribeSpotInstanceRequestsInput = input
	return &ec2.DescribeSpotInstanceRequestsOutput{
		SpotInstanceRequests: []*ec2.SpotInstanceRequest{
			&ec2.SpotInstanceRequest{
				InstanceId: makeStringPtr("instance_id"),
				State:      makeStringPtr(cloud.SpotStatusActive),
				SpotInstanceRequestId: makeStringPtr("instance_id"),
			},
		},
	}, nil
}

// CancelSpotInstanceRequests is a mock for ec2.CancelSpotInstanceRequests.
func (c *AWSClientMock) CancelSpotInstanceRequests(input *ec2.CancelSpotInstanceRequestsInput) error {
	c.CancelSpotInstanceRequestsInput = input
	return nil
}

// DescribeVolumes is a mock for ec2.DescribeVolumes.
func (c *AWSClientMock) DescribeVolumes(input *ec2.DescribeVolumesInput) (*ec2.DescribeVolumesOutput, error) {
	c.DescribeVolumesInput = input
	return &ec2.DescribeVolumesOutput{}, nil
}

// DescribeSpotPriceHistory is a mock for ec2.DescribeSpotPriceHistory.
func (c *AWSClientMock) DescribeSpotPriceHistory(input *ec2.DescribeSpotPriceHistoryInput) (*ec2.DescribeSpotPriceHistoryOutput, error) {
	c.DescribeSpotPriceHistoryInput = input
	return &ec2.DescribeSpotPriceHistoryOutput{}, nil
}
