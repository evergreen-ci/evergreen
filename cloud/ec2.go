package cloud

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// EC2ProviderSettings describes properties of managed instances.
type EC2ProviderSettings struct {
	// Region is the EC2 region in which the instance will start. Empty is equivalent to the Evergreen default region.
	// This should remain one of the first fields to speed up the birch document iterator.
	Region string `mapstructure:"region" json:"region" bson:"region,omitempty"`

	// AMI is the AMI ID.
	AMI string `mapstructure:"ami" json:"ami,omitempty" bson:"ami,omitempty"`

	// InstanceType is the EC2 instance type.
	InstanceType string `mapstructure:"instance_type" json:"instance_type,omitempty" bson:"instance_type,omitempty"`

	// IPv6 is set to true if the instance should have only an IPv6 address.
	IPv6 bool `mapstructure:"ipv6" json:"ipv6,omitempty" bson:"ipv6,omitempty"`

	// DoNotAssignPublicIPv4Address, if true, skips assigning a public IPv4
	// address to task hosts. An AWS SSH key will be assigned to the host only
	// if a public IPv4 address is also assigned.
	// Does not apply if the host is using IPv6 or if the host is a spawn host
	// or host.create host, which need an IPv4 address for SSH.
	DoNotAssignPublicIPv4Address bool `mapstructure:"do_not_assign_public_ipv4_address" json:"do_not_assign_public_ipv4_address,omitempty" bson:"do_not_assign_public_ipv4_address,omitempty"`

	// KeyName is the AWS SSH key name.
	KeyName string `mapstructure:"key_name" json:"key_name,omitempty" bson:"key_name,omitempty"`

	// MountPoints are the disk mount points for EBS volumes.
	MountPoints []MountPoint `mapstructure:"mount_points" json:"mount_points,omitempty" bson:"mount_points,omitempty"`

	// SecurityGroupIDs is a list of security group IDs.
	SecurityGroupIDs []string `mapstructure:"security_group_ids" json:"security_group_ids,omitempty" bson:"security_group_ids,omitempty"`

	// IAMInstanceProfileARN is the Amazon Resource Name (ARN) of the instance profile.
	IAMInstanceProfileARN string `mapstructure:"iam_instance_profile_arn,omitempty" json:"iam_instance_profile_arn,omitempty"  bson:"iam_instance_profile_arn,omitempty"`

	// SubnetId is only set in a VPC. Either subnet id or vpc name must set.
	SubnetId string `mapstructure:"subnet_id" json:"subnet_id,omitempty" bson:"subnet_id,omitempty"`

	// Tenancy, if set, determines how EC2 instances are distributed across
	// physical hardware.
	Tenancy evergreen.EC2Tenancy `mapstructure:"tenancy" json:"tenancy,omitempty" bson:"tenancy,omitempty"`

	// VpcName is used to get the subnet ID automatically. Either subnet id or vpc name must set.
	VpcName string `mapstructure:"vpc_name" json:"vpc_name,omitempty" bson:"vpc_name,omitempty"`

	// IsVpc is set to true if the security group is part of a VPC.
	IsVpc bool `mapstructure:"is_vpc" json:"is_vpc,omitempty" bson:"is_vpc,omitempty"`

	// UserData specifies configuration that runs after the instance starts.
	UserData string `mapstructure:"user_data" json:"user_data,omitempty" bson:"user_data,omitempty"`

	// MergeUserDataParts specifies whether multiple user data parts should be
	// merged into a single user data part.
	// EVG-7760: This is primarily a workaround for a problem with Windows not
	// allowing multiple scripts of the same type as part of a multipart user
	// data upload.
	MergeUserDataParts bool `mapstructure:"merge_user_data_parts" json:"merge_user_data_parts,omitempty" bson:"merge_user_data_parts,omitempty"`

	// ElasticIPsEnabled determines if hosts can use elastic IPs to obtain their
	// IP addresses.
	ElasticIPsEnabled bool `mapstructure:"elastic_ips_enabled" json:"elastic_ips_enabled,omitempty" bson:"elastic_ips_enabled,omitempty"`
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

	if s.IsVpc && s.SubnetId == "" {
		catcher.New("must set a default subnet for a VPC")
	}

	if s.Tenancy != "" {
		catcher.ErrorfWhen(!evergreen.IsValidEC2Tenancy(s.Tenancy), "invalid tenancy '%s', allowed values are: %s", s.Tenancy, evergreen.ValidEC2Tenancies)
	}

	_, err := makeBlockDeviceMappings(s.MountPoints)
	catcher.Wrap(err, "block device mappings invalid")

	if s.UserData != "" {
		_, err = parseUserData(s.UserData)
		catcher.Wrap(err, "user data is malformed")
	}

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

func (s *EC2ProviderSettings) getRegion() string {
	if s.Region != "" {
		return s.Region
	}
	return evergreen.DefaultEC2Region
}

const (
	defaultIops = 3000
)

const (
	checkSuccessAttempts   = 10
	checkSuccessInitPeriod = 2 * time.Second
	checkSuccessMaxDelay   = time.Minute
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
	// region is the AWS region specified by the distro.
	region string
	// account is the AWS account in which API calls are being made.
	account string
	// role is the role to assume when making AWS API calls for a distro.
	role string
}

// ec2Manager starts and configures instances in EC2.
type ec2Manager struct {
	*EC2ManagerOptions
	env      evergreen.Environment
	settings *evergreen.Settings
}

// Configure loads settings from the config file.
func (m *ec2Manager) Configure(ctx context.Context, settings *evergreen.Settings) error {
	m.settings = settings

	if m.region == "" {
		m.region = evergreen.DefaultEC2Region
	}

	role, err := getRoleForAccount(settings, m.account)
	if err != nil {
		return errors.Wrap(err, "getting role for account")
	}
	m.role = role

	return nil
}

func getRoleForAccount(settings *evergreen.Settings, account string) (string, error) {
	if account == "" {
		return "", nil
	}

	for _, m := range settings.Providers.AWS.AccountRoles {
		if m.Account == account {
			return m.Role, nil
		}
	}

	return "", errors.Errorf("account '%s' has no associated role", account)
}

func (m *ec2Manager) setupClient(ctx context.Context) error {
	return m.client.Create(ctx, m.role, m.region)
}

func (m *ec2Manager) spawnOnDemandHost(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings, blockDevices []types.BlockDeviceMapping) error {
	ctx, span := tracer.Start(ctx, "spawnOnDemandHost")
	defer span.End()

	input := &ec2.RunInstancesInput{
		MinCount:            aws.Int32(1),
		MaxCount:            aws.Int32(1),
		ImageId:             &ec2Settings.AMI,
		InstanceType:        types.InstanceType(ec2Settings.InstanceType),
		BlockDeviceMappings: blockDevices,
		TagSpecifications:   makeTagSpecifications(makeTags(h)),
	}

	if ec2Settings.IAMInstanceProfileARN != "" {
		input.IamInstanceProfile = &types.IamInstanceProfileSpecification{Arn: aws.String(ec2Settings.IAMInstanceProfileARN)}
	}

	assignPublicIPv4 := shouldAssignPublicIPv4Address(h, ec2Settings)
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "getting admin settings")
	}
	useElasticIP := assignPublicIPv4 && canUseElasticIP(settings, ec2Settings, m.account, h)
	if useElasticIP && h.IPAllocationID == "" {
		// If the host can't be allocated an IP address, continue on error
		// because the host should fall back to using an AWS-provided IP
		// address. Using an elastic IP address is a best-effort attempt to save
		// money.
		grip.Notice(message.WrapError(allocateIPAddressForHost(ctx, h), message.Fields{
			"message": "could not allocate elastic IP address for host, falling back to using AWS-managed IP",
			"host_id": h.Id,
		}))
	}

	if assignPublicIPv4 {
		// Only set an SSH key for the host if the host actually has a public
		// IPv4 address. Hosts that don't have a public IPv4 address aren't
		// reachable with SSH even if a key is set.
		input.KeyName = aws.String(ec2Settings.KeyName)
	}
	if ec2Settings.IsVpc {
		// Fall back to using an AWS-provided IPv4 address if this host needs a
		// public IPv4 address and it hasn't been allocated a elastic IP
		// address.
		useAWSIPv4Addr := assignPublicIPv4 && h.IPAllocationID == ""
		input.NetworkInterfaces = []types.InstanceNetworkInterfaceSpecification{
			{
				AssociatePublicIpAddress: aws.Bool(useAWSIPv4Addr),
				DeviceIndex:              aws.Int32(0),
				Groups:                   ec2Settings.SecurityGroupIDs,
				SubnetId:                 &ec2Settings.SubnetId,
			},
		}
		if ec2Settings.IPv6 {
			input.NetworkInterfaces[0].Ipv6AddressCount = aws.Int32(1)
		}
	} else {
		input.SecurityGroups = ec2Settings.SecurityGroupIDs
	}
	if ec2Settings.Tenancy != "" {
		input.Placement = &types.Placement{Tenancy: types.Tenancy(ec2Settings.Tenancy)}
	}

	if ec2Settings.UserData != "" {
		expanded, err := expandUserData(ec2Settings.UserData, m.settings.Expansions)
		if err != nil {
			return errors.Wrap(err, "expanding user data")
		}
		ec2Settings.UserData = expanded
	}

	userData, err := makeUserData(ctx, m.env, h, ec2Settings.UserData, ec2Settings.MergeUserDataParts)
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

	reservation, err := m.client.RunInstances(ctx, input)
	if err != nil || reservation == nil {
		if err == EC2InsufficientCapacityError {
			previousSubnet := h.GetSubnetID()
			// try again in another AZ
			if subnetErr := m.setNextSubnet(ctx, h); subnetErr == nil {
				return errors.Wrap(err, "got EC2InsufficientCapacityError, will try next available subnet")
			} else {
				grip.Error(message.WrapError(subnetErr, message.Fields{
					"message":         "couldn't increment subnet",
					"host_id":         h.Id,
					"host_provider":   h.Distro.Provider,
					"distro":          h.Distro.Id,
					"previous_subnet": previousSubnet,
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

		if h.SpawnOptions.SpawnedByTask {
			detailErr := task.AddHostCreateDetails(ctx, h.StartedBy, h.Id, h.SpawnOptions.TaskExecutionNumber, err)
			grip.Error(message.WrapError(detailErr, message.Fields{
				"message":       "error adding host create error details",
				"host_id":       h.Id,
				"host_provider": h.Distro.Provider,
				"distro":        h.Distro.Id,
			}))
		}

		if err != nil {
			return errors.Wrap(err, "RunInstances API call returned an error")
		}

		msg := "reservation was nil"
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

	return h.UpdateCachedDistroProviderSettings(ctx, []*birch.Document{newSettingsDocument})
}

// SpawnHost spawns a new host.
func (m *ec2Manager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameEc2OnDemand {
		return nil, errors.Errorf("can't spawn EC2 instance for distro '%s': distro provider is '%s'",
			h.Distro.Id, h.Distro.Provider)
	}

	if err := m.setupClient(ctx); err != nil {
		return nil, errors.Wrap(err, "creating client")
	}

	ec2Settings := &EC2ProviderSettings{}
	err := ec2Settings.FromDistroSettings(h.Distro, m.region)
	if err != nil {
		return nil, errors.Wrap(err, "getting EC2 settings")
	}
	if err = ec2Settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid EC2 settings in distro %s: %+v", h.Distro.Id, ec2Settings)
	}
	// The KeyName is used by AWS to determine which public key to put on the host.
	ec2Settings.KeyName, err = getKeyName(ctx, h, m.settings, m.client)
	if err != nil {
		return nil, errors.Wrap(err, "getting key name")
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
	if err = m.spawnOnDemandHost(ctx, h, ec2Settings, blockDevices); err != nil {
		return nil, errors.Wrap(err, "spawning on-demand host")
	}
	grip.Debug(message.Fields{
		"message":       "spawned on-demand host",
		"host_id":       h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
	})

	return h, nil
}

// getResources returns a slice of the AWS resources for the given host
func (m *ec2Manager) getResources(ctx context.Context, h *host.Host) ([]string, error) {
	volumeIDs, err := m.client.GetVolumeIDs(ctx, h)
	if err != nil {
		return nil, errors.Wrapf(err, "getting volume IDs for host '%s'", h.Id)
	}

	resources := []string{h.Id}
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
		Resources: resources,
		Tags:      hostToEC2Tags(tags),
	})
	if err != nil {
		return errors.Wrapf(err, "creating tags using client for host '%s'", h.Id)
	}
	h.AddTags(tags)

	return errors.Wrapf(h.SetTags(ctx), "creating tags in DB for host '%s'", h.Id)
}

// deleteTags removes the specified tags by their keys in the client and db
func (m *ec2Manager) deleteTags(ctx context.Context, h *host.Host, keys []string) error {
	resources, err := m.getResources(ctx, h)
	if err != nil {
		return errors.Wrap(err, "getting host resources")
	}
	deleteTagSlice := make([]types.Tag, len(keys))
	for i := range keys {
		deleteTagSlice[i] = types.Tag{Key: &keys[i]}
	}
	_, err = m.client.DeleteTags(ctx, &ec2.DeleteTagsInput{
		Resources: resources,
		Tags:      deleteTagSlice,
	})
	if err != nil {
		return errors.Wrapf(err, "deleting tags using client for host '%s'", h.Id)
	}
	h.DeleteTags(keys)

	return errors.Wrapf(h.SetTags(ctx), "deleting tags in DB for host '%s'", h.Id)
}

// setInstanceType changes the instance type in the client and db
func (m *ec2Manager) setInstanceType(ctx context.Context, h *host.Host, instanceType string) error {
	_, err := m.client.ModifyInstanceAttribute(ctx, &ec2.ModifyInstanceAttributeInput{
		InstanceId: aws.String(h.Id),
		InstanceType: &types.AttributeValue{
			Value: aws.String(instanceType),
		},
	})
	if err != nil {
		return errors.Wrapf(err, "changing instance type using client for host '%s'", h.Id)
	}

	return errors.Wrapf(h.SetInstanceType(ctx, instanceType), "changing instance type in DB for host '%s'", h.Id)
}

func (m *ec2Manager) CheckInstanceType(ctx context.Context, instanceType string) error {
	if err := m.setupClient(ctx); err != nil {
		return errors.Wrap(err, "creating client")
	}
	output, err := m.client.DescribeInstanceTypeOfferings(ctx, &ec2.DescribeInstanceTypeOfferingsInput{})
	if err != nil {
		return errors.Wrapf(err, "describing instance types offered for region '%s'", m.region)
	}
	for _, availableType := range output.InstanceTypeOfferings {
		if availableType.InstanceType == types.InstanceType(instanceType) {
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
			Resources: resources,
			Tags: []types.Tag{
				{
					Key:   aws.String(evergreen.TagExpireOn),
					Value: aws.String(expireOnValue),
				},
			},
		})
		if err != nil {
			return errors.Wrapf(err, "changing expire-on tag using client for host '%s", h.Id)
		}
	}

	if noExpiration {
		var userTimeZone string
		u, err := user.FindOneByIdContext(ctx, h.StartedBy)
		if err != nil {
			return errors.Wrapf(err, "finding owner '%s' for host '%s'", h.StartedBy, h.Id)
		}
		if u != nil {
			userTimeZone = u.Settings.Timezone
		}
		if err := h.MarkShouldNotExpire(ctx, expireOnValue, userTimeZone); err != nil {
			return errors.Wrapf(err, "marking host should not expire in DB for host '%s'", h.Id)
		}

		// Getting the instance status adds/updates the cached host data
		// (including unexpirable host information like persistent DNS names and
		// IP addresses) if the unexpirable host is running.
		_, err = m.GetInstanceState(ctx, h)
		grip.Error(message.WrapError(err, message.Fields{
			"message":    "could not get instance info to assign persistent DNS name",
			"dashboard":  "evergreen sleep schedule health",
			"host_id":    h.Id,
			"started_by": h.StartedBy,
		}))

		return nil
	}

	grip.Error(message.WrapError(deleteHostPersistentDNSName(ctx, m.env, h, m.client), message.Fields{
		"message":    "could not delete host's persistent DNS name",
		"op":         "delete",
		"dashboard":  "evergreen sleep schedule health",
		"host_id":    h.Id,
		"started_by": h.StartedBy,
	}))

	return errors.Wrapf(h.MarkShouldExpire(ctx, expireOnValue), "marking host should in DB for host '%s'", h.Id)
}

// setSleepScheduleOptions updates a host's sleep schedule options.
func (m *ec2Manager) setSleepScheduleOptions(ctx context.Context, h *host.Host, opts host.SleepScheduleOptions) error {
	if err := opts.Validate(); err != nil {
		return errors.Wrap(err, "invalid new sleep schedule options")
	}

	now := time.Now()

	var updatedSchedule host.SleepScheduleInfo
	if !h.SleepSchedule.IsZero() {
		// A sleep schedule already exists - overwrite just the recurring sleep
		// schedule settings while preserving existing unrelated settings.
		updatedSchedule = h.SleepSchedule
		updatedSchedule.WholeWeekdaysOff = opts.WholeWeekdaysOff
		updatedSchedule.DailyStartTime = opts.DailyStartTime
		updatedSchedule.DailyStopTime = opts.DailyStopTime
		updatedSchedule.TimeZone = opts.TimeZone
	} else {
		schedule, err := host.NewSleepScheduleInfo(opts)
		if err != nil {
			return errors.Wrap(err, "creating new sleep schedule")
		}
		updatedSchedule = *schedule
	}

	return h.UpdateSleepSchedule(ctx, updatedSchedule, now)
}

// extendExpiration extends a host's expiration time by the number of hours specified
func (m *ec2Manager) extendExpiration(ctx context.Context, h *host.Host, extension time.Duration) error {
	return errors.Wrapf(h.SetExpirationTime(ctx, h.ExpirationTime.Add(extension)), "extending expiration time in DB for host '%s'", h.Id)
}

// ModifyHost modifies a spawn host according to the changes specified by a HostModifyOptions struct.
func (m *ec2Manager) ModifyHost(ctx context.Context, h *host.Host, opts host.HostModifyOptions) error {
	if err := m.setupClient(ctx); err != nil {
		return errors.Wrap(err, "creating client")
	}

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
	if !opts.SleepScheduleOptions.IsZero() {
		catcher.Wrap(m.setSleepScheduleOptions(ctx, h, opts.SleepScheduleOptions), "updating host sleep schedule")
	}
	if opts.AddHours != 0 {
		if err := h.ValidateExpirationExtension(opts.AddHours); err != nil {
			catcher.Add(err)
		} else {
			catcher.Add(m.extendExpiration(ctx, h, opts.AddHours))
		}
	}
	if opts.AddTemporaryExemptionHours > 0 {
		extendBy := time.Duration(opts.AddTemporaryExemptionHours) * time.Hour
		exemptUntil, err := h.GetTemporaryExemption(extendBy)
		catcher.Wrap(err, "getting temporary exemption")
		if err == nil {
			catcher.Wrap(h.SetTemporaryExemption(ctx, exemptUntil), "setting temporary exemption")
		}
	}
	if opts.NewName != "" {
		catcher.Add(h.SetDisplayName(ctx, opts.NewName))
	}
	if opts.AttachVolume != "" {
		volume, err := host.ValidateVolumeCanBeAttached(ctx, opts.AttachVolume)
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
	if err := m.setupClient(ctx); err != nil {
		return nil, errors.Wrap(err, "creating client")
	}

	instanceIdToHostMap := map[string]*host.Host{}
	hostsToCheck := []string{}

	for i := range hosts {
		instanceIdToHostMap[hosts[i].Id] = &hosts[i]
		hostsToCheck = append(hostsToCheck, hosts[i].Id)
	}

	if len(hostsToCheck) == 0 {
		return map[string]CloudStatus{}, nil
	}

	out, err := m.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: hostsToCheck,
	})
	if err != nil {
		return nil, errors.Wrap(err, "describing instances")
	}
	if err = validateEc2DescribeInstancesOutput(out); err != nil {
		return nil, errors.Wrap(err, "invalid describe instances response")
	}

	reservationsMap := map[string]types.Instance{}
	for i := range out.Reservations {
		reservationsMap[*out.Reservations[i].Instances[0].InstanceId] = out.Reservations[i].Instances[0]
	}

	hostsToCache := make([]hostInstancePair, 0, len(hostsToCheck))
	hostIDsToCache := make([]string, 0, len(hostsToCheck))
	hostToStatusMap := make(map[string]CloudStatus, len(hostsToCheck))
	for i := range hostsToCheck {
		hostID := hostsToCheck[i]
		instance, ok := reservationsMap[hostID]
		if !ok {
			hostToStatusMap[hostID] = StatusNonExistent
			continue
		}
		status := ec2StateToEvergreenStatus(instance.State)
		if status == StatusRunning {
			hostsToCache = append(hostsToCache, hostInstancePair{
				host:     instanceIdToHostMap[hostID],
				instance: &instance,
			})
			hostIDsToCache = append(hostIDsToCache, hostID)
		}
		hostToStatusMap[hostID] = status
	}

	// Cache instance information so we can make fewer calls to AWS's API.
	grip.Error(message.WrapError(cacheAllHostData(ctx, m.env, m.client, hostsToCache...), message.Fields{
		"message":   "error bulk updating cached host data",
		"num_hosts": len(hostIDsToCache),
		"host_ids":  hostIDsToCache,
	}))

	return hostToStatusMap, nil
}

// GetInstanceState returns a universal status code representing the state
// of an ec2 and a state reason if available. The state reason should not be
// used to determine the status of the ec2 but rather to provide additional
// context about the state of the ec2 state.
// For more information about ec2 state's, look in to the instance.StateReason field.
func (m *ec2Manager) GetInstanceState(ctx context.Context, h *host.Host) (CloudInstanceState, error) {
	info := CloudInstanceState{Status: StatusUnknown}

	if err := m.setupClient(ctx); err != nil {
		return info, errors.Wrap(err, "creating client")
	}

	instance, err := m.client.GetInstanceInfo(ctx, h.Id)
	if err != nil {
		if isEC2InstanceNotFound(err) {
			info.Status = StatusNonExistent
			return info, nil
		}
		return info, err
	}

	if info.Status = ec2StateToEvergreenStatus(instance.State); info.Status == StatusRunning {
		// Cache instance information so we can make fewer calls to AWS's API.
		pair := hostInstancePair{host: h, instance: instance}
		grip.Error(message.WrapError(cacheAllHostData(ctx, m.env, m.client, pair), message.Fields{
			"message": "can't update host cached data",
			"type":    "ec2",
			"host_id": h.Id,
		}))
	}

	if instance.StateReason != nil {
		info.StateReason = utility.FromStringPtr(instance.StateReason.Message)
	}

	return info, nil
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

	if err := m.setupClient(ctx); err != nil {
		return errors.Wrap(err, "creating client")
	}

	if !IsEC2InstanceID(h.Id) {
		return errors.Wrap(h.Terminate(ctx, user, fmt.Sprintf("detected invalid instance ID '%s'", h.Id)), "terminating instance in DB")
	}

	if h.NoExpiration {
		// Clean up remaining DNS records for unexpirable hosts.
		grip.Error(message.WrapError(deleteHostPersistentDNSName(ctx, m.env, h, m.client), message.Fields{
			"message":    "could not delete host's persistent DNS name",
			"op":         "delete",
			"dashboard":  "evergreen sleep schedule health",
			"host_id":    h.Id,
			"started_by": h.StartedBy,
		}))
	}

	resp, err := m.client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{h.Id},
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
			"instance_id":   aws.ToString(stateChange.InstanceId),
			"host_id":       h.Id,
			"distro":        h.Distro.Id,
		})
	}

	grip.Error(message.WrapError(releaseIPAddressForHost(ctx, h), message.Fields{
		"message":        "could not release elastic IP address from host",
		"provider":       h.Distro.Provider,
		"host_id":        h.Id,
		"association_id": h.IPAssociationID,
		"allocation_id":  h.IPAllocationID,
	}))

	for _, vol := range h.Volumes {
		volDB, err := host.FindVolumeByID(ctx, vol.VolumeID)
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

		if err = host.UnsetVolumeHost(ctx, volDB.ID); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"host_id":   h.Id,
				"volume_id": volDB.ID,
				"op":        "terminating host",
				"message":   "problem un-setting host info on volume records",
			}))
		}
	}

	return errors.Wrap(h.Terminate(ctx, user, reason), "terminating host in DB")
}

