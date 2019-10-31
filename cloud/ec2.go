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

const (
	checkSuccessDelay      = 5 * time.Second
	checkSuccessRetries    = 10
	checkSuccessInitPeriod = time.Second
)

// EC2ManagerOptions are used to construct a new ec2Manager.
type EC2ManagerOptions struct {
	// client is the client library for communicating with AWS.
	client AWSClient

	// provider is the type
	provider ec2ProviderType

	// region is the AWS region specified by distro
	region string
}

// ec2Manager starts and configures instances in EC2.
type ec2Manager struct {
	*EC2ManagerOptions
	credentials *credentials.Credentials
	env         evergreen.Environment
	settings    *evergreen.Settings
}

// GetSettings returns a pointer to the manager's configuration settings struct.
func (m *ec2Manager) GetSettings() ProviderSettings {
	return &EC2ProviderSettings{}
}

// Configure loads credentials or other settings from the config file.
func (m *ec2Manager) Configure(ctx context.Context, settings *evergreen.Settings) error {
	m.settings = settings

	key, secret, err := GetEC2Key(m.region, settings)
	if err != nil {
		return errors.Wrap(err, "Problem getting EC2 keys")
	}

	m.credentials = credentials.NewStaticCredentialsFromCreds(credentials.Value{
		AccessKeyID:     key,
		SecretAccessKey: secret,
	})

	return nil
}

func (m *ec2Manager) spawnOnDemandHost(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings, blockDevices []*ec2.BlockDeviceMapping) error {
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
			return errors.Wrap(err, "problem expanding user data")
		}
		ec2Settings.UserData = expanded
	}

	userData, err := bootstrapUserData(ctx, m.env, h, ec2Settings.UserData)
	if err != nil {
		return errors.Wrap(err, "could not add bootstrap script to user data")
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
		if h.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData {
			grip.Error(message.WrapError(h.DeleteJasperCredentials(ctx, m.env), message.Fields{
				"message": "problem cleaning up user data credentials",
				"host":    h.Id,
				"distro":  h.Distro.Id,
			}))
		}
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
			return errors.Wrap(err, msg)
		}
		msg = "reservation was nil"
		grip.Error(message.Fields{
			"message":       msg,
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		})
		return errors.New(msg)
	}

	if len(reservation.Instances) < 1 {
		if h.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData {
			grip.Error(message.WrapError(h.DeleteJasperCredentials(ctx, m.env), message.Fields{
				"message": "problem cleaning up user data credentials",
				"host":    h.Id,
				"distro":  h.Distro.Id,
			}))
		}
		return errors.New("reservation has no instances")
	}

	instance := reservation.Instances[0]
	grip.Debug(message.Fields{
		"message":       "started ec2 instance",
		"host":          *instance.InstanceId,
		"distro":        h.Distro.Id,
		"host_provider": h.Distro.Provider,
	})
	h.Id = *instance.InstanceId

	return nil
}

func (m *ec2Manager) spawnSpotHost(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings, blockDevices []*ec2.BlockDeviceMapping) error {
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
			return errors.Wrap(err, "problem expanding user data")
		}
		ec2Settings.UserData = expanded
	}

	userData, err := bootstrapUserData(ctx, m.env, h, ec2Settings.UserData)
	if err != nil {
		return errors.Wrap(err, "could not add bootstrap script to user data")
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
		if h.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData {
			grip.Error(message.WrapError(h.DeleteJasperCredentials(ctx, m.env), message.Fields{
				"message": "problem cleaning up user data credentials",
				"host":    h.Id,
				"distro":  h.Distro.Id,
			}))
		}
		grip.Error(errors.Wrapf(h.Remove(), "error removing intent host %s", h.Id))
		return errors.Wrap(err, "RequestSpotInstances API call returned an error")
	}

	spotReqRes := spotResp.SpotInstanceRequests[0]
	if *spotReqRes.State != SpotStatusOpen && *spotReqRes.State != SpotStatusActive {
		if h.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData {
			grip.Error(message.WrapError(h.DeleteJasperCredentials(ctx, m.env), message.Fields{
				"message": "problem cleaning up user data credentials",
				"host":    h.Id,
				"distro":  h.Distro.Id,
			}))
		}
		err = errors.Errorf("Spot request %s was found in state %s on intent host %s",
			*spotReqRes.SpotInstanceRequestId, *spotReqRes.State, h.Id)
		return err
	}

	h.Id = *spotReqRes.SpotInstanceRequestId
	return nil
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

	if h.InstanceType != "" {
		ec2Settings.InstanceType = h.InstanceType
	} else {
		h.InstanceType = ec2Settings.InstanceType
	}
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
		err := m.spawnOnDemandHost(ctx, h, ec2Settings, blockDevices)
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
		err = m.spawnSpotHost(ctx, h, ec2Settings, blockDevices)
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

	event.LogHostStarted(h.Id)
	return h, nil
}

