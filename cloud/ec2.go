package cloud

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
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

	// If set, overrides key from credentials
	AWSKeyID string `mapstructure:"aws_access_key_id" bson:"aws_access_key_id,omitempty"`

	// If set, overrides secret from credentials
	AWSSecret string `mapstructure:"aws_secret_access_key" bson:"aws_secret_access_key,omitempty"`

	// InstanceType is the EC2 instance type.
	InstanceType string `mapstructure:"instance_type" json:"instance_type,omitempty" bson:"instance_type,omitempty"`

	// IPv6 is set to true if the instance should have only an IPv6 address.
	IPv6 bool `mapstructure:"ipv6" json:"ipv6,omitempty" bson:"ipv6,omitempty"`

	// KeyName is the AWS SSH key name.
	KeyName string `mapstructure:"key_name" json:"key_name,omitempty" bson:"key_name,omitempty"`

	// MountPoints are the disk mount points for EBS volumes.
	MountPoints []MountPoint `mapstructure:"mount_points" json:"mount_points,omitempty" bson:"mount_points,omitempty"`

	// SecurityGroupIDs is a list of security group IDs.
	SecurityGroupIDs []string `mapstructure:"security_group_ids" json:"security_group_ids,omitempty" bson:"security_group_ids,omitempty"`

	// SubnetId is only set in a VPC. Either subnet id or vpc name must set.
	SubnetId string `mapstructure:"subnet_id" json:"subnet_id,omitempty" bson:"subnet_id,omitempty"`

	// VpcName is used to get the subnet ID automatically. Either subnet id or vpc name must set.
	VpcName string `mapstructure:"vpc_name" json:"vpc_name,omitempty" bson:"vpc_name,omitempty"`

	// IsVpc is set to true if the security group is part of a VPC.
	IsVpc bool `mapstructure:"is_vpc" json:"is_vpc,omitempty" bson:"is_vpc,omitempty"`

	// BidPrice is the price we are willing to pay for a spot instance.
	BidPrice float64 `mapstructure:"bid_price" json:"bid_price,omitempty" bson:"bid_price,omitempty"`

	// UserData are commands to run after the instance starts.
	UserData string `mapstructure:"user_data" json:"user_data" bson:"user_data,omitempty"`

	// Region is the EC2 region in which the instance will start. If empty,
	// the ec2Manager will spawn in "us-east-1".
	Region string `mapstructure:"region" json:"region" bson:"region,omitempty"`
}

