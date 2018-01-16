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
	"github.com/evergreen-ci/evergreen/hostutil"
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
	return h.Distro.Provider == evergreen.ProviderNameEc2SpotNew
}
func isHostOnDemand(h *host.Host) bool {
	return h.Distro.Provider == evergreen.ProviderNameEc2OnDemandNew
}

// NewEC2ProviderSettings describes properties of managed instances.
type NewEC2ProviderSettings struct {
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
	// SubnetId is only set in a VPC.
	SubnetId string `mapstructure:"subnet_id" json:"subnet_id,omitempty" bson:"subnet_id,omitempty"`

	// IsVpc is set to true if the security group is part of a VPC.
	IsVpc bool `mapstructure:"is_vpc" json:"is_vpc,omitempty" bson:"is_vpc,omitempty"`

	// BidPrice is the price we are willing to pay for a spot instance.
	BidPrice float64 `mapstructure:"bid_price" json:"bid_price,omitempty" bson:"bid_price,omitempty"`
}

// Validate that essential NewEC2ProviderSettings fields are not empty.
func (s *NewEC2ProviderSettings) Validate() error {
	if s.AMI == "" || s.InstanceType == "" || s.SecurityGroup == "" || s.KeyName == "" {
		return errors.New("AMI, instance type, security group, and key name must not be empty")
	}
	if s.BidPrice < 0 {
		return errors.New("Bid price must not be negative")
	}
	if _, err := newMakeBlockDeviceMappings(s.MountPoints); err != nil {
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
	return &NewEC2ProviderSettings{}
}

// Configure loads credentials or other settings from the config file.
func (m *ec2Manager) Configure(settings *evergreen.Settings) error {
	if settings.Providers.AWS.Id == "" || settings.Providers.AWS.Secret == "" {
		return errors.New("AWS ID and Secret must not be blank")
	}

	m.credentials = credentials.NewStaticCredentialsFromCreds(credentials.Value{
		AccessKeyID:     settings.Providers.AWS.Id,
		SecretAccessKey: settings.Providers.AWS.Secret,
	})

	return nil
}

func (m *ec2Manager) spawnOnDemandHost(h *host.Host, ec2Settings *NewEC2ProviderSettings, blockDevices []*ec2.BlockDeviceMapping) ([]*string, error) {
	input := &ec2.RunInstancesInput{
		MinCount:            makeInt64Ptr(1),
		MaxCount:            makeInt64Ptr(1),
		ImageId:             &ec2Settings.AMI,
		KeyName:             &ec2Settings.KeyName,
		InstanceType:        &ec2Settings.InstanceType,
		BlockDeviceMappings: blockDevices,
	}

	if ec2Settings.IsVpc {
		input.SecurityGroupIds = []*string{&ec2Settings.SecurityGroup}
		input.SubnetId = &ec2Settings.SubnetId
		input.NetworkInterfaces = []*ec2.InstanceNetworkInterfaceSpecification{
			&ec2.InstanceNetworkInterfaceSpecification{
				AssociatePublicIpAddress: makeBoolPtr(true),
				Groups:   []*string{&ec2Settings.SecurityGroup},
				SubnetId: &ec2Settings.SubnetId,
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
	reservation, err := m.client.RunInstances(input)
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

	resp, err := m.client.DescribeInstances(&ec2.DescribeInstancesInput{
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

func (m *ec2Manager) spawnSpotHost(h *host.Host, ec2Settings *NewEC2ProviderSettings, blockDevices []*ec2.BlockDeviceMapping) ([]*string, error) {
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
		spotRequest.LaunchSpecification.SecurityGroupIds = []*string{&ec2Settings.SecurityGroup}
		spotRequest.LaunchSpecification.SubnetId = &ec2Settings.SubnetId
		spotRequest.LaunchSpecification.NetworkInterfaces = []*ec2.InstanceNetworkInterfaceSpecification{
			&ec2.InstanceNetworkInterfaceSpecification{
				AssociatePublicIpAddress: makeBoolPtr(true),
				Groups:   []*string{&ec2Settings.SecurityGroup},
				SubnetId: &ec2Settings.SubnetId,
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
	spotResp, err := m.client.RequestSpotInstances(spotRequest)
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
func (m *ec2Manager) SpawnHost(h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameEc2OnDemandNew && h.Distro.Provider != evergreen.ProviderNameEc2SpotNew {
		return nil, errors.Errorf("Can't spawn instance of %s for distro %s: provider is %s",
			evergreen.ProviderNameEc2OnDemand, h.Distro.Id, h.Distro.Provider)
	}

	ec2Settings := &NewEC2ProviderSettings{}
	if err := mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings); err != nil {
		return nil, errors.Wrapf(err, "Error decoding params for distro %+v: %+v", h.Distro.Id, ec2Settings)
	}

	if err := ec2Settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid EC2 settings in distro %s: and %+v", h.Distro.Id, ec2Settings)
	}

	blockDevices, err := newMakeBlockDeviceMappings(ec2Settings.MountPoints)
	if err != nil {
		return nil, errors.Wrap(err, "error making block device mappings")
	}
	h.InstanceType = ec2Settings.InstanceType

	if err = m.client.Create(m.credentials); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	var resources []*string
	provider, err := m.getProvider(h, ec2Settings)
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
		resources, err = m.spawnOnDemandHost(h, ec2Settings, blockDevices)
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
		resources, err = m.spawnSpotHost(h, ec2Settings, blockDevices)
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
	if _, err = m.client.CreateTags(&ec2.CreateTagsInput{
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
func (m *ec2Manager) GetInstanceStatus(h *host.Host) (CloudStatus, error) {
	if err := m.client.Create(m.credentials); err != nil {
		return StatusUnknown, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()
	if isHostOnDemand(h) {
		info, err := m.client.GetInstanceInfo(h.Id)
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
		return m.getSpotInstanceStatus(h.Id)
	}
	return StatusUnknown, errors.New("type must be on-demand or spot")
}

// TerminateInstance terminates the EC2 instance.
func (m *ec2Manager) TerminateInstance(h *host.Host) error {
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
	if isHostSpot(h) {
		instanceId, err := m.cancelSpotRequest(h)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "error canceling spot request",
				"host":          h.Id,
				"host_provider": h.Distro.Provider,
				"distro":        h.Distro.Id,
			}))
			return errors.Wrap(err, "error canceling spot request")
		}
		// the spot request wasn't fulfilled, so don't attempt to terminate in ec2
		if instanceId == "" {
			return errors.Wrap(h.Terminate(), "failed to terminate instance in db")
		}
	}

	resp, err := m.client.TerminateInstances(&ec2.TerminateInstancesInput{
		InstanceIds: []*string{makeStringPtr(instanceId)},
	})
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error terminating instance",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return err
	}

	for _, stateChange := range resp.TerminatingInstances {
		grip.Info(message.Fields{
			"message":       "terminated instance",
			"host_provider": h.Distro.Provider,
			"instance_id":   *stateChange.InstanceId,
			"host":          h.Id,
			"distro":        h.Distro.Id,
		})
	}

	return errors.Wrap(h.Terminate(), "failed to terminate instance in db")
}

func (m *ec2Manager) cancelSpotRequest(h *host.Host) (string, error) {
	spotDetails, err := m.client.DescribeSpotInstanceRequests(&ec2.DescribeSpotInstanceRequestsInput{
		SpotInstanceRequestIds: []*string{makeStringPtr(h.Id)},
	})
	if err != nil {
		if ec2err, ok := err.(awserr.Error); ok {
			if ec2err.Code() == EC2ErrorSpotRequestNotFound {
				return "", h.Terminate()
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
	if _, err = m.client.CancelSpotInstanceRequests(&ec2.CancelSpotInstanceRequestsInput{
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
func (m *ec2Manager) IsUp(h *host.Host) (bool, error) {
	if err := m.client.Create(m.credentials); err != nil {
		return false, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()
	info, err := m.client.GetInstanceInfo(h.Id)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error getting instance info",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return false, err
	}
	if ec2StatusToEvergreenStatus(*info.State.Name).String() == EC2StatusRunning {
		return true, nil
	}
	return false, nil
}

// OnUp is called when the host is up.
func (m *ec2Manager) OnUp(*host.Host) error {
	// Unused since we set tags in SpawnHost()
	return nil
}

// IsSSHReachable returns whether or not the host can be reached via SSH.
func (m *ec2Manager) IsSSHReachable(h *host.Host, keyName string) (bool, error) {
	opts, err := m.GetSSHOptions(h, keyName)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error getting ssh options",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return false, err
	}
	return hostutil.CheckSSHResponse(context.TODO(), h, opts)
}

// GetDNSName returns the DNS name for the host.
func (m *ec2Manager) GetDNSName(h *host.Host) (string, error) {
	info, err := m.client.GetInstanceInfo(h.Id)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error getting instance info",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return "", errors.Wrap(err, "error getting instance info")
	}
	return *info.PublicDnsName, nil
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

func (m *ec2Manager) getSpotInstanceStatus(id string) (CloudStatus, error) {
	spotDetails, err := m.client.DescribeSpotInstanceRequests(&ec2.DescribeSpotInstanceRequestsInput{
		SpotInstanceRequestIds: []*string{makeStringPtr(id)},
	})
	if err != nil {
		err = errors.Wrapf(err, "failed to get spot request info for %s", id)
		return StatusUnknown, err
	}

	spotInstance := spotDetails.SpotInstanceRequests[0]
	//Spot request has been fulfilled, so get status of the instance itself
	if *spotInstance.InstanceId != "" {
		instanceInfo, err := m.client.GetInstanceInfo(*spotInstance.InstanceId)
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

func (m *ec2Manager) CostForDuration(h *host.Host, start, end time.Time) (float64, error) {
	if end.Before(start) || util.IsZeroTime(start) || util.IsZeroTime(end) {
		return 0, errors.New("task timing data is malformed")
	}
	if err := m.client.Create(m.credentials); err != nil {
		return 0, errors.Wrap(err, "error creating client")
	}

	t := timeRange{start: start, end: end}
	ec2Cost, err := pkgCachingPriceFetcher.getEC2Cost(m.client, h, t)
	if err != nil {
		return 0, errors.Wrap(err, "error fetching ec2 cost")
	}
	ebsCost, err := pkgCachingPriceFetcher.getEBSCost(m.client, h, t)
	if err != nil {
		return 0, errors.Wrap(err, "error fetching ebs cost")
	}
	return ec2Cost + ebsCost, nil
}