// getResources returns a slice of the AWS resources for the given host
func (m *ec2Manager) getResources(ctx context.Context, h *host.Host) ([]string, error) {
	instanceID := h.Id
	if isHostSpot(h) {
		instanceID, err := m.client.GetSpotInstanceId(ctx, h)
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error getting spot request info",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get spot request info for %s", h.Id)
		}
		if instanceID == "" {
			return nil, errors.WithStack(errors.New("spot instance does not yet have an instanceId"))
		}
	}
	volumeIDs, err := m.client.GetVolumeIDs(ctx, h)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get volume IDs for '%s'", h.Id)
	}
	resources := []string{instanceID}
	resources = append(resources, volumeIDs...)
	return resources, nil
}

// addTags adds or updates the specified tags in the client and db
func (m *ec2Manager) addTags(ctx context.Context, h *host.Host, tags []host.Tag) error {
	resources, err := m.getResources(ctx, h)
	if err != nil {
		return errors.Wrap(err, "error getting host resources")
	}
	createTagSlice := make([]*ec2.Tag, len(tags))
	for i := range tags {
		createTagSlice[i] = &ec2.Tag{Key: &tags[i].Key, Value: &tags[i].Value}
	}
	_, err = m.client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: aws.StringSlice(resources),
		Tags:      createTagSlice,
	})
	if err != nil {
		return errors.Wrapf(err, "error creating tags using client for '%s'", h.Id)
	}
	h.AddTags(tags)

	return errors.Wrapf(h.SetTags(), "error creating tags in db for '%s'", h.Id)
}

// deleteTags removes the specified tags by their keys in the client and db
func (m *ec2Manager) deleteTags(ctx context.Context, h *host.Host, keys []string) error {
	resources, err := m.getResources(ctx, h)
	if err != nil {
		return errors.Wrap(err, "error getting host resources")
	}
	deleteTagSlice := make([]*ec2.Tag, len(keys))
	for i := range keys {
		deleteTagSlice[i] = &ec2.Tag{Key: &keys[i]}
	}
	_, err = m.client.DeleteTags(ctx, &ec2.DeleteTagsInput{
		Resources: aws.StringSlice(resources),
		Tags:      deleteTagSlice,
	})
	if err != nil {
		return errors.Wrapf(err, "error deleting tags using client for '%s'", h.Id)
	}
	h.DeleteTags(keys)

	return errors.Wrapf(h.SetTags(), "error deleting tags in db for '%s'", h.Id)
}

// setInstanceType changes the instance type in the client and db
func (m *ec2Manager) setInstanceType(ctx context.Context, h *host.Host, instanceType string) error {
	_, err := m.client.ModifyInstanceAttribute(ctx, &ec2.ModifyInstanceAttributeInput{
		InstanceId: aws.String(h.Id),
		InstanceType: &ec2.AttributeValue{
			Value: aws.String(instanceType),
		},
	})
	if err != nil {
		return errors.Wrapf(err, "error changing instance type using client for '%s'", h.Id)
	}

	return errors.Wrapf(h.SetInstanceType(instanceType), "error changing instance type in db for '%s'", h.Id)
}

// setNoExpiration changes whether a host should expire
func (m *ec2Manager) setNoExpiration(ctx context.Context, h *host.Host, noExpiration bool) error {
	resources, err := m.getResources(ctx, h)
	if err != nil {
		return errors.Wrap(err, "error getting host resources")
	}
	expireOnValue := expireInDays(spawnHostExpireDays)
	_, err = m.client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: aws.StringSlice(resources),
		Tags: []*ec2.Tag{
			{
				Key:   aws.String("expire-on"),
				Value: aws.String(expireOnValue),
			},
		},
	})
	if err != nil {
		return errors.Wrapf(err, "error changing expire-on tag using client for '%s", h.Id)
	}

	if noExpiration {
		return errors.Wrapf(h.MarkShouldNotExpire(expireOnValue), "error marking host should not expire in db for '%s'", h.Id)
	}
	return errors.Wrapf(h.MarkShouldExpire(expireOnValue), "error marking host should in db for '%s'", h.Id)
}

