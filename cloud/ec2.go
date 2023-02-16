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
	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

func isHostSpot(h *host.Host) bool {
	return h.Distro.Provider == evergreen.ProviderNameEc2Spot
}
func isHostOnDemand(h *host.Host) bool {
	return h.Distro.Provider == evergreen.ProviderNameEc2OnDemand
}

// EC2ProviderSettings describes properties of managed instances.
type EC2ProviderSettings struct {
	// Region is the EC2 region in which the instance will start. Empty is equivalent to the Evergreen default region.
	// This should remain one of the first fields to speed up the birch document iterator.
	Region string `mapstructure:"region" json:"region" bson:"region,omitempty"`

	// AMI is the AMI ID.
	AMI string `mapstructure:"ami" json:"ami,omitempty" bson:"ami,omitempty"`

	// If set, overrides key from credentials
	AWSKeyID string `mapstructure:"aws_access_key_id" json:"aws_access_key_id,omitempty" bson:"aws_access_key_id,omitempty"`

	// If set, overrides secret from credentials
	AWSSecret string `mapstructure:"aws_secret_access_key" json:"aws_access_key,omitempty" bson:"aws_secret_access_key,omitempty"`

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

	// FallbackToOnDemand is set to true if on-demand should be tried in the case of capacity errors
	// from spot/fleet instance creation.
	FallbackToOnDemand bool `mapstructure:"fallback" json:"fallback" bson:"fallback"`

	// BidPrice is the price we are willing to pay for a spot instance.
	BidPrice float64 `mapstructure:"bid_price" json:"bid_price,omitempty" bson:"bid_price,omitempty"`

	// UserData specifies configuration that runs after the instance starts.
	UserData string `mapstructure:"user_data" json:"user_data,omitempty" bson:"user_data,omitempty"`

	// MergeUserDataParts specifies whether multiple user data parts should be
	// merged into a single user data part.
	// EVG-7760: This is primarily a workaround for a problem with Windows not
	// allowing multiple scripts of the same type as part of a multipart user
	// data upload.
	MergeUserDataParts bool `mapstructure:"merge_user_data_parts" json:"merge_user_data_parts,omitempty" bson:"merge_user_data_parts,omitempty"`

	// FleetOptions specifies options for creating host with Fleet. It is ignored by other managers.
	FleetOptions FleetConfig `mapstructure:"fleet_options" json:"fleet_options,omitempty" bson:"fleet_options,omitempty"`
}

// Validate that essential EC2ProviderSettings fields are not empty.
func (s *EC2ProviderSettings) Validate() error {
	catcher := grip.NewBasicCatcher()

	if s.AMI == "" {
		catcher.New("AMI must not be empty")
	}

	if s.InstanceType == "" {
		catcher.New("instance type must not be empty")
	}

	if len(s.SecurityGroupIDs) == 0 {
		catcher.New("Security groups must not be empty")
	}

	if s.BidPrice < 0 {
		catcher.New("bid price must not be negative")
	}

	if s.IsVpc && s.SubnetId == "" {
		catcher.New("must set a default subnet for a VPC")
	}

	_, err := makeBlockDeviceMappings(s.MountPoints)
	catcher.Wrap(err, "block device mappings invalid")

	if s.UserData != "" {
		_, err = parseUserData(s.UserData)
		catcher.Wrap(err, "user data is malformed")
	}

	catcher.Wrap(s.FleetOptions.validate(), "invalid fleet options")

	return catcher.Resolve()
}

// region is only provided if we want to filter by region
func (s *EC2ProviderSettings) FromDistroSettings(d distro.Distro, region string) error {
	if len(d.ProviderSettingsList) != 0 {
		settingsDoc, err := d.GetProviderSettingByRegion(region)
		if err != nil {
			return errors.Wrapf(err, "providers list doesn't contain region '%s'", region)
		}
		return s.FromDocument(settingsDoc)
	}
	return nil
}

func (s *EC2ProviderSettings) ToDocument() (*birch.Document, error) {
	s.Region = s.getRegion()
	bytes, err := bson.Marshal(s)
	if err != nil {
		return nil, errors.Wrap(err, "marshalling provider setting into BSON")
	}
	doc := birch.Document{}
	if err = doc.UnmarshalBSON(bytes); err != nil {
		return nil, errors.Wrap(err, "umarshalling settings bytes into document")
	}
	return &doc, nil
}