// Validate that essential EC2ProviderSettings fields are not empty.
func (s *EC2ProviderSettings) Validate() error {
	if s.AMI == "" || s.InstanceType == "" {
		return errors.New("AMI, instance type, and key name must not be empty")
	}
	if len(s.SecurityGroupIDs) == 0 {
		return errors.New("Security groups must not be empty")
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

func (s *EC2ProviderSettings) fromDistroSettings(d distro.Distro) error {
	if d.ProviderSettings != nil {
		if err := mapstructure.Decode(d.ProviderSettings, s); err != nil {
			return errors.Wrapf(err, "Error decoding params for distro %s: %+v", d.Id, s)
		}
		grip.Debug(message.Fields{
			"message": "mapstructure comparison",
			"input":   *d.ProviderSettings,
			"output":  *s,
		})
	}
	if err := s.Validate(); err != nil {
		return errors.Wrapf(err, "Invalid EC2 settings in distro %s: %+v", d.Id, s)
	}

	return nil
}

func (s *EC2ProviderSettings) getSecurityGroups() []*string {
	groups := []*string{}
	if len(s.SecurityGroupIDs) > 0 {
		for _, group := range s.SecurityGroupIDs {
			groups = append(groups, aws.String(group))
		}
		return groups
	}
	return groups
}

func (s *EC2ProviderSettings) getRegion() string {
	if s.Region != "" {
		return s.Region
	}
	return defaultRegion
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
	settings    *evergreen.Settings
}

// NewEC2Manager creates a new manager of EC2 spot and on-demand instances.
func NewEC2Manager(opts *EC2ManagerOptions) Manager {
	return &ec2Manager{EC2ManagerOptions: opts}
}

// Configure loads credentials or other settings from the config file.
func (m *ec2Manager) Configure(ctx context.Context, settings *evergreen.Settings) error {
	m.settings = settings

	if settings.Providers.AWS.EC2Key == "" || settings.Providers.AWS.EC2Secret == "" {
		return errors.New("AWS ID and Secret must not be blank")
	}

	m.credentials = credentials.NewStaticCredentialsFromCreds(credentials.Value{
		AccessKeyID:     settings.Providers.AWS.EC2Key,
		SecretAccessKey: settings.Providers.AWS.EC2Secret,
	})

	return nil
}

func (m *ec2Manager) spawnOnDemandHost(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings, blockDevices []*ec2.BlockDeviceMapping) ([]string, error) {
	input := &ec2.RunInstancesInput{
		MinCount:            aws.Int64(1),
		MaxCount:            aws.Int64(1),
		ImageId:             &ec2Settings.AMI,
		KeyName:             &ec2Settings.KeyName,
		InstanceType:        &ec2Settings.InstanceType,
		BlockDeviceMappings: blockDevices,
	}

	if ec2Settings.IsVpc {
		input.NetworkInterfaces = []*ec2.InstanceNetworkInterfaceSpecification{
			&ec2.InstanceNetworkInterfaceSpecification{
				AssociatePublicIpAddress: aws.Bool(true),
				DeviceIndex:              aws.Int64(0),
				Groups:                   ec2Settings.getSecurityGroups(),
				SubnetId:                 &ec2Settings.SubnetId,
			},
		}
		if ec2Settings.IPv6 {
			input.NetworkInterfaces[0].SetIpv6AddressCount(1).SetAssociatePublicIpAddress(false)
		}
	} else {
		input.SecurityGroups = ec2Settings.getSecurityGroups()
	}

	if ec2Settings.UserData != "" {
		expanded, err := expandUserData(ec2Settings.UserData, m.settings.Expansions)
		if err != nil {
			return nil, errors.Wrap(err, "problem expanding user data")
		}
		ec2Settings.UserData = expanded
	}

	userData, err := bootstrapUserData(ctx, evergreen.GetEnvironment(), h, ec2Settings.UserData)
	if err != nil {
		return nil, errors.Wrap(err, "could not add bootstrap script to user data")
	}
	ec2Settings.UserData = userData

	if ec2Settings.UserData != "" {
		userData := base64.StdEncoding.EncodeToString([]byte(ec2Settings.UserData))
		input.UserData = &userData
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
	resources := []string{h.Id}
	for _, vol := range instance.BlockDeviceMappings {
		if *vol.DeviceName == "" {
			continue
		}

		resources = append(resources, *vol.Ebs.VolumeId)
	}
	return resources, nil
}

func (m *ec2Manager) spawnSpotHost(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings, blockDevices []*ec2.BlockDeviceMapping) ([]string, error) {
	spotRequest := &ec2.RequestSpotInstancesInput{
		SpotPrice:     aws.String(fmt.Sprintf("%v", ec2Settings.BidPrice)),
		InstanceCount: aws.Int64(1),
		LaunchSpecification: &ec2.RequestSpotLaunchSpecification{
			ImageId:             aws.String(ec2Settings.AMI),
			KeyName:             aws.String(ec2Settings.KeyName),
			InstanceType:        aws.String(ec2Settings.InstanceType),
			BlockDeviceMappings: blockDevices,
		},
	}

	if ec2Settings.IsVpc {
		spotRequest.LaunchSpecification.NetworkInterfaces = []*ec2.InstanceNetworkInterfaceSpecification{
			&ec2.InstanceNetworkInterfaceSpecification{
				AssociatePublicIpAddress: aws.Bool(true),
				DeviceIndex:              aws.Int64(0),
				Groups:                   ec2Settings.getSecurityGroups(),
				SubnetId:                 &ec2Settings.SubnetId,
			},
		}
		if ec2Settings.IPv6 {
			spotRequest.LaunchSpecification.NetworkInterfaces[0].SetIpv6AddressCount(1).SetAssociatePublicIpAddress(false)
		}
	} else {
		spotRequest.LaunchSpecification.SecurityGroups = ec2Settings.getSecurityGroups()
	}

	if ec2Settings.UserData != "" {
		expanded, err := expandUserData(ec2Settings.UserData, m.settings.Expansions)
		if err != nil {
			return nil, errors.Wrap(err, "problem expanding user data")
		}
		ec2Settings.UserData = expanded
	}

	userData, err := bootstrapUserData(ctx, evergreen.GetEnvironment(), h, ec2Settings.UserData)
	if err != nil {
		return nil, errors.Wrap(err, "could not add bootstrap script to user data")
	}
	ec2Settings.UserData = userData

	if ec2Settings.UserData != "" {
		userData := base64.StdEncoding.EncodeToString([]byte(ec2Settings.UserData))
		spotRequest.LaunchSpecification.UserData = &userData
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
	resources := []string{*spotReqRes.SpotInstanceRequestId}
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
	err := ec2Settings.fromDistroSettings(h.Distro)
	if err != nil {
		return nil, errors.Wrap(err, "error getting EC2 settings")
	}

	if ec2Settings.AWSKeyID != "" {
		m.credentials = credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     ec2Settings.AWSKeyID,
			SecretAccessKey: ec2Settings.AWSSecret,
		})
	}

	if err = m.client.Create(m.credentials, ec2Settings.getRegion()); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	if ec2Settings.KeyName == "" && !h.UserHost {
		if !h.SpawnOptions.SpawnedByTask {
			return nil, errors.New("key name must not be empty")
		}
		k, err := m.client.GetKey(ctx, h)
		if err != nil {
			return nil, errors.Wrap(err, "not spawning host, problem creating key")
		}
		ec2Settings.KeyName = k
	}

	blockDevices, err := makeBlockDeviceMappings(ec2Settings.MountPoints)
	if err != nil {
		return nil, errors.Wrap(err, "error making block device mappings")
	}
	h.InstanceType = ec2Settings.InstanceType

	var resources []string
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

	if err = m.client.SetTags(ctx, resources, h); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error attaching tags",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return nil, errors.Wrapf(err, "failed to attach tags for %s", h.Id)
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

// GetInstanceStatuses returns the current status of a slice of EC2 instances.
func (m *ec2Manager) GetInstanceStatuses(ctx context.Context, hosts []host.Host) ([]CloudStatus, error) {
	if err := m.client.Create(m.credentials, defaultRegion); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}

	spotHostIDs := []*string{}
	onDemandHostIDs := []*string{}
	instanceIdToHostMap := map[string]string{}
	hostToStatusMap := map[string]CloudStatus{}
	hostsToCheck := []*string{}

	// Populate spot and on-demand slices
	for i := range hosts {
		if isHostSpot(&hosts[i]) {
			spotHostIDs = append(spotHostIDs, &hosts[i].Id)
		}
		if isHostOnDemand(&hosts[i]) {
			instanceIdToHostMap[hosts[i].Id] = hosts[i].Id
			onDemandHostIDs = append(onDemandHostIDs, &hosts[i].Id)
		}
	}

	// Get instance IDs for spot instances
	if len(spotHostIDs) > 0 {
		spotOut, err := m.client.DescribeSpotInstanceRequests(ctx, &ec2.DescribeSpotInstanceRequestsInput{
			SpotInstanceRequestIds: spotHostIDs,
		})
		if err != nil {
			return nil, errors.Wrap(err, "error describing spot instances")
		}
		if len(spotOut.SpotInstanceRequests) != len(spotHostIDs) {
			return nil, errors.New("programmer error: length of spot instance requests != length of spot host IDs")
		}
		spotInstanceRequestsMap := map[string]*ec2.SpotInstanceRequest{}
		for i := range spotOut.SpotInstanceRequests {
			spotInstanceRequestsMap[*spotOut.SpotInstanceRequests[i].SpotInstanceRequestId] = spotOut.SpotInstanceRequests[i]
		}
		for i := range spotHostIDs {
			if spotInstanceRequestsMap[*spotHostIDs[i]].InstanceId == nil || *spotInstanceRequestsMap[*spotHostIDs[i]].InstanceId == "" {
				hostToStatusMap[*spotHostIDs[i]] = cloudStatusFromSpotStatus(*spotInstanceRequestsMap[*spotHostIDs[i]].State)
				continue
			}
			hostsToCheck = append(hostsToCheck, spotInstanceRequestsMap[*spotHostIDs[i]].InstanceId)
			instanceIdToHostMap[*spotInstanceRequestsMap[*spotHostIDs[i]].InstanceId] = *spotHostIDs[i]
		}
	}

	// Get host statuses
	hostsToCheck = append(hostsToCheck, onDemandHostIDs...)
	if len(hostsToCheck) > 0 {
		out, err := m.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: hostsToCheck,
		})
		if err != nil {
			return nil, errors.Wrap(err, "error describing instances")
		}
		reservationsMap := map[string]string{}
		for i := range out.Reservations {
			reservationsMap[*out.Reservations[i].Instances[0].InstanceId] = *out.Reservations[i].Instances[0].State.Name
		}
		for i := range hostsToCheck {
			hostToStatusMap[instanceIdToHostMap[*hostsToCheck[i]]] = ec2StatusToEvergreenStatus(reservationsMap[*hostsToCheck[i]])
		}
	}

	// Populate cloud statuses
	statuses := []CloudStatus{}
	for _, h := range hosts {
		statuses = append(statuses, hostToStatusMap[h.Id])
	}
	return statuses, nil
}

