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
	"github.com/evergreen-ci/evergreen/model/host"
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
}

// Validate that essential EC2ProviderSettings fields are not empty.
func (s *EC2ProviderSettings) Validate() error {
	if s.AMI == "" {
		return errors.New("AMI must not be empty")
	}
	if s.InstanceType == "" {
		return errors.New("instance type must not be empty")
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
	if s.UserData != "" {
		if _, err := parseUserData(s.UserData); err != nil {
			return errors.Wrap(err, "user data is malformed")
		}
	}
	return nil
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
		return nil, errors.Wrap(err, "error marshalling provider setting into bson")
	}
	doc := birch.Document{}
	if err = doc.UnmarshalBSON(bytes); err != nil {
		return nil, errors.Wrap(err, "error umarshalling settings bytes into document")
	}
	return &doc, nil
}

func (s *EC2ProviderSettings) FromDocument(doc *birch.Document) error {
	bytes, err := doc.MarshalBSON()
	if err != nil {
		return errors.Wrap(err, "error marshalling provider setting into bson")
	}
	if err := bson.Unmarshal(bytes, s); err != nil {
		return errors.Wrap(err, "error unmarshalling bson into provider settings")
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
	checkSuccessAttempts   = 10
	checkSuccessInitPeriod = 2 * time.Second
)

const (
	VolumeTypeStandard = "standard"
	VolumeTypeIo2      = "io1"
	VolumeTypeGp2      = "gp2"
	VolumeTypeSc1      = "sc1"
	VolumeTypeSt1      = "st1"
)

var (
	ValidVolumeTypes = []string{
		VolumeTypeStandard,
		VolumeTypeIo2,
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

// GetSettings returns a pointer to the manager's configuration settings struct.
func (m *ec2Manager) GetSettings() ProviderSettings {
	return &EC2ProviderSettings{}
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
		return errors.Wrap(err, "Problem getting EC2 keys")
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
			return errors.Wrap(err, "problem expanding user data")
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
		return errors.Wrap(err, "could not make user data")
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
		return errors.Wrap(err, "can't get provider settings")
	}

	supportingSubnets, err := typeCache.subnetsWithInstanceType(ctx, m.settings, m.client, instanceRegionPair{instanceType: h.InstanceType, region: ec2Settings.getRegion()})
	if err != nil {
		return errors.Wrapf(err, "can't get supported subnets for instance type '%s'", h.InstanceType)
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
		return errors.Wrap(err, "can't convert provider settings to document")
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
			return errors.Wrap(err, "problem expanding user data")
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
		return errors.Wrap(err, "could not make user data")
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
		grip.Error(errors.Wrapf(h.Remove(), "error removing intent host %s", h.Id))
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

	if err := m.client.Create(m.credentials, m.region); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	ec2Settings := &EC2ProviderSettings{}
	err := ec2Settings.FromDistroSettings(h.Distro, m.region)
	if err != nil {
		return nil, errors.Wrap(err, "error getting EC2 settings")
	}
	if err = ec2Settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid EC2 settings in distro %s: %+v", h.Distro.Id, ec2Settings)
	}
	if ec2Settings.KeyName == "" && !h.UserHost {
		if !h.SpawnOptions.SpawnedByTask {
			return nil, errors.New("key name must not be empty")
		}
		var k string
		k, err = m.client.GetKey(ctx, h)
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
	_, err = m.client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: aws.StringSlice(resources),
		Tags:      hostToEC2Tags(tags),
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

func (m *ec2Manager) CheckInstanceType(ctx context.Context, instanceType string) error {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()
	output, err := m.client.DescribeInstanceTypeOfferings(ctx, &ec2.DescribeInstanceTypeOfferingsInput{})
	if err != nil {
		return errors.Wrapf(err, "error describe instance types offered for region '%s", m.region)
	}
	for _, availableType := range output.InstanceTypeOfferings {
		if availableType.InstanceType != nil && (*availableType.InstanceType) == instanceType {
			return nil
		}
	}
	return errors.Errorf("type '%s' is unavailable in region '%s'", instanceType, m.region)
}

// setNoExpiration changes whether a host should expire
func (m *ec2Manager) setNoExpiration(ctx context.Context, h *host.Host, noExpiration bool) error {
	expireOnValue := expireInDays(evergreen.SpawnHostExpireDays)
	if !host.IsIntentHostId(h.Id) {
		resources, err := m.getResources(ctx, h)
		if err != nil {
			return errors.Wrap(err, "error getting host resources")
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
			return errors.Wrapf(err, "error changing expire-on tag using client for '%s", h.Id)
		}
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
	if err := m.client.Create(m.credentials, m.region); err != nil {
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
			catcher.Add(errors.Errorf("can't attach volume in zone '%s' to host in zone '%s'", volume.AvailabilityZone, h.Zone))
			return catcher.Resolve()
		}
		attachment := host.VolumeAttachment{VolumeID: opts.AttachVolume, IsHome: false}
		if err = m.AttachVolume(ctx, h, &attachment); err != nil {
			catcher.Wrapf(err, "attaching volume '%s' to host '%s'", volume.ID, h.Id)
		}
	}

	if opts.AddKey != "" {
		if err := h.AddPubKey(ctx, opts.AddKey); err != nil {
			catcher.Wrapf(err, "can't add key to host '%s'", h.Id)
		}
	}

	return catcher.Resolve()
}

// GetInstanceStatuses returns the current status of a slice of EC2 instances.
func (m *ec2Manager) GetInstanceStatuses(ctx context.Context, hosts []host.Host) ([]CloudStatus, error) {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return nil, errors.Wrap(err, "error creating client")
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
				// Terminate an unknown host in the db
				for _, h := range hosts {
					if h.Id == *hostsToCheck[i] {
						grip.Error(message.WrapError(h.Terminate(evergreen.User, "host is missing from DescribeInstances response"), message.Fields{
							"message":       "can't mark instance as terminated",
							"host_id":       h.Id,
							"host_provider": h.Distro.Provider,
							"distro":        h.Distro.Id,
						}))
					}
				}
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

	if err := m.client.Create(m.credentials, m.region); err != nil {
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
		// terminate an unknown host in the db
		if err == noReservationError {
			grip.Error(message.WrapError(h.Terminate(evergreen.User, "host is unknown to AWS"), message.Fields{
				"message":       "can't mark instance as terminated",
				"host_id":       h.Id,
				"host_provider": h.Distro.Provider,
				"distro":        h.Distro.Id,
			}))
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
			return status, errors.Wrapf(err, "can't cache host data for '%s'", h.Id)
		}
	}

	return status, nil
}

func (m *ec2Manager) SetPortMappings(context.Context, *host.Host, *host.Host) error {
	return errors.New("can't set port mappings with ec2 provider")
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
			"host_id": h.Id,
			"distro":  h.Distro.Id,
		}))
	}

	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "error creating client")
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
			return errors.Wrap(err, "can't query for volumes")
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
				return errors.Wrapf(err, "error updating volume '%s' expiration", volDB.ID)
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

	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	var instance *ec2.Instance
	instance, err := m.client.GetInstanceInfo(ctx, h.Id)
	if err != nil {
		return errors.Wrap(err, "error getting instance info before stopping")
	}

	// if instance is already stopped, return early
	if ec2StatusToEvergreenStatus(*instance.State.Name) == StatusStopped {
		grip.Info(message.Fields{
			"message":       "instance already stopped",
			"user":          user,
			"host_provider": h.Distro.Provider,
			"host_id":       h.Id,
			"distro":        h.Distro.Id,
		})

		return errors.Wrap(h.SetStopped(user), "failed to mark instance as stopped in db")
	}

	prevStatus := h.Status
	if err = h.SetStopping(user); err != nil {
		return errors.Wrap(err, "failed to mark instance as stopping in db")
	}

	_, err = m.client.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []*string{aws.String(h.Id)},
	})
	if err != nil {
		if err2 := h.SetStatus(prevStatus, user, ""); err2 != nil {
			return errors.Wrapf(err2, "failed to revert status from stopping to '%s'", prevStatus)
		}
		return errors.Wrapf(err, "error stopping EC2 instance '%s'", h.Id)
	}

	// Check whether instance stopped
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			var instance *ec2.Instance
			instance, err = m.client.GetInstanceInfo(ctx, h.Id)
			if err != nil {
				return false, errors.Wrap(err, "error getting instance info")
			}
			if ec2StatusToEvergreenStatus(*instance.State.Name) == StatusStopped {
				return false, nil
			}
			return true, errors.New("host is not stopped")
		}, utility.RetryOptions{
			MaxAttempts: checkSuccessAttempts,
			MinDelay:    checkSuccessInitPeriod,
		})

	if err != nil {
		if err2 := h.SetStatus(prevStatus, user, ""); err2 != nil {
			return errors.Wrapf(err2, "failed to revert status from stopping to '%s'", prevStatus)
		}
		return errors.Wrap(err, "error checking if spawnhost stopped")
	}

	grip.Info(message.Fields{
		"message":       "stopped instance",
		"user":          user,
		"host_provider": h.Distro.Provider,
		"host_id":       h.Id,
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

	if err := m.client.Create(m.credentials, m.region); err != nil {
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

	var instance *ec2.Instance

	// Check whether instance is running
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			instance, err = m.client.GetInstanceInfo(ctx, h.Id)
			if err != nil {
				return false, errors.Wrap(err, "error getting instance info")
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
		return errors.Wrap(err, "error checking if spawnhost started")
	}

	if err = cacheHostData(ctx, h, instance, m.client); err != nil {
		return errors.Wrapf(err, "can't cache host data for instance '%s'", h.Id)
	}

	grip.Info(message.Fields{
		"message":       "started instance",
		"user":          user,
		"host_provider": h.Distro.Provider,
		"host_id":       h.Id,
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
			"host_id":       h.Id,
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
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		})
		return "", errors.Wrapf(err, "Failed to cancel spot request for host %s", h.Id)
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
		return false, errors.Wrap(err, "error checking if instance is up")
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
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "error creating client")
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
			return errors.Wrap(err, "error generating initial device name")
		}
		attachment.DeviceName = deviceName
	}

	volume, err := m.client.AttachVolume(ctx, &ec2.AttachVolumeInput{
		InstanceId: aws.String(h.Id),
		Device:     aws.String(attachment.DeviceName),
		VolumeId:   aws.String(attachment.VolumeID),
	}, opts)
	if err != nil {
		return errors.Wrapf(err, "error attaching volume '%s' to host '%s'", attachment.VolumeID, h.Id)
	}
	if volume != nil && volume.Device != nil {
		attachment.DeviceName = *volume.Device
	}
	return errors.Wrapf(h.AddVolumeToHost(attachment), "error attaching volume '%s' to host '%s' in db", attachment.VolumeID, h.Id)
}