func (s *EC2ProviderSettings) FromDocument(doc *birch.Document) error {
	bytes, err := doc.MarshalBSON()
	if err != nil {
		return errors.Wrap(err, "marshalling provider setting into BSON")
	}
	if err := bson.Unmarshal(bytes, s); err != nil {
		return errors.Wrap(err, "unmarshalling BSON into EC2 provider settings")
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
	return evergreen.DefaultEC2Region
}

// FleetConfig specifies how the EC2 Fleet manager should spawn hosts.
type FleetConfig struct {
	// UseOnDemand will cause Fleet to use on-demand instances to instantiate hosts. Defaults to spot instances.
	UseOnDemand bool `mapstructure:"use_on_demand" json:"use_on_demand,omitempty" bson:"use_on_demand,omitempty"`

	// UseCapacityOptimized will cause Fleet to use the capacity-optimized allocation strategy for spawning hosts. Defaults to the AWS default (lowest-cost).
	// See https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-fleet-allocation-strategy.html for more information about Fleet allocation strategies.
	UseCapacityOptimized bool `mapstructure:"use_capacity_optimized" json:"use_capacity_optimized,omitempty" bson:"use_capacity_optimized,omitempty"`
}

func (f *FleetConfig) awsTargetCapacityType() *string {
	if f.UseOnDemand {
		return aws.String(ec2.DefaultTargetCapacityTypeOnDemand)
	}

	return aws.String(ec2.DefaultTargetCapacityTypeSpot)
}

func (f *FleetConfig) awsAllocationStrategy() *string {
	if !f.UseOnDemand && f.UseCapacityOptimized {
		return aws.String(ec2.SpotAllocationStrategyCapacityOptimized)
	}

	return nil
}

func (f *FleetConfig) validate() error {
	if f.UseOnDemand && f.UseCapacityOptimized {
		return errors.New("on-demand instances can't use the capacity-optimized allocation strategy")
	}

	return nil
}

type ec2ProviderType int

const (
	onDemandProvider ec2ProviderType = iota
	spotProvider
)

const (
	SpotStatusOpen     = "open"
	SpotStatusActive   = "active"
	SpotStatusClosed   = "closed"
	SpotStatusCanceled = "cancelled"
	SpotStatusFailed   = "failed"

	EC2ErrorSpotRequestNotFound = "InvalidSpotInstanceRequestID.NotFound"

	defaultIops = 3000
)

const (
	checkSuccessAttempts   = 12
	checkSuccessInitPeriod = 2 * time.Second
	checkSuccessMaxDelay   = 2 * time.Minute
)

const (
	VolumeTypeStandard = "standard"
	VolumeTypeIo1      = "io1"
	VolumeTypeGp3      = "gp3"
	VolumeTypeGp2      = "gp2"
	VolumeTypeSc1      = "sc1"
	VolumeTypeSt1      = "st1"
)

var (
	ValidVolumeTypes = []string{
		VolumeTypeStandard,
		VolumeTypeIo1,
		VolumeTypeGp3,
		VolumeTypeGp2,
		VolumeTypeSc1,
		VolumeTypeSt1,
	}
)

// EC2ManagerOptions are used to construct a new ec2Manager.
type EC2ManagerOptions struct {
	// client is the client library for communicating with AWS.
	client AWSClient

	// provider is the type
	provider ec2ProviderType

	// region is the AWS region specified by distro
	region string

	// providerKey is the AWS credential key
	providerKey string

	// providerSecret is the AWS credential secret
	providerSecret string
}

// ec2Manager starts and configures instances in EC2.
type ec2Manager struct {
	*EC2ManagerOptions
	credentials *credentials.Credentials
	env         evergreen.Environment
	settings    *evergreen.Settings
}

// Configure loads credentials or other settings from the config file.
func (m *ec2Manager) Configure(ctx context.Context, settings *evergreen.Settings) error {
	m.settings = settings

	if m.region == "" {
		m.region = evergreen.DefaultEC2Region
	}

	var err error
	m.providerKey, m.providerSecret, err = GetEC2Key(settings)
	if err != nil {
		return errors.Wrap(err, "getting EC2 keys")
	}
	if m.providerKey == "" || m.providerSecret == "" {
		return errors.New("provider key/secret can't be empty")
	}

	m.credentials = credentials.NewStaticCredentialsFromCreds(credentials.Value{
		AccessKeyID:     m.providerKey,
		SecretAccessKey: m.providerSecret,
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
		TagSpecifications:   makeTagSpecifications(makeTags(h)),
	}

	if ec2Settings.IsVpc {
		input.NetworkInterfaces = []*ec2.InstanceNetworkInterfaceSpecification{
			{
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
			return errors.Wrap(err, "expanding user data")
		}
		ec2Settings.UserData = expanded
	}

	settings := *m.settings
	// Use the latest service flags instead of those cached in the environment.
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return errors.Wrap(err, "getting service flags")
	}
	settings.ServiceFlags = *flags
	userData, err := makeUserData(ctx, &settings, h, ec2Settings.UserData, ec2Settings.MergeUserDataParts)
	if err != nil {
		return errors.Wrap(err, "making user data")
	}
	ec2Settings.UserData = userData

	if ec2Settings.UserData != "" {
		if err = validateUserDataSize(ec2Settings.UserData, h.Distro.Id); err != nil {
			return errors.WithStack(err)
		}
		userData := base64.StdEncoding.EncodeToString([]byte(ec2Settings.UserData))
		input.UserData = &userData
	}

	grip.Debug(message.Fields{
		"message":       "starting on-demand instance",
		"args":          input,
		"host_id":       h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
	})
	reservation, err := m.client.RunInstances(ctx, input)
	if err != nil || reservation == nil {
		if err == EC2InsufficientCapacityError {
			// try again in another AZ
			if subnetErr := m.setNextSubnet(ctx, h); subnetErr == nil {
				msg := "got EC2InsufficientCapacityError"
				grip.Info(message.Fields{
					"message":       msg,
					"action":        "retrying",
					"host_id":       h.Id,
					"host_provider": h.Distro.Provider,
					"distro":        h.Distro.Id,
				})
				return errors.Wrap(err, msg)
			} else {
				grip.Error(message.WrapError(subnetErr, message.Fields{
					"message":       "couldn't increment subnet",
					"host_id":       h.Id,
					"host_provider": h.Distro.Provider,
					"distro":        h.Distro.Id,
				}))
			}
		}

		if h.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData {
			grip.Error(message.WrapError(h.DeleteJasperCredentials(ctx, m.env), message.Fields{
				"message": "problem cleaning up user data credentials",
				"host_id": h.Id,
				"distro":  h.Distro.Id,
			}))
		}
		grip.WarningWhen(err == EC2InsufficientCapacityError, message.WrapError(err, message.Fields{
			"message":       "RunInstances API call encountered insufficient capacity",
			"action":        "removing",
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		msg := "RunInstances API call returned an error"
		grip.ErrorWhen(err != EC2InsufficientCapacityError, message.WrapError(err, message.Fields{
			"message":       msg,
			"action":        "removing",
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		grip.Error(message.WrapError(h.Remove(), message.Fields{
			"message":       "error removing intent host",
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		if h.SpawnOptions.SpawnedByTask {
			detailErr := task.AddHostCreateDetails(h.StartedBy, h.Id, h.SpawnOptions.TaskExecutionNumber, err)
			grip.Error(message.WrapError(detailErr, message.Fields{
				"message":       "error adding host create error details",
				"host_id":       h.Id,
				"host_provider": h.Distro.Provider,
				"distro":        h.Distro.Id,
			}))
		}
		if err != nil {
			return errors.Wrap(err, msg)
		}
		msg = "reservation was nil"
		grip.Error(message.Fields{
			"message":       msg,
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		})
		return errors.New(msg)
	}

	if len(reservation.Instances) < 1 {
		if h.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData {
			grip.Error(message.WrapError(h.DeleteJasperCredentials(ctx, m.env), message.Fields{
				"message": "problem cleaning up user data credentials",
				"host_id": h.Id,
				"distro":  h.Distro.Id,
			}))
		}
		return errors.New("reservation has no instances")
	}

	instance := reservation.Instances[0]
	grip.Debug(message.Fields{
		"message":       "started ec2 instance",
		"host_id":       *instance.InstanceId,
		"distro":        h.Distro.Id,
		"host_provider": h.Distro.Provider,
	})
	h.Id = *instance.InstanceId

	return nil
}

// setNextSubnet sets the subnet in the host's cached distro to the next one that supports this instance type.
// If the current subnet doesn't support this instance type it's set to the first that does.
func (m *ec2Manager) setNextSubnet(ctx context.Context, h *host.Host) error {
	ec2Settings := &EC2ProviderSettings{}
	if err := ec2Settings.FromDistroSettings(h.Distro, m.region); err != nil {
		return errors.Wrap(err, "getting provider settings")
	}

	supportingSubnets, err := typeCache.subnetsWithInstanceType(ctx, m.settings, m.client, instanceRegionPair{instanceType: h.InstanceType, region: ec2Settings.getRegion()})
	if err != nil {
		return errors.Wrapf(err, "getting supported subnets for instance type '%s'", h.InstanceType)
	}
	if len(supportingSubnets) == 0 {
		return errors.Errorf("instance type '%s' is not supported by any configured subnet for region '%s'", h.InstanceType, ec2Settings.getRegion())
	}

	if len(supportingSubnets) == 1 && supportingSubnets[0].SubnetID == ec2Settings.SubnetId {
		return errors.Errorf("no other subnets support '%s'", h.InstanceType)
	}

	nextSubnetIndex := 0
	for i, subnet := range supportingSubnets {
		if subnet.SubnetID == ec2Settings.SubnetId {
			nextSubnetIndex = i + 1
			break
		}
	}

	ec2Settings.SubnetId = supportingSubnets[nextSubnetIndex%len(supportingSubnets)].SubnetID
	newSettingsDocument, err := ec2Settings.ToDocument()
	if err != nil {
		return errors.Wrap(err, "convert provider settings to document")
	}

	return h.UpdateCachedDistroProviderSettings([]*birch.Document{newSettingsDocument})
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
			{
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
			return errors.Wrap(err, "expanding user data")
		}
		ec2Settings.UserData = expanded
	}

	settings := *m.settings
	// Use the latest service flags instead of those cached in the environment.
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return errors.Wrap(err, "getting service flags")
	}
	settings.ServiceFlags = *flags
	userData, err := makeUserData(ctx, &settings, h, ec2Settings.UserData, ec2Settings.MergeUserDataParts)
	if err != nil {
		return errors.Wrap(err, "making user data")
	}
	ec2Settings.UserData = userData

	if ec2Settings.UserData != "" {
		if err = validateUserDataSize(ec2Settings.UserData, h.Distro.Id); err != nil {
			return errors.WithStack(err)
		}
		userData := base64.StdEncoding.EncodeToString([]byte(ec2Settings.UserData))
		spotRequest.LaunchSpecification.UserData = &userData
	}

	grip.Debug(message.Fields{
		"message":       "starting spot instance",
		"args":          spotRequest,
		"host_id":       h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
	})
	spotResp, err := m.client.RequestSpotInstances(ctx, spotRequest)
	if err != nil {
		if h.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData {
			grip.Error(message.WrapError(h.DeleteJasperCredentials(ctx, m.env), message.Fields{
				"message": "problem cleaning up user data credentials",
				"host_id": h.Id,
				"distro":  h.Distro.Id,
			}))
		}
		grip.Error(errors.Wrapf(h.Remove(), "removing intent host '%s'", h.Id))
		return errors.Wrap(err, "RequestSpotInstances API call returned an error")
	}

	spotReqRes := spotResp.SpotInstanceRequests[0]
	if *spotReqRes.State != SpotStatusOpen && *spotReqRes.State != SpotStatusActive {
		if h.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData {
			grip.Error(message.WrapError(h.DeleteJasperCredentials(ctx, m.env), message.Fields{
				"message": "problem cleaning up user data credentials",
				"host_id": h.Id,
				"distro":  h.Distro.Id,
			}))
		}
		err = errors.Errorf("spot request '%s' was found in state '%s' on intent host '%s'",
			*spotReqRes.SpotInstanceRequestId, *spotReqRes.State, h.Id)
		return err
	}

	h.Id = *spotReqRes.SpotInstanceRequestId
	return nil
}

// SpawnHost spawns a new host.
func (m *ec2Manager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameEc2OnDemand &&
		h.Distro.Provider != evergreen.ProviderNameEc2Spot {
		return nil, errors.Errorf("can't spawn EC2 instance for distro '%s': distro provider is '%s'",
			h.Distro.Id, h.Distro.Provider)
	}

	if err := m.client.Create(m.credentials, m.region); err != nil {
		return nil, errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	ec2Settings := &EC2ProviderSettings{}
	err := ec2Settings.FromDistroSettings(h.Distro, m.region)
	if err != nil {
		return nil, errors.Wrap(err, "getting EC2 settings")
	}
	if err = ec2Settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid EC2 settings in distro %s: %+v", h.Distro.Id, ec2Settings)
	}
	if ec2Settings.KeyName == "" && !h.UserHost {
		if !h.SpawnOptions.SpawnedByTask {
			return nil, errors.New("key name must not be empty")
		}
		var k string
		k, err = m.client.GetKey(ctx, h)
		if err != nil {
			return nil, errors.Wrap(err, "getting key name")
		}
		ec2Settings.KeyName = k
	}

	blockDevices, err := makeBlockDeviceMappings(ec2Settings.MountPoints)
	if err != nil {
		return nil, errors.Wrap(err, "making block device mappings")
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
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return nil, errors.Wrap(err, msg)
	}
	if provider == onDemandProvider {
		if err = m.spawnOnDemandHost(ctx, h, ec2Settings, blockDevices); err != nil {
			msg := "error spawning on-demand host"
			grip.Error(message.WrapError(err, message.Fields{
				"message":       msg,
				"host_id":       h.Id,
				"host_provider": h.Distro.Provider,
				"distro":        h.Distro.Id,
			}))
			return nil, errors.Wrap(err, msg)
		}
		grip.Debug(message.Fields{
			"message":       "spawned on-demand host",
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		})
	} else if provider == spotProvider {
		err = m.spawnSpotHost(ctx, h, ec2Settings, blockDevices)
		if err != nil {
			msg := "error spawning spot host"
			grip.Error(message.WrapError(err, message.Fields{
				"message":       msg,
				"host_id":       h.Id,
				"host_provider": h.Distro.Provider,
				"distro":        h.Distro.Id,
			}))
			if h.ShouldFallbackToOnDemand() && (strings.Contains(err.Error(), EC2InsufficientCapacity) || strings.Contains(err.Error(), EC2UnfulfillableCapacity)) {
				// Spot creation fails relatively frequently due to InsufficientCapacity or UnfulfillableCapacity,
				// so we fall back to OnDemand if requested since that is more consistently reliable.
				event.LogHostFallback(h.Id)
				grip.Info(message.Fields{
					"message":  "could not spawn spot instance, falling back to on-demand",
					"host_id":  h.Id,
					"host_tag": h.Tag,
					"distro":   h.Distro.Id,
					"provider": h.Provider,
				})
				h.Provider = evergreen.ProviderNameEc2OnDemand
				h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
				if err = m.spawnOnDemandHost(ctx, h, ec2Settings, blockDevices); err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"message":       "spawning on-demand host",
						"host_id":       h.Id,
						"host_provider": h.Distro.Provider,
						"distro":        h.Distro.Id,
					}))
					return nil, errors.Wrapf(err, "spawning on-demand host '%s'", h.Id)
				}
				grip.Debug(message.Fields{
					"message":       "spawned on-demand host",
					"host_id":       h.Id,
					"host_provider": h.Distro.Provider,
					"distro":        h.Distro.Id,
				})
				return h, nil
			}
			return nil, errors.Wrap(err, msg)
		}
		grip.Debug(message.Fields{
			"message":       "spawned spot host",
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		})
	}

	return h, nil
}

// getResources returns a slice of the AWS resources for the given host
func (m *ec2Manager) getResources(ctx context.Context, h *host.Host) ([]string, error) {
	instanceID := h.Id
	if isHostSpot(h) {
		var err error
		instanceID, err = m.client.GetSpotInstanceId(ctx, h)
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error getting spot request info",
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		if err != nil {
			return nil, errors.Wrapf(err, "getting spot request info for host '%s'", h.Id)
		}
		if instanceID == "" {
			return nil, errors.New("spot instance does not yet have an instanceId")
		}
	}

	volumeIDs, err := m.client.GetVolumeIDs(ctx, h)
	if err != nil {
		return nil, errors.Wrapf(err, "getting volume IDs for host '%s'", h.Id)
	}

	resources := []string{instanceID}
	resources = append(resources, volumeIDs...)
	return resources, nil
}

// addTags adds or updates the specified tags in the client and db
func (m *ec2Manager) addTags(ctx context.Context, h *host.Host, tags []host.Tag) error {
	resources, err := m.getResources(ctx, h)
	if err != nil {
		return errors.Wrap(err, "getting host resources")
	}
	_, err = m.client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: aws.StringSlice(resources),
		Tags:      hostToEC2Tags(tags),
	})
	if err != nil {
		return errors.Wrapf(err, "creating tags using client for host '%s'", h.Id)
	}
	h.AddTags(tags)

	return errors.Wrapf(h.SetTags(), "creating tags in DB for host '%s'", h.Id)
}

// deleteTags removes the specified tags by their keys in the client and db
func (m *ec2Manager) deleteTags(ctx context.Context, h *host.Host, keys []string) error {
	resources, err := m.getResources(ctx, h)
	if err != nil {
		return errors.Wrap(err, "getting host resources")
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
		return errors.Wrapf(err, "deleting tags using client for host '%s'", h.Id)
	}
	h.DeleteTags(keys)

	return errors.Wrapf(h.SetTags(), "deleting tags in DB for host '%s'", h.Id)
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
		return errors.Wrapf(err, "changing instance type using client for host '%s'", h.Id)
	}

	return errors.Wrapf(h.SetInstanceType(instanceType), "changing instance type in DB for host '%s'", h.Id)
}

func (m *ec2Manager) CheckInstanceType(ctx context.Context, instanceType string) error {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "creating client")
	}
	defer m.client.Close()
	output, err := m.client.DescribeInstanceTypeOfferings(ctx, &ec2.DescribeInstanceTypeOfferingsInput{})
	if err != nil {
		return errors.Wrapf(err, "describing instance types offered for region '%s'", m.region)
	}
	for _, availableType := range output.InstanceTypeOfferings {
		if availableType.InstanceType != nil && (*availableType.InstanceType) == instanceType {
			return nil
		}
	}
	return errors.Errorf("instance type '%s' is unavailable in region '%s'", instanceType, m.region)
}

