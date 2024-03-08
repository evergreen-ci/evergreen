package cloud

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/smithy-go"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
)

const MockIPV6 = "abcd:1234:459c:2d00:cfe4:843b:1d60:8e47"
const MockIPV4 = "12.34.56.78"

var noReservationError = errors.New("no reservation returned for instance")

// AWSClient is a wrapper for aws-sdk-go so we can use a mock in testing.
type AWSClient interface {
	// Create a new aws-sdk-client or mock if one does not exist, otherwise no-op.
	Create(context.Context, string) error

	// Close an aws-sdk-client or mock.
	Close()

	// RunInstances is a wrapper for ec2.RunInstances.
	RunInstances(context.Context, *ec2.RunInstancesInput) (*ec2.RunInstancesOutput, error)

	// DescribeInstances is a wrapper for ec2.DescribeInstances.
	DescribeInstances(context.Context, *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error)

	// ModifyInstanceAttribute is a wrapper for ec2.ModifyInstanceAttribute.
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

	// CreateVolume is a wrapper for ec2.CreateVolume.
	CreateVolume(context.Context, *ec2.CreateVolumeInput) (*ec2.CreateVolumeOutput, error)

	// DeleteVolume is a wrapper for ec2.DeleteWrapper.
	DeleteVolume(context.Context, *ec2.DeleteVolumeInput) (*ec2.DeleteVolumeOutput, error)

	// AttachVolume is a wrapper for ec2.AttachVolume. Generates device name on error if applicable.
	AttachVolume(context.Context, *ec2.AttachVolumeInput, generateDeviceNameOptions) (*ec2.AttachVolumeOutput, error)

	// DetachVolume is a wrapper for ec2.DetachVolume.
	DetachVolume(context.Context, *ec2.DetachVolumeInput) (*ec2.DetachVolumeOutput, error)

	// ModifyVolume is a wrapper for ec2.ModifyVolume.
	ModifyVolume(context.Context, *ec2.ModifyVolumeInput) (*ec2.ModifyVolumeOutput, error)

	// DescribeVolumes is a wrapper for ec2.DescribeVolumes.
	DescribeVolumes(context.Context, *ec2.DescribeVolumesInput) (*ec2.DescribeVolumesOutput, error)

	// DescribeSubnets is a wrapper for ec2.DescribeSubnets.
	DescribeSubnets(context.Context, *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error)

	// DescribeVpcs is a wrapper for ec2.DescribeVpcs.
	DescribeVpcs(context.Context, *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error)

	// GetInstanceInfo returns info about an ec2 instance.
	GetInstanceInfo(context.Context, string) (*types.Instance, error)

	// CreateKeyPair is a wrapper for ec2.CreateKeyPair.
	CreateKeyPair(context.Context, *ec2.CreateKeyPairInput) (*ec2.CreateKeyPairOutput, error)

	// ImportKeyPair is a wrapper for ec2.ImportKeyPair.
	ImportKeyPair(context.Context, *ec2.ImportKeyPairInput) (*ec2.ImportKeyPairOutput, error)

	// DeleteKeyPair is a wrapper for ec2.DeleteKeyPair.
	DeleteKeyPair(context.Context, *ec2.DeleteKeyPairInput) (*ec2.DeleteKeyPairOutput, error)

	// CreateLaunchTemplate is a wrapper for ec2.CreateLaunchTemplate.
	CreateLaunchTemplate(context.Context, *ec2.CreateLaunchTemplateInput) (*ec2.CreateLaunchTemplateOutput, error)

	// DeleteLaunchTemplate is a wrapper for ec2.DeleteLaunchTemplate.
	DeleteLaunchTemplate(context.Context, *ec2.DeleteLaunchTemplateInput) (*ec2.DeleteLaunchTemplateOutput, error)

	// GetLaunchTemplates gets all the launch templates that match the input.
	GetLaunchTemplates(context.Context, *ec2.DescribeLaunchTemplatesInput) ([]types.LaunchTemplate, error)

	// CreateFleet is a wrapper for ec2.CreateFleet.
	CreateFleet(context.Context, *ec2.CreateFleetInput) (*ec2.CreateFleetOutput, error)

	GetKey(context.Context, *host.Host) (string, error)

	SetTags(context.Context, []string, *host.Host) error

	GetInstanceBlockDevices(context.Context, *host.Host) ([]types.InstanceBlockDeviceMapping, error)

	GetVolumeIDs(context.Context, *host.Host) ([]string, error)

	GetPublicDNSName(ctx context.Context, h *host.Host) (string, error)

	// ChangeResourceRecordSets is a wrapper for route53.ChangeResourceRecordSets.
	ChangeResourceRecordSets(context.Context, *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error)
}

