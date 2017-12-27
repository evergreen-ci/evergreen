package cloudnew

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// EC2ProviderSettings describes properties of managed instances.
type EC2ProviderSettings struct {
	// AMI is the AMI ID.
	AMI string `mapstructure:"ami" json:"ami,omitempty" bson:"ami,omitempty"`

	// InstanceType is the EC2 instance type.
	InstanceType string `mapstructure:"instance_type" json:"instance_type,omitempty" bson:"instance_type,omitempty"`

	// KeyName is the AWS SSH key name.
	KeyName string `mapstructure:"key_name" json:"key_name,omitempty" bson:"key_name,omitempty"`

	// MountPoints are the disk mount points for EBS volumes.
	MountPoints []cloud.MountPoint `mapstructure:"mount_points" json:"mount_points,omitempty" bson:"mount_points,omitempty"`
	// SecurityGroup is the security group name in EC2 classic and the security group ID in a VPC.
	SecurityGroup string `mapstructure:"security_group" json:"security_group,omitempty" bson:"security_group,omitempty"`
	// SubnetId is only set in a VPC.
	SubnetId string `mapstructure:"subnet_id" json:"subnet_id,omitempty" bson:"subnet_id,omitempty"`

	// IsVpc is set to true if the security group is part of a VPC.
	IsVpc bool `mapstructure:"is_vpc" json:"is_vpc,omitempty" bson:"is_vpc,omitempty"`
}

// Validate that essential EC2ProviderSettings fields are not empty.
func (s *EC2ProviderSettings) Validate() error {
	if s.AMI == "" || s.InstanceType == "" || s.SecurityGroup == "" || s.KeyName == "" {
		return errors.New("AMI, instance type, security group, and key name must not be empty")
	}
	return nil
}

// EC2ManagerOptions are used to construct a new ec2Manager.
type EC2ManagerOptions struct {
	// client is the client library for communicating with AWS.
	client AWSClient
}

// ec2Manager starts and configures instances in EC2.
type ec2Manager struct {
	*EC2ManagerOptions
	credentials *credentials.Credentials
}

// NewEC2Manager creates a new manager of EC2 spot and on-demand instances.
func NewEC2Manager(opts *EC2ManagerOptions) cloud.CloudManager {
	return &ec2Manager{opts, nil}
}

// GetSettings returns a pointer to the manager's configuration settings struct.
func (m *ec2Manager) GetSettings() cloud.ProviderSettings {
	return &EC2ProviderSettings{}
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

// SpawnHost spawns a new host.
func (m *ec2Manager) SpawnHost(h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameEc2OnDemand {
		return nil, errors.Errorf("Can't spawn instance of %s for distro %s: provider is %s",
			evergreen.ProviderNameEc2OnDemand, h.Distro.Id, h.Distro.Provider)
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

	if err := m.client.Create(m.credentials); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}

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
	} else {
		input.SecurityGroups = []*string{&ec2Settings.SecurityGroup}
	}

	reservation, err := m.client.RunInstances(input)
	if err != nil || reservation == nil {
		grip.Error(errors.Wrapf(h.Remove(), "error removing intent host %s", h.Id))
		return nil, errors.Wrap(err, "RunInstances API call returned an error")
	}

	if len(reservation.Instances) < 1 {
		return nil, errors.New("reservation has no instances")
	}

	instance := reservation.Instances[0]
	grip.Debug(message.Fields{
		"msg":      "started ec2 instance",
		"id":       *instance.InstanceId,
		"key_name": *instance.KeyName,
	})

	max := 5
	var resp *ec2.DescribeInstancesOutput
	for i := 0; i <= max; i++ {
		resp, err = m.client.DescribeInstances(&ec2.DescribeInstancesInput{
			InstanceIds: []*string{instance.InstanceId},
		})
		if err != nil {
			if i == max {
				err = errors.Wrap(err, "error querying for instance info and retries exhausted")
				grip.Error(message.Fields{
					"error":    err,
					"instance": *instance.InstanceId,
				})
				return nil, err
			}
			time.Sleep(30 * time.Second)
			continue
		}
		break
	}

	grip.Debug(message.Fields{
		"msg":      "describe instances returned data",
		"host_id":  h.Id,
		"response": resp,
	})

	if len(resp.Reservations) < 1 {
		return nil, errors.New("describe instances response has no reservations")
	}

	h.Id = *instance.InstanceId
	h.InstanceType = ec2Settings.InstanceType

	instance = resp.Reservations[0].Instances[0]
	grip.Debug(message.Fields{
		"msg":         "successfully spawned new host",
		"instance_id": h.Id,
		"instance":    instance,
	})

	tags := makeTags(h)
	resources := []*string{
		instance.InstanceId,
	}

	for _, vol := range instance.BlockDeviceMappings {
		if *vol.DeviceName == "" {
			continue
		}

		resources = append(resources, vol.Ebs.VolumeId)
	}

	tagSlice := []*ec2.Tag{}
	for tag := range tags {
		key := tag
		val := tags[tag]
		tagSlice = append(tagSlice, &ec2.Tag{Key: &key, Value: &val})
	}
	if err = m.client.CreateTags(&ec2.CreateTagsInput{
		Resources: resources,
		Tags:      tagSlice,
	}); err != nil {
		err = errors.Wrapf(err, "failed to attach tags for %s", h.Id)
		grip.Error(err)
		return nil, err
	}

	grip.Debugf("attached tags for '%s'", *instance.InstanceId)
	event.LogHostStarted(h.Id)

	return nil, nil
}

