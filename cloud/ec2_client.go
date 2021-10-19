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
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const MockIPV6 = "abcd:1234:459c:2d00:cfe4:843b:1d60:8e47"
const MockIPV4 = "12.34.56.78"

var noReservationError = errors.New("no reservation returned for instance")

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

	// ModifyInstanceAttribute is a wrapper for ec2.ModifyInstanceAttribute
	ModifyInstanceAttribute(context.Context, *ec2.ModifyInstanceAttributeInput) (*ec2.ModifyInstanceAttributeOutput, error)

	// DescribeInstanceTypeOfferings is a wrapper for ec2.DescribeInstanceTypeOfferings.
	DescribeInstanceTypeOfferings(context.Context, *ec2.DescribeInstanceTypeOfferingsInput) (*ec2.DescribeInstanceTypeOfferingsOutput, error)

	// CreateTags is a wrapper for ec2.CreateTags.
	CreateTags(context.Context, *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error)

	// DeleteTags is a wrapper for ec2.DeleteTags.
	DeleteTags(context.Context, *ec2.DeleteTagsInput) (*ec2.DeleteTagsOutput, error)

	// TerminateInstances is a wrapper for ec2.TerminateInstances.
	TerminateInstances(context.Context, *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error)

	// StopInstances is a wrapper for ec2.StopInstances.
	StopInstances(context.Context, *ec2.StopInstancesInput) (*ec2.StopInstancesOutput, error)

	// StartInstances is a wrapper for ec2.StartInstances.
	StartInstances(context.Context, *ec2.StartInstancesInput) (*ec2.StartInstancesOutput, error)

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

	// CreateVolume is a wrapper for ec2.CreateVolume.
	CreateVolume(context.Context, *ec2.CreateVolumeInput) (*ec2.Volume, error)

	// DeleteVolume is a wrapper for ec2.DeleteWrapper.
	DeleteVolume(context.Context, *ec2.DeleteVolumeInput) (*ec2.DeleteVolumeOutput, error)

	// AttachVolume is a wrapper for ec2.AttachVolume. Generates device name on error if applicable.
	AttachVolume(context.Context, *ec2.AttachVolumeInput, generateDeviceNameOptions) (*ec2.VolumeAttachment, error)

	// DetachVolume is a wrapper for ec2.DetachVolume.
	DetachVolume(context.Context, *ec2.DetachVolumeInput) (*ec2.VolumeAttachment, error)

	// ModifyVolume is a wrapper for ec2.ModifyVolume.
	ModifyVolume(context.Context, *ec2.ModifyVolumeInput) (*ec2.ModifyVolumeOutput, error)

	// DescribeVolumes is a wrapper for ec2.DescribeVolumes.
	DescribeVolumes(context.Context, *ec2.DescribeVolumesInput) (*ec2.DescribeVolumesOutput, error)

	// DescribeSpotPriceHistory is a wrapper for ec2.DescribeSpotPriceHistory.
	DescribeSpotPriceHistory(context.Context, *ec2.DescribeSpotPriceHistoryInput) (*ec2.DescribeSpotPriceHistoryOutput, error)

	// DescribeSubnets is a wrapper for ec2.DescribeSubnets.
	DescribeSubnets(context.Context, *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error)

	// DescribeVpcs is a wrapper for ec2.DescribeVpcs.
	DescribeVpcs(context.Context, *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error)

	// GetInstanceInfo returns info about an ec2 instance.
	GetInstanceInfo(context.Context, string) (*ec2.Instance, error)

	// CreateKeyPair is a wrapper for ec2.CreateKeyPairWithContext.
	CreateKeyPair(context.Context, *ec2.CreateKeyPairInput) (*ec2.CreateKeyPairOutput, error)

	// ImportKeyPair is a wrapper for ec2.ImportKeyPairWithContext.
	ImportKeyPair(context.Context, *ec2.ImportKeyPairInput) (*ec2.ImportKeyPairOutput, error)

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

	GetKey(context.Context, *host.Host) (string, error)

	SetTags(context.Context, []string, *host.Host) error

	GetInstanceBlockDevices(context.Context, *host.Host) ([]*ec2.InstanceBlockDeviceMapping, error)

	GetVolumeIDs(context.Context, *host.Host) ([]string, error)

	GetPublicDNSName(ctx context.Context, h *host.Host) (string, error)
}