// setNoExpiration changes whether a host should expire
func (m *ec2Manager) setNoExpiration(ctx context.Context, h *host.Host, noExpiration bool) error {
	expireOnValue := expireInDays(evergreen.SpawnHostExpireDays)
	if !host.IsIntentHostId(h.Id) {
		resources, err := m.getResources(ctx, h)
		if err != nil {
			return errors.Wrap(err, "getting host resources")
		}
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
			return errors.Wrapf(err, "changing expire-on tag using client for host '%s", h.Id)
		}
	}

	if noExpiration {
		return errors.Wrapf(h.MarkShouldNotExpire(expireOnValue), "marking host should not expire in DB for host '%s'", h.Id)
	}
	return errors.Wrapf(h.MarkShouldExpire(expireOnValue), "marking host should in DB for host '%s'", h.Id)
}

// extendExpiration extends a host's expiration time by the number of hours specified
func (m *ec2Manager) extendExpiration(ctx context.Context, h *host.Host, extension time.Duration) error {
	return errors.Wrapf(h.SetExpirationTime(h.ExpirationTime.Add(extension)), "extending expiration time in DB for host '%s'", h.Id)
}

// ModifyHost modifies a spawn host according to the changes specified by a HostModifyOptions struct.
func (m *ec2Manager) ModifyHost(ctx context.Context, h *host.Host, opts host.HostModifyOptions) error {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	// Validate modify options for user errors that should prevent all modifications
	if err := validateEC2HostModifyOptions(h, opts); err != nil {
		return errors.Wrap(err, "validating EC2 host modify options")
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
		if err := h.PastMaxExpiration(opts.AddHours); err != nil {
			catcher.Add(err)
		} else {
			catcher.Add(m.extendExpiration(ctx, h, opts.AddHours))
		}
	}
	if opts.NewName != "" {
		catcher.Add(h.SetDisplayName(opts.NewName))
	}
	if opts.AttachVolume != "" {
		volume, err := host.ValidateVolumeCanBeAttached(opts.AttachVolume)
		if err != nil {
			catcher.Add(err)
			return catcher.Resolve()
		}
		if volume.AvailabilityZone != h.Zone {
			catcher.Errorf("cannot attach volume in zone '%s' to host in zone '%s'", volume.AvailabilityZone, h.Zone)
			return catcher.Resolve()
		}
		attachment := host.VolumeAttachment{VolumeID: opts.AttachVolume, IsHome: false}
		if err = m.AttachVolume(ctx, h, &attachment); err != nil {
			catcher.Wrapf(err, "attaching volume '%s' to host '%s'", volume.ID, h.Id)
		}
	}

	if opts.AddKey != "" {
		if err := addPublicKey(ctx, h, opts.AddKey); err != nil {
			catcher.Wrapf(err, "adding public key to host '%s'", h.Id)
		}
	}

	return catcher.Resolve()
}