// CanSpawn indicates if a host can be spawned.
func (m *ec2Manager) CanSpawn() (bool, error) {
	return true, nil
}

// GetInstanceStatus returns the current status of an EC2 instance.
func (m *ec2Manager) GetInstanceStatus(h *host.Host) (cloud.CloudStatus, error) {
	info, err := m.getInstanceInfo(h)
	if err != nil {
		return cloud.StatusUnknown, err
	}
	return ec2StatusToEvergreenStatus(*info.State.Name), nil
}

// TerminateInstance terminates the EC2 instance.
func (m *ec2Manager) TerminateInstance(h *host.Host) error {
	// terminate the instance
	if h.Status == evergreen.HostTerminated {
		err := errors.Errorf("Can not terminate %s - already marked as "+
			"terminated!", h.Id)
		grip.Error(err)
		return err
	}

	if err := m.client.Create(m.credentials); err != nil {
		return errors.Wrap(err, "error creating client")
	}

	resp, err := m.client.TerminateInstances(&ec2.TerminateInstancesInput{
		InstanceIds: []*string{&h.Id},
	})
	if err != nil {
		return err
	}

	for _, stateChange := range resp.TerminatingInstances {
		grip.Info(message.Fields{
			"msg":         "terminated instance",
			"instance_id": stateChange.InstanceId,
		})
	}

	return h.Terminate()
}

// IsUp returns whether a host is up.
func (m *ec2Manager) IsUp(h *host.Host) (bool, error) {
	info, err := m.getInstanceInfo(h)
	if err != nil {
		return false, err
	}
	if ec2StatusToEvergreenStatus(*info.State.Name).String() == cloud.EC2StatusRunning {
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
		return false, err
	}
	return hostutil.CheckSSHResponse(context.TODO(), h, opts)
}

// GetDNSName returns the DNS name for the host.
func (m *ec2Manager) GetDNSName(h *host.Host) (string, error) {
	info, err := m.getInstanceInfo(h)
	if err != nil {
		return "", err
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

// makeInt64Ptr is necessary because Go does not allow you to write `&int64(1)`.
func makeInt64Ptr(i int64) *int64 {
	return &i
}

// makeStringPtr is necessary because Go does not allow you to write `&"foo"`.
func makeStringPtr(s string) *string {
	return &s
}

func (m *ec2Manager) getInstanceInfo(h *host.Host) (*ec2.Instance, error) {
	if err := m.client.Create(m.credentials); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}
	resp, err := m.client.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{&h.Id},
	})
	if err != nil {
		return nil, errors.Wrap(err, "EC2 API returns error for DescribeInstances")
	}
	reservation := resp.Reservations
	if len(reservation) == 0 {
		err = errors.Errorf("No reservation found for instance id: %s", h.Id)
		grip.Error(err)
		return nil, err
	}

	instances := reservation[0].Instances
	if len(instances) == 0 {
		err = errors.Errorf("'%s' was not found in reservation '%s'",
			h.Id, *resp.Reservations[0].ReservationId)
		grip.Error(err)
		return nil, err
	}

	return instances[0], nil
}