// GetInstanceStatus returns the current status of an EC2 instance.
func (m *ec2Manager) GetInstanceStatus(ctx context.Context, h *host.Host) (CloudStatus, error) {
	ec2Settings := &EC2ProviderSettings{}
	err := ec2Settings.fromDistroSettings(h.Distro)
	if err != nil {
		return StatusUnknown, errors.Wrap(err, "problem getting settings from host")
	}
	if err := m.client.Create(m.credentials, ec2Settings.getRegion()); err != nil {
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
		return m.getSpotInstanceStatus(ctx, h)
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
	ec2Settings := &EC2ProviderSettings{}
	err := ec2Settings.fromDistroSettings(h.Distro)
	if err != nil {
		return errors.Wrap(err, "problem getting settings from host")
	}
	if err = m.client.Create(m.credentials, ec2Settings.getRegion()); err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	instanceId := h.Id
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
		InstanceIds: []*string{aws.String(instanceId)},
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
	instanceId, err := m.client.GetSpotInstanceId(ctx, h)
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
	if _, err = m.client.CancelSpotInstanceRequests(ctx, &ec2.CancelSpotInstanceRequestsInput{
		SpotInstanceRequestIds: []*string{aws.String(h.Id)},
	}); err != nil {
		if ec2err, ok := err.(awserr.Error); ok {
			if ec2err.Code() == EC2ErrorSpotRequestNotFound {
				return "", h.Terminate(evergreen.User)
			}
		}
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

	return instanceId, nil
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
		ec2Settings := &EC2ProviderSettings{}
		err := ec2Settings.fromDistroSettings(h.Distro)
		if err != nil {
			return errors.Wrap(err, "problem getting region from host")
		}
		if err = m.client.Create(m.credentials, ec2Settings.getRegion()); err != nil {
			return errors.Wrap(err, "error creating client")
		}
		defer m.client.Close()
		instanceId, err := m.client.GetSpotInstanceId(ctx, h)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "error getting spot request info",
				"host":          h.Id,
				"host_provider": h.Distro.Provider,
				"distro":        h.Distro.Id,
			}))
			return errors.Wrapf(err, "failed to get spot request info for %s", h.Id)
		}
		if instanceId == "" {
			return errors.WithStack(errors.New("spot instance does not yet have an instanceId"))
		}
		tags := makeTags(h)
		tags["spot"] = "true" // mark this as a spot instance
		resources := []*string{
			&instanceId,
		}
		tagSlice := []*ec2.Tag{}
		for tag := range tags {
			key := tag
			val := tags[tag]
			tagSlice = append(tagSlice, &ec2.Tag{Key: &key, Value: &val})
		}

		resp, err := m.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: []*string{&instanceId},
		})
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "error running describe instances",
				"host":          h.Id,
				"host_provider": h.Distro.Provider,
				"distro":        h.Distro.Id,
			}))
		}
		if resp == nil || len(resp.Reservations) == 0 || len(resp.Reservations[0].Instances) == 0 {
			grip.Error(message.Fields{
				"message":       "error finding instance",
				"host":          h.Id,
				"host_provider": h.Distro.Provider,
				"distro":        h.Distro.Id,
			})
			return errors.Errorf("error finding instance for %s", h.Id)
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

func (m *ec2Manager) retrieveInstance(ctx context.Context, h *host.Host) (*ec2.Instance, error) {
	var instance *ec2.Instance

	ec2Settings := &EC2ProviderSettings{}
	err := ec2Settings.fromDistroSettings(h.Distro)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting region from host")
	}
	if err = m.client.Create(m.credentials, ec2Settings.getRegion()); err != nil {
		return nil, errors.Wrap(err, "error creating client")
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
			return nil, errors.Wrap(err, "error getting instance info")
		}
	} else {
		var instanceId string
		instanceId, err = m.client.GetSpotInstanceId(ctx, h)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get spot request info for %s", h.Id)
		}
		if instanceId == "" {
			return nil, errors.WithStack(errors.New("spot instance does not yet have an instanceId"))
		}
		instance, err = m.client.GetInstanceInfo(ctx, instanceId)
		if err != nil {
			return nil, errors.Wrap(err, "error getting instance info")
		}
	}

	// Cache launch time and availability zone in host document, since we
	// have access to this information now. Cost jobs will use this
	// information later.
	if err = cacheHostData(ctx, h, instance, m.client); err != nil {
		return nil, errors.Wrapf(err, "can't update host '%s'", h.Id)
	}

	return instance, nil
}

