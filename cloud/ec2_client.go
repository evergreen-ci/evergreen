package cloud

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen/model/host"
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
	RunInstances(context.Context, *ec2.RunInstancesInput) (*ec2.Reservation, error)

	// DescribeInstances is a wrapper for ec2.DescribeInstances.
	DescribeInstances(context.Context, *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error)

	// CreateTags is a wrapper for ec2.CreateTags.
	CreateTags(context.Context, *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error)

	// TerminateInstances is a wrapper for ec2.TerminateInstances.
	TerminateInstances(context.Context, *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error)

	// RequestSpotInstances is a wrapper for ec2.RequestSpotInstances.
	RequestSpotInstances(context.Context, *ec2.RequestSpotInstancesInput) (*ec2.RequestSpotInstancesOutput, error)

	// DescribeSpotInstanceRequests is a wrapper for ec2.DescribeSpotInstanceRequests.
	DescribeSpotInstanceRequests(ctx context.Context, input *ec2.DescribeSpotInstanceRequestsInput) (*ec2.DescribeSpotInstanceRequestsOutput, error)

	// DescribeSpotRequestsAndSave is a wrapper for DescribeSpotInstanceRequests that also saves instance IDs to the db.
	DescribeSpotRequestsAndSave(context.Context, []*host.Host) (*ec2.DescribeSpotInstanceRequestsOutput, error)

	// GetSpotInstanceId returns the instance ID if already saved, otherwise looks it up.
	GetSpotInstanceId(context.Context, *host.Host) (string, error)

	// CancelSpotInstanceRequests is a wrapper for ec2.CancelSpotInstanceRequests.
	CancelSpotInstanceRequests(context.Context, *ec2.CancelSpotInstanceRequestsInput) (*ec2.CancelSpotInstanceRequestsOutput, error)

	// DescribeVolumes is a wrapper for ec2.DescribeVolumes.
	DescribeVolumes(context.Context, *ec2.DescribeVolumesInput) (*ec2.DescribeVolumesOutput, error)

	// DescribeSpotPriceHistory is a wrapper for ec2.DescribeSpotPriceHistory.
	DescribeSpotPriceHistory(context.Context, *ec2.DescribeSpotPriceHistoryInput) (*ec2.DescribeSpotPriceHistoryOutput, error)

	// DescribeSubnets is a wrapper for ec2.DescribeSubnets.
	DescribeSubnets(context.Context, *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error)

	// DescribeVpcs is a wrapper for ec2.DescribeVpcs.
	DescribeVpcs(context.Context, *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error)

	GetInstanceInfo(context.Context, string) (*ec2.Instance, error)
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
		c.httpClient = util.GetHTTPClient()
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
		util.PutHTTPClient(c.httpClient)
		c.httpClient = nil
	}
}