// extendExpiration extends a host's expiration time by the number of hours specified
func (m *ec2Manager) extendExpiration(ctx context.Context, h *host.Host, extension time.Duration) error {
	return errors.Wrapf(h.SetExpirationTime(h.ExpirationTime.Add(extension)), "error extending expiration time in db for '%s'", h.Id)
}

// ModifyHost modifies a spawn host according to the changes specified by a HostModifyOptions struct.
func (m *ec2Manager) ModifyHost(ctx context.Context, h *host.Host, opts host.HostModifyOptions) error {
	ec2Settings := &EC2ProviderSettings{}
	if err := ec2Settings.fromDistroSettings(h.Distro); err != nil {
		return errors.Wrap(err, "error getting EC2 settings")
	}
	if err := m.client.Create(m.credentials, ec2Settings.getRegion()); err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	// Validate modify options for user errors that should prevent all modifications
	if err := validateEC2HostModifyOptions(h, opts); err != nil {
		return errors.Wrap(err, "error validating EC2 host modify options")
	}

	// Attempt all requested modifications and catch errors from client or db
	catcher := grip.NewBasicCatcher()
	if opts.InstanceType != "" {
		catcher.Add(m.setInstanceType(ctx, h, opts.InstanceType))
	}
	if len(opts.DeleteInstanceTags) > 0 {
		catcher.Add(m.deleteTags(ctx, h, opts.DeleteInstanceTags))
	}
	if len(opts.AddInstanceTags) > 0 {
		catcher.Add(m.addTags(ctx, h, opts.AddInstanceTags))
	}
	if opts.NoExpiration != nil {
		catcher.Add(m.setNoExpiration(ctx, h, *opts.NoExpiration))
	}
	if opts.AddHours > 0 {
		catcher.Add(m.extendExpiration(ctx, h, opts.AddHours))
	}

	return catcher.Resolve()
}