// awsClientImpl wraps ec2.EC2.
type awsClientImpl struct { //nolint:all
	config     *aws.Config
	httpClient *http.Client
	client     *ec2.Client
	r53Client  *route53.Client
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
func (c *awsClientImpl) Create(ctx context.Context, region string) error {
	if region == "" {
		return errors.New("region must not be empty")
	}
	if c.config == nil {
		c.httpClient = utility.GetHTTPClient()
		config, err := config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithHTTPClient(c.httpClient),
		)
		if err != nil {
			return errors.Wrap(err, "loading config")
		}
		otelaws.AppendMiddlewares(&config.APIOptions)

		c.config = &config
	}
	c.client = ec2.NewFromConfig(*c.config)
	c.r53Client = route53.NewFromConfig(*c.config)
	return nil
}

func (c *awsClientImpl) Close() {
	if c.httpClient != nil {
		utility.PutHTTPClient(c.httpClient)
		c.httpClient = nil
	}
}

// RunInstances is a wrapper for ec2.RunInstances.
func (c *awsClientImpl) RunInstances(ctx context.Context, input *ec2.RunInstancesInput) (*ec2.RunInstancesOutput, error) {
	var output *ec2.RunInstancesOutput
	var err error
	input.ClientToken = aws.String(utility.RandomString())

	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("RunInstances", fmt.Sprintf("%T", c), input)
			output, err = c.client.RunInstances(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
					if strings.Contains(apiErr.Error(), EC2InsufficientCapacity) {
						return false, EC2InsufficientCapacityError
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

// DescribeInstances is a wrapper for ec2.DescribeInstances
func (c *awsClientImpl) DescribeInstances(ctx context.Context, input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	var output *ec2.DescribeInstancesOutput
	var err error
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("DescribeInstances", fmt.Sprintf("%T", c), input)
			output, err = c.client.DescribeInstances(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("ModifyInstanceAttribute", fmt.Sprintf("%T", c), input)
			output, err = c.client.ModifyInstanceAttribute(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("DescribeInstanceTypeOfferings", fmt.Sprintf("%T", c), input)
			output, err = c.client.DescribeInstanceTypeOfferings(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("CreateTags", fmt.Sprintf("%T", c), input)
			output, err = c.client.CreateTags(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("DeleteTags", fmt.Sprintf("%T", c), input)
			output, err = c.client.DeleteTags(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("TerminateInstances", fmt.Sprintf("%T", c), input)
			output, err = c.client.TerminateInstances(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					if strings.Contains(apiErr.Error(), EC2ErrorNotFound) {
						grip.Debug(message.WrapError(apiErr, message.Fields{
							"client":          fmt.Sprintf("%T", c),
							"message":         "instance ID not found in AWS",
							"args":            input,
							"ec2_err_message": apiErr.ErrorMessage(),
							"ec2_err_code":    apiErr.ErrorCode(),
						}))
						return false, nil
					}
					grip.Debug(message.WrapError(apiErr, msg))
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("StopInstances", fmt.Sprintf("%T", c), input)
			output, err = c.client.StopInstances(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("StartInstances", fmt.Sprintf("%T", c), input)
			output, err = c.client.StartInstances(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
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
func (c *awsClientImpl) CreateVolume(ctx context.Context, input *ec2.CreateVolumeInput) (*ec2.CreateVolumeOutput, error) {
	var output *ec2.CreateVolumeOutput
	var err error
	input.ClientToken = aws.String(utility.RandomString())
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("CreateVolume", fmt.Sprintf("%T", c), input)
			output, err = c.client.CreateVolume(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
					if strings.Contains(apiErr.Error(), EC2InvalidParam) {
						return false, err
					}
					return true, err
				}
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("DeleteVolume", fmt.Sprintf("%T", c), input)
			output, err = c.client.DeleteVolume(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
					if strings.Contains(apiErr.Error(), EC2VolumeNotFound) {
						return false, nil
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

// ModifyVolume is a wrapper for ec2.ModifyWrapper.
func (c *awsClientImpl) ModifyVolume(ctx context.Context, input *ec2.ModifyVolumeInput) (*ec2.ModifyVolumeOutput, error) {
	var output *ec2.ModifyVolumeOutput
	var err error
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("ModifyVolume", fmt.Sprintf("%T", c), input)
			output, err = c.client.ModifyVolume(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
					if ModifyVolumeBadRequest(apiErr) {
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
func (c *awsClientImpl) AttachVolume(ctx context.Context, input *ec2.AttachVolumeInput, opts generateDeviceNameOptions) (*ec2.AttachVolumeOutput, error) {
	var output *ec2.AttachVolumeOutput
	var err error
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("AttachVolume", fmt.Sprintf("%T", c), input)
			output, err = c.client.AttachVolume(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
					if AttachVolumeBadRequest(apiErr) {
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
func (c *awsClientImpl) DetachVolume(ctx context.Context, input *ec2.DetachVolumeInput) (*ec2.DetachVolumeOutput, error) {
	var output *ec2.DetachVolumeOutput
	var err error
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("DetachVolume", fmt.Sprintf("%T", c), input)
			output, err = c.client.DetachVolume(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("DescribeVolumes", fmt.Sprintf("%T", c), input)
			output, err = c.client.DescribeVolumes(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("DescribeSubnets", fmt.Sprintf("%T", c), input)
			output, err = c.client.DescribeSubnets(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
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
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("DescribeVpcs", fmt.Sprintf("%T", c), input)
			output, err = c.client.DescribeVpcs(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
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

func (c *awsClientImpl) GetInstanceInfo(ctx context.Context, id string) (*types.Instance, error) {
	if host.IsIntentHostId(id) {
		return nil, errors.Errorf("host ID '%s' is for an intent host", id)
	}
	resp, err := c.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{id},
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
		err = errors.Errorf("host '%s' was not found in reservation '%s'",
			id, *resp.Reservations[0].ReservationId)
		return nil, err
	}

	return &instances[0], nil
}

// CreateKeyPair is a wrapper for ec2.CreateKeyPair.
func (c *awsClientImpl) CreateKeyPair(ctx context.Context, input *ec2.CreateKeyPairInput) (*ec2.CreateKeyPairOutput, error) {
	var output *ec2.CreateKeyPairOutput
	var err error
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("CreateKeyPair", fmt.Sprintf("%T", c), input)
			output, err = c.client.CreateKeyPair(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
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
	err = utility.Retry(
		ctx, func() (bool, error) {
			msg := makeAWSLogMessage("ImportKeyPair", fmt.Sprintf("%T", c), input)
			output, err = c.client.ImportKeyPair(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
					// Don't retry if the key already exists
					if apiErr.ErrorCode() == EC2DuplicateKeyPair {
						return false, apiErr
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

// DeleteKeyPair is a wrapper for ec2.DeleteKeyPair.
func (c *awsClientImpl) DeleteKeyPair(ctx context.Context, input *ec2.DeleteKeyPairInput) (*ec2.DeleteKeyPairOutput, error) {
	var output *ec2.DeleteKeyPairOutput
	var err error
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("DeleteKeyPair", fmt.Sprintf("%T", c), input)
			output, err = c.client.DeleteKeyPair(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
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

// CreateLaunchTemplate is a wrapper for ec2.CreateLaunchTemplate.
func (c *awsClientImpl) CreateLaunchTemplate(ctx context.Context, input *ec2.CreateLaunchTemplateInput) (*ec2.CreateLaunchTemplateOutput, error) {
	var output *ec2.CreateLaunchTemplateOutput
	var err error
	input.ClientToken = aws.String(utility.RandomString())
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("CreateLaunchTemplate", fmt.Sprintf("%T", c), input)
			output, err = c.client.CreateLaunchTemplate(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
					// Don't retry if the template was already created.
					if strings.Contains(apiErr.Error(), ec2TemplateNameExists) {
						grip.Info(msg)
						return false, ec2TemplateNameExistsError
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

func (c *awsClientImpl) GetLaunchTemplates(ctx context.Context, input *ec2.DescribeLaunchTemplatesInput) ([]types.LaunchTemplate, error) {
	var templates []types.LaunchTemplate
	err := utility.Retry(
		ctx,
		func() (bool, error) {
			templates = []types.LaunchTemplate{}
			msg := makeAWSLogMessage("DescribeLaunchTemplates", fmt.Sprintf("%T", c), input)
			paginator := ec2.NewDescribeLaunchTemplatesPaginator(c.client, input)
			for paginator.HasMorePages() {
				output, err := paginator.NextPage(ctx)
				if err != nil {
					var apiErr smithy.APIError
					if errors.As(err, &apiErr) {
						grip.Debug(message.WrapError(apiErr, msg))
					}
					return true, err
				}
				templates = append(templates, output.LaunchTemplates...)
			}
			grip.Info(msg)
			return false, nil
		}, awsClientDefaultRetryOptions())
	if err != nil {
		return nil, err
	}
	return templates, nil
}

// DeleteLaunchTemplate is a wrapper for ec2.DeleteLaunchTemplate.
func (c *awsClientImpl) DeleteLaunchTemplate(ctx context.Context, input *ec2.DeleteLaunchTemplateInput) (*ec2.DeleteLaunchTemplateOutput, error) {
	var output *ec2.DeleteLaunchTemplateOutput
	var err error
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("DeleteLaunchTemplate", fmt.Sprintf("%T", c), input)
			output, err = c.client.DeleteLaunchTemplate(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
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

// CreateFleet is a wrapper for ec2.CreateFleet.
func (c *awsClientImpl) CreateFleet(ctx context.Context, input *ec2.CreateFleetInput) (*ec2.CreateFleetOutput, error) {
	var output *ec2.CreateFleetOutput
	var err error
	input.ClientToken = aws.String(utility.RandomString())
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("CreateFleet", fmt.Sprintf("%T", c), input)
			output, err = c.client.CreateFleet(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
					if strings.Contains(apiErr.Error(), EC2InsufficientCapacity) {
						return false, err
					}
				}
				return true, err
			}
			// CreateFleet has oddball behavior. If there is a
			// RequestLimitExceeded error while using CreateFleet in instant mode,
			// usually (but not always!) EC2.CreateFleet will not return an
			// error. Instead it will populate the output.Error array with a single
			// item describing the error, using an error type that does _not_
			// implement smithy.APIError. We therefore have to check this case in addition
			// to the standard `err != nil` case above.
			if !ec2CreateFleetResponseContainsInstance(output) {
				if len(output.Errors) > 0 {
					err = &smithy.GenericAPIError{
						Code:    utility.FromStringPtr(output.Errors[0].ErrorCode),
						Message: utility.FromStringPtr(output.Errors[0].ErrorMessage),
					}
					grip.Debug(message.WrapError(err, msg))
					return true, err
				}
				err := errors.New("CreateFleet response contained neither an instance ID nor error")
				grip.Error(message.WrapError(err, msg))
				// This condition is unexpected, so do not retry.
				return false, err
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
		return "", errors.Wrapf(err, "finding task '%s'", h.StartedBy)
	}
	if t == nil {
		return "", errors.Errorf("task '%s' not found", h.StartedBy)
	}
	k, err := model.GetAWSKeyForProject(t.Project)
	if err != nil {
		return "", errors.Wrap(err, "getting key for project")
	}
	if k.Name != "" {
		return k.Name, nil
	}

	newKey, err := c.makeNewKey(ctx, t.Project, h)
	if err != nil {
		return "", errors.Wrap(err, "creating new key")
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
		return "", errors.Wrap(err, "creating key pair")
	}

	if err := model.SetAWSKeyForProject(project, &model.AWSSSHKey{Name: name, Value: *resp.KeyMaterial}); err != nil {
		return "", errors.Wrap(err, "setting key for project")
	}

	return name, nil
}

// SetTags creates the initial tags for an EC2 host and updates the database with the
// host's Evergreen-generated tags.
func (c *awsClientImpl) SetTags(ctx context.Context, resources []string, h *host.Host) error {
	tags := hostToEC2Tags(makeTags(h))
	if _, err := c.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: resources,
		Tags:      tags,
	}); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error attaching tags",
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return errors.Wrapf(err, "attaching tags for host '%s'", h.Id)
	}

	// Push instance tag changes to database
	if err := h.SetTags(ctx); err != nil {
		return errors.Wrap(err, "updating instance tags in the database")
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

func (c *awsClientImpl) GetInstanceBlockDevices(ctx context.Context, h *host.Host) ([]types.InstanceBlockDeviceMapping, error) {
	instance, err := c.GetInstanceInfo(ctx, h.Id)
	if err != nil {
		return nil, errors.Wrap(err, "getting instance info")
	}

	return instance.BlockDeviceMappings, nil
}

func (c *awsClientImpl) GetVolumeIDs(ctx context.Context, h *host.Host) ([]string, error) {
	if h.Volumes == nil {
		devices, err := c.GetInstanceBlockDevices(ctx, h)
		if err != nil {
			return nil, errors.Wrap(err, "getting devices")
		}
		if err := h.SetVolumes(ctx, makeVolumeAttachments(devices)); err != nil {
			return nil, errors.Wrap(err, "saving host volumes")
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

	instance, err := c.GetInstanceInfo(ctx, h.Id)
	if err != nil {
		return "", errors.Wrap(err, "getting instance info")
	}

	return *instance.PublicDnsName, nil
}

func (c *awsClientImpl) ChangeResourceRecordSets(ctx context.Context, input *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
	var output *route53.ChangeResourceRecordSetsOutput
	var err error

	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("ChangeResourceRecordSets", fmt.Sprintf("%T", c), input)
			output, err = c.r53Client.ChangeResourceRecordSets(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
					if strings.Contains(apiErr.Error(), r53InvalidInput) {
						return false, err
					}
					if strings.Contains(apiErr.Error(), r53InvalidChangeBatch) {
						// This error means the record has already been deleted,
						// so the delete operation was already successful.
						return false, nil
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

// awsClientMock mocks ec2.EC2.
type awsClientMock struct { //nolint
	*ec2.RunInstancesInput
	*ec2.DescribeInstancesInput
	*ec2.DescribeInstanceTypeOfferingsInput
	*ec2.CreateTagsInput
	*ec2.DeleteTagsInput
	*ec2.ModifyInstanceAttributeInput
	*ec2.TerminateInstancesInput
	*ec2.StopInstancesInput
	*ec2.StartInstancesInput
	*ec2.CreateVolumeInput
	*ec2.DeleteVolumeInput
	*ec2.AttachVolumeInput
	*ec2.DetachVolumeInput
	*ec2.ModifyVolumeInput
	*ec2.DescribeVolumesInput
	*ec2.DescribeSubnetsInput
	*ec2.DescribeVpcsInput
	*ec2.CreateKeyPairInput
	*ec2.ImportKeyPairInput
	*ec2.DeleteKeyPairInput
	*ec2.CreateLaunchTemplateInput
	*ec2.DeleteLaunchTemplateInput
	*ec2.CreateFleetInput

	*types.Instance
	*ec2.DescribeInstancesOutput
	RequestGetInstanceInfoError error
	*ec2.DescribeInstanceTypeOfferingsOutput

	launchTemplates []types.LaunchTemplate

	*route53.ChangeResourceRecordSetsInput
	*route53.ChangeResourceRecordSetsOutput
}

// Create a new mock client.
func (c *awsClientMock) Create(ctx context.Context, region string) error {
	return nil
}

func (c *awsClientMock) Close() {}

// RunInstances is a mock for ec2.RunInstances.
func (c *awsClientMock) RunInstances(ctx context.Context, input *ec2.RunInstancesInput) (*ec2.RunInstancesOutput, error) {
	c.RunInstancesInput = input
	return &ec2.RunInstancesOutput{
		Instances: []types.Instance{
			{
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
	ipv6 := types.InstanceIpv6Address{}
	ipv6.Ipv6Address = aws.String(MockIPV6)
	return &ec2.DescribeInstancesOutput{
		Reservations: []types.Reservation{
			{
				Instances: []types.Instance{
					{
						InstanceId:   aws.String(input.InstanceIds[0]),
						InstanceType: "instance_type",
						State: &types.InstanceState{
							Name: types.InstanceStateNameRunning,
						},
						PublicDnsName:    aws.String("public_dns_name"),
						PublicIpAddress:  aws.String("127.0.0.1"),
						PrivateIpAddress: aws.String(MockIPV4),
						NetworkInterfaces: []types.InstanceNetworkInterface{
							{
								Ipv6Addresses: []types.InstanceIpv6Address{
									ipv6,
								},
							},
						},
						Placement: &types.Placement{
							AvailabilityZone: aws.String("us-east-1a"),
						},
						LaunchTime: aws.Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)),
						BlockDeviceMappings: []types.InstanceBlockDeviceMapping{
							{
								Ebs: &types.EbsInstanceBlockDevice{
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
	c.Instance = &types.Instance{
		InstanceId: aws.String("id"),
		State: &types.InstanceState{
			Name: types.InstanceStateNameStopped,
		},
	}
	return &ec2.StopInstancesOutput{}, nil
}

// StartInstances is a mock for ec2.StartInstances.
func (c *awsClientMock) StartInstances(ctx context.Context, input *ec2.StartInstancesInput) (*ec2.StartInstancesOutput, error) {
	c.StartInstancesInput = input
	c.Instance = &types.Instance{
		InstanceId: aws.String("id"),
		State: &types.InstanceState{
			Name: types.InstanceStateNameRunning,
		},
		Placement: &types.Placement{
			AvailabilityZone: aws.String("us-east-1a"),
		},
		PublicDnsName:    aws.String("public_dns_name"),
		PublicIpAddress:  aws.String("127.0.0.1"),
		PrivateIpAddress: aws.String("12.34.56.78"),
		LaunchTime:       aws.Time(time.Now()),
	}
	return &ec2.StartInstancesOutput{}, nil
}

// CreateVolume is a mock for ec2.CreateVolume.
func (c *awsClientMock) CreateVolume(ctx context.Context, input *ec2.CreateVolumeInput) (*ec2.CreateVolumeOutput, error) {
	c.CreateVolumeInput = input
	return &ec2.CreateVolumeOutput{
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
func (c *awsClientMock) AttachVolume(ctx context.Context, input *ec2.AttachVolumeInput, opts generateDeviceNameOptions) (*ec2.AttachVolumeOutput, error) {
	c.AttachVolumeInput = input
	return nil, nil
}

// DetachVolume is a mock for ec2.DetachVolume.
func (c *awsClientMock) DetachVolume(ctx context.Context, input *ec2.DetachVolumeInput) (*ec2.DetachVolumeOutput, error) {
	c.DetachVolumeInput = input
	return nil, nil
}

// DescribeVolumes is a mock for ec2.DescribeVolumes.
func (c *awsClientMock) DescribeVolumes(ctx context.Context, input *ec2.DescribeVolumesInput) (*ec2.DescribeVolumesOutput, error) {
	c.DescribeVolumesInput = input
	return &ec2.DescribeVolumesOutput{
		Volumes: []types.Volume{
			{
				VolumeId: aws.String(input.VolumeIds[0]),
				Size:     aws.Int32(10),
			},
		},
	}, nil
}

// DescribeSubnets is a mock for ec2.DescribeSubnets.
func (c *awsClientMock) DescribeSubnets(ctx context.Context, input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	c.DescribeSubnetsInput = input
	return &ec2.DescribeSubnetsOutput{
		Subnets: []types.Subnet{
			{
				SubnetId: aws.String("subnet-654321"),
				Tags: []types.Tag{
					{Key: aws.String("Name"), Value: aws.String("mysubnet_us-east-1a")},
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
		Vpcs: []types.Vpc{
			{VpcId: aws.String("vpc-123456")},
		},
	}, nil
}

func (c *awsClientMock) GetInstanceInfo(ctx context.Context, id string) (*types.Instance, error) {
	if c.RequestGetInstanceInfoError != nil {
		return nil, c.RequestGetInstanceInfoError
	}

	if c.Instance != nil {
		return c.Instance, nil
	}

	instance := &types.Instance{}
	instance.Placement = &types.Placement{}
	instance.Placement.AvailabilityZone = aws.String("us-east-1a")
	instance.InstanceType = "m3.4xlarge"
	instance.LaunchTime = aws.Time(time.Now())
	instance.PublicDnsName = aws.String("public_dns_name")
	instance.PublicIpAddress = aws.String("127.0.0.1")
	instance.PrivateIpAddress = aws.String(MockIPV4)
	ipv6 := types.InstanceIpv6Address{}
	ipv6.Ipv6Address = aws.String(MockIPV6)
	instance.NetworkInterfaces = []types.InstanceNetworkInterface{
		{
			Ipv6Addresses: []types.InstanceIpv6Address{
				ipv6,
			},
		},
	}
	instance.State = &types.InstanceState{}
	instance.State.Name = "running"
	instance.BlockDeviceMappings = []types.InstanceBlockDeviceMapping{
		{
			Ebs: &types.EbsInstanceBlockDevice{
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

// CreateLaunchTemplate is a mock for ec2.CreateLaunchTemplate
func (c *awsClientMock) CreateLaunchTemplate(ctx context.Context, input *ec2.CreateLaunchTemplateInput) (*ec2.CreateLaunchTemplateOutput, error) {
	c.CreateLaunchTemplateInput = input

	for _, lt := range c.launchTemplates {
		if utility.FromStringPtr(input.LaunchTemplateName) == utility.FromStringPtr(lt.LaunchTemplateName) {
			return nil, ec2TemplateNameExistsError
		}
	}
	c.launchTemplates = append(c.launchTemplates, types.LaunchTemplate{
		LaunchTemplateName: input.LaunchTemplateName,
	})

	return nil, nil
}

// DeleteLaunchTemplate is a mock for ec2.DeleteLaunchTemplate
func (c *awsClientMock) DeleteLaunchTemplate(ctx context.Context, input *ec2.DeleteLaunchTemplateInput) (*ec2.DeleteLaunchTemplateOutput, error) {
	c.DeleteLaunchTemplateInput = input

	index := 0
	for _, template := range c.launchTemplates {
		if utility.FromStringPtr(template.LaunchTemplateId) != utility.FromStringPtr(input.LaunchTemplateId) {
			c.launchTemplates[index] = template
			index++
		}
	}
	c.launchTemplates = c.launchTemplates[:index]

	return &ec2.DeleteLaunchTemplateOutput{}, nil
}

func (c *awsClientMock) GetLaunchTemplates(ctx context.Context, input *ec2.DescribeLaunchTemplatesInput) ([]types.LaunchTemplate, error) {
	return c.launchTemplates, nil
}

// CreateFleet is a mock for ec2.CreateFleet
func (c *awsClientMock) CreateFleet(ctx context.Context, input *ec2.CreateFleetInput) (*ec2.CreateFleetOutput, error) {
	c.CreateFleetInput = input
	return &ec2.CreateFleetOutput{
		Instances: []types.CreateFleetInstance{
			{
				InstanceIds: []string{
					"i-12345",
				},
			},
		},
	}, nil
}

func (c *awsClientMock) GetKey(ctx context.Context, h *host.Host) (string, error) {
	return "evg_auto_evergreen", nil
}

func (c *awsClientMock) SetTags(ctx context.Context, resources []string, h *host.Host) error {
	tagSlice := []types.Tag{}
	if _, err := c.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: resources,
		Tags:      tagSlice,
	}); err != nil {
		return err
	}
	return nil
}

func (c *awsClientMock) GetInstanceBlockDevices(ctx context.Context, h *host.Host) ([]types.InstanceBlockDeviceMapping, error) {
	return []types.InstanceBlockDeviceMapping{
		{
			DeviceName: aws.String("device_name"),
			Ebs: &types.EbsInstanceBlockDevice{
				VolumeId: aws.String("volume_id"),
			},
		},
	}, nil
}

func (c *awsClientMock) GetVolumeIDs(ctx context.Context, h *host.Host) ([]string, error) {
	if h.Volumes == nil {
		devices, err := c.GetInstanceBlockDevices(ctx, h)
		if err != nil {
			return nil, errors.Wrap(err, "getting devices")
		}
		if err := h.SetVolumes(ctx, makeVolumeAttachments(devices)); err != nil {
			return nil, errors.Wrap(err, "setting host volumes")
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

func (c *awsClientMock) ChangeResourceRecordSets(ctx context.Context, input *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
	c.ChangeResourceRecordSetsInput = input
	return c.ChangeResourceRecordSetsOutput, nil
}

func makeAWSLogMessage(name, client string, args interface{}) message.Fields {
	msg := message.Fields{
		"message":  "AWS API call",
		"api_name": name,
		"client":   client,
	}

	argMap := make(map[string]interface{})
	if err := mapstructure.Decode(args, &argMap); err == nil {
		msg["args"] = argMap
	} else {
		msg["args"] = fmt.Sprintf("%+v", args)
	}

	return msg
}
