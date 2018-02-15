package cloud

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func isHostSpot(h *host.Host) bool {
	return h.Distro.Provider == evergreen.ProviderNameEc2Spot
}
func isHostOnDemand(h *host.Host) bool {
	return h.Distro.Provider == evergreen.ProviderNameEc2OnDemand
}

// EC2ProviderSettings describes properties of managed instances.
type EC2ProviderSettings struct {
	// AMI is the AMI ID.
	AMI string `mapstructure:"ami" json:"ami,omitempty" bson:"ami,omitempty"`

	// InstanceType is the EC2 instance type.
	InstanceType string `mapstructure:"instance_type" json:"instance_type,omitempty" bson:"instance_type,omitempty"`

	// KeyName is the AWS SSH key name.
	KeyName string `mapstructure:"key_name" json:"key_name,omitempty" bson:"key_name,omitempty"`

	// MountPoints are the disk mount points for EBS volumes.
	MountPoints []MountPoint `mapstructure:"mount_points" json:"mount_points,omitempty" bson:"mount_points,omitempty"`
	// SecurityGroup is the security group name in EC2 classic and the security group ID in a VPC.
	SecurityGroup string `mapstructure:"security_group" json:"security_group,omitempty" bson:"security_group,omitempty"`

	// SubnetId is only set in a VPC. Either subnet id or vpc name must set.
	SubnetId string `mapstructure:"subnet_id" json:"subnet_id,omitempty" bson:"subnet_id,omitempty"`

	// VpcName is used to get the subnet ID automatically. Either subnet id or vpc name must set.
	VpcName string `mapstructure:"vpc_name" json:"vpc_name,omitempty" bson:"vpc_name,omitempty"`

	// IsVpc is set to true if the security group is part of a VPC.
	IsVpc bool `mapstructure:"is_vpc" json:"is_vpc,omitempty" bson:"is_vpc,omitempty"`

	// BidPrice is the price we are willing to pay for a spot instance.
	BidPrice float64 `mapstructure:"bid_price" json:"bid_price,omitempty" bson:"bid_price,omitempty"`
}

// Validate that essential EC2ProviderSettings fields are not empty.
func (s *EC2ProviderSettings) Validate() error {
	if s.AMI == "" || s.InstanceType == "" || s.SecurityGroup == "" || s.KeyName == "" {
		return errors.New("AMI, instance type, security group, and key name must not be empty")
	}
	if s.BidPrice < 0 {
		return errors.New("Bid price must not be negative")
	}
	if s.IsVpc && s.SubnetId == "" {
		return errors.New("must set a default subnet for a vpc")
	}
	if _, err := makeBlockDeviceMappings(s.MountPoints); err != nil {
		return errors.Wrap(err, "block device mappings invalid")
	}
	return nil
}

type ec2ProviderType int

const (
	onDemandProvider ec2ProviderType = iota
	spotProvider
	autoProvider
)

const (
	SpotStatusOpen     = "open"
	SpotStatusActive   = "active"
	SpotStatusClosed   = "closed"
	SpotStatusCanceled = "cancelled"
	SpotStatusFailed   = "failed"

	EC2ErrorSpotRequestNotFound = "InvalidSpotInstanceRequestID.NotFound"
)

// EC2ManagerOptions are used to construct a new ec2Manager.
type EC2ManagerOptions struct {
	// client is the client library for communicating with AWS.
	client AWSClient

	// provider is the type
	provider ec2ProviderType
}

// ec2Manager starts and configures instances in EC2.
type ec2Manager struct {
	*EC2ManagerOptions
	credentials *credentials.Credentials
}

// NewEC2Manager creates a new manager of EC2 spot and on-demand instances.
func NewEC2Manager(opts *EC2ManagerOptions) CloudManager {
	return &ec2Manager{opts, nil}
}

// GetSettings returns a pointer to the manager's configuration settings struct.
func (m *ec2Manager) GetSettings() ProviderSettings {
	return &EC2ProviderSettings{}
}