// GetInstanceStatuses returns the current status of a slice of EC2 instances.
func (m *ec2Manager) GetInstanceStatuses(ctx context.Context, hosts []host.Host) ([]CloudStatus, error) {
	if err := m.client.Create(m.credentials, defaultRegion); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}

	spotHosts := []*host.Host{}
	instanceIdToHostMap := map[string]*host.Host{}
	hostToStatusMap := map[string]CloudStatus{}
	hostsToCheck := []*string{}

	// Populate spot and on-demand slices
	for i := range hosts {
		if isHostOnDemand(&hosts[i]) {
			instanceIdToHostMap[hosts[i].Id] = &hosts[i]
			hostsToCheck = append(hostsToCheck, &hosts[i].Id)
		}
		if isHostSpot(&hosts[i]) {
			if hosts[i].ExternalIdentifier != "" {
				instanceIdToHostMap[hosts[i].ExternalIdentifier] = &hosts[i]
				hostsToCheck = append(hostsToCheck, &hosts[i].ExternalIdentifier)
			} else {
				spotHosts = append(spotHosts, &hosts[i])
			}
		}
	}

	// Get instance IDs for spot instances
	if len(spotHosts) > 0 {
		spotOut, err := m.client.DescribeSpotRequestsAndSave(ctx, spotHosts)
		if err != nil {
			return nil, errors.Wrap(err, "error describing spot instances")
		}
		if len(spotOut.SpotInstanceRequests) != len(spotHosts) {
			return nil, errors.New("programmer error: length of spot instance requests != length of spot host IDs")
		}
		spotInstanceRequestsMap := map[string]*ec2.SpotInstanceRequest{}
		for i := range spotOut.SpotInstanceRequests {
			spotInstanceRequestsMap[*spotOut.SpotInstanceRequests[i].SpotInstanceRequestId] = spotOut.SpotInstanceRequests[i]
		}
		for i := range spotHosts {
			if spotInstanceRequestsMap[spotHosts[i].Id].InstanceId == nil || *spotInstanceRequestsMap[spotHosts[i].Id].InstanceId == "" {
				hostToStatusMap[spotHosts[i].Id] = cloudStatusFromSpotStatus(*spotInstanceRequestsMap[spotHosts[i].Id].State)
				continue
			}
			hostsToCheck = append(hostsToCheck, spotInstanceRequestsMap[spotHosts[i].Id].InstanceId)
			instanceIdToHostMap[*spotInstanceRequestsMap[spotHosts[i].Id].InstanceId] = spotHosts[i]
		}
	}

	// Get host statuses
	if len(hostsToCheck) > 0 {
		out, err := m.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: hostsToCheck,
		})
		if err != nil {
			return nil, errors.Wrap(err, "error describing instances")
		}
		if err = validateEc2DescribeInstancesOutput(out); err != nil {
			return nil, errors.Wrap(err, "invalid describe instances response")
		}
		reservationsMap := map[string]*ec2.Instance{}
		for i := range out.Reservations {
			reservationsMap[*out.Reservations[i].Instances[0].InstanceId] = out.Reservations[i].Instances[0]
		}
		for i := range hostsToCheck {
			instance, ok := reservationsMap[*hostsToCheck[i]]
			if !ok {
				return nil, errors.Errorf("host '%s' not included in DescribeInstances response", *hostsToCheck[i])
			}
			status := ec2StatusToEvergreenStatus(*instance.State.Name)
			if status == StatusRunning {
				// cache instance information so we can make fewer calls to AWS's API
				if err = cacheHostData(ctx, instanceIdToHostMap[*hostsToCheck[i]], instance, m.client); err != nil {
					return nil, errors.Wrapf(err, "can't cache host data for '%s'", *hostsToCheck[i])
				}
			}
			hostToStatusMap[instanceIdToHostMap[*hostsToCheck[i]].Id] = status
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
	status := StatusUnknown

	ec2Settings := &EC2ProviderSettings{}
	err := ec2Settings.fromDistroSettings(h.Distro)
	if err != nil {
		return status, errors.Wrap(err, "problem getting settings from host")
	}
	if err := m.client.Create(m.credentials, ec2Settings.getRegion()); err != nil {
		return status, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	id := h.Id
	if isHostSpot(h) {
		if h.ExternalIdentifier != "" {
			id = h.ExternalIdentifier
		} else {
			spotDetails, err := m.client.DescribeSpotRequestsAndSave(ctx, []*host.Host{h})
			if err != nil {
				err = errors.Wrapf(err, "failed to get spot request info for %s", h.Id)
				return status, err
			}
			if len(spotDetails.SpotInstanceRequests) == 0 {
				return status, errors.Errorf("'%s' has no corresponding spot instance request", h.Id)
			}
			spotInstance := spotDetails.SpotInstanceRequests[0]
			if spotInstance.InstanceId == nil || *spotInstance.InstanceId == "" {
				return cloudStatusFromSpotStatus(*spotInstance.State), nil
			}

			id = *spotInstance.InstanceId
		}
	}

	instance, err := m.client.GetInstanceInfo(ctx, id)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error getting instance info",
			"host":          h.Id,
			"instance_id":   id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return status, err
	}
	status = ec2StatusToEvergreenStatus(*instance.State.Name)

	if status == StatusRunning {
		// cache instance information so we can make fewer calls to AWS's API
		if err = cacheHostData(ctx, h, instance, m.client); err != nil {
			return status, errors.Wrapf(err, "can't cache host data for '%s'", h.Id)
		}
	}

	return status, nil
}

// TerminateInstance terminates the EC2 instance.
func (m *ec2Manager) TerminateInstance(ctx context.Context, h *host.Host, user, reason string) error {
	// terminate the instance
	if h.Status == evergreen.HostTerminated {
		err := errors.Errorf("Can not terminate %s - already marked as "+
			"terminated!", h.Id)
		return err
	}

	if h.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData {
		grip.Error(message.WrapError(h.DeleteJasperCredentials(ctx, m.env), message.Fields{
			"message": "problem deleting Jasper credentials during host termination",
			"host":    h.Id,
			"distro":  h.Distro.Id,
		}))
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
			return errors.Wrap(h.Terminate(user, "spot request was not fulfilled"), "failed to terminate instance in db")
		}
	}

	if !strings.HasPrefix(instanceId, "i-") {
		return errors.Wrap(h.Terminate(user, fmt.Sprintf("detected invalid instance ID %s", instanceId)), "failed to terminate instance in db")
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

	return errors.Wrap(h.Terminate(user, reason), "failed to terminate instance in db")
}