// addPublicKey adds a public key to the authorized keys for SSH.
func addPublicKey(ctx context.Context, h *host.Host, key string) error {
	if logs, err := h.RunSSHCommand(ctx, h.AddPublicKeyScript(key)); err != nil {
		return errors.Wrap(err, logs)
	}
	return nil
}

// GetInstanceStatuses returns the current status of a slice of EC2 instances.
func (m *ec2Manager) GetInstanceStatuses(ctx context.Context, hosts []host.Host) (map[string]CloudStatus, error) {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return nil, errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

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
			return nil, errors.Wrap(err, "describing spot instances")
		}
		spotInstanceRequestsMap := map[string]*ec2.SpotInstanceRequest{}
		for i := range spotOut.SpotInstanceRequests {
			spotInstanceRequestsMap[*spotOut.SpotInstanceRequests[i].SpotInstanceRequestId] = spotOut.SpotInstanceRequests[i]
		}
		for i := range spotHosts {
			spotHostID := spotHosts[i].Id
			spotRequest, ok := spotInstanceRequestsMap[spotHostID]
			if !ok {
				hostToStatusMap[spotHostID] = StatusNonExistent
				continue
			}
			instanceID := aws.StringValue(spotRequest.InstanceId)
			if instanceID == "" {
				hostToStatusMap[spotHostID] = cloudStatusFromSpotStatus(*spotRequest.State)
				continue
			}
			hostsToCheck = append(hostsToCheck, &instanceID)
			instanceIdToHostMap[instanceID] = spotHosts[i]
		}
	}

	// Get host statuses
	if len(hostsToCheck) > 0 {
		out, err := m.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: hostsToCheck,
		})
		if err != nil {
			return nil, errors.Wrap(err, "describing instances")
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
				hostToStatusMap[*hostsToCheck[i]] = StatusNonExistent
				continue
			}
			status := ec2StatusToEvergreenStatus(*instance.State.Name)
			if status == StatusRunning {
				// cache instance information so we can make fewer calls to AWS's API
				if err = cacheHostData(ctx, instanceIdToHostMap[*hostsToCheck[i]], instance, m.client); err != nil {
					return nil, errors.Wrapf(err, "caching EC2 host data for host '%s'", *hostsToCheck[i])
				}
			}
			hostToStatusMap[instanceIdToHostMap[*hostsToCheck[i]].Id] = status
		}
	}

	return hostToStatusMap, nil
}