// Configure loads credentials or other settings from the config file.
func (m *ec2Manager) Configure(ctx context.Context, settings *evergreen.Settings) error {
	if settings.Providers.AWS.Id == "" || settings.Providers.AWS.Secret == "" {
		return errors.New("AWS ID and Secret must not be blank")
	}

	m.credentials = credentials.NewStaticCredentialsFromCreds(credentials.Value{
		AccessKeyID:     settings.Providers.AWS.Id,
		SecretAccessKey: settings.Providers.AWS.Secret,
	})

	return nil
}

func (m *ec2Manager) spawnOnDemandHost(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings, blockDevices []*ec2.BlockDeviceMapping) ([]*string, error) {
	input := &ec2.RunInstancesInput{
		MinCount:            makeInt64Ptr(1),
		MaxCount:            makeInt64Ptr(1),
		ImageId:             &ec2Settings.AMI,
		KeyName:             &ec2Settings.KeyName,
		InstanceType:        &ec2Settings.InstanceType,
		BlockDeviceMappings: blockDevices,
	}

	if ec2Settings.IsVpc {
		input.NetworkInterfaces = []*ec2.InstanceNetworkInterfaceSpecification{
			&ec2.InstanceNetworkInterfaceSpecification{
				AssociatePublicIpAddress: makeBoolPtr(true),
				DeviceIndex:              makeInt64Ptr(0),
				Groups:                   []*string{&ec2Settings.SecurityGroup},
				SubnetId:                 &ec2Settings.SubnetId,
			},
		}
	} else {
		input.SecurityGroups = []*string{&ec2Settings.SecurityGroup}
	}

	grip.Debug(message.Fields{
		"message":       "starting on-demand instance",
		"args":          input,
		"host":          h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
	})
	reservation, err := m.client.RunInstances(ctx, input)
	if err != nil || reservation == nil {
		msg := "RunInstances API call returned an error"
		grip.Error(message.WrapError(err, message.Fields{
			"message":       msg,
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		grip.Error(message.WrapError(h.Remove(), message.Fields{
			"message":       "error removing intent host",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		if err != nil {
			return nil, errors.Wrap(err, msg)
		}
		msg = "reservation was nil"
		grip.Error(message.Fields{
			"message":       msg,
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		})
		return nil, errors.New(msg)
	}

	if len(reservation.Instances) < 1 {
		return nil, errors.New("reservation has no instances")
	}

	instance := reservation.Instances[0]
	grip.Debug(message.Fields{
		"message":       "started ec2 instance",
		"host":          *instance.InstanceId,
		"distro":        h.Distro.Id,
		"host_provider": h.Distro.Provider,
	})

	resp, err := m.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []*string{instance.InstanceId},
	})
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"host_provider": h.Distro.Provider,
			"host":          h.Id,
			"distro":        h.Distro.Id,
			"message":       "error describing instance",
		}))
		return nil, errors.Wrap(err, "error describing instance")
	}

	grip.Debug(message.Fields{
		"message":       "describe instances returned data",
		"host_provider": h.Distro.Provider,
		"host":          h.Id,
		"distro":        h.Distro.Id,
		"response":      resp,
	})

	if len(resp.Reservations) < 1 {
		return nil, errors.New("describe instances response has no reservations")
	}

	instance = resp.Reservations[0].Instances[0]
	h.Id = *instance.InstanceId
	resources := []*string{instance.InstanceId}
	for _, vol := range instance.BlockDeviceMappings {
		if *vol.DeviceName == "" {
			continue
		}

		resources = append(resources, vol.Ebs.VolumeId)
	}
	return resources, nil
}