// StopInstance stops a running EC2 instance.
func (m *ec2Manager) StopInstance(ctx context.Context, h *host.Host, user string) error {
	// Check if already stopped or not running
	if h.Status == evergreen.HostStopped {
		return errors.Errorf("cannot stop '%s' - already marked as stopped", h.Id)
	} else if h.Status != evergreen.HostRunning {
		return errors.Errorf("cannot stop '%s' - host is not running", h.Id)
	}

	ec2Settings := &EC2ProviderSettings{}
	if err := ec2Settings.fromDistroSettings(h.Distro); err != nil {
		return errors.Wrap(err, "problem getting settings from host")
	}
	if err := m.client.Create(m.credentials, ec2Settings.getRegion()); err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	if err := h.SetStopping(user); err != nil {
		return errors.Wrap(err, "failed to mark instance as stopping in db")
	}

	_, err := m.client.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []*string{aws.String(h.Id)},
	})
	if err != nil {
		return errors.Wrapf(err, "error stopping EC2 instance '%s'", h.Id)
	}

	// Delay exponential backoff
	time.Sleep(checkSuccessDelay)

	// Check whether instance stopped
	err = util.Retry(
		ctx,
		func() (bool, error) {
			instance, err := m.client.GetInstanceInfo(ctx, h.Id)
			if err != nil {
				return false, errors.Wrap(err, "error getting instance info")
			}
			if ec2StatusToEvergreenStatus(*instance.State.Name) == StatusStopped {
				return false, nil
			}
			return true, errors.New("host is not stopped")
		}, checkSuccessRetries, checkSuccessInitPeriod, 0)

	if err != nil {
		return errors.Wrap(err, "error checking if spawnhost stopped")
	}

	grip.Info(message.Fields{
		"message":       "stopped instance",
		"user":          user,
		"host_provider": h.Distro.Provider,
		"host":          h.Id,
		"distro":        h.Distro.Id,
	})

	return errors.Wrap(h.SetStopped(user), "failed to mark instance as stopped in db")
}

// StartInstance starts a stopped EC2 instance.
func (m *ec2Manager) StartInstance(ctx context.Context, h *host.Host, user string) error {
	// Check that target instance is stopped
	if h.Status != evergreen.HostStopped {
		return errors.Errorf("cannot start '%s' - host is not stopped", h.Id)
	}

	ec2Settings := &EC2ProviderSettings{}
	if err := ec2Settings.fromDistroSettings(h.Distro); err != nil {
		return errors.Wrap(err, "problem getting settings from host")
	}
	if err := m.client.Create(m.credentials, ec2Settings.getRegion()); err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	// Make request to start the instance
	_, err := m.client.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []*string{aws.String(h.Id)},
	})
	if err != nil {
		return errors.Wrapf(err, "error starting EC2 instance '%s'", h.Id)
	}

	// Check whether instance is running
	err = util.Retry(
		ctx,
		func() (bool, error) {
			instance, err := m.client.GetInstanceInfo(ctx, h.Id)
			if err != nil {
				return false, errors.Wrap(err, "error getting instance info")
			}
			if ec2StatusToEvergreenStatus(*instance.State.Name) == StatusRunning {
				return false, nil
			}
			return true, errors.New("host is not started")
		}, checkSuccessRetries, checkSuccessInitPeriod, 0)

	if err != nil {
		return errors.Wrap(err, "error checking if spawnhost started")
	}

	grip.Info(message.Fields{
		"message":       "started instance",
		"user":          user,
		"host_provider": h.Distro.Provider,
		"host":          h.Id,
		"distro":        h.Distro.Id,
	})

	return errors.Wrap(h.SetRunning(user), "failed to mark instance as running in db")
}

