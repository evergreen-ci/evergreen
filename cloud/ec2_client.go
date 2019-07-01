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
	"github.com/aws/aws-sdk-go/service/pricing"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const defaultRegion = "us-east-1"
const MockIPV6 = "abcd:1234:459c:2d00:cfe4:843b:1d60:8e47"

// AWSClient is a wrapper for aws-sdk-go so we can use a mock in testing.
type AWSClient interface {
	// Create a new aws-sdk-client or mock if one does not exist, otherwise no-op.
	Create(*credentials.Credentials, string) error

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

	// DescribeVolumes is a wrapper for ec2.DescribeVolumes.
	DescribeVolumes(context.Context, *ec2.DescribeVolumesInput) (*ec2.DescribeVolumesOutput, error)

	// DescribeSpotPriceHistory is a wrapper for ec2.DescribeSpotPriceHistory.
	DescribeSpotPriceHistory(context.Context, *ec2.DescribeSpotPriceHistoryInput) (*ec2.DescribeSpotPriceHistoryOutput, error)

	// GetInstanceInfo returns info about an ec2 instance.
	GetInstanceInfo(context.Context, string) (*ec2.Instance, error)

	// CreateKeyPair is a wrapper for ec2.CreateKeyPairWithContext.
	CreateKeyPair(context.Context, *ec2.CreateKeyPairInput) (*ec2.CreateKeyPairOutput, error)

	// DeleteKeyPair is a wrapper for ec2.DeleteKeyPairWithContext.
	DeleteKeyPair(context.Context, *ec2.DeleteKeyPairInput) (*ec2.DeleteKeyPairOutput, error)

	// GetProducts is a wrapper for pricing.GetProducts.
	GetProducts(context.Context, *pricing.GetProductsInput) (*pricing.GetProductsOutput, error)

	// CreateLaunchTemplate is a wrapper for ec2.CreateLaunchTemplateWithContext
	CreateLaunchTemplate(context.Context, *ec2.CreateLaunchTemplateInput) (*ec2.CreateLaunchTemplateOutput, error)

	// DeleteLaunchTemplate is a wrapper for ec2.DeleteLaunchTemplateWithContext
	DeleteLaunchTemplate(context.Context, *ec2.DeleteLaunchTemplateInput) (*ec2.DeleteLaunchTemplateOutput, error)

	// CreateFleet is a wrapper for ec2.CreateFleetWithContext
	CreateFleet(context.Context, *ec2.CreateFleetInput) (*ec2.CreateFleetOutput, error)
}

// awsClientImpl wraps ec2.EC2.
type awsClientImpl struct { //nolint
	session    *session.Session
	httpClient *http.Client
	pricing    *pricing.Pricing
	*ec2.EC2
}

const (
	awsClientImplRetries     = 10
	awsClientImplStartPeriod = time.Second
)

// Create a new aws-sdk-client if one does not exist, otherwise no-op.
func (c *awsClientImpl) Create(creds *credentials.Credentials, region string) error {
	if creds == nil {
		return errors.New("creds must not be nil")
	}
	if region == "" {
		return errors.New("region must not be empty")
	}
	if c.session == nil {
		c.httpClient = util.GetHTTPClient()
		s, err := session.NewSession(&aws.Config{
			HTTPClient:  c.httpClient,
			Region:      aws.String(region),
			Credentials: creds,
		})
		if err != nil {
			return errors.Wrap(err, "error creating session")
		}
		c.session = s
	}
	c.EC2 = ec2.New(c.session)
	c.pricing = pricing.New(c.session)
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
	err = util.Retry(
		ctx,
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
		}, awsClientImplRetries, awsClientImplStartPeriod, 0)
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
	err = util.Retry(
		ctx,
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
		}, awsClientImplRetries, awsClientImplStartPeriod, 0)
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
	err = util.Retry(
		ctx,
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
		}, awsClientImplRetries, awsClientImplStartPeriod, 0)
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
	err = util.Retry(
		ctx,
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
		}, awsClientImplRetries, awsClientImplStartPeriod, 0)
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
	err = util.Retry(
		ctx,
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
		}, awsClientImplRetries, awsClientImplStartPeriod, 0)
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
	err = util.Retry(
		ctx,
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
		}, awsClientImplRetries, awsClientImplStartPeriod, 0)
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
		InstanceIds: []*string{aws.String(id)},
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