func (m *ec2Manager) spawnSpotHost(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings, blockDevices []*ec2.BlockDeviceMapping) ([]*string, error) {
	spotRequest := &ec2.RequestSpotInstancesInput{
		SpotPrice:     makeStringPtr(fmt.Sprintf("%v", ec2Settings.BidPrice)),
		InstanceCount: makeInt64Ptr(1),
		LaunchSpecification: &ec2.RequestSpotLaunchSpecification{
			ImageId:             makeStringPtr(ec2Settings.AMI),
			KeyName:             makeStringPtr(ec2Settings.KeyName),
			InstanceType:        makeStringPtr(ec2Settings.InstanceType),
			BlockDeviceMappings: blockDevices,
		},
	}

	if ec2Settings.IsVpc {
		spotRequest.LaunchSpecification.NetworkInterfaces = []*ec2.InstanceNetworkInterfaceSpecification{
			&ec2.InstanceNetworkInterfaceSpecification{
				AssociatePublicIpAddress: makeBoolPtr(true),
				DeviceIndex:              makeInt64Ptr(0),
				Groups:                   []*string{&ec2Settings.SecurityGroup},
				SubnetId:                 &ec2Settings.SubnetId,
			},
		}
	} else {
		spotRequest.LaunchSpecification.SecurityGroups = []*string{&ec2Settings.SecurityGroup}
	}

	grip.Debug(message.Fields{
		"message":       "starting spot instance",
		"args":          spotRequest,
		"host":          h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
	})
	spotResp, err := m.client.RequestSpotInstances(ctx, spotRequest)
	if err != nil {
		grip.Error(errors.Wrapf(h.Remove(), "error removing intent host %s", h.Id))
		return nil, errors.Wrap(err, "RequestSpotInstances API call returned an error")
	}

	spotReqRes := spotResp.SpotInstanceRequests[0]
	if *spotReqRes.State != SpotStatusOpen && *spotReqRes.State != SpotStatusActive {
		err = errors.Errorf("Spot request %s was found in state %s on intent host %s",
			*spotReqRes.SpotInstanceRequestId, *spotReqRes.State, h.Id)
		return nil, err
	}

	h.Id = *spotReqRes.SpotInstanceRequestId
	resources := []*string{spotReqRes.SpotInstanceRequestId}
	return resources, nil
}

// SpawnHost spawns a new host.
func (m *ec2Manager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameEc2OnDemand &&
		h.Distro.Provider != evergreen.ProviderNameEc2Spot &&
		h.Distro.Provider != evergreen.ProviderNameEc2Auto {
		return nil, errors.Errorf("Can't spawn instance for distro %s: provider is %s",
			h.Distro.Id, h.Distro.Provider)
	}

	ec2Settings := &EC2ProviderSettings{}
	if err := mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings); err != nil {
		return nil, errors.Wrapf(err, "Error decoding params for distro %+v: %+v", h.Distro.Id, ec2Settings)
	}

	if err := ec2Settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid EC2 settings in distro %s: and %+v", h.Distro.Id, ec2Settings)
	}

	blockDevices, err := makeBlockDeviceMappings(ec2Settings.MountPoints)
	if err != nil {
		return nil, errors.Wrap(err, "error making block device mappings")
	}
	h.InstanceType = ec2Settings.InstanceType

	if err = m.client.Create(m.credentials); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	var resources []*string
	provider, err := m.getProvider(ctx, h, ec2Settings)
	if err != nil {
		msg := "error getting provider"
		grip.Error(message.WrapError(err, message.Fields{
			"message":       msg,
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return nil, errors.Wrap(err, msg)
	}
	if provider == onDemandProvider {
		resources, err = m.spawnOnDemandHost(ctx, h, ec2Settings, blockDevices)
		if err != nil {
			msg := "error spawning on-demand host"
			grip.Error(message.WrapError(err, message.Fields{
				"message":       msg,
				"host":          h.Id,
				"host_provider": h.Distro.Provider,
				"distro":        h.Distro.Id,
			}))
			return nil, errors.Wrap(err, msg)
		}
		grip.Debug(message.Fields{
			"message":       "spawned on-demand host",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		})
	} else if provider == spotProvider {
		resources, err = m.spawnSpotHost(ctx, h, ec2Settings, blockDevices)
		if err != nil {
			msg := "error spawning spot host"
			grip.Error(message.WrapError(err, message.Fields{
				"message":       msg,
				"host":          h.Id,
				"host_provider": h.Distro.Provider,
				"distro":        h.Distro.Id,
			}))
			return nil, errors.Wrap(err, msg)
		}
		grip.Debug(message.Fields{
			"message":       "spawned spot host",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		})
	}

	grip.Debug(message.Fields{
		"message":       "attaching tags for host",
		"host":          h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
	})
	tags := makeTags(h)
	tagSlice := []*ec2.Tag{}
	for tag := range tags {
		key := tag
		val := tags[tag]
		tagSlice = append(tagSlice, &ec2.Tag{Key: &key, Value: &val})
	}
	if _, err = m.client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: resources,
		Tags:      tagSlice,
	}); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error attaching tags",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		err = errors.Wrapf(err, "failed to attach tags for %s", h.Id)
		return nil, err
	}

	grip.Debug(message.Fields{
		"message":       "attached tags for host",
		"host":          h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
	})
	event.LogHostStarted(h.Id)

	return h, nil
}

