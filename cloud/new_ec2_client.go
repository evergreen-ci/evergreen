package cloud

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const defaultRegion = "us-east-1"

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

	// DescribeSubnets is a wrapper for ec2.DescribeSubnets.
	DescribeSubnets(*ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error)

	// DescribeVpcs is a wrapper for ec2.DescribeVpcs.
	DescribeVpcs(*ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error)

	GetInstanceInfo(string) (*ec2.Instance, error)
}

// awsClientImpl wraps ec2.EC2.
type awsClientImpl struct { //nolint
	session    *session.Session
	httpClient *http.Client
	*ec2.EC2
}

const (
	awsClientImplRetries     = 10
	awsClientImplStartPeriod = time.Second
)

// Create a new aws-sdk-client if one does not exist, otherwise no-op.
func (c *awsClientImpl) Create(creds *credentials.Credentials) error {
	if creds == nil {
		return errors.New("creds must not be nil")
	}
	if c.session == nil {
		c.httpClient = util.GetHttpClient()
		s, err := session.NewSession(&aws.Config{
			HTTPClient:  c.httpClient,
			Region:      makeStringPtr(defaultRegion),
			Credentials: creds,
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
	if c.httpClient != nil {
		util.PutHttpClient(c.httpClient)
		c.httpClient = nil
	}
}

// RunInstances is a wrapper for ec2.RunInstances.
func (c *awsClientImpl) RunInstances(input *ec2.RunInstancesInput) (*ec2.Reservation, error) {
	var output *ec2.Reservation
	var err error
	grip.Debug(message.Fields{
		"client":  fmt.Sprintf("%T", c),
		"message": "running RunInstances",
		"args":    input,
	})
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.RunInstances(input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, message.Fields{
						"client":  fmt.Sprintf("%T", c),
						"message": "error running RunInstances",
						"args":    input,
					}))
				}
				return true, err
			}
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// DescribeInstances is a wrapper for ec2.DescribeInstances
func (c *awsClientImpl) DescribeInstances(input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	var output *ec2.DescribeInstancesOutput
	var err error
	grip.Debug(message.Fields{
		"client":  fmt.Sprintf("%T", c),
		"message": "running DescribeInstances",
		"args":    input,
	})
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.DescribeInstances(input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, message.Fields{
						"client":  fmt.Sprintf("%T", c),
						"message": "error running DescribeInstances",
						"args":    input,
					}))
				}
				return true, err
			}
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// CreateTags is a wrapper for ec2.CreateTags.
func (c *awsClientImpl) CreateTags(input *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
	var output *ec2.CreateTagsOutput
	var err error
	grip.Debug(message.Fields{
		"client":  fmt.Sprintf("%T", c),
		"message": "running CreateTags",
		"args":    input,
	})
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.CreateTags(input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, message.Fields{
						"client":  fmt.Sprintf("%T", c),
						"message": "error running CreateTags",
						"args":    input,
					}))
				}
				return true, err
			}
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// TerminateInstances is a wrapper for ec2.TerminateInstances.
func (c *awsClientImpl) TerminateInstances(input *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
	var output *ec2.TerminateInstancesOutput
	var err error
	grip.Debug(message.Fields{
		"client":  fmt.Sprintf("%T", c),
		"message": "running TerminateInstances",
		"args":    input,
	})
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.TerminateInstances(input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, message.Fields{
						"client":  fmt.Sprintf("%T", c),
						"message": "error running TerminateInstances",
						"args":    input,
					}))
				}
				return true, err
			}
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// RequestSpotInstances is a wrapper for ec2.RequestSpotInstances.
func (c *awsClientImpl) RequestSpotInstances(input *ec2.RequestSpotInstancesInput) (*ec2.RequestSpotInstancesOutput, error) {
	var output *ec2.RequestSpotInstancesOutput
	var err error
	grip.Debug(message.Fields{
		"client":  fmt.Sprintf("%T", c),
		"message": "running RequestSpotInstances",
		"args":    input,
	})
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.RequestSpotInstances(input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, message.Fields{
						"client":  fmt.Sprintf("%T", c),
						"message": "error running RequestSpotInstances",
						"args":    input,
					}))
				}
				return true, err
			}
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// DescribeSpotInstanceRequests is a wrapper for ec2.DescribeSpotInstanceRequests.
func (c *awsClientImpl) DescribeSpotInstanceRequests(input *ec2.DescribeSpotInstanceRequestsInput) (*ec2.DescribeSpotInstanceRequestsOutput, error) {
	var output *ec2.DescribeSpotInstanceRequestsOutput
	var err error
	grip.Debug(message.Fields{
		"client":  fmt.Sprintf("%T", c),
		"message": "running DescribeSpotInstanceRequests",
		"args":    input,
	})
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.DescribeSpotInstanceRequests(input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, message.Fields{
						"client":  fmt.Sprintf("%T", c),
						"message": "error running DescribeSpotInstanceRequests",
						"args":    input,
					}))
				}
				return true, err
			}
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// CancelSpotInstanceRequests is a wrapper for ec2.CancelSpotInstanceRequests.
func (c *awsClientImpl) CancelSpotInstanceRequests(input *ec2.CancelSpotInstanceRequestsInput) (*ec2.CancelSpotInstanceRequestsOutput, error) {
	var output *ec2.CancelSpotInstanceRequestsOutput
	var err error
	grip.Debug(message.Fields{
		"client":  fmt.Sprintf("%T", c),
		"message": "running CancelSpotInstanceRequests",
		"args":    input,
	})
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.CancelSpotInstanceRequests(input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, message.Fields{
						"client":  fmt.Sprintf("%T", c),
						"message": "error running CancelSpotInstanceRequests",
						"args":    input,
					}))
				}
				return true, err
			}
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// DescribeVolumes is a wrapper for ec2.DescribeVolumes.
func (c *awsClientImpl) DescribeVolumes(input *ec2.DescribeVolumesInput) (*ec2.DescribeVolumesOutput, error) {
	var output *ec2.DescribeVolumesOutput
	var err error
	grip.Debug(message.Fields{
		"client":  fmt.Sprintf("%T", c),
		"message": "running DescribeVolumes",
		"args":    input,
	})
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.DescribeVolumes(input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, message.Fields{
						"client":  fmt.Sprintf("%T", c),
						"message": "error running DescribeVolumes",
						"args":    input,
					}))
				}
				return true, err
			}
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// DescribeSpotPriceHistory is a wrapper for ec2.DescribeSpotPriceHistory.
func (c *awsClientImpl) DescribeSpotPriceHistory(input *ec2.DescribeSpotPriceHistoryInput) (*ec2.DescribeSpotPriceHistoryOutput, error) {
	var output *ec2.DescribeSpotPriceHistoryOutput
	var err error
	grip.Debug(message.Fields{
		"client":  fmt.Sprintf("%T", c),
		"message": "running DescribeSpotPriceHistory",
		"args":    input,
	})
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.DescribeSpotPriceHistory(input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, message.Fields{
						"client":  fmt.Sprintf("%T", c),
						"message": "error running DescribeSpotPriceHistory",
						"args":    input,
					}))
				}
				return true, err
			}
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// DescribeSubnets is a wrapper for ec2.DescribeSubnets.
func (c *awsClientImpl) DescribeSubnets(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	var output *ec2.DescribeSubnetsOutput
	var err error
	grip.Debug(message.Fields{
		"client":  fmt.Sprintf("%T", c),
		"message": "running DescribeSubnets",
		"args":    input,
	})
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.DescribeSubnets(input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, message.Fields{
						"client":  fmt.Sprintf("%T", c),
						"message": "error running DescribeSubnets",
						"args":    input,
					}))
				}
				return true, err
			}
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// DescribeVpcs is a wrapper for ec2.DescribeVpcs.
func (c *awsClientImpl) DescribeVpcs(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	var output *ec2.DescribeVpcsOutput
	var err error
	grip.Debug(message.Fields{
		"client":  fmt.Sprintf("%T", c),
		"message": "running DescribeVpcs",
		"args":    input,
	})
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.DescribeVpcs(input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, message.Fields{
						"client":  fmt.Sprintf("%T", c),
						"message": "error running DescribeVpcs",
						"args":    input,
					}))
				}
				return true, err
			}
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (c *awsClientImpl) GetInstanceInfo(id string) (*ec2.Instance, error) {
	if strings.HasPrefix(id, "sir") {
		return nil, errors.Errorf("id appears to be a spot instance request ID, not a host ID (%s)", id)
	}
	resp, err := c.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{makeStringPtr(id)},
	})
	if err != nil {
		return nil, errors.Wrap(err, "EC2 API returned error for DescribeInstances")
	}
	reservation := resp.Reservations
	if len(reservation) == 0 {
		err = errors.Errorf("No reservation found for instance id: %s", id)
		return nil, err
	}

	instances := reservation[0].Instances
	if len(instances) == 0 {
		err = errors.Errorf("'%s' was not found in reservation '%s'",
			id, *resp.Reservations[0].ReservationId)
		return nil, err
	}

	return instances[0], nil
}

// awsClientMock mocks ec2.EC2.
type awsClientMock struct { //nolint
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
	*ec2.DescribeSubnetsInput
	*ec2.DescribeVpcsInput
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

// DescribeSubnets is a mock for ec2.DescribeSubnets.
func (c *awsClientMock) DescribeSubnets(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	c.DescribeSubnetsInput = input
	return &ec2.DescribeSubnetsOutput{}, nil
}

// DescribeVpcs is a mock for ec2.DescribeVpcs.
func (c *awsClientMock) DescribeVpcs(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	c.DescribeVpcsInput = input
	return &ec2.DescribeVpcsOutput{}, nil
}

func (c *awsClientMock) GetInstanceInfo(id string) (*ec2.Instance, error) {
	instance := &ec2.Instance{}
	instance.Placement = &ec2.Placement{}
	instance.Placement.AvailabilityZone = makeStringPtr("us-east-1a")
	instance.InstanceType = makeStringPtr("m3.4xlarge")
	instance.LaunchTime = makeTimePtr(time.Now())
	instance.PublicDnsName = makeStringPtr("public_dns_name")
	instance.State = &ec2.InstanceState{}
	instance.State.Name = makeStringPtr("running")
	return instance, nil
}