func (m *ec2Manager) DetachVolume(ctx context.Context, h *host.Host, volumeID string) error {
	v, err := host.FindVolumeByID(volumeID)
	if err != nil {
		return errors.Wrapf(err, "can't get volume '%s'", volumeID)
	}
	if v == nil {
		return errors.Errorf("no volume '%s' found", volumeID)
	}

	if err = m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	_, err = m.client.DetachVolume(ctx, &ec2.DetachVolumeInput{
		InstanceId: aws.String(h.Id),
		VolumeId:   aws.String(volumeID),
	})
	if err != nil {
		return errors.Wrapf(err, "error detaching volume '%s' from host '%s' in client", volumeID, h.Id)
	}

	if v.Expiration.Before(time.Now().Add(evergreen.DefaultSpawnHostExpiration)) {
		if err = m.modifyVolumeExpiration(ctx, v, time.Now().Add(evergreen.DefaultSpawnHostExpiration)); err != nil {
			return errors.Wrapf(err, "can't update expiration for volume '%s'", volumeID)
		}
	}

	return errors.Wrapf(h.RemoveVolumeFromHost(volumeID), "error detaching volume '%s' from host '%s' in db", volumeID, h.Id)
}

func (m *ec2Manager) CreateVolume(ctx context.Context, volume *host.Volume) (*host.Volume, error) {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	volume.Expiration = time.Now().Add(evergreen.DefaultSpawnHostExpiration)
	volumeTags := []*ec2.Tag{
		{Key: aws.String(evergreen.TagOwner), Value: aws.String(volume.CreatedBy)},
		{Key: aws.String(evergreen.TagExpireOn), Value: aws.String(expireInDays(evergreen.SpawnHostExpireDays))},
	}

	resp, err := m.client.CreateVolume(ctx, &ec2.CreateVolumeInput{
		AvailabilityZone: aws.String(volume.AvailabilityZone),
		VolumeType:       aws.String(volume.Type),
		Size:             aws.Int64(int64(volume.Size)),
		TagSpecifications: []*ec2.TagSpecification{
			{ResourceType: aws.String(ec2.ResourceTypeVolume), Tags: volumeTags},
		},
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
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	_, err := m.client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
		VolumeId: aws.String(volume.ID),
	})
	if err != nil {
		return errors.Wrapf(err, "error deleting volume '%s' in client", volume.ID)
	}

	return errors.Wrapf(volume.Remove(), "error deleting volume '%s' in db", volume.ID)
}