// CreateKeyPair is a wrapper for ec2.CreateKeyPair.
func (c *awsClientImpl) CreateKeyPair(ctx context.Context, input *ec2.CreateKeyPairInput) (*ec2.CreateKeyPairOutput, error) {
	var output *ec2.CreateKeyPairOutput
	var err error
	msg := makeAWSLogMessage("CreateKeyPair", fmt.Sprintf("%T", c), input)
	err = util.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.CreateKeyPairWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod, 0)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// DeleteKeyPair is a wrapper for ec2.DeleteKeyPair.
func (c *awsClientImpl) DeleteKeyPair(ctx context.Context, input *ec2.DeleteKeyPairInput) (*ec2.DeleteKeyPairOutput, error) {
	var output *ec2.DeleteKeyPairOutput
	var err error
	msg := makeAWSLogMessage("DeleteKeyPair", fmt.Sprintf("%T", c), input)
	err = util.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.DeleteKeyPairWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod, 0)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// GetProducts is a wrapper for pricing.GetProducts.
func (c *awsClientImpl) GetProducts(ctx context.Context, input *pricing.GetProductsInput) (*pricing.GetProductsOutput, error) {
	var output *pricing.GetProductsOutput
	var err error
	msg := makeAWSLogMessage("GetProducts", fmt.Sprintf("%T", c), input)
	err = util.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.pricing.GetProductsWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod, 0)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// CreateLaunchTemplate is a wrapper for ec2.CreateLaunchTemplateWithContext
func (c *awsClientImpl) CreateLaunchTemplate(ctx context.Context, input *ec2.CreateLaunchTemplateInput) (*ec2.CreateLaunchTemplateOutput, error) {
	var output *ec2.CreateLaunchTemplateOutput
	var err error
	msg := makeAWSLogMessage("CreateLaunchTemplate", fmt.Sprintf("%T", c), input)
	err = util.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.CreateLaunchTemplateWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod, 0)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// DeleteLaunchTemplate is a wrapper for ec2.DeleteLaunchTemplateWithContext
func (c *awsClientImpl) DeleteLaunchTemplate(ctx context.Context, input *ec2.DeleteLaunchTemplateInput) (*ec2.DeleteLaunchTemplateOutput, error) {
	var output *ec2.DeleteLaunchTemplateOutput
	var err error
	msg := makeAWSLogMessage("DeleteLaunchTemplate", fmt.Sprintf("%T", c), input)
	err = util.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.DeleteLaunchTemplateWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod, 0)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// CreateFleet is a wrapper for ec2.CreateFleetWithContext
func (c *awsClientImpl) CreateFleet(ctx context.Context, input *ec2.CreateFleetInput) (*ec2.CreateFleetOutput, error) {
	var output *ec2.CreateFleetOutput
	var err error
	msg := makeAWSLogMessage("CreateFleet", fmt.Sprintf("%T", c), input)
	err = util.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.CreateFleetWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod, 0)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// awsClientMock mocks ec2.EC2.
type awsClientMock struct { //nolint
	*credentials.Credentials
	*ec2.RunInstancesInput
	*ec2.DescribeInstancesInput
	*ec2.CreateTagsInput
	*ec2.TerminateInstancesInput
	*ec2.DescribeVolumesInput
	*ec2.DescribeSpotPriceHistoryInput
	*ec2.CreateKeyPairInput
	*ec2.DeleteKeyPairInput
	*pricing.GetProductsInput
	*ec2.CreateLaunchTemplateInput
	*ec2.DeleteLaunchTemplateInput
	*ec2.CreateFleetInput

	*ec2.DescribeInstancesOutput
	*ec2.CreateLaunchTemplateOutput
}