// StopInstance stops a running EC2 instance.
func (m *ec2Manager) StopInstance(ctx context.Context, h *host.Host, shouldKeepOff bool, user string) error {
	if !utility.StringSliceContains(evergreen.StoppableHostStatuses, h.Status) {
		return errors.Errorf("host cannot be stopped because its status ('%s') is not a stoppable state", h.Status)
	}

	if err := m.setupClient(ctx); err != nil {
		return errors.Wrap(err, "creating client")
	}

	out, err := m.client.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []string{h.Id},
	})
	if err != nil {
		return errors.Wrapf(err, "stopping EC2 instance '%s'", h.Id)
	}

	if len(out.StoppingInstances) == 1 {
		// Stopping a host can be quite fast, so sometimes EC2 will say the host
		// is stopping or already stopped in its response.
		instance := out.StoppingInstances[0]
		status := ec2StateToEvergreenStatus(instance.CurrentState)
		switch status {
		case StatusStopping:
			grip.Error(message.WrapError(h.SetStopping(ctx, user), message.Fields{
				"message": "could not mark host as stopping, continuing to poll instance status anyways",
				"host_id": h.Id,
				"user":    user,
			}))
		case StatusStopped:
			grip.Info(message.Fields{
				"message":       "stopped instance",
				"user":          user,
				"host_provider": h.Distro.Provider,
				"host_id":       h.Id,
				"distro":        h.Distro.Id,
			})
			return errors.Wrap(h.SetStopped(ctx, shouldKeepOff, user), "marking DB host as stopped")
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
			info, err := m.GetInstanceState(ctx, h)
			if err != nil {
				return false, errors.Wrap(err, "getting instance status")
			}
			if info.Status == StatusStopped {
				return false, nil
			}
			return true, errors.Errorf("host is not stopped, current status is '%s' because '%s'", info.Status, info.StateReason)
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

	return errors.Wrap(h.SetStopped(ctx, shouldKeepOff, user), "marking DB host as stopped")
}

// StartInstance starts a stopped EC2 instance.
func (m *ec2Manager) StartInstance(ctx context.Context, h *host.Host, user string) error {
	if !utility.StringSliceContains(evergreen.StartableHostStatuses, h.Status) {
		return errors.Errorf("host cannot be started because its status ('%s') is not a startable state", h.Status)
	}

	if err := m.setupClient(ctx); err != nil {
		return errors.Wrap(err, "creating client")
	}

	_, err := m.client.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []string{h.Id},
	})
	if err != nil {
		return errors.Wrapf(err, "starting EC2 instance '%s'", h.Id)
	}

	// Instances start asynchronously, so before we can say the host is running,
	// we have to poll the status until it's actually running.
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			info, err := m.GetInstanceState(ctx, h)
			if err != nil {
				return false, errors.Wrap(err, "getting instance status")
			}
			if info.Status == StatusRunning {
				return false, nil
			}
			return true, errors.Errorf("host is not started, current status is '%s' because '%s'", info.Status, info.StateReason)
		}, utility.RetryOptions{
			MaxAttempts: checkSuccessAttempts,
			MinDelay:    checkSuccessInitPeriod,
			MaxDelay:    checkSuccessMaxDelay,
		})

	if err != nil {
		return errors.Wrap(err, "checking if spawn host started")
	}

	grip.Info(message.Fields{
		"message":       "started instance",
		"user":          user,
		"host_provider": h.Distro.Provider,
		"host_id":       h.Id,
		"distro":        h.Distro.Id,
	})

	return errors.Wrap(h.SetRunning(ctx, user), "marking host as running")
}

