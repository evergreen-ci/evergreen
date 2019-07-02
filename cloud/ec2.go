package cloud

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

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
	if s.IsVpc && s.SubnetId == "" {
		return errors.New("must set a default subnet for a vpc")
	}
	if _, err := makeBlockDeviceMappings(s.MountPoints); err != nil {
		return errors.Wrap(err, "block device mappings invalid")
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
	settings    *evergreen.Settings
}

// NewEC2Manager creates a new manager of EC2 spot and on-demand instances.
func NewEC2Manager(opts *EC2ManagerOptions) Manager {
	return &ec2Manager{EC2ManagerOptions: opts}
}

// GetSettings returns a pointer to the manager's configuration settings struct.
func (m *ec2Manager) GetSettings() ProviderSettings {
	return &EC2ProviderSettings{}
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

func (m *ec2Manager) spawnOnDemandHost(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings, blockDevices []*ec2.BlockDeviceMapping) ([]*string, error) {
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
		expanded, err := m.expandUserData(ec2Settings.UserData)
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
	resources := []*string{instance.InstanceId}
	for _, vol := range instance.BlockDeviceMappings {
		if *vol.DeviceName == "" {
			continue
		}

		resources = append(resources, vol.Ebs.VolumeId)
	}
	return resources, nil
}

func (m *ec2Manager) spawnSpotHost(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings, blockDevices []*ec2.LaunchTemplateBlockDeviceMappingRequest) ([]*string, error) {
	// Cleanup
	var templateID *string
	defer func() {
		if templateID != nil {
			// Delete launch template
			_, err := m.client.DeleteLaunchTemplate(ctx, &ec2.DeleteLaunchTemplateInput{LaunchTemplateId: templateID})
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "can't delete launch template",
				"host":     h.Id,
				"template": *templateID,
			}))
		}
	}()

	// Upload a launch template for this host
	launchTemplate := &ec2.RequestLaunchTemplateData{
		ImageId:             aws.String(ec2Settings.AMI),
		KeyName:             aws.String(ec2Settings.KeyName),
		InstanceType:        aws.String(ec2Settings.InstanceType),
		BlockDeviceMappings: blockDevices,
	}

	if ec2Settings.IsVpc {
		launchTemplate.NetworkInterfaces = []*ec2.LaunchTemplateInstanceNetworkInterfaceSpecificationRequest{
			{
				AssociatePublicIpAddress: aws.Bool(true),
				DeviceIndex:              aws.Int64(0),
				Groups:                   ec2Settings.getSecurityGroups(),
				SubnetId:                 &ec2Settings.SubnetId,
			},
		}
		if ec2Settings.IPv6 {
			launchTemplate.NetworkInterfaces[0].SetIpv6AddressCount(1).SetAssociatePublicIpAddress(false)
		}
	} else {
		launchTemplate.SecurityGroups = ec2Settings.getSecurityGroups()
	}

	userData, err := bootstrapUserData(ctx, evergreen.GetEnvironment(), h, ec2Settings.UserData)
	if err != nil {
		return nil, errors.Wrap(err, "could not add bootstrap script to user data")
	}
	ec2Settings.UserData = userData

	if ec2Settings.UserData != "" {
		var expanded string
		expanded, err = m.expandUserData(ec2Settings.UserData)
		if err != nil {
			return nil, errors.Wrap(err, "problem expanding user data")
		}
		userData := base64.StdEncoding.EncodeToString([]byte(expanded))
		launchTemplate.UserData = &userData
	}

	createTemplateResponse, err := m.client.CreateLaunchTemplate(ctx, &ec2.CreateLaunchTemplateInput{
		LaunchTemplateData: launchTemplate,
		// mandatory field may only contain letters, numbers, and the following characters: - ( ) . / _
		LaunchTemplateName: aws.String(fmt.Sprintf("%s", util.RandomString())),
	})
	if err != nil {
		return nil, errors.Wrap(err, "can't upload config template to AWS")
	}
	err = validateCreateTemplateResponse(createTemplateResponse)
	if err != nil {
		return nil, errors.Wrap(err, "invalid create template response")
	}
	templateID = createTemplateResponse.LaunchTemplate.LaunchTemplateId

	// Create a fleet with a single spot instance from the launch template
	createFleetInput := &ec2.CreateFleetInput{
		LaunchTemplateConfigs: []*ec2.FleetLaunchTemplateConfigRequest{
			{
				LaunchTemplateSpecification: &ec2.FleetLaunchTemplateSpecificationRequest{
					LaunchTemplateId: templateID,
					Version:          aws.String(strconv.Itoa(int(*createTemplateResponse.LaunchTemplate.LatestVersionNumber))),
				},
			},
		},
		TargetCapacitySpecification: &ec2.TargetCapacitySpecificationRequest{
			TotalTargetCapacity:       aws.Int64(1),
			DefaultTargetCapacityType: aws.String(ec2.DefaultTargetCapacityTypeSpot),
		},
		Type: aws.String(ec2.FleetTypeInstant),
	}
	createFleetResponse, err := m.client.CreateFleet(ctx, createFleetInput)
	if err != nil {
		return nil, errors.Wrap(err, "error creating fleet")
	}
	err = validateCreateFleetResponse(createFleetResponse)
	if err != nil {
		return nil, errors.Wrap(err, "invalid create fleet response")
	}

	h.Id = *createFleetResponse.Instances[0].InstanceIds[0]
	instanceInfo, err := m.client.GetInstanceInfo(ctx, h.Id)
	if err != nil {
		return nil, errors.Wrap(err, "can't get instance descriptions")
	}

	// return a slice of resources to be tagged
	resources := []*string{createFleetResponse.Instances[0].InstanceIds[0]}
	for _, vol := range instanceInfo.BlockDeviceMappings {
		if *vol.DeviceName == "" {
			continue
		}

		resources = append(resources, vol.Ebs.VolumeId)
	}

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
	if h.Distro.ProviderSettings != nil {
		if err := mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings); err != nil {
			return nil, errors.Wrapf(err, "Error decoding params for distro %s: %+v", h.Distro.Id, ec2Settings)
		}
	}
	grip.Debug(message.Fields{
		"message":   "mapstructure comparison",
		"input":     *h.Distro.ProviderSettings,
		"output":    *ec2Settings,
		"inputraw":  fmt.Sprintf("%#v", *h.Distro.ProviderSettings),
		"outputraw": fmt.Sprintf("%#v", *ec2Settings),
	})
	if err := ec2Settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid EC2 settings in distro %s: and %+v", h.Distro.Id, ec2Settings)
	}

	if ec2Settings.AWSKeyID != "" {
		m.credentials = credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     ec2Settings.AWSKeyID,
			SecretAccessKey: ec2Settings.AWSSecret,
		})
	}

	if ec2Settings.KeyName == "" && !h.UserHost {
		if !h.SpawnOptions.SpawnedByTask {
			return nil, errors.New("key name must not be empty")
		}
		k, err := m.getKey(ctx, h)
		if err != nil {
			return nil, errors.Wrap(err, "not spawning host, problem creating key")
		}
		ec2Settings.KeyName = k
	}

	h.InstanceType = ec2Settings.InstanceType

	r, err := getRegion(h)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting region for host")
	}
	if err = m.client.Create(m.credentials, r); err != nil {
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
		var blockDevices []*ec2.BlockDeviceMapping
		blockDevices, err = makeBlockDeviceMappings(ec2Settings.MountPoints)
		if err != nil {
			return nil, errors.Wrap(err, "error making block device mappings")
		}
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
		var blockDevices []*ec2.LaunchTemplateBlockDeviceMappingRequest
		blockDevices, err = makeBlockDeviceMappingsTemplate(ec2Settings.MountPoints)
		if err != nil {
			return nil, errors.Wrap(err, "error making block device mappings")
		}
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

func (m *ec2Manager) getKey(ctx context.Context, h *host.Host) (string, error) {
	const keyPrefix = "evg_auto_"
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
	name := keyPrefix + t.Project
	newKey, err := m.makeNewKey(ctx, name, h)
	if err != nil {
		return "", errors.Wrap(err, "problem creating new key")
	}
	if err := model.SetAWSKeyForProject(t.Project, &model.AWSSSHKey{Name: name, Value: newKey}); err != nil {
		return "", errors.Wrap(err, "problem setting key")
	}
	return name, nil
}

// GetInstanceStatuses returns the current status of a slice of EC2 instances.
func (m *ec2Manager) GetInstanceStatuses(ctx context.Context, hosts []host.Host) ([]CloudStatus, error) {
	if err := m.client.Create(m.credentials, defaultRegion); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}

	hostIDs := make([]string, 0, len(hosts))
	for i := range hosts {
		hostIDs = append(hostIDs, hosts[i].Id)
	}

	// Get host statuses
	statuses := make([]CloudStatus, 0, len(hosts))
	if len(hostIDs) > 0 {
		out, err := m.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: aws.StringSlice(hostIDs),
		})
		if err != nil {
			return nil, errors.Wrap(err, "error describing instances")
		}
		reservationsMap := map[string]string{}
		for i := range out.Reservations {
			reservationsMap[*out.Reservations[i].Instances[0].InstanceId] = *out.Reservations[i].Instances[0].State.Name
		}
		for i := range hostIDs {
			statuses = append(statuses, ec2StatusToEvergreenStatus(reservationsMap[hostIDs[i]]))
		}
	}

	return statuses, nil
}