// awsClientImpl wraps ec2.EC2.
type awsClientImpl struct { //nolint
	session    *session.Session
	httpClient *http.Client
	pricing    *pricing.Pricing
	*ec2.EC2
}

type generateDeviceNameOptions struct {
	isWindows           bool
	existingDeviceNames []string
}

const (
	awsClientImplAttempts    = 9
	awsClientImplStartPeriod = time.Second
)

func awsClientDefaultRetryOptions() utility.RetryOptions {
	return utility.RetryOptions{
		MaxAttempts: awsClientImplAttempts,
		MinDelay:    awsClientImplStartPeriod,
	}
}

// Create a new aws-sdk-client if one does not exist, otherwise no-op.
func (c *awsClientImpl) Create(creds *credentials.Credentials, region string) error {
	if creds == nil {
		return errors.New("creds must not be nil")
	}
	if region == "" {
		return errors.New("region must not be empty")
	}
	if c.session == nil {
		c.httpClient = utility.GetHTTPClient()
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
		utility.PutHTTPClient(c.httpClient)
		c.httpClient = nil
	}
}

// RunInstances is a wrapper for ec2.RunInstances.
func (c *awsClientImpl) RunInstances(ctx context.Context, input *ec2.RunInstancesInput) (*ec2.Reservation, error) {
	var output *ec2.Reservation
	var err error
	msg := makeAWSLogMessage("RunInstances", fmt.Sprintf("%T", c), input)
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.RunInstancesWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					if strings.Contains(ec2err.Code(), EC2InsufficientCapacity) {
						return false, EC2InsufficientCapacityError
					}
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.DescribeInstancesWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (c *awsClientImpl) ModifyInstanceAttribute(ctx context.Context, input *ec2.ModifyInstanceAttributeInput) (*ec2.ModifyInstanceAttributeOutput, error) {
	var output *ec2.ModifyInstanceAttributeOutput
	var err error
	msg := makeAWSLogMessage("ModifyInstanceAttribute", fmt.Sprintf("%T", c), input)
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.ModifyInstanceAttributeWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (c *awsClientImpl) DescribeInstanceTypeOfferings(ctx context.Context, input *ec2.DescribeInstanceTypeOfferingsInput) (*ec2.DescribeInstanceTypeOfferingsOutput, error) {
	var output *ec2.DescribeInstanceTypeOfferingsOutput
	var err error
	msg := makeAWSLogMessage("DescribeInstanceTypeOfferings", fmt.Sprintf("%T", c), input)
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.DescribeInstanceTypeOfferingsWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.CreateTagsWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
	if err != nil {
		return nil, err
	}
	return output, nil
}

// DeleteTags is a wrapper for ec2.DeleteTags.
func (c *awsClientImpl) DeleteTags(ctx context.Context, input *ec2.DeleteTagsInput) (*ec2.DeleteTagsOutput, error) {
	var output *ec2.DeleteTagsOutput
	var err error
	msg := makeAWSLogMessage("DeleteTags", fmt.Sprintf("%T", c), input)
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.DeleteTagsWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
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
	err = utility.Retry(
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

					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
	if err != nil {
		return nil, err
	}
	return output, nil
}

// StopInstances is a wrapper for ec2.StopInstances.
func (c *awsClientImpl) StopInstances(ctx context.Context, input *ec2.StopInstancesInput) (*ec2.StopInstancesOutput, error) {
	var output *ec2.StopInstancesOutput
	var err error
	msg := makeAWSLogMessage("StopInstances", fmt.Sprintf("%T", c), input)
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.StopInstancesWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
	if err != nil {
		return nil, err
	}
	return output, nil
}

// StartInstances is a wrapper for ec2.StartInstances.
func (c *awsClientImpl) StartInstances(ctx context.Context, input *ec2.StartInstancesInput) (*ec2.StartInstancesOutput, error) {
	var output *ec2.StartInstancesOutput
	var err error
	msg := makeAWSLogMessage("StartInstances", fmt.Sprintf("%T", c), input)
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.StartInstancesWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.RequestSpotInstancesWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.DescribeSpotInstanceRequestsWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
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
		spotRequestIds = append(spotRequestIds, aws.String(h.Id))
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.CancelSpotInstanceRequestsWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Error(message.WrapError(ec2err, msg))
					if ec2err.Code() == EC2ErrorSpotRequestNotFound {
						return false, err
					}
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
	if err != nil {
		return nil, err
	}
	return output, nil
}

// CreateVolume is a wrapper for ec2.CreateVolume.
func (c *awsClientImpl) CreateVolume(ctx context.Context, input *ec2.CreateVolumeInput) (*ec2.Volume, error) {
	var output *ec2.Volume
	var err error
	msg := makeAWSLogMessage("CreateVolume", fmt.Sprintf("%T", c), input)
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.CreateVolumeWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					if strings.Contains(ec2err.Error(), EC2InvalidParam) {
						return false, err
					}
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
	if err != nil {
		return nil, err
	}

	return output, nil
}

// DeleteVolume is a wrapper for ec2.DeleteWrapper.
func (c *awsClientImpl) DeleteVolume(ctx context.Context, input *ec2.DeleteVolumeInput) (*ec2.DeleteVolumeOutput, error) {
	var output *ec2.DeleteVolumeOutput
	var err error
	msg := makeAWSLogMessage("DeleteVolume", fmt.Sprintf("%T", c), input)
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.DeleteVolumeWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					if strings.Contains(ec2err.Error(), EC2VolumeNotFound) {
						return false, nil
					}
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
	if err != nil {
		return nil, err
	}

	return output, nil
}

// ModifyVolume is a wrapper for ec2.ModifyWrapper.
func (c *awsClientImpl) ModifyVolume(ctx context.Context, input *ec2.ModifyVolumeInput) (*ec2.ModifyVolumeOutput, error) {
	var output *ec2.ModifyVolumeOutput
	var err error
	msg := makeAWSLogMessage("ModifyVolume", fmt.Sprintf("%T", c), input)
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.ModifyVolumeWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
					if ModifyVolumeBadRequest(ec2err) {
						return false, err
					}
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
	if err != nil {
		return nil, err
	}

	return output, nil
}

// AttachVolume is a wrapper for ec2.AttachVolume.
func (c *awsClientImpl) AttachVolume(ctx context.Context, input *ec2.AttachVolumeInput, opts generateDeviceNameOptions) (*ec2.VolumeAttachment, error) {
	var output *ec2.VolumeAttachment
	var err error
	msg := makeAWSLogMessage("AttachVolume", fmt.Sprintf("%T", c), input)
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.AttachVolumeWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
					if AttachVolumeBadRequest(ec2err) {
						return false, err
					}
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
	if err != nil {
		return nil, err
	}

	return output, nil
}

// DetachVolume is a wrapper for ec2.DetachVolume.
func (c *awsClientImpl) DetachVolume(ctx context.Context, input *ec2.DetachVolumeInput) (*ec2.VolumeAttachment, error) {
	var output *ec2.VolumeAttachment
	var err error
	msg := makeAWSLogMessage("DetachVolume", fmt.Sprintf("%T", c), input)
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.DetachVolumeWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.DescribeVolumesWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.DescribeSpotPriceHistoryWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.DescribeSubnetsWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.DescribeVpcsWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (c *awsClientImpl) GetInstanceInfo(ctx context.Context, id string) (*ec2.Instance, error) {
	if strings.HasPrefix(id, "sir") {
		return nil, errors.Errorf("id appears to be a spot instance request ID, not a host ID (%s)", id)
	}
	if host.IsIntentHostId(id) {
		return nil, errors.Errorf("host ID '%s' is for an intent host", id)
	}
	resp, err := c.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(id)},
	})
	if err != nil {
		return nil, errors.Wrap(err, "EC2 API returned error for DescribeInstances")
	}
	reservation := resp.Reservations
	if len(reservation) == 0 {
		return nil, noReservationError
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.CreateKeyPairWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (c *awsClientImpl) ImportKeyPair(ctx context.Context, input *ec2.ImportKeyPairInput) (*ec2.ImportKeyPairOutput, error) {
	var output *ec2.ImportKeyPairOutput
	var err error
	msg := makeAWSLogMessage("ImportKeyPair", fmt.Sprintf("%T", c), input)
	err = utility.Retry(
		ctx, func() (bool, error) {
			output, err = c.EC2.ImportKeyPairWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					// Don't retry if the key already exists
					if ec2err.Code() == EC2DuplicateKeyPair {
						grip.Info(msg)
						return false, ec2err
					}
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.DeleteKeyPairWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.pricing.GetProductsWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.CreateLaunchTemplateWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.DeleteLaunchTemplateWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			output, err = c.EC2.CreateFleetWithContext(ctx, input)
			if err != nil {
				if ec2err, ok := err.(awserr.Error); ok {
					if strings.Contains(ec2err.Code(), EC2InsufficientCapacity) {
						return false, err
					}
					grip.Debug(message.WrapError(ec2err, msg))
				}
				return true, err
			}
			// CreateFleetWithContext has oddball behavior. If there is a
			// RequestLimitExceeded error while using CreateFleet in instant mode,
			// usually (but not always!) EC2.CreateFleetWithContext will not return an
			// error. Instead it will populate the output.Error array with a single
			// item describing the error, using an error type that does _not_
			// implement awserr.Error. We therefore have to check this case in addition
			// to the standard `err != nil` case above.
			if !ec2CreateFleetResponseContainsInstance(output) {
				if len(output.Errors) > 0 {
					grip.Debug(message.WrapError(errors.New(output.Errors[0].String()), msg))
					return true, errors.Errorf("Got error in CreateFleet response: %s", output.Errors[0].String())
				}
				grip.Error(message.WrapError(errors.New("No instance ID and no error in CreateFleet response"), msg))
				// This condition is unexpected, so do not retry.
				return false, errors.New("No instance ID no error in create fleet response")
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (c *awsClientImpl) GetKey(ctx context.Context, h *host.Host) (string, error) {
	t, err := task.FindOneId(h.StartedBy)
	if err != nil {
		return "", errors.Wrapf(err, "problem finding task %s", h.StartedBy)
	}
	if t == nil {
		return "", errors.Errorf("no task found %s", h.StartedBy)
	}
	k, err := model.GetAWSKeyForProject(t.Project)
	if err != nil {
		return "", errors.Wrap(err, "problem getting key for project")
	}
	if k.Name != "" {
		return k.Name, nil
	}

	newKey, err := c.makeNewKey(ctx, t.Project, h)
	if err != nil {
		return "", errors.Wrap(err, "problem creating new key")
	}
	return newKey, nil
}

func (c *awsClientImpl) makeNewKey(ctx context.Context, project string, h *host.Host) (string, error) {
	name := "evg_auto_" + project
	_, err := c.DeleteKeyPair(ctx, &ec2.DeleteKeyPairInput{KeyName: aws.String(name)})
	if err != nil { // error does not indicate a problem, but log anyway for debugging
		grip.Debug(message.WrapError(err, message.Fields{
			"message":  "problem deleting key",
			"key_name": name,
		}))
	}
	resp, err := c.CreateKeyPair(ctx, &ec2.CreateKeyPairInput{KeyName: aws.String(name)})
	if err != nil {
		return "", errors.Wrap(err, "problem creating key pair")
	}

	if err := model.SetAWSKeyForProject(project, &model.AWSSSHKey{Name: name, Value: *resp.KeyMaterial}); err != nil {
		return "", errors.Wrap(err, "problem setting key")
	}

	return name, nil
}

// SetTags creates the initial tags for an EC2 host and updates the database with the
// host's Evergreen-generated tags.
func (c *awsClientImpl) SetTags(ctx context.Context, resources []string, h *host.Host) error {
	tags := hostToEC2Tags(makeTags(h))
	if _, err := c.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: aws.StringSlice(resources),
		Tags:      tags,
	}); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error attaching tags",
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return errors.Wrapf(err, "failed to attach tags for %s", h.Id)
	}

	// Push instance tag changes to database
	if err := h.SetTags(); err != nil {
		return errors.Wrap(err, "failed to update instance tags in database")
	}

	grip.Debug(message.Fields{
		"message":       "attached tags for host",
		"host_id":       h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
		"tags":          h.InstanceTags,
	})

	return nil
}

func (c *awsClientImpl) GetInstanceBlockDevices(ctx context.Context, h *host.Host) ([]*ec2.InstanceBlockDeviceMapping, error) {
	id, err := c.getHostInstanceID(ctx, h)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get instance ID for '%s'", h.Id)
	}

	instance, err := c.GetInstanceInfo(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "error getting instance info")
	}

	return instance.BlockDeviceMappings, nil
}

func (c *awsClientImpl) GetVolumeIDs(ctx context.Context, h *host.Host) ([]string, error) {
	if h.Volumes == nil {
		devices, err := c.GetInstanceBlockDevices(ctx, h)
		if err != nil {
			return nil, errors.Wrap(err, "error getting devices")
		}
		if err := h.SetVolumes(makeVolumeAttachments(devices)); err != nil {
			return nil, errors.Wrap(err, "error saving host volumes")
		}
	}

	// Get string slice of volume IDs
	volumeIDs := []string{}
	for _, attachment := range h.Volumes {
		volumeIDs = append(volumeIDs, attachment.VolumeID)
	}

	return volumeIDs, nil
}

func (c *awsClientImpl) GetPublicDNSName(ctx context.Context, h *host.Host) (string, error) {
	if h.Host != "" {
		return h.Host, nil
	}

	id, err := c.getHostInstanceID(ctx, h)
	if err != nil {
		return "", errors.Wrapf(err, "can't get instance ID for '%s'", h.Id)
	}

	instance, err := c.GetInstanceInfo(ctx, id)
	if err != nil {
		return "", errors.Wrap(err, "error getting instance info")
	}

	return *instance.PublicDnsName, nil
}

func (c *awsClientImpl) getHostInstanceID(ctx context.Context, h *host.Host) (string, error) {
	id := h.Id
	if isHostSpot(h) {
		var err error
		id, err = c.GetSpotInstanceId(ctx, h)
		if err != nil {
			return "", errors.Wrapf(err, "failed to get spot request info for %s", h.Id)
		}
		if id == "" {
			return "", errors.WithStack(errors.New("spot instance does not yet have an instanceId"))
		}
	}

	return id, nil
}

// awsClientMock mocks ec2.EC2.
type awsClientMock struct { //nolint
	*credentials.Credentials
	*ec2.RunInstancesInput
	*ec2.DescribeInstancesInput
	*ec2.DescribeInstanceTypeOfferingsInput
	*ec2.CreateTagsInput
	*ec2.DeleteTagsInput
	*ec2.ModifyInstanceAttributeInput
	*ec2.TerminateInstancesInput
	*ec2.StopInstancesInput
	*ec2.StartInstancesInput
	*ec2.RequestSpotInstancesInput
	*ec2.DescribeSpotInstanceRequestsInput
	*ec2.CancelSpotInstanceRequestsInput
	*ec2.CreateVolumeInput
	*ec2.DeleteVolumeInput
	*ec2.AttachVolumeInput
	*ec2.DetachVolumeInput
	*ec2.ModifyVolumeInput
	*ec2.DescribeVolumesInput
	*ec2.DescribeSpotPriceHistoryInput
	*ec2.DescribeSubnetsInput
	*ec2.DescribeVpcsInput
	*ec2.CreateKeyPairInput
	*ec2.ImportKeyPairInput
	*ec2.DeleteKeyPairInput
	*pricing.GetProductsInput
	*ec2.CreateLaunchTemplateInput
	*ec2.DeleteLaunchTemplateInput
	*ec2.CreateFleetInput

	*ec2.Instance
	*ec2.DescribeSpotInstanceRequestsOutput
	*ec2.DescribeInstancesOutput
	*ec2.CreateLaunchTemplateOutput
	*ec2.DescribeInstanceTypeOfferingsOutput
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
						InstanceId:   input.InstanceIds[0],
						InstanceType: aws.String("instance_type"),
						State: &ec2.InstanceState{
							Name: aws.String(ec2.InstanceStateNameRunning),
						},
						PublicDnsName:    aws.String("public_dns_name"),
						PrivateIpAddress: aws.String(MockIPV4),
						NetworkInterfaces: []*ec2.InstanceNetworkInterface{
							&ec2.InstanceNetworkInterface{
								Ipv6Addresses: []*ec2.InstanceIpv6Address{
									&ipv6,
								},
							},
						},
						Placement: &ec2.Placement{
							AvailabilityZone: aws.String("us-east-1a"),
						},
						LaunchTime: aws.Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)),
						BlockDeviceMappings: []*ec2.InstanceBlockDeviceMapping{
							&ec2.InstanceBlockDeviceMapping{
								Ebs: &ec2.EbsInstanceBlockDevice{
									VolumeId: aws.String("volume_id"),
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

// DeleteTags is a mock for ec2.DeleteTags.
func (c *awsClientMock) DeleteTags(ctx context.Context, input *ec2.DeleteTagsInput) (*ec2.DeleteTagsOutput, error) {
	c.DeleteTagsInput = input
	return nil, nil
}

func (c *awsClientMock) ModifyInstanceAttribute(ctx context.Context, input *ec2.ModifyInstanceAttributeInput) (*ec2.ModifyInstanceAttributeOutput, error) {
	c.ModifyInstanceAttributeInput = input
	return nil, nil
}

func (c *awsClientMock) DescribeInstanceTypeOfferings(ctx context.Context, input *ec2.DescribeInstanceTypeOfferingsInput) (*ec2.DescribeInstanceTypeOfferingsOutput, error) {
	c.DescribeInstanceTypeOfferingsInput = input
	return c.DescribeInstanceTypeOfferingsOutput, nil
}

// TerminateInstances is a mock for ec2.TerminateInstances.
func (c *awsClientMock) TerminateInstances(ctx context.Context, input *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
	c.TerminateInstancesInput = input
	return &ec2.TerminateInstancesOutput{}, nil
}

// StopInstances is a mock for ec2.StopInstances.
func (c *awsClientMock) StopInstances(ctx context.Context, input *ec2.StopInstancesInput) (*ec2.StopInstancesOutput, error) {
	c.StopInstancesInput = input
	c.Instance = &ec2.Instance{
		InstanceId: aws.String("id"),
		State: &ec2.InstanceState{
			Name: aws.String(ec2.InstanceStateNameStopped),
		},
	}
	return &ec2.StopInstancesOutput{}, nil
}

// StartInstances is a mock for ec2.StartInstances.
func (c *awsClientMock) StartInstances(ctx context.Context, input *ec2.StartInstancesInput) (*ec2.StartInstancesOutput, error) {
	c.StartInstancesInput = input
	c.Instance = &ec2.Instance{
		InstanceId: aws.String("id"),
		State: &ec2.InstanceState{
			Name: aws.String(ec2.InstanceStateNameRunning),
		},
		Placement: &ec2.Placement{
			AvailabilityZone: aws.String("us-east-1a"),
		},
		PublicDnsName:    aws.String("public_dns_name"),
		PrivateIpAddress: aws.String("12.34.56.78"),
		LaunchTime:       aws.Time(time.Now()),
	}
	return &ec2.StartInstancesOutput{}, nil
}

// RequestSpotInstances is a mock for ec2.RequestSpotInstances.
func (c *awsClientMock) RequestSpotInstances(ctx context.Context, input *ec2.RequestSpotInstancesInput) (*ec2.RequestSpotInstancesOutput, error) {
	c.RequestSpotInstancesInput = input
	return &ec2.RequestSpotInstancesOutput{
		SpotInstanceRequests: []*ec2.SpotInstanceRequest{
			&ec2.SpotInstanceRequest{
				InstanceId:            aws.String("instance_id"),
				State:                 aws.String(SpotStatusOpen),
				SpotInstanceRequestId: aws.String("instance_id"),
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
				InstanceId:            aws.String("instance_id"),
				State:                 aws.String(SpotStatusActive),
				SpotInstanceRequestId: aws.String("instance_id"),
			},
		},
	}, nil
}

func (c *awsClientMock) DescribeSpotRequestsAndSave(ctx context.Context, hosts []*host.Host) (*ec2.DescribeSpotInstanceRequestsOutput, error) {
	spotRequestIds := []*string{}
	for idx := range hosts {
		h := hosts[idx]
		spotRequestIds = append(spotRequestIds, aws.String(h.Id))
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

// CreateVolume is a mock for ec2.CreateVolume.
func (c *awsClientMock) CreateVolume(ctx context.Context, input *ec2.CreateVolumeInput) (*ec2.Volume, error) {
	c.CreateVolumeInput = input
	return &ec2.Volume{
		VolumeId:         aws.String("test-volume"),
		VolumeType:       input.VolumeType,
		AvailabilityZone: input.AvailabilityZone,
		Size:             input.Size,
	}, nil
}

// DeleteVolume is a mock for ec2.DeleteVolume.
func (c *awsClientMock) DeleteVolume(ctx context.Context, input *ec2.DeleteVolumeInput) (*ec2.DeleteVolumeOutput, error) {
	c.DeleteVolumeInput = input
	return nil, nil
}

func (c *awsClientMock) ModifyVolume(ctx context.Context, input *ec2.ModifyVolumeInput) (*ec2.ModifyVolumeOutput, error) {
	c.ModifyVolumeInput = input
	return nil, nil
}

// AttachVolume is a mock for ec2.AttachVolume.
func (c *awsClientMock) AttachVolume(ctx context.Context, input *ec2.AttachVolumeInput, opts generateDeviceNameOptions) (*ec2.VolumeAttachment, error) {
	c.AttachVolumeInput = input
	return nil, nil
}

// DetachVolume is a mock for ec2.DetachVolume.
func (c *awsClientMock) DetachVolume(ctx context.Context, input *ec2.DetachVolumeInput) (*ec2.VolumeAttachment, error) {
	c.DetachVolumeInput = input
	return nil, nil
}

// DescribeVolumes is a mock for ec2.DescribeVolumes.
func (c *awsClientMock) DescribeVolumes(ctx context.Context, input *ec2.DescribeVolumesInput) (*ec2.DescribeVolumesOutput, error) {
	c.DescribeVolumesInput = input
	return &ec2.DescribeVolumesOutput{
		Volumes: []*ec2.Volume{
			&ec2.Volume{
				VolumeId: input.VolumeIds[0],
				Size:     aws.Int64(10),
			},
		},
	}, nil
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

// DescribeSubnets is a mock for ec2.DescribeSubnets.
func (c *awsClientMock) DescribeSubnets(ctx context.Context, input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	c.DescribeSubnetsInput = input
	return &ec2.DescribeSubnetsOutput{
		Subnets: []*ec2.Subnet{
			&ec2.Subnet{
				SubnetId: aws.String("subnet-654321"),
				Tags: []*ec2.Tag{
					&ec2.Tag{Key: aws.String("Name"), Value: aws.String("mysubnet_us-east-1a")},
				},
				AvailabilityZone: aws.String("us-east-1a"),
			},
		},
	}, nil
}

// DescribeVpcs is a mock for ec2.DescribeVpcs.
func (c *awsClientMock) DescribeVpcs(ctx context.Context, input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	c.DescribeVpcsInput = input
	return &ec2.DescribeVpcsOutput{
		Vpcs: []*ec2.Vpc{
			&ec2.Vpc{VpcId: aws.String("vpc-123456")},
		},
	}, nil
}

func (c *awsClientMock) GetInstanceInfo(ctx context.Context, id string) (*ec2.Instance, error) {
	if c.Instance != nil {
		return c.Instance, nil
	}

	instance := &ec2.Instance{}
	instance.Placement = &ec2.Placement{}
	instance.Placement.AvailabilityZone = aws.String("us-east-1a")
	instance.InstanceType = aws.String("m3.4xlarge")
	instance.LaunchTime = aws.Time(time.Now())
	instance.PublicDnsName = aws.String("public_dns_name")
	instance.PrivateIpAddress = aws.String(MockIPV4)
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
	instance.BlockDeviceMappings = []*ec2.InstanceBlockDeviceMapping{
		&ec2.InstanceBlockDeviceMapping{
			Ebs: &ec2.EbsInstanceBlockDevice{
				VolumeId: aws.String("volume_id"),
			},
			DeviceName: aws.String("device_name"),
		},
	}
	instance.LaunchTime = aws.Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
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

func (c *awsClientMock) ImportKeyPair(ctx context.Context, input *ec2.ImportKeyPairInput) (*ec2.ImportKeyPairOutput, error) {
	c.ImportKeyPairInput = input
	return &ec2.ImportKeyPairOutput{KeyName: input.KeyName}, nil
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

func (c *awsClientMock) GetKey(ctx context.Context, h *host.Host) (string, error) {
	return "evg_auto_evergreen", nil
}

func (c *awsClientMock) SetTags(ctx context.Context, resources []string, h *host.Host) error {
	tagSlice := []*ec2.Tag{}
	if _, err := c.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: aws.StringSlice(resources),
		Tags:      tagSlice,
	}); err != nil {
		return err
	}
	return nil
}

func (c *awsClientMock) GetInstanceBlockDevices(ctx context.Context, h *host.Host) ([]*ec2.InstanceBlockDeviceMapping, error) {
	return []*ec2.InstanceBlockDeviceMapping{
		&ec2.InstanceBlockDeviceMapping{
			DeviceName: aws.String("device_name"),
			Ebs: &ec2.EbsInstanceBlockDevice{
				VolumeId: aws.String("volume_id"),
			},
		},
	}, nil
}

func (c *awsClientMock) GetVolumeIDs(ctx context.Context, h *host.Host) ([]string, error) {
	if h.Volumes == nil {
		devices, err := c.GetInstanceBlockDevices(ctx, h)
		if err != nil {
			return nil, errors.Wrap(err, "error getting devices")
		}
		if err := h.SetVolumes(makeVolumeAttachments(devices)); err != nil {
			return nil, errors.Wrap(err, "error setting host volumes")
		}
	}

	return []string{"volume_id"}, nil
}

func (c *awsClientMock) GetPublicDNSName(ctx context.Context, h *host.Host) (string, error) {
	if h.Host != "" {
		return h.Host, nil
	}

	return "public_dns_name", nil
}

func makeAWSLogMessage(name, client string, args interface{}) message.Fields {
	return message.Fields{
		"message":  "AWS API call",
		"api_name": name,
		"client":   client,
		"args":     args,
	}
}