// RunInstances is a wrapper for ec2.RunInstances.
func (c *awsClientImpl) RunInstances(ctx context.Context, input *ec2.RunInstancesInput) (*ec2.Reservation, error) {
	var output *ec2.Reservation
	var err error
	msg := makeAWSLogMessage("RunInstances", fmt.Sprintf("%T", c), input)
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.RunInstancesWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// DescribeInstances is a wrapper for ec2.DescribeInstances
func (c *awsClientImpl) DescribeInstances(ctx context.Context, input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	var output *ec2.DescribeInstancesOutput
	var err error
	msg := makeAWSLogMessage("DescribeInstances", fmt.Sprintf("%T", c), input)
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.DescribeInstancesWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// CreateTags is a wrapper for ec2.CreateTags.
func (c *awsClientImpl) CreateTags(ctx context.Context, input *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
	var output *ec2.CreateTagsOutput
	var err error
	msg := makeAWSLogMessage("CreateTags", fmt.Sprintf("%T", c), input)
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.CreateTagsWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// TerminateInstances is a wrapper for ec2.TerminateInstances.
func (c *awsClientImpl) TerminateInstances(ctx context.Context, input *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
	var output *ec2.TerminateInstancesOutput
	var err error
	msg := makeAWSLogMessage("TerminateInstances", fmt.Sprintf("%T", c), input)
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.TerminateInstancesWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					if strings.Contains(ec2err.Code(), EC2ErrorNotFound) {
						grip.Debug(message.WrapError(ec2err, message.Fields{
							"client":          fmt.Sprintf("%T", c),
							"message":         "instance ID not found in AWS",
							"args":            input,
							"ec2_err_message": ec2err.Message(),
							"ec2_err_code":    ec2err.Code(),
						}))
						return false, nil
					}

					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// RequestSpotInstances is a wrapper for ec2.RequestSpotInstances.
func (c *awsClientImpl) RequestSpotInstances(ctx context.Context, input *ec2.RequestSpotInstancesInput) (*ec2.RequestSpotInstancesOutput, error) {
	var output *ec2.RequestSpotInstancesOutput
	var err error
	msg := makeAWSLogMessage("RequestSpotInstances", fmt.Sprintf("%T", c), input)
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.RequestSpotInstancesWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// DescribeSpotInstanceRequests is a wrapper for ec2.DescribeSpotInstanceRequests.
func (c *awsClientImpl) DescribeSpotInstanceRequests(ctx context.Context, input *ec2.DescribeSpotInstanceRequestsInput) (*ec2.DescribeSpotInstanceRequestsOutput, error) {
	var output *ec2.DescribeSpotInstanceRequestsOutput
	var err error
	msg := makeAWSLogMessage("DescribeSpotInstanceRequests", fmt.Sprintf("%T", c), input)
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.DescribeSpotInstanceRequestsWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (c *awsClientImpl) DescribeSpotRequestsAndSave(ctx context.Context, hosts []*host.Host) (*ec2.DescribeSpotInstanceRequestsOutput, error) {
	spotRequestIds := []*string{}
	for idx := range hosts {
		h := hosts[idx]
		if h == nil {
			return nil, errors.New("unable to describe spot request for nil host")
		}
		spotRequestIds = append(spotRequestIds, makeStringPtr(h.Id))
	}
	apiInput := &ec2.DescribeSpotInstanceRequestsInput{
		SpotInstanceRequestIds: spotRequestIds,
	}

	instances, err := c.DescribeSpotInstanceRequests(ctx, apiInput)
	if err != nil {
		return nil, errors.Wrap(err, "error describing spot requests")
	}

	catcher := grip.NewSimpleCatcher()
	for idx := range hosts {
		h := hosts[idx]
		for idx := range instances.SpotInstanceRequests {
			instance := instances.SpotInstanceRequests[idx]
			if instance.SpotInstanceRequestId != nil && *instance.SpotInstanceRequestId == h.Id {
				if instance.InstanceId != nil {
					h.ExternalIdentifier = *instance.InstanceId
				}
			}
		}
		if h.ExternalIdentifier != "" {
			catcher.Add(h.SetExtId())
		}
	}

	return instances, catcher.Resolve()
}

func (c *awsClientImpl) GetSpotInstanceId(ctx context.Context, h *host.Host) (string, error) {
	if h == nil {
		return "", errors.New("unable to get spot instance for nil host")
	}
	if h.ExternalIdentifier != "" {
		return h.ExternalIdentifier, nil
	}

	_, err := c.DescribeSpotRequestsAndSave(ctx, []*host.Host{h})
	if err != nil {
		return "", err
	}

	return h.ExternalIdentifier, nil
}

// CancelSpotInstanceRequests is a wrapper for ec2.CancelSpotInstanceRequests.
func (c *awsClientImpl) CancelSpotInstanceRequests(ctx context.Context, input *ec2.CancelSpotInstanceRequestsInput) (*ec2.CancelSpotInstanceRequestsOutput, error) {
	var output *ec2.CancelSpotInstanceRequestsOutput
	var err error
	msg := makeAWSLogMessage("CancelSpotInstanceRequests", fmt.Sprintf("%T", c), input)
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.CancelSpotInstanceRequestsWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// DescribeVolumes is a wrapper for ec2.DescribeVolumes.
func (c *awsClientImpl) DescribeVolumes(ctx context.Context, input *ec2.DescribeVolumesInput) (*ec2.DescribeVolumesOutput, error) {
	var output *ec2.DescribeVolumesOutput
	var err error
	msg := makeAWSLogMessage("DescribeVolumes", fmt.Sprintf("%T", c), input)
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.DescribeVolumesWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// DescribeSpotPriceHistory is a wrapper for ec2.DescribeSpotPriceHistory.
func (c *awsClientImpl) DescribeSpotPriceHistory(ctx context.Context, input *ec2.DescribeSpotPriceHistoryInput) (*ec2.DescribeSpotPriceHistoryOutput, error) {
	var output *ec2.DescribeSpotPriceHistoryOutput
	var err error
	msg := makeAWSLogMessage("DescribeSpotPriceHistory", fmt.Sprintf("%T", c), input)
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.DescribeSpotPriceHistoryWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// DescribeSubnets is a wrapper for ec2.DescribeSubnets.
func (c *awsClientImpl) DescribeSubnets(ctx context.Context, input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	var output *ec2.DescribeSubnetsOutput
	var err error
	msg := makeAWSLogMessage("DescribeSubnets", fmt.Sprintf("%T", c), input)
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.DescribeSubnetsWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// DescribeVpcs is a wrapper for ec2.DescribeVpcs.
func (c *awsClientImpl) DescribeVpcs(ctx context.Context, input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	var output *ec2.DescribeVpcsOutput
	var err error
	msg := makeAWSLogMessage("DescribeVpcs", fmt.Sprintf("%T", c), input)
	_, err = util.Retry(
		func() (bool, error) {
			output, err = c.EC2.DescribeVpcsWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod)
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (c *awsClientImpl) GetInstanceInfo(ctx context.Context, id string) (*ec2.Instance, error) {
	if strings.HasPrefix(id, "sir") {
		return nil, errors.Errorf("id appears to be a spot instance request ID, not a host ID (%s)", id)
	}
	resp, err := c.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
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

	*ec2.DescribeSpotInstanceRequestsOutput
	*ec2.DescribeInstancesOutput
}

// Create a new mock client.
func (c *awsClientMock) Create(creds *credentials.Credentials) error {
	c.Credentials = creds
	return nil
}

func (c *awsClientMock) Close() {}

// RunInstances is a mock for ec2.RunInstances.
func (c *awsClientMock) RunInstances(ctx context.Context, input *ec2.RunInstancesInput) (*ec2.Reservation, error) {
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
func (c *awsClientMock) DescribeInstances(ctx context.Context, input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	c.DescribeInstancesInput = input
	if c.DescribeInstancesOutput != nil {
		return c.DescribeInstancesOutput, nil
	}
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
func (c *awsClientMock) CreateTags(ctx context.Context, input *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
	c.CreateTagsInput = input
	return nil, nil
}

// TerminateInstances is a mock for ec2.TerminateInstances.
func (c *awsClientMock) TerminateInstances(ctx context.Context, input *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
	c.TerminateInstancesInput = input
	return &ec2.TerminateInstancesOutput{}, nil
}

// RequestSpotInstances is a mock for ec2.RequestSpotInstances.
func (c *awsClientMock) RequestSpotInstances(ctx context.Context, input *ec2.RequestSpotInstancesInput) (*ec2.RequestSpotInstancesOutput, error) {
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
func (c *awsClientMock) DescribeSpotInstanceRequests(ctx context.Context, input *ec2.DescribeSpotInstanceRequestsInput) (*ec2.DescribeSpotInstanceRequestsOutput, error) {
	c.DescribeSpotInstanceRequestsInput = input
	if c.DescribeSpotInstanceRequestsOutput != nil {
		return c.DescribeSpotInstanceRequestsOutput, nil
	}
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

func (c *awsClientMock) DescribeSpotRequestsAndSave(ctx context.Context, hosts []*host.Host) (*ec2.DescribeSpotInstanceRequestsOutput, error) {
	spotRequestIds := []*string{}
	for idx := range hosts {
		h := hosts[idx]
		spotRequestIds = append(spotRequestIds, makeStringPtr(h.Id))
	}
	apiInput := &ec2.DescribeSpotInstanceRequestsInput{
		SpotInstanceRequestIds: spotRequestIds,
	}

	instances, err := c.DescribeSpotInstanceRequests(ctx, apiInput)
	if err != nil {
		return nil, err
	}

	for idx := range hosts {
		h := hosts[idx]
		for idx := range instances.SpotInstanceRequests {
			instance := instances.SpotInstanceRequests[idx]
			if instance.SpotInstanceRequestId != nil && *instance.SpotInstanceRequestId == h.Id {
				if instance.InstanceId != nil {
					h.ExternalIdentifier = *instance.InstanceId
				}
			}
		}
	}

	return instances, nil
}

func (c *awsClientMock) GetSpotInstanceId(ctx context.Context, h *host.Host) (string, error) {
	if h.ExternalIdentifier != "" {
		return h.ExternalIdentifier, nil
	}

	_, err := c.DescribeSpotRequestsAndSave(ctx, []*host.Host{h})
	if err != nil {
		return "", err
	}

	return h.ExternalIdentifier, nil
}

// CancelSpotInstanceRequests is a mock for ec2.CancelSpotInstanceRequests.
func (c *awsClientMock) CancelSpotInstanceRequests(ctx context.Context, input *ec2.CancelSpotInstanceRequestsInput) (*ec2.CancelSpotInstanceRequestsOutput, error) {
	c.CancelSpotInstanceRequestsInput = input
	return nil, nil
}

// DescribeVolumes is a mock for ec2.DescribeVolumes.
func (c *awsClientMock) DescribeVolumes(ctx context.Context, input *ec2.DescribeVolumesInput) (*ec2.DescribeVolumesOutput, error) {
	c.DescribeVolumesInput = input
	return &ec2.DescribeVolumesOutput{}, nil
}

// DescribeSpotPriceHistory is a mock for ec2.DescribeSpotPriceHistory.
func (c *awsClientMock) DescribeSpotPriceHistory(ctx context.Context, input *ec2.DescribeSpotPriceHistoryInput) (*ec2.DescribeSpotPriceHistoryOutput, error) {
	c.DescribeSpotPriceHistoryInput = input
	return &ec2.DescribeSpotPriceHistoryOutput{
		SpotPriceHistory: []*ec2.SpotPrice{
			&ec2.SpotPrice{
				SpotPrice:        makeStringPtr("1.0"),
				AvailabilityZone: makeStringPtr("us-east-1a"),
			},
		},
	}, nil
}

// DescribeSubnets is a mock for ec2.DescribeSubnets.
func (c *awsClientMock) DescribeSubnets(ctx context.Context, input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	c.DescribeSubnetsInput = input
	return &ec2.DescribeSubnetsOutput{
		Subnets: []*ec2.Subnet{
			&ec2.Subnet{
				SubnetId: makeStringPtr("subnet-654321"),
			},
		},
	}, nil
}

// DescribeVpcs is a mock for ec2.DescribeVpcs.
func (c *awsClientMock) DescribeVpcs(ctx context.Context, input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	c.DescribeVpcsInput = input
	return &ec2.DescribeVpcsOutput{
		Vpcs: []*ec2.Vpc{
			&ec2.Vpc{VpcId: makeStringPtr("vpc-123456")},
		},
	}, nil
}

func (c *awsClientMock) GetInstanceInfo(ctx context.Context, id string) (*ec2.Instance, error) {
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

func makeAWSLogMessage(name, client string, args interface{}) message.Fields {
	return message.Fields{
		"message":  "AWS API call",
		"api_name": name,
		"client":   client,
		"args":     args,
	}
}