func (m *ec2Manager) cancelSpotRequest(ctx context.Context, h *host.Host) (string, error) {
	instanceId, err := m.client.GetSpotInstanceId(ctx, h)
	if err != nil {
		if ec2err, ok := err.(awserr.Error); ok {
			if ec2err.Code() == EC2ErrorSpotRequestNotFound {
				return "", h.Terminate(evergreen.User, "unable to find spot request")
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
				return "", h.Terminate(evergreen.User, "unable to find spot request")
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
	grip.Debug(message.Fields{
		"message":       "host is up",
		"host":          h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
		"is_spot":       isHostSpot(h),
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

	resources, err := m.getResources(ctx, h)
	if err != nil {
		return errors.Wrap(err, "error getting resources")
	}

	if err = m.client.SetTags(ctx, resources, h); err != nil {
		return errors.Wrap(err, "error settings tags")
	}

	return nil
}

func (m *ec2Manager) AttachVolume(ctx context.Context, h *host.Host, attachment *host.VolumeAttachment) error {
	if err := m.client.Create(m.credentials, evergreen.DefaultEC2Region); err != nil {
		return errors.Wrap(err, "error creating client")
	}

	_, err := m.client.AttachVolume(ctx, &ec2.AttachVolumeInput{
		InstanceId: aws.String(h.Id),
		Device:     aws.String(attachment.DeviceName),
		VolumeId:   aws.String(attachment.VolumeID),
	})
	if err != nil {
		return errors.Wrapf(err, "error attaching volume '%s' to host '%s'", attachment.VolumeID, h.Id)
	}

	return errors.Wrapf(h.AddVolumeToHost(attachment), "error attaching volume '%s' to host '%s' in db", attachment.VolumeID, h.Id)
}

func (m *ec2Manager) DetachVolume(ctx context.Context, h *host.Host, volumeID string) error {
	if err := m.client.Create(m.credentials, evergreen.DefaultEC2Region); err != nil {
		return errors.Wrap(err, "error creating client")
	}

	_, err := m.client.DetachVolume(ctx, &ec2.DetachVolumeInput{
		InstanceId: aws.String(h.Id),
		VolumeId:   aws.String(volumeID),
	})
	if err != nil {
		return errors.Wrapf(err, "error attaching volume '%s' to host '%s' in client", volumeID, h.Id)
	}

	return errors.Wrapf(h.RemoveVolumeFromHost(volumeID), "error detaching volume '%s' from host '%s' in db", volumeID, h.Id)
}

func (m *ec2Manager) CreateVolume(ctx context.Context, volume *host.Volume) (*host.Volume, error) {
	if err := m.client.Create(m.credentials, evergreen.DefaultEC2Region); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}
	resp, err := m.client.CreateVolume(ctx, &ec2.CreateVolumeInput{
		AvailabilityZone: aws.String(volume.AvailabilityZone),
		VolumeType:       aws.String(volume.Type),
		Size:             aws.Int64(int64(volume.Size)),
	})

	if err != nil {
		return nil, errors.Wrap(err, "error creating volume in client")
	}
	if resp.VolumeId == nil {
		return nil, errors.New("new volume returned by EC2 does not have an ID")
	}

	volume.ID = *resp.VolumeId
	if err = volume.Insert(); err != nil {
		return nil, errors.Wrap(err, "error creating volume in db")
	}

	return volume, nil
}

func (m *ec2Manager) DeleteVolume(ctx context.Context, volume *host.Volume) error {
	if err := m.client.Create(m.credentials, evergreen.DefaultEC2Region); err != nil {
		return errors.Wrap(err, "error creating client")
	}

	_, err := m.client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
		VolumeId: aws.String(volume.ID),
	})
	if err != nil {
		return errors.Wrapf(err, "error deleting volume '%s' in client", volume.ID)
	}

	return errors.Wrapf(volume.Remove(), "error deleting volume '%s' in db", volume.ID)
}

// GetDNSName returns the DNS name for the host.
func (m *ec2Manager) GetDNSName(ctx context.Context, h *host.Host) (string, error) {
	ec2Settings := &EC2ProviderSettings{}
	err := ec2Settings.fromDistroSettings(h.Distro)
	if err != nil {
		return "", errors.Wrap(err, "problem getting region from host")
	}
	if err = m.client.Create(m.credentials, ec2Settings.getRegion()); err != nil {
		return "", errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	return m.client.GetPublicDNSName(ctx, h)
}

// TimeTilNextPayment returns how long until the next payment is due for a host.
func (m *ec2Manager) TimeTilNextPayment(host *host.Host) time.Duration {
	return timeTilNextEC2Payment(host)
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

func (m *ec2Manager) CostForDuration(ctx context.Context, h *host.Host, start, end time.Time) (float64, error) {
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