func (m *ec2Manager) AttachVolume(ctx context.Context, h *host.Host, attachment *host.VolumeAttachment) error {
	if err := m.setupClient(ctx); err != nil {
		return errors.Wrap(err, "creating client")
	}

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
	return errors.Wrapf(h.AddVolumeToHost(ctx, attachment), "attaching volume '%s' to host '%s' in DB", attachment.VolumeID, h.Id)
}

func (m *ec2Manager) DetachVolume(ctx context.Context, h *host.Host, volumeID string) error {
	v, err := host.FindVolumeByID(ctx, volumeID)
	if err != nil {
		return errors.Wrapf(err, "getting volume '%s'", volumeID)
	}
	if v == nil {
		return errors.Errorf("volume '%s' not found", volumeID)
	}

	if err = m.setupClient(ctx); err != nil {
		return errors.Wrap(err, "creating client")
	}

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

	return errors.Wrapf(h.RemoveVolumeFromHost(ctx, volumeID), "detaching volume '%s' from host '%s' in DB", volumeID, h.Id)
}

func (m *ec2Manager) CreateVolume(ctx context.Context, volume *host.Volume) (*host.Volume, error) {
	if err := m.setupClient(ctx); err != nil {
		return nil, errors.Wrap(err, "creating client")
	}

	volume.Expiration = time.Now().Add(evergreen.DefaultSpawnHostExpiration)
	volumeTags := []types.Tag{
		{Key: aws.String(evergreen.TagOwner), Value: aws.String(volume.CreatedBy)},
		{Key: aws.String(evergreen.TagExpireOn), Value: aws.String(expireInDays(evergreen.SpawnHostExpireDays))},
	}
	input := &ec2.CreateVolumeInput{
		AvailabilityZone: aws.String(volume.AvailabilityZone),
		VolumeType:       types.VolumeType(volume.Type),
		Size:             aws.Int32(volume.Size),
		TagSpecifications: []types.TagSpecification{
			{ResourceType: types.ResourceTypeVolume, Tags: volumeTags},
		},
	}

	if volume.Throughput > 0 {
		input.Throughput = aws.Int32(volume.Throughput)
	}

	if volume.IOPS > 0 {
		input.Iops = aws.Int32(volume.IOPS)
	} else if volume.Type == VolumeTypeIo1 { // Iops is required for io1.
		input.Iops = aws.Int32(defaultIops)
	}

	resp, err := m.client.CreateVolume(ctx, input)

	if err != nil {
		return nil, errors.Wrap(err, "creating volume in client")
	}
	if resp.VolumeId == nil {
		return nil, errors.New("new volume returned by EC2 does not have an ID")
	}

	volume.ID = *resp.VolumeId
	if err = volume.Insert(ctx); err != nil {
		return nil, errors.Wrap(err, "creating volume in DB")
	}

	return volume, nil
}