func getRegion(h *host.Host) (string, error) {
	ec2Settings := &EC2ProviderSettings{}
	if h.Distro.ProviderSettings != nil {
		if err := mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings); err != nil {
			return "", errors.Wrapf(err, "Error decoding params for distro %s: %+v", h.Distro.Id, ec2Settings)
		}
	}
	r := defaultRegion
	if ec2Settings.Region != "" {
		r = ec2Settings.Region
	}
	return r, nil
}

// GetInstanceStatus returns the current status of an EC2 instance.
func (m *ec2Manager) GetInstanceStatus(ctx context.Context, h *host.Host) (CloudStatus, error) {
	r, err := getRegion(h)
	if err != nil {
		return StatusUnknown, errors.Wrap(err, "problem getting region from host")
	}
	if err = m.client.Create(m.credentials, r); err != nil {
		return StatusUnknown, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

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
}

// TerminateInstance terminates the EC2 instance.
func (m *ec2Manager) TerminateInstance(ctx context.Context, h *host.Host, user string) error {
	// terminate the instance
	if h.Status == evergreen.HostTerminated {
		err := errors.Errorf("Can not terminate %s - already marked as "+
			"terminated!", h.Id)
		return err
	}
	r, err := getRegion(h)
	if err != nil {
		return errors.Wrap(err, "problem getting region from host")
	}
	if err = m.client.Create(m.credentials, r); err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	instanceId := h.Id
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

// OnUp is a noop for ec2
func (m *ec2Manager) OnUp(ctx context.Context, h *host.Host) error {
	return nil
}

func (m *ec2Manager) makeNewKey(ctx context.Context, name string, h *host.Host) (string, error) {
	r, err := getRegion(h)
	if err != nil {
		return "", errors.Wrap(err, "problem getting region from host")
	}
	if err = m.client.Create(m.credentials, r); err != nil {
		return "", errors.Wrap(err, "error creating client")
	}
	_, err = m.client.DeleteKeyPair(ctx, &ec2.DeleteKeyPairInput{KeyName: aws.String(name)})
	if err != nil { // error does not indicate a problem, but log anyway for debugging
		grip.Debug(message.WrapError(err, message.Fields{
			"message":  "problem deleting key",
			"key_name": name,
		}))
	}
	resp, err := m.client.CreateKeyPair(ctx, &ec2.CreateKeyPairInput{KeyName: aws.String(name)})
	if err != nil {
		return "", errors.Wrap(err, "problem creating key pair")
	}
	return *resp.KeyMaterial, nil
}

func (m *ec2Manager) retrieveInstance(ctx context.Context, h *host.Host) (*ec2.Instance, error) {
	var instance *ec2.Instance
	var err error

	r, err := getRegion(h)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting region from host")
	}
	if err = m.client.Create(m.credentials, r); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

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

	// Cache launch time and availability zone in host document, since we
	// have access to this information now. Cost jobs will use this
	// information later.
	h.Zone = *instance.Placement.AvailabilityZone
	h.StartTime = *instance.LaunchTime
	h.VolumeTotalSize, err = getVolumeSize(ctx, m.client, h)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting volume size for host %s", h.Id)
	}
	if err := h.CacheHostData(); err != nil {
		return nil, errors.Wrap(err, "error updating host document in db")
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

func (m *ec2Manager) CostForDuration(ctx context.Context, h *host.Host, start, end time.Time, s *evergreen.Settings) (float64, error) {
	if end.Before(start) || util.IsZeroTime(start) || util.IsZeroTime(end) {
		return 0, errors.New("task timing data is malformed")
	}
	r, err := getRegion(h)
	if err != nil {
		return 0, errors.Wrap(err, "problem getting region from host")
	}
	if err = m.client.Create(m.credentials, r); err != nil {
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

func (m *ec2Manager) expandUserData(userData string) (string, error) {
	exp := util.NewExpansions(m.settings.Expansions)
	expanded, err := exp.ExpandString(userData)
	if err != nil {
		return "", errors.Wrap(err, "error expanding userdata script")
	}
	return expanded, nil
}