// Create a new mock client.
func (c *awsClientMock) Create(creds *credentials.Credentials, region string) error {
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
				InstanceId: aws.String("instance_id"),
				KeyName:    aws.String("keyName"),
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
	ipv6 := ec2.InstanceIpv6Address{}
	ipv6.SetIpv6Address(MockIPV6)
	return &ec2.DescribeInstancesOutput{
		Reservations: []*ec2.Reservation{
			&ec2.Reservation{
				Instances: []*ec2.Instance{
					&ec2.Instance{
						InstanceId:   aws.String("instance_id"),
						InstanceType: aws.String("instance_type"),
						State: &ec2.InstanceState{
							Name: aws.String(ec2.InstanceStateNameRunning),
						},
						PublicDnsName: aws.String("public_dns_name"),
						NetworkInterfaces: []*ec2.InstanceNetworkInterface{
							&ec2.InstanceNetworkInterface{
								Ipv6Addresses: []*ec2.InstanceIpv6Address{
									&ipv6,
								},
							},
						},
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
				SpotPrice:        aws.String("1.0"),
				AvailabilityZone: aws.String("us-east-1a"),
			},
		},
	}, nil
}

func (c *awsClientMock) GetInstanceInfo(ctx context.Context, id string) (*ec2.Instance, error) {
	instance := &ec2.Instance{}
	instance.Placement = &ec2.Placement{}
	instance.Placement.AvailabilityZone = aws.String("us-east-1a")
	instance.InstanceType = aws.String("m3.4xlarge")
	instance.LaunchTime = aws.Time(time.Now())
	instance.PublicDnsName = aws.String("public_dns_name")
	ipv6 := ec2.InstanceIpv6Address{}
	ipv6.SetIpv6Address(MockIPV6)
	instance.NetworkInterfaces = []*ec2.InstanceNetworkInterface{
		&ec2.InstanceNetworkInterface{
			Ipv6Addresses: []*ec2.InstanceIpv6Address{
				&ipv6,
			},
		},
	}
	instance.State = &ec2.InstanceState{}
	instance.State.Name = aws.String("running")
	return instance, nil
}

// CreateKeyPair is a mock for ec2.CreateKeyPair.
func (c *awsClientMock) CreateKeyPair(ctx context.Context, input *ec2.CreateKeyPairInput) (*ec2.CreateKeyPairOutput, error) {
	c.CreateKeyPairInput = input
	return &ec2.CreateKeyPairOutput{
		KeyName:     aws.String("key_name"),
		KeyMaterial: aws.String("key_material"),
	}, nil
}

// DeleteKeyPair is a mock for ec2.DeleteKeyPair.
func (c *awsClientMock) DeleteKeyPair(ctx context.Context, input *ec2.DeleteKeyPairInput) (*ec2.DeleteKeyPairOutput, error) {
	c.DeleteKeyPairInput = input
	return &ec2.DeleteKeyPairOutput{}, nil
}

// GetProducts is a mock for pricing.GetProducts.
func (c *awsClientMock) GetProducts(ctx context.Context, input *pricing.GetProductsInput) (*pricing.GetProductsOutput, error) {
	c.GetProductsInput = input
	return &pricing.GetProductsOutput{}, nil
}

// CreateLaunchTemplate is a mock for ec2.CreateLaunchTemplateWithContext
func (c *awsClientMock) CreateLaunchTemplate(ctx context.Context, input *ec2.CreateLaunchTemplateInput) (*ec2.CreateLaunchTemplateOutput, error) {
	c.CreateLaunchTemplateInput = input
	if c.CreateLaunchTemplateOutput == nil {
		c.CreateLaunchTemplateOutput = &ec2.CreateLaunchTemplateOutput{
			LaunchTemplate: &ec2.LaunchTemplate{
				LaunchTemplateId:    aws.String("templateID"),
				LatestVersionNumber: aws.Int64(1),
			},
		}
	}

	return c.CreateLaunchTemplateOutput, nil
}

// DeleteLaunchTemplate is a mock for ec2.DeleteLaunchTemplateWithContext
func (c *awsClientMock) DeleteLaunchTemplate(ctx context.Context, input *ec2.DeleteLaunchTemplateInput) (*ec2.DeleteLaunchTemplateOutput, error) {
	c.DeleteLaunchTemplateInput = input
	return &ec2.DeleteLaunchTemplateOutput{}, nil
}

// CreateFleet is a mock for ec2.CreateFleetWithContext
func (c *awsClientMock) CreateFleet(ctx context.Context, input *ec2.CreateFleetInput) (*ec2.CreateFleetOutput, error) {
	c.CreateFleetInput = input
	return &ec2.CreateFleetOutput{
		Instances: []*ec2.CreateFleetInstance{
			{
				InstanceIds: []*string{
					aws.String("i-12345"),
				},
			},
		},
	}, nil
}

func makeAWSLogMessage(name, client string, args interface{}) message.Fields {
	return message.Fields{
		"message":  "AWS API call",
		"api_name": name,
		"client":   client,
		"args":     args,
	}
}