func (m *ec2Manager) DeleteVolume(ctx context.Context, volume *host.Volume) error {
	if err := m.setupClient(ctx); err != nil {
		return errors.Wrap(err, "creating client")
	}

	_, err := m.client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
		VolumeId: aws.String(volume.ID),
	})
	if err != nil {
		return errors.Wrapf(err, "deleting volume '%s' in client", volume.ID)
	}

	return errors.Wrapf(volume.Remove(ctx), "deleting volume '%s' in DB", volume.ID)
}

func (m *ec2Manager) GetVolumeAttachment(ctx context.Context, volumeID string) (*VolumeAttachment, error) {
	if err := m.setupClient(ctx); err != nil {
		return nil, errors.Wrap(err, "creating client")
	}

	volumeInfo, err := m.client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
		VolumeIds: []string{volumeID},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "describing volume '%s'", volumeID)
	}

	if volumeInfo == nil || len(volumeInfo.Volumes) == 0 {
		return nil, errors.Errorf("no volume '%s' found in EC2", volumeID)
	}

	// no attachments found
	if len(volumeInfo.Volumes[0].Attachments) == 0 {
		return nil, nil
	}

	ec2Attachment := volumeInfo.Volumes[0].Attachments[0]
	if ec2Attachment.VolumeId == nil ||
		ec2Attachment.Device == nil ||
		ec2Attachment.InstanceId == nil {
		return nil, errors.Errorf("AWS returned an invalid volume attachment %+v", ec2Attachment)
	}

	attachment := &VolumeAttachment{
		VolumeID:   *ec2Attachment.VolumeId,
		DeviceName: *ec2Attachment.Device,
		HostID:     *ec2Attachment.InstanceId,
	}

	return attachment, nil
}