// CanSpawn indicates if a host can be spawned.
func (m *ec2Manager) CanSpawn() (bool, error) {
	return true, nil
}

// GetInstanceStatus returns the current status of an EC2 instance.
func (m *ec2Manager) GetInstanceStatus(ctx context.Context, h *host.Host) (CloudStatus, error) {
	if err := m.client.Create(m.credentials); err != nil {
		return StatusUnknown, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()
	if isHostOnDemand(h) {
		info, err := m.client.GetInstanceInfo(ctx, h.Id)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "error getting instance info",
				"host":          h.Id,
				"host_provider": h.Distro.Provider,
				"distro":        h.Distro.Id,
			}))
			return StatusUnknown, err
		}
		return ec2StatusToEvergreenStatus(*info.State.Name), nil
	} else if isHostSpot(h) {
		return m.getSpotInstanceStatus(ctx, h.Id)
	}
	return StatusUnknown, errors.New("type must be on-demand or spot")
}

// TerminateInstance terminates the EC2 instance.
func (m *ec2Manager) TerminateInstance(ctx context.Context, h *host.Host, user string) error {
	// terminate the instance
	if h.Status == evergreen.HostTerminated {
		err := errors.Errorf("Can not terminate %s - already marked as "+
			"terminated!", h.Id)
		return err
	}

	if err := m.client.Create(m.credentials); err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	instanceId := h.Id
	var err error
	if isHostSpot(h) {
		instanceId, err = m.cancelSpotRequest(ctx, h)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "error canceling spot request",
				"host":          h.Id,
				"host_provider": h.Distro.Provider,
				"user":          user,
				"distro":        h.Distro.Id,
			}))
			return errors.Wrap(err, "error canceling spot request")
		}
		// the spot request wasn't fulfilled, so don't attempt to terminate in ec2
		if instanceId == "" {
			return errors.Wrap(h.Terminate(user), "failed to terminate instance in db")
		}
	}

	if !strings.HasPrefix(instanceId, "i-") {
		return errors.Wrap(h.Terminate(user), "failed to terminate instance in db")
	}
	resp, err := m.client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []*string{makeStringPtr(instanceId)},
	})
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error terminating instance",
			"user":          user,
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return err
	}

	for _, stateChange := range resp.TerminatingInstances {
		grip.Info(message.Fields{
			"message":       "terminated instance",
			"user":          user,
			"host_provider": h.Distro.Provider,
			"instance_id":   *stateChange.InstanceId,
			"host":          h.Id,
			"distro":        h.Distro.Id,
		})
	}

	return errors.Wrap(h.Terminate(user), "failed to terminate instance in db")
}

func (m *ec2Manager) cancelSpotRequest(ctx context.Context, h *host.Host) (string, error) {
	spotDetails, err := m.client.DescribeSpotInstanceRequests(ctx, &ec2.DescribeSpotInstanceRequestsInput{
		SpotInstanceRequestIds: []*string{makeStringPtr(h.Id)},
	})
	if err != nil {
		if ec2err, ok := err.(awserr.Error); ok {
			if ec2err.Code() == EC2ErrorSpotRequestNotFound {
				return "", h.Terminate(evergreen.User)
			}
		}
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error getting spot request info",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return "", errors.Wrapf(err, "failed to get spot request info for %s", h.Id)
	}
	grip.Info(message.Fields{
		"message":       "canceling spot request",
		"host":          h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
	})
	if _, err = m.client.CancelSpotInstanceRequests(ctx, &ec2.CancelSpotInstanceRequestsInput{
		SpotInstanceRequestIds: []*string{makeStringPtr(h.Id)},
	}); err != nil {
		grip.Error(message.Fields{
			"message":       "failed to cancel spot request",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		})
		return "", errors.Wrapf(err, "Failed to cancel spot request for host %s", h.Id)
	}
	grip.Info(message.Fields{
		"message":       "canceled spot request",
		"host":          h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
	})

	if spotDetails.SpotInstanceRequests[0].InstanceId != nil {
		return *spotDetails.SpotInstanceRequests[0].InstanceId, nil
	}
	return "", nil
}