// GetDNSName returns the DNS name for the host.
func (m *ec2Manager) GetDNSName(ctx context.Context, h *host.Host) (string, error) {
	instance, err := m.retrieveInstance(ctx, h)
	if err != nil {
		return "", errors.Wrapf(err, "error retrieving instance")
	}
	// set IPv6 address, if applicable
	for _, networkInterface := range instance.NetworkInterfaces {
		if len(networkInterface.Ipv6Addresses) > 0 {
			err = h.SetIPv6Address(*networkInterface.Ipv6Addresses[0].Ipv6Address)
			if err != nil {
				return "", errors.Wrap(err, "error setting ipv6 address")
			}
			break
		}
	}
	return *instance.PublicDnsName, nil
}

// GetSSHOptions returns the command-line args to pass to SSH.
func (m *ec2Manager) GetSSHOptions(h *host.Host, keyName string) ([]string, error) {
	return h.GetSSHOptions(keyName)
}

// TimeTilNextPayment returns how long until the next payment is due for a host.
func (m *ec2Manager) TimeTilNextPayment(host *host.Host) time.Duration {
	return timeTilNextEC2Payment(host)
}

func (m *ec2Manager) getSpotInstanceStatus(ctx context.Context, h *host.Host) (CloudStatus, error) {
	spotDetails, err := m.client.DescribeSpotRequestsAndSave(ctx, []*host.Host{h})
	if err != nil {
		err = errors.Wrapf(err, "failed to get spot request info for %s", h.Id)
		return StatusUnknown, err
	}
	if len(spotDetails.SpotInstanceRequests) == 0 {
		return StatusUnknown, errors.Errorf("failed to get spot request info for %s", h.Id)
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
	return cloudStatusFromSpotStatus(*spotInstance.State), nil
}

func cloudStatusFromSpotStatus(state string) CloudStatus {
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
		return StatusUnknown
	}
}

func (m *ec2Manager) CostForDuration(ctx context.Context, h *host.Host, start, end time.Time, s *evergreen.Settings) (float64, error) {
	if end.Before(start) || util.IsZeroTime(start) || util.IsZeroTime(end) {
		return 0, errors.New("task timing data is malformed")
	}
	ec2Settings := &EC2ProviderSettings{}
	err := ec2Settings.fromDistroSettings(h.Distro)
	if err != nil {
		return 0, errors.Wrap(err, "problem getting region from host")
	}
	if err = m.client.Create(m.credentials, ec2Settings.getRegion()); err != nil {
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
	total := ec2Cost + ebsCost

	if total < 0 {
		return 0, errors.Errorf("cost appears to be less than 0 (%g) which is impossible", total)
	}

	return total, nil
}