// GetInstanceStatus returns the current status of an EC2 instance.
func (m *ec2Manager) GetInstanceStatus(ctx context.Context, h *host.Host) (CloudStatus, error) {
	status := StatusUnknown

	if err := m.client.Create(m.credentials, m.region); err != nil {
		return status, errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	id := h.Id
	if isHostSpot(h) {
		if h.ExternalIdentifier != "" {
			id = h.ExternalIdentifier
		} else {
			spotDetails, err := m.client.DescribeSpotRequestsAndSave(ctx, []*host.Host{h})
			if err != nil {
				err = errors.Wrapf(err, "getting spot request info for host '%s'", h.Id)
				return status, err
			}
			if len(spotDetails.SpotInstanceRequests) == 0 {
				return status, errors.Errorf("host '%s' has no corresponding spot instance request", h.Id)
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
		if err == noReservationError {
			return StatusNonExistent, nil
		}
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error getting instance info",
			"host_id":       h.Id,
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
			return status, errors.Wrapf(err, "caching EC2 host data for host '%s'", h.Id)
		}
	}

	return status, nil
}

func (m *ec2Manager) SetPortMappings(context.Context, *host.Host, *host.Host) error {
	return errors.New("can't set port mappings with EC2 provider")
}

// TerminateInstance terminates the EC2 instance.
func (m *ec2Manager) TerminateInstance(ctx context.Context, h *host.Host, user, reason string) error {
	// terminate the instance
	if h.Status == evergreen.HostTerminated {
		return errors.Errorf("cannot terminate host '%s' because it's already marked as terminated", h.Id)
	}

	if h.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData {
		grip.Error(message.WrapError(h.DeleteJasperCredentials(ctx, m.env), message.Fields{
			"message": "problem deleting Jasper credentials during host termination",
			"host_id": h.Id,
			"distro":  h.Distro.Id,
		}))
	}

	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	instanceId := h.Id
	if isHostSpot(h) {
		var err error
		instanceId, err = m.cancelSpotRequest(ctx, h)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "error canceling spot request",
				"host_id":       h.Id,
				"host_provider": h.Distro.Provider,
				"user":          user,
				"distro":        h.Distro.Id,
			}))
			return errors.Wrap(err, "canceling spot request")
		}
		// the spot request wasn't fulfilled, so don't attempt to terminate in ec2
		if instanceId == "" {
			return errors.Wrap(h.Terminate(user, "spot request was not fulfilled"), "terminating host in DB")
		}
	}

	if !IsEC2InstanceID(instanceId) {
		return errors.Wrap(h.Terminate(user, fmt.Sprintf("detected invalid instance ID '%s'", instanceId)), "terminating instance in DB")
	}
	resp, err := m.client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []*string{aws.String(instanceId)},
	})
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error terminating instance",
			"user":          user,
			"host_id":       h.Id,
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
			"host_id":       h.Id,
			"distro":        h.Distro.Id,
		})
	}

	for _, vol := range h.Volumes {
		volDB, err := host.FindVolumeByID(vol.VolumeID)
		if err != nil {
			return errors.Wrap(err, "finding volumes for host")
		}
		if volDB == nil {
			continue
		}

		if volDB.Expiration.Before(time.Now().Add(evergreen.UnattachedVolumeExpiration)) {
			if err = m.modifyVolumeExpiration(ctx, volDB, time.Now().Add(evergreen.UnattachedVolumeExpiration)); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message": "error updating volume expiration",
					"user":    user,
					"host_id": h.Id,
					"volume":  volDB.ID,
				}))
				return errors.Wrapf(err, "updating expiration for volume '%s'", volDB.ID)
			}
		}

		if err = host.UnsetVolumeHost(volDB.ID); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"host_id":   h.Id,
				"volume_id": volDB.ID,
				"op":        "terminating host",
				"message":   "problem un-setting host info on volume records",
			}))
		}
	}

	return errors.Wrap(h.Terminate(user, reason), "terminating host in DB")
}