// IsUp returns whether a host is up.
func (m *ec2Manager) IsUp(ctx context.Context, h *host.Host) (bool, error) {
	status, err := m.GetInstanceStatus(ctx, h)
	if err != nil {
		return false, errors.Wrap(err, "error checking if instance is up")
	}
	if status == StatusRunning {
		return true, nil
	}
	return false, nil
}

// OnUp is called when the host is up.
func (m *ec2Manager) OnUp(ctx context.Context, h *host.Host) error {
	if isHostSpot(h) {
		grip.Debug(message.Fields{
			"message":       "spot host is up, attaching tags",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		})
		if err := m.client.Create(m.credentials); err != nil {
			return errors.Wrap(err, "error creating client")
		}
		defer m.client.Close()
		spotDetails, err := m.client.DescribeSpotInstanceRequests(ctx, &ec2.DescribeSpotInstanceRequestsInput{
			SpotInstanceRequestIds: []*string{makeStringPtr(h.Id)},
		})
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "error getting spot request info",
				"host":          h.Id,
				"host_provider": h.Distro.Provider,
				"distro":        h.Distro.Id,
			}))
			return errors.Wrapf(err, "failed to get spot request info for %s", h.Id)
		}
		tags := makeTags(h)
		tags["spot"] = "true" // mark this as a spot instance
		resources := []*string{
			spotDetails.SpotInstanceRequests[0].InstanceId,
		}
		tagSlice := []*ec2.Tag{}
		for tag := range tags {
			key := tag
			val := tags[tag]
			tagSlice = append(tagSlice, &ec2.Tag{Key: &key, Value: &val})
		}

		resp, err := m.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: []*string{spotDetails.SpotInstanceRequests[0].InstanceId},
		})
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "error running describe instances",
				"host":          h.Id,
				"host_provider": h.Distro.Provider,
				"distro":        h.Distro.Id,
			}))
		}
		instance := resp.Reservations[0].Instances[0]
		for _, vol := range instance.BlockDeviceMappings {
			if *vol.DeviceName == "" {
				continue
			}
			resources = append(resources, vol.Ebs.VolumeId)
		}

		if _, err = m.client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: resources,
			Tags:      tagSlice,
		}); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "error attaching tags",
				"host":          h.Id,
				"host_provider": h.Distro.Provider,
				"distro":        h.Distro.Id,
			}))
			err = errors.Wrapf(err, "failed to attach tags for %s", h.Id)
			return err
		}
	}
	return nil
}

// GetDNSName returns the DNS name for the host.
func (m *ec2Manager) GetDNSName(ctx context.Context, h *host.Host) (string, error) {
	var instance *ec2.Instance
	var err error

	if err = m.client.Create(m.credentials); err != nil {
		return "", errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	if isHostOnDemand(h) {
		instance, err = m.client.GetInstanceInfo(ctx, h.Id)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "error getting instance info",
				"host":          h.Id,
				"host_provider": h.Distro.Provider,
				"distro":        h.Distro.Id,
			}))
			return "", errors.Wrap(err, "error getting instance info")
		}
	} else {
		spotDetails, err := m.client.DescribeSpotInstanceRequests(ctx, &ec2.DescribeSpotInstanceRequestsInput{
			SpotInstanceRequestIds: []*string{makeStringPtr(h.Id)},
		})
		if err != nil {
			return "", errors.Wrapf(err, "failed to get spot request info for %s", h.Id)
		}
		if spotDetails.SpotInstanceRequests[0].InstanceId == nil {
			return "", errors.WithStack(errors.New("spot instance does not yet have an instanceId"))
		}
		if *spotDetails.SpotInstanceRequests[0].InstanceId != "" {
			instance, err = m.client.GetInstanceInfo(ctx, *spotDetails.SpotInstanceRequests[0].InstanceId)
			if err != nil {
				return "", errors.Wrap(err, "error getting instance info")
			}
		}
	}
	return *instance.PublicDnsName, nil
}