func (m *ec2Manager) GetVolumeAttachment(ctx context.Context, volumeID string) (*host.VolumeAttachment, error) {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	volumeInfo, err := m.client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
		VolumeIds: aws.StringSlice([]string{volumeID}),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error describing volume '%s'", volumeID)
	}

	if volumeInfo == nil || len(volumeInfo.Volumes) == 0 || volumeInfo.Volumes[0] == nil {
		return nil, errors.Errorf("no volume '%s' found in ec2", volumeID)
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
		return nil, errors.New("aws returned an invalid volume attachment")
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
		return errors.Wrapf(err, "can't update expiration for volume '%s'", volume.ID)
	}

	_, err := m.client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: []*string{aws.String(volume.ID)},
		Tags: []*ec2.Tag{{
			Key:   aws.String(evergreen.TagExpireOn),
			Value: aws.String(newExpiration.Add(time.Hour * 24 * evergreen.SpawnHostExpireDays).Format(evergreen.ExpireOnFormat))}},
	})
	if err != nil {
		return errors.Wrapf(err, "can't update expire-on tag for volume '%s'", volume.ID)
	}

	return nil
}

func (m *ec2Manager) ModifyVolume(ctx context.Context, volume *host.Volume, opts *model.VolumeModifyOptions) error {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	if !utility.IsZeroTime(opts.Expiration) {
		if err := m.modifyVolumeExpiration(ctx, volume, opts.Expiration); err != nil {
			return errors.Wrapf(err, "error modifying volume '%s' expiration", volume.ID)
		}
		if err := volume.SetNoExpiration(false); err != nil {
			return errors.Wrapf(err, "error clearing volume '%s' no-expiration in db", volume.ID)
		}
	}

	if opts.NoExpiration && opts.HasExpiration {
		return errors.New("can't set no expiration and has expiration")
	}

	if opts.NoExpiration {
		if err := m.modifyVolumeExpiration(ctx, volume, time.Now().Add(evergreen.SpawnHostNoExpirationDuration)); err != nil {
			return errors.Wrapf(err, "error modifying volume '%s' background expiration", volume.ID)
		}
		if err := volume.SetNoExpiration(true); err != nil {
			return errors.Wrapf(err, "error setting volume '%s' no-expiration in db", volume.ID)
		}
	}

	if opts.HasExpiration {
		if err := volume.SetNoExpiration(false); err != nil {
			return errors.Wrapf(err, "error clearing volume '%s' no-expiration in db", volume.ID)
		}
	}

	if opts.Size > 0 {
		_, err := m.client.ModifyVolume(ctx, &ec2.ModifyVolumeInput{
			VolumeId: aws.String(volume.ID),
			Size:     aws.Int64(int64(opts.Size)),
		})
		if err != nil {
			return errors.Wrapf(err, "error modifying volume '%s' size in client", volume.ID)
		}
		if err = volume.SetSize(opts.Size); err != nil {
			return errors.Wrapf(err, "error modifying volume '%s' size in db", volume.ID)
		}
	}

	if opts.NewName != "" {
		if err := volume.SetDisplayName(opts.NewName); err != nil {
			return errors.Wrapf(err, "error modifying volume '%s' name in db", volume.ID)
		}
	}
	return nil
}

// GetDNSName returns the DNS name for the host.
func (m *ec2Manager) GetDNSName(ctx context.Context, h *host.Host) (string, error) {
	if err := m.client.Create(m.credentials, m.region); err != nil {
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

func (m *ec2Manager) AddSSHKey(ctx context.Context, pair evergreen.SSHKeyPair) error {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	return errors.Wrap(addSSHKey(ctx, m.client, pair), "could not add SSH key")
}