// StopInstance stops a running EC2 instance.
func (m *ec2Manager) StopInstance(ctx context.Context, h *host.Host, user string) error {
	if h.Status == evergreen.HostStopped {
		return errors.Errorf("cannot stop host '%s' because it is already marked as stopped", h.Id)
	} else if h.Status != evergreen.HostRunning && h.Status != evergreen.HostStopping {
		return errors.Errorf("cannot stop host '%s' because its status ('%s') is not a stoppable state", h.Id, h.Status)
	}

	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	out, err := m.client.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []*string{aws.String(h.Id)},
	})
	if err != nil {
		return errors.Wrapf(err, "stopping EC2 instance '%s'", h.Id)
	}

	if len(out.StoppingInstances) == 1 {
		instance := out.StoppingInstances[0]
		status := ec2StatusToEvergreenStatus(aws.StringValue(instance.CurrentState.Name))
		switch status {
		case StatusStopping:
			grip.Error(message.WrapError(h.SetStopping(user), message.Fields{
				"message": "could not mark host as stopping, continuing to poll instance status anyways",
				"host_id": h.Id,
				"user":    user,
			}))
		case StatusStopped:
			return errors.Wrap(h.SetStopped(user), "marking DB host as stopped")
		default:
			return errors.Errorf("instance is in unexpected state '%s'", status)
		}
	}
	grip.WarningWhen(len(out.StoppingInstances) != 1, message.Fields{
		"message":                "StopInstances should have returned exactly one instance in the success response",
		"num_stopping_instances": len(out.StoppingInstances),
		"user":                   user,
		"host_id":                h.Id,
		"host_provider":          h.Distro.Provider,
	})

	// Instances stop asynchronously, so before we can say the host is stopped,
	// we have to poll the status until it's actually stopped.
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			var instance *ec2.Instance
			instance, err = m.client.GetInstanceInfo(ctx, h.Id)
			if err != nil {
				return false, errors.Wrap(err, "getting instance info")
			}
			status := ec2StatusToEvergreenStatus(*instance.State.Name)
			if status == StatusStopped {
				return false, nil
			}
			return true, errors.Errorf("host is not stopped, current status is '%s'", status)
		}, utility.RetryOptions{
			MaxAttempts: checkSuccessAttempts,
			MinDelay:    checkSuccessInitPeriod,
			MaxDelay:    checkSuccessMaxDelay,
		})
	if err != nil {
		return errors.Wrap(err, "checking if spawn host stopped")
	}

	grip.Info(message.Fields{
		"message":       "stopped instance",
		"user":          user,
		"host_provider": h.Distro.Provider,
		"host_id":       h.Id,
		"distro":        h.Distro.Id,
	})

	return errors.Wrap(h.SetStopped(user), "marking DB host as stopped")
}