// GetSSHOptions returns the command-line args to pass to SSH.
func (m *ec2Manager) GetSSHOptions(h *host.Host, keyName string) ([]string, error) {
	if keyName == "" {
		return nil, errors.New("No key specified for EC2 host")
	}
	opts := []string{"-i", keyName}
	hasKnownHostsFile := false

	for _, opt := range h.Distro.SSHOptions {
		opt = strings.Trim(opt, " \t")
		opts = append(opts, "-o", opt)
		if strings.HasPrefix(opt, "UserKnownHostsFile") {
			hasKnownHostsFile = true
		}
	}

	if !hasKnownHostsFile {
		opts = append(opts, "-o", "UserKnownHostsFile=/dev/null")
	}
	return opts, nil
}

// TimeTilNextPayment returns how long until the next payment is due for a host.
func (m *ec2Manager) TimeTilNextPayment(host *host.Host) time.Duration {
	return timeTilNextEC2Payment(host)
}

// GetInstanceName returns the name of an instance.
func (m *ec2Manager) GetInstanceName(d *distro.Distro) string {
	return d.GenerateName()
}

func (m *ec2Manager) getSpotInstanceStatus(ctx context.Context, id string) (CloudStatus, error) {
	spotDetails, err := m.client.DescribeSpotInstanceRequests(ctx, &ec2.DescribeSpotInstanceRequestsInput{
		SpotInstanceRequestIds: []*string{makeStringPtr(id)},
	})
	if err != nil {
		err = errors.Wrapf(err, "failed to get spot request info for %s", id)
		return StatusUnknown, err
	}

	spotInstance := spotDetails.SpotInstanceRequests[0]
	//Spot request has been fulfilled, so get status of the instance itself
	if spotInstance.InstanceId != nil && *spotInstance.InstanceId != "" {
		instanceInfo, err := m.client.GetInstanceInfo(ctx, *spotInstance.InstanceId)
		if err != nil {
			return StatusUnknown, errors.Wrap(err, "Got an error checking spot details")
		}
		return ec2StatusToEvergreenStatus(*instanceInfo.State.Name), nil
	}

	//Spot request is not fulfilled. Either it's failed/closed for some reason,
	//or still pending evaluation
	return cloudStatusFromSpotStatus(id, *spotInstance.State), nil
}

func cloudStatusFromSpotStatus(id, state string) CloudStatus {
	switch state {
	case SpotStatusOpen:
		return StatusPending
	case SpotStatusActive:
		return StatusPending
	case SpotStatusClosed:
		return StatusTerminated
	case SpotStatusCanceled:
		return StatusTerminated
	case SpotStatusFailed:
		return StatusFailed
	default:
		grip.Error(message.Fields{
			"message": "Unexpected status code in spot request",
			"code":    state,
			"id":      id,
		})
		return StatusUnknown
	}
}

func (m *ec2Manager) CostForDuration(ctx context.Context, h *host.Host, start, end time.Time) (float64, error) {
	if end.Before(start) || util.IsZeroTime(start) || util.IsZeroTime(end) {
		return 0, errors.New("task timing data is malformed")
	}
	if err := m.client.Create(m.credentials); err != nil {
		return 0, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	t := timeRange{start: start, end: end}
	ec2Cost, err := pkgCachingPriceFetcher.getEC2Cost(ctx, m.client, h, t)
	if err != nil {
		return 0, errors.Wrap(err, "error fetching ec2 cost")
	}
	ebsCost, err := pkgCachingPriceFetcher.getEBSCost(ctx, m.client, h, t)
	if err != nil {
		return 0, errors.Wrap(err, "error fetching ebs cost")
	}
	return ec2Cost + ebsCost, nil
}