func (m *ec2Manager) modifyVolumeExpiration(ctx context.Context, volume *host.Volume, newExpiration time.Time) error {
	if err := volume.SetExpiration(ctx, newExpiration); err != nil {
		return errors.Wrapf(err, "updating expiration for volume '%s'", volume.ID)
	}

	_, err := m.client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: []string{volume.ID},
		Tags: []types.Tag{{
			Key:   aws.String(evergreen.TagExpireOn),
			Value: aws.String(newExpiration.Add(time.Hour * 24 * evergreen.SpawnHostExpireDays).Format(evergreen.ExpireOnFormat))}},
	})
	if err != nil {
		return errors.Wrapf(err, "updating expire-on tag for volume '%s'", volume.ID)
	}

	return nil
}

func (m *ec2Manager) ModifyVolume(ctx context.Context, volume *host.Volume, opts *model.VolumeModifyOptions) error {
	if err := m.setupClient(ctx); err != nil {
		return errors.Wrap(err, "creating client")
	}

	if !utility.IsZeroTime(opts.Expiration) {
		if err := m.modifyVolumeExpiration(ctx, volume, opts.Expiration); err != nil {
			return errors.Wrapf(err, "modifying volume '%s' expiration", volume.ID)
		}
		if err := volume.SetNoExpiration(ctx, false); err != nil {
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
		if err := volume.SetNoExpiration(ctx, true); err != nil {
			return errors.Wrapf(err, "setting volume '%s' no-expiration in DB", volume.ID)
		}
	}

	if opts.HasExpiration {
		if err := volume.SetNoExpiration(ctx, false); err != nil {
			return errors.Wrapf(err, "clearing volume '%s' no-expiration in DB", volume.ID)
		}
	}

	if opts.Size > 0 {
		_, err := m.client.ModifyVolume(ctx, &ec2.ModifyVolumeInput{
			VolumeId: aws.String(volume.ID),
			Size:     aws.Int32(opts.Size),
		})
		if err != nil {
			return errors.Wrapf(err, "modifying volume '%s' size in client", volume.ID)
		}
		if err = volume.SetSize(ctx, opts.Size); err != nil {
			return errors.Wrapf(err, "modifying volume '%s' size in DB", volume.ID)
		}
	}

	if opts.NewName != "" {
		if err := volume.SetDisplayName(ctx, opts.NewName); err != nil {
			return errors.Wrapf(err, "modifying volume '%s' name in DB", volume.ID)
		}
	}
	return nil
}

// GetDNSName returns the DNS name for the host.
func (m *ec2Manager) GetDNSName(ctx context.Context, h *host.Host) (string, error) {
	if err := m.setupClient(ctx); err != nil {
		return "", errors.Wrap(err, "creating client")
	}

	return m.client.GetPublicDNSName(ctx, h)
}

// TimeTilNextPayment returns how long until the next payment is due for a host.
func (m *ec2Manager) TimeTilNextPayment(host *host.Host) time.Duration {
	return timeTilNextEC2Payment(host)
}

func (m *ec2Manager) AssociateIP(ctx context.Context, h *host.Host) error {
	if h.IPAllocationID == "" {
		return nil
	}
	if err := m.setupClient(ctx); err != nil {
		return errors.Wrap(err, "creating client")
	}
	return errors.Wrapf(associateIPAddressForHost(ctx, m.client, h), "associating allocated IP address '%s' with host '%s'", h.IPAllocationID, h.Id)
}

// CleanupIP releases the host's IP address.
func (m *ec2Manager) CleanupIP(ctx context.Context, h *host.Host) error {
	return releaseIPAddressForHost(ctx, h)
}

// Cleanup is a noop for the EC2 provider.
func (m *ec2Manager) Cleanup(context.Context) error {
	return nil
}