// StartInstance starts a stopped EC2 instance.
func (m *ec2Manager) StartInstance(ctx context.Context, h *host.Host, user string) error {
	if h.Status != evergreen.HostStopped {
		return errors.Errorf("cannot start host '%s' because its status is '%s'", h.Id, h.Status)
	}

	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	_, err := m.client.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []*string{aws.String(h.Id)},
	})
	if err != nil {
		return errors.Wrapf(err, "starting EC2 instance '%s'", h.Id)
	}

	var instance *ec2.Instance

	// Instances start asynchronously, so before we can say the host is running,
	// we have to poll the status until it's actually running.
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			instance, err = m.client.GetInstanceInfo(ctx, h.Id)
			if err != nil {
				return false, errors.Wrap(err, "getting instance info")
			}
			if ec2StatusToEvergreenStatus(*instance.State.Name) == StatusRunning {
				return false, nil
			}
			return true, errors.New("host is not started")
		}, utility.RetryOptions{
			MaxAttempts: checkSuccessAttempts,
			MinDelay:    checkSuccessInitPeriod,
		})

	if err != nil {
		return errors.Wrap(err, "checking if spawn host started")
	}

	if err = cacheHostData(ctx, h, instance, m.client); err != nil {
		return errors.Wrapf(err, "cache EC2 host data for host '%s'", h.Id)
	}

	grip.Info(message.Fields{
		"message":       "started instance",
		"user":          user,
		"host_provider": h.Distro.Provider,
		"host_id":       h.Id,
		"distro":        h.Distro.Id,
	})

	return errors.Wrap(h.SetRunning(user), "failed to mark instance as running in DB")
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
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return "", errors.Wrapf(err, "getting spot request info for host '%s'", h.Id)
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
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		})
		return "", errors.Wrapf(err, "cancelling spot request for host '%s'", h.Id)
	}
	grip.Info(message.Fields{
		"message":       "canceled spot request",
		"host_id":       h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
	})

	return instanceId, nil
}

// IsUp returns whether a host is up.
func (m *ec2Manager) IsUp(ctx context.Context, h *host.Host) (bool, error) {
	status, err := m.GetInstanceStatus(ctx, h)
	if err != nil {
		return false, errors.Wrap(err, "checking if instance is up")
	}
	if status == StatusRunning {
		return true, nil
	}
	return false, nil
}

// OnUp is called when the host is up.
func (m *ec2Manager) OnUp(ctx context.Context, h *host.Host) error {
	if isHostOnDemand(h) {
		// On-demand hosts and its volumes are already tagged in the request for
		// the instance.
		return nil
	}

	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	resources, err := m.getResources(ctx, h)
	if err != nil {
		return errors.Wrap(err, "getting resources")
	}

	if err = m.client.SetTags(ctx, resources, h); err != nil {
		return errors.Wrap(err, "setting tags")
	}

	return nil
}

func (m *ec2Manager) AttachVolume(ctx context.Context, h *host.Host, attachment *host.VolumeAttachment) error {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	opts := generateDeviceNameOptions{
		isWindows:           h.Distro.IsWindows(),
		existingDeviceNames: h.HostVolumeDeviceNames(),
	}
	// if no device name is provided, generate a unique device name
	if attachment.DeviceName == "" {
		deviceName, err := generateDeviceNameForVolume(opts)
		if err != nil {
			return errors.Wrap(err, "generating initial device name")
		}
		attachment.DeviceName = deviceName
	}

	volume, err := m.client.AttachVolume(ctx, &ec2.AttachVolumeInput{
		InstanceId: aws.String(h.Id),
		Device:     aws.String(attachment.DeviceName),
		VolumeId:   aws.String(attachment.VolumeID),
	}, opts)
	if err != nil {
		return errors.Wrapf(err, "attaching volume '%s' to host '%s'", attachment.VolumeID, h.Id)
	}
	if volume != nil && volume.Device != nil {
		attachment.DeviceName = *volume.Device
	}
	return errors.Wrapf(h.AddVolumeToHost(attachment), "attaching volume '%s' to host '%s' in DB", attachment.VolumeID, h.Id)
}

func (m *ec2Manager) DetachVolume(ctx context.Context, h *host.Host, volumeID string) error {
	v, err := host.FindVolumeByID(volumeID)
	if err != nil {
		return errors.Wrapf(err, "getting volume '%s'", volumeID)
	}
	if v == nil {
		return errors.Errorf("volume '%s' not found", volumeID)
	}

	if err = m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	_, err = m.client.DetachVolume(ctx, &ec2.DetachVolumeInput{
		InstanceId: aws.String(h.Id),
		VolumeId:   aws.String(volumeID),
	})
	if err != nil {
		return errors.Wrapf(err, "detaching volume '%s' from host '%s' in client", volumeID, h.Id)
	}

	if v.Expiration.Before(time.Now().Add(evergreen.DefaultSpawnHostExpiration)) {
		if err = m.modifyVolumeExpiration(ctx, v, time.Now().Add(evergreen.DefaultSpawnHostExpiration)); err != nil {
			return errors.Wrapf(err, "updating expiration for volume '%s'", volumeID)
		}
	}

	return errors.Wrapf(h.RemoveVolumeFromHost(volumeID), "detaching volume '%s' from host '%s' in DB", volumeID, h.Id)
}

func (m *ec2Manager) CreateVolume(ctx context.Context, volume *host.Volume) (*host.Volume, error) {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return nil, errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	volume.Expiration = time.Now().Add(evergreen.DefaultSpawnHostExpiration)
	volumeTags := []*ec2.Tag{
		{Key: aws.String(evergreen.TagOwner), Value: aws.String(volume.CreatedBy)},
		{Key: aws.String(evergreen.TagExpireOn), Value: aws.String(expireInDays(evergreen.SpawnHostExpireDays))},
	}
	input := &ec2.CreateVolumeInput{
		AvailabilityZone: aws.String(volume.AvailabilityZone),
		VolumeType:       aws.String(volume.Type),
		Size:             aws.Int64(int64(volume.Size)),
		TagSpecifications: []*ec2.TagSpecification{
			{ResourceType: aws.String(ec2.ResourceTypeVolume), Tags: volumeTags},
		},
	}

	if volume.Throughput > 0 {
		input.Throughput = aws.Int64(int64(volume.Throughput))
	}

	if volume.IOPS > 0 {
		input.Iops = aws.Int64(int64(volume.IOPS))
	} else if volume.Type == VolumeTypeIo1 { // Iops is required for io1.
		input.Iops = aws.Int64(int64(defaultIops))
	}

	resp, err := m.client.CreateVolume(ctx, input)

	if err != nil {
		return nil, errors.Wrap(err, "creating volume in client")
	}
	if resp.VolumeId == nil {
		return nil, errors.New("new volume returned by EC2 does not have an ID")
	}

	volume.ID = *resp.VolumeId
	if err = volume.Insert(); err != nil {
		return nil, errors.Wrap(err, "creating volume in DB")
	}

	return volume, nil
}

