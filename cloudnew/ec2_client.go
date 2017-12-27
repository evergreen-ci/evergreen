package cloudnew

import (
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
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

// AWSClientMock mocks ec2.EC2.
type AWSClientMock struct {
	*credentials.Credentials
	*ec2.RunInstancesInput
	*ec2.DescribeInstancesInput
	*ec2.CreateTagsInput
	*ec2.TerminateInstancesInput
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