func (m *ec2Manager) DeleteVolume(ctx context.Context, volume *host.Volume) error {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	_, err := m.client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
		VolumeId: aws.String(volume.ID),
	})
	if err != nil {
		return errors.Wrapf(err, "deleting volume '%s' in client", volume.ID)
	}

	return errors.Wrapf(volume.Remove(), "deleting volume '%s' in DB", volume.ID)
}

func (m *ec2Manager) GetVolumeAttachment(ctx context.Context, volumeID string) (*host.VolumeAttachment, error) {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return nil, errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	volumeInfo, err := m.client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
		VolumeIds: aws.StringSlice([]string{volumeID}),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "describing volume '%s'", volumeID)
	}

	if volumeInfo == nil || len(volumeInfo.Volumes) == 0 || volumeInfo.Volumes[0] == nil {
		return nil, errors.Errorf("no volume '%s' found in EC2", volumeID)
	}

	// no attachments found
	if len(volumeInfo.Volumes[0].Attachments) == 0 {
		return nil, nil
	}

	ec2Attachment := volumeInfo.Volumes[0].Attachments[0]
	if ec2Attachment == nil ||
		ec2Attachment.VolumeId == nil ||
		ec2Attachment.Device == nil ||
		ec2Attachment.InstanceId == nil {
		return nil, errors.Errorf("AWS returned an invalid volume attachment %+v", ec2Attachment)
	}

	attachment := &host.VolumeAttachment{
		VolumeID:   *ec2Attachment.VolumeId,
		DeviceName: *ec2Attachment.Device,
		HostID:     *ec2Attachment.InstanceId,
	}

	return attachment, nil
}

func (m *ec2Manager) modifyVolumeExpiration(ctx context.Context, volume *host.Volume, newExpiration time.Time) error {
	if err := volume.SetExpiration(newExpiration); err != nil {
		return errors.Wrapf(err, "updating expiration for volume '%s'", volume.ID)
	}

	_, err := m.client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: []*string{aws.String(volume.ID)},
		Tags: []*ec2.Tag{{
			Key:   aws.String(evergreen.TagExpireOn),
			Value: aws.String(newExpiration.Add(time.Hour * 24 * evergreen.SpawnHostExpireDays).Format(evergreen.ExpireOnFormat))}},
	})
	if err != nil {
		return errors.Wrapf(err, "updating expire-on tag for volume '%s'", volume.ID)
	}

	return nil
}

func (m *ec2Manager) ModifyVolume(ctx context.Context, volume *host.Volume, opts *model.VolumeModifyOptions) error {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	if !utility.IsZeroTime(opts.Expiration) {
		if err := m.modifyVolumeExpiration(ctx, volume, opts.Expiration); err != nil {
			return errors.Wrapf(err, "modifying volume '%s' expiration", volume.ID)
		}
		if err := volume.SetNoExpiration(false); err != nil {
			return errors.Wrapf(err, "clearing volume '%s' no-expiration in DB", volume.ID)
		}
	}

	if opts.NoExpiration && opts.HasExpiration {
		return errors.New("can't set both no expiration and has expiration")
	}

	if opts.NoExpiration {
		if err := m.modifyVolumeExpiration(ctx, volume, time.Now().Add(evergreen.SpawnHostNoExpirationDuration)); err != nil {
			return errors.Wrapf(err, "modifying volume '%s' background expiration", volume.ID)
		}
		if err := volume.SetNoExpiration(true); err != nil {
			return errors.Wrapf(err, "setting volume '%s' no-expiration in DB", volume.ID)
		}
	}

	if opts.HasExpiration {
		if err := volume.SetNoExpiration(false); err != nil {
			return errors.Wrapf(err, "clearing volume '%s' no-expiration in DB", volume.ID)
		}
	}

	if opts.Size > 0 {
		_, err := m.client.ModifyVolume(ctx, &ec2.ModifyVolumeInput{
			VolumeId: aws.String(volume.ID),
			Size:     aws.Int64(int64(opts.Size)),
		})
		if err != nil {
			return errors.Wrapf(err, "modifying volume '%s' size in client", volume.ID)
		}
		if err = volume.SetSize(opts.Size); err != nil {
			return errors.Wrapf(err, "modifying volume '%s' size in DB", volume.ID)
		}
	}

	if opts.NewName != "" {
		if err := volume.SetDisplayName(opts.NewName); err != nil {
			return errors.Wrapf(err, "modifying volume '%s' name in DB", volume.ID)
		}
	}
	return nil
}

// GetDNSName returns the DNS name for the host.
func (m *ec2Manager) GetDNSName(ctx context.Context, h *host.Host) (string, error) {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return "", errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	return m.client.GetPublicDNSName(ctx, h)
}

// TimeTilNextPayment returns how long until the next payment is due for a host.
func (m *ec2Manager) TimeTilNextPayment(host *host.Host) time.Duration {
	return timeTilNextEC2Payment(host)
}

// Cleanup is a noop for the EC2 provider.
func (m *ec2Manager) Cleanup(context.Context) error {
	return nil
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

func (m *ec2Manager) AddSSHKey(ctx context.Context, pair evergreen.SSHKeyPair) error {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	return errors.Wrap(addSSHKey(ctx, m.client, pair), "adding public SSH key")
}

func (m *ec2Manager) getProvider(ctx context.Context, h *host.Host, ec2settings *EC2ProviderSettings) (ec2ProviderType, error) {
	if h.UserHost || m.provider == onDemandProvider {
		h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
		return onDemandProvider, nil
	}
	if m.provider == spotProvider {
		h.Distro.Provider = evergreen.ProviderNameEc2Spot
		return spotProvider, nil
	}
	return 0, errors.Errorf("provider type is %d, expected %d or %d", m.provider, onDemandProvider, spotProvider)
}
