package cloud

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/user"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53Types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/smithy-go"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	EC2ErrorNotFound        = "InvalidInstanceID.NotFound"
	EC2DuplicateKeyPair     = "InvalidKeyPair.Duplicate"
	EC2InsufficientCapacity = "InsufficientInstanceCapacity"
	EC2InvalidParam         = "InvalidParameterValue"
	EC2VolumeNotFound       = "InvalidVolume.NotFound"
	EC2VolumeResizeRate     = "VolumeModificationRateExceeded"
	ec2TemplateNameExists   = "InvalidLaunchTemplateName.AlreadyExistsException"

	r53InvalidInput       = "InvalidInput"
	r53InvalidChangeBatch = "InvalidChangeBatch"

	// This means the role doing the assuming does not have permission to assume it.
	stsErrorAccessDenied = "AccessDenied"
	// This means the role to be assumed does not exist or does not have a trust relationship with the role doing the assuming.
	stsErrorAssumeRoleAccessDenied = "AssumeRoleAccessDenied"
)

var (
	EC2InsufficientCapacityError = errors.New(EC2InsufficientCapacity)
	ec2TemplateNameExistsError   = errors.New(ec2TemplateNameExists)

	// Linux and Windows are billed by the second.
	// See https://aws.amazon.com/ec2/pricing/on-demand/
	byTheSecondBillingOS = []string{"linux", "windows"}

	// Commercial Linux distributions are billed by the hour
	// See https://aws.amazon.com/linux/commercial-linux/faqs/#Pricing_and_Billing
	commercialLinuxDistros = []string{"suse"}
)

type MountPoint struct {
	VirtualName string `mapstructure:"virtual_name" json:"virtual_name,omitempty" bson:"virtual_name,omitempty"`
	DeviceName  string `mapstructure:"device_name" json:"device_name,omitempty" bson:"device_name,omitempty"`
	Size        int32  `mapstructure:"size" json:"size,omitempty" bson:"size,omitempty"`
	Iops        int32  `mapstructure:"iops" json:"iops,omitempty" bson:"iops,omitempty"`
	Throughput  int32  `mapstructure:"throughput" json:"throughput,omitempty" bson:"throughput,omitempty"`
	SnapshotID  string `mapstructure:"snapshot_id" json:"snapshot_id,omitempty" bson:"snapshot_id,omitempty"`
	VolumeType  string `mapstructure:"volume_type" json:"volume_type,omitempty" bson:"volume_type,omitempty"`
}

var (
	// bson fields for the EC2ProviderSettings struct
	AMIKey            = bsonutil.MustHaveTag(EC2ProviderSettings{}, "AMI")
	InstanceTypeKey   = bsonutil.MustHaveTag(EC2ProviderSettings{}, "InstanceType")
	SecurityGroupsKey = bsonutil.MustHaveTag(EC2ProviderSettings{}, "SecurityGroupIDs")
	KeyNameKey        = bsonutil.MustHaveTag(EC2ProviderSettings{}, "KeyName")
	MountPointsKey    = bsonutil.MustHaveTag(EC2ProviderSettings{}, "MountPoints")
)

var (
	// bson fields for the MountPoint struct
	VirtualNameKey = bsonutil.MustHaveTag(MountPoint{}, "VirtualName")
	DeviceNameKey  = bsonutil.MustHaveTag(MountPoint{}, "DeviceName")
	SizeKey        = bsonutil.MustHaveTag(MountPoint{}, "Size")
	VolumeTypeKey  = bsonutil.MustHaveTag(MountPoint{}, "VolumeType")
)

// AztoRegion takes an availability zone and returns the region id.
func AztoRegion(az string) string {
	// an amazon region is just the availability zone minus the final letter
	return az[:len(az)-1]
}

// ec2StateToEvergreenStatus returns a "universal" status code based on EC2's
// provider-specific status codes.
func ec2StateToEvergreenStatus(ec2State *types.InstanceState) CloudStatus {
	if ec2State == nil {
		return StatusUnknown
	}
	switch ec2State.Name {
	case types.InstanceStateNamePending:
		return StatusInitializing
	case types.InstanceStateNameRunning:
		return StatusRunning
	case types.InstanceStateNameStopped:
		return StatusStopped
	case types.InstanceStateNameStopping:
		return StatusStopping
	case types.InstanceStateNameTerminated, types.InstanceStateNameShuttingDown:
		return StatusTerminated
	default:
		grip.Error(message.Fields{
			"message": "got an unknown EC2 state name",
			"status":  ec2State.Name,
		})
		return StatusUnknown
	}
}

// expireInDays creates an expire-on string in the format YYYY-MM-DD for numDays days
// in the future.
func expireInDays(numDays int) string {
	return time.Now().AddDate(0, 0, numDays).Format(evergreen.ExpireOnFormat)
}

// makeTags populates a slice of tags based on a host object, which contain keys
// for the user, owner, hostname, and if it's a spawnhost or not.
func makeTags(intentHost *host.Host) []host.Tag {
	// get requester host name
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// get requester user name
	var username string
	user, err := user.Current()
	if err != nil {
		username = "unknown"
	} else {
		username = user.Name
	}

	// The expire-on tag is required by MongoDB's AWS reaping policy.
	// The reaper is an external script that scans every ec2 instance for an expire-on tag,
	// and if that tag is passed the reaper terminates the host. This reaping occurs to
	// ensure that any hosts that we forget about or that fail to terminate do not stay alive
	// forever.
	expireOn := expireInDays(evergreen.HostExpireDays)
	if intentHost.UserHost {
		// If this is a spawn host, use a different expiration date.
		expireOn = expireInDays(evergreen.SpawnHostExpireDays)
	}

	systemTags := []host.Tag{
		{Key: evergreen.TagName, Value: intentHost.Id, CanBeModified: false},
		{Key: evergreen.TagDistro, Value: intentHost.Distro.Id, CanBeModified: false},
		{Key: evergreen.TagEvergreenService, Value: hostname, CanBeModified: false},
		{Key: evergreen.TagUsername, Value: username, CanBeModified: false},
		{Key: evergreen.TagOwner, Value: intentHost.StartedBy, CanBeModified: false},
		{Key: evergreen.TagMode, Value: "production", CanBeModified: false},
		{Key: evergreen.TagStartTime, Value: intentHost.CreationTime.Format(evergreen.NameTimeFormat), CanBeModified: false},
		{Key: evergreen.TagExpireOn, Value: expireOn, CanBeModified: false},
		{Key: evergreen.TagAllowRemoteAccess, Value: "true", CanBeModified: false},
	}

	if intentHost.UserHost {
		systemTags = append(systemTags, host.Tag{Key: "mode", Value: "testing", CanBeModified: false})
	}

	// Add Evergreen-generated tags to host object
	intentHost.AddTags(systemTags)

	return intentHost.InstanceTags
}

func hostToEC2Tags(hostTags []host.Tag) []types.Tag {
	var tags []types.Tag
	for _, tag := range hostTags {
		tags = append(tags, types.Tag{Key: aws.String(tag.Key), Value: aws.String(tag.Value)})
	}
	return tags
}

func makeTagTemplate(hostTags []host.Tag) []types.LaunchTemplateTagSpecificationRequest {
	tags := hostToEC2Tags(hostTags)
	tagTemplates := []types.LaunchTemplateTagSpecificationRequest{
		{
			ResourceType: types.ResourceTypeInstance,
			Tags:         tags,
		},
		// every host has at least a root volume that needs to be tagged
		{
			ResourceType: types.ResourceTypeVolume,
			Tags:         tags,
		},
	}

	return tagTemplates
}

func makeTagSpecifications(hostTags []host.Tag) []types.TagSpecification {
	tags := hostToEC2Tags(hostTags)
	return []types.TagSpecification{
		{
			ResourceType: types.ResourceTypeInstance,
			Tags:         tags,
		},
		{
			ResourceType: types.ResourceTypeVolume,
			Tags:         tags,
		},
	}
}

func timeTilNextEC2Payment(h *host.Host) time.Duration {
	if UsesHourlyBilling(&h.Distro) {
		return timeTilNextHourlyPayment(h)
	}

	upTime := time.Since(h.StartTime)
	if upTime < time.Minute {
		return time.Minute - upTime
	}

	return time.Second
}

// UsesHourlyBilling returns if a distro is billed hourly.
func UsesHourlyBilling(d *distro.Distro) bool {
	byTheSecondOS := false
	for _, arch := range byTheSecondBillingOS {
		byTheSecondOS = byTheSecondOS || strings.Contains(d.Arch, arch)
	}

	commercialLinuxDistro := false
	for _, distro := range commercialLinuxDistros {
		commercialLinuxDistro = commercialLinuxDistro || strings.Contains(d.Id, distro)
	}

	return !byTheSecondOS || commercialLinuxDistro
}

// Determines how long until a payment is due for the specified host, for hosts
// that bill hourly. Returns the next time that it would take for the host to be
// up for an integer number of hours
func timeTilNextHourlyPayment(host *host.Host) time.Duration {
	now := time.Now()
	var startTime time.Time
	if host.StartTime.After(host.CreationTime) {
		startTime = host.StartTime
	} else {
		startTime = host.CreationTime
	}

	// the time since the host was started
	timeSinceCreation := now.Sub(startTime)

	// the hours since the host was created, rounded up
	hoursRoundedUp := time.Duration(math.Ceil(timeSinceCreation.Hours()))

	// the next round number of hours the host will have been up - the time
	// that the next payment will be due
	nextPaymentTime := startTime.Add(hoursRoundedUp * time.Hour)

	return nextPaymentTime.Sub(now)
}

func expandUserData(userData string, expansions map[string]string) (string, error) {
	exp := util.NewExpansions(expansions)
	expanded, err := exp.ExpandString(userData)
	if err != nil {
		return "", errors.Wrap(err, "expanding user data script")
	}
	return expanded, nil
}

// 16kB
const userDataSizeLimit = 16 * 1024

func validateUserDataSize(userData, distroID string) error {
	if len(userData) < userDataSizeLimit {
		return nil
	}
	err := errors.New("user data size limit exceeded")
	grip.Error(message.WrapError(err, message.Fields{
		"size":     len(userData),
		"max_size": userDataSizeLimit,
		"distro":   distroID,
	}))
	return errors.WithStack(err)
}

// hostInstancePair represents a host and its associated EC2 instance.
type hostInstancePair struct {
	host     *host.Host
	instance *types.Instance
}

// cacheAllHostData caches information about an EC2 instance for the associated
// host.
func cacheAllHostData(ctx context.Context, env evergreen.Environment, client AWSClient, pairs ...hostInstancePair) error {
	catcher := grip.NewBasicCatcher()
	hostsToCache := map[string]host.CloudProviderData{}
	for _, hostAndInstance := range pairs {
		h := hostAndInstance.host
		instance := hostAndInstance.instance
		data, err := makeCloudProviderData(instance)
		if err != nil {
			catcher.Wrapf(err, "making cloud provider data for host '%s'", h.Id)
			continue
		}

		hostsToCache[h.Id] = *data

		if h.NoExpiration {
			// This is not a bulk operation for convenience because it's assumed
			// that the number of unexpirable hosts is small.
			grip.Error(message.WrapError(setHostPersistentDNSName(ctx, env, h, utility.FromStringPtr(instance.PublicIpAddress), client), message.Fields{
				"message":    "could not update host's persistent DNS name",
				"op":         "upsert",
				"dashboard":  "evergreen sleep schedule health",
				"host_id":    h.Id,
				"started_by": h.StartedBy,
			}))
		}

		h.Zone = data.Zone
		h.StartTime = data.StartedAt
		h.Host = data.PublicDNS
		h.PublicIPv4 = data.PublicIPv4
		h.IPv4 = data.PrivateIPv4
		h.IP = data.IPv6
		h.Volumes = data.Volumes
	}

	catcher.Wrap(host.CacheAllCloudProviderData(ctx, env, hostsToCache), "bulk caching host data")

	return catcher.Resolve()
}

func makeCloudProviderData(instance *types.Instance) (*host.CloudProviderData, error) {
	if err := validateCloudProviderData(instance); err != nil {
		return nil, errors.Wrap(err, "EC2 provided invalid instance data")
	}

	var ipv6 string
	for _, networkInterface := range instance.NetworkInterfaces {
		if len(networkInterface.Ipv6Addresses) > 0 {
			ipv6 = *networkInterface.Ipv6Addresses[0].Ipv6Address
			break
		}
	}

	return &host.CloudProviderData{
		Zone:        utility.FromStringPtr(instance.Placement.AvailabilityZone),
		StartedAt:   utility.FromTimePtr(instance.LaunchTime),
		PublicDNS:   utility.FromStringPtr(instance.PublicDnsName),
		Volumes:     makeVolumeAttachments(instance.BlockDeviceMappings),
		PublicIPv4:  utility.FromStringPtr(instance.PublicIpAddress),
		PrivateIPv4: utility.FromStringPtr(instance.PrivateIpAddress),
		IPv6:        ipv6,
	}, nil
}

// validateCloudProviderData checks that all the required instance data from the
// cloud provider is present to cache it in the host.
func validateCloudProviderData(instance *types.Instance) error {
	catcher := grip.NewBasicCatcher()
	catcher.ErrorfWhen(instance.Placement == nil || instance.Placement.AvailabilityZone == nil, "instance missing availability zone")
	catcher.ErrorfWhen(instance.LaunchTime == nil, "instance missing launch time")
	catcher.ErrorfWhen(instance.PublicDnsName == nil, "instance missing public DNS name")
	catcher.ErrorfWhen(instance.PublicIpAddress == nil, "instance missing public IP address")
	catcher.ErrorfWhen(instance.PrivateIpAddress == nil, "instance missing private IP address")
	return catcher.Resolve()
}

// persistentDNSRecordTTLSecs is the number of seconds that a DNS record can be
// cached. It's intentionally very low so that changes to the DNS name's IP
// address propagate quickly to users.
const persistentDNSRecordTTLSecs = 1

// setHostPersistentDNSName sets a host's persistent DNS record with its
// associated IP address and sets it on the host.
func setHostPersistentDNSName(ctx context.Context, env evergreen.Environment, h *host.Host, ipv4Addr string, client AWSClient) error {
	if ipv4Addr == "" {
		return errors.New("instance did not include an IP address")
	}

	settings := env.Settings()
	if settings.Providers.AWS.PersistentDNS.Domain == "" || settings.Providers.AWS.PersistentDNS.HostedZoneID == "" {
		return errors.New("missing required domain and hosted zone ID in persistent DNS admin settings")
	}

	dnsName, err := h.GeneratePersistentDNSName(ctx, settings.Providers.AWS.PersistentDNS.Domain)
	if err != nil {
		return errors.Wrap(err, "getting host's persistent DNS name")
	}
	in := route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(settings.Providers.AWS.PersistentDNS.HostedZoneID),
		ChangeBatch: &r53Types.ChangeBatch{
			Changes: []r53Types.Change{
				{
					Action: r53Types.ChangeActionUpsert,
					ResourceRecordSet: &r53Types.ResourceRecordSet{
						Name: aws.String(dnsName),
						Type: r53Types.RRTypeA,
						ResourceRecords: []r53Types.ResourceRecord{
							{Value: aws.String(ipv4Addr)},
						},
						TTL: aws.Int64(persistentDNSRecordTTLSecs),
					},
				},
			},
		},
	}
	if _, err := client.ChangeResourceRecordSets(ctx, &in); err != nil {
		return errors.Wrapf(err, "upserting persistent DNS name '%s' for host '%s' into Route 53", dnsName, h.Id)
	}
	if err := h.SetPersistentDNSInfo(ctx, dnsName, ipv4Addr); err != nil {
		return errors.Wrap(err, "setting host's persistent DNS name")
	}

	return nil
}

// deleteHostPersistentDNSName deletes a host's persistent DNS record and unsets
// it from the host.
func deleteHostPersistentDNSName(ctx context.Context, env evergreen.Environment, h *host.Host, client AWSClient) error {
	if h.PersistentDNSName == "" || h.PublicIPv4 == "" {
		return nil
	}

	settings := env.Settings()
	if settings.Providers.AWS.PersistentDNS.Domain == "" || settings.Providers.AWS.PersistentDNS.HostedZoneID == "" {
		return errors.New("missing required domain and hosted zone ID in persistent DNS admin settings")
	}

	in := route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(settings.Providers.AWS.PersistentDNS.HostedZoneID),
		ChangeBatch: &r53Types.ChangeBatch{
			Changes: []r53Types.Change{
				{
					Action: r53Types.ChangeActionDelete,
					ResourceRecordSet: &r53Types.ResourceRecordSet{
						// The record name, value, type, and TTL must match the
						// one in Route 53 exactly or else this will not be able
						// to delete.
						Name: aws.String(h.PersistentDNSName),
						Type: r53Types.RRTypeA,
						ResourceRecords: []r53Types.ResourceRecord{
							{Value: aws.String(h.PublicIPv4)},
						},
						TTL: aws.Int64(persistentDNSRecordTTLSecs),
					},
				},
			},
		},
	}
	if _, err := client.ChangeResourceRecordSets(ctx, &in); err != nil {
		return errors.Wrapf(err, "deleting persistent DNS name '%s' for host '%s' in Route 53", h.PersistentDNSName, h.Id)
	}
	if err := h.UnsetPersistentDNSInfo(ctx); err != nil {
		return errors.Wrap(err, "setting host's persistent DNS name")
	}

	return nil
}

// templateNameInvalidRegex matches any character that may not be included a launch template name.
// Names may only contain word characters ([a-zA-Z0-9_]) and the following special characters: ( ) . / -
var templateNameInvalidRegex = regexp.MustCompile("[^\\w()./-]+") //nolint:gosimple

func cleanLaunchTemplateName(name string) string {
	return templateNameInvalidRegex.ReplaceAllString(name, "")
}

// formats /dev/sd[f-p]and xvd[f-p] taken from https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/device_naming.html
func generateDeviceNameForVolume(opts generateDeviceNameOptions) (string, error) {
	letters := "fghijklmnop"
	pattern := "/dev/sd%c"
	if opts.isWindows {
		pattern = "xvd%c"
	}
	for _, char := range letters {
		curName := fmt.Sprintf(pattern, char)
		if !utility.StringSliceContains(opts.existingDeviceNames, curName) {
			return curName, nil
		}
	}
	return "", errors.New("no available device names to generate")
}

func makeBlockDeviceMappings(mounts []MountPoint) ([]types.BlockDeviceMapping, error) {
	if len(mounts) == 0 {
		return nil, nil
	}
	mappings := []types.BlockDeviceMapping{}
	for _, mount := range mounts {
		if mount.DeviceName == "" {
			return nil, errors.New("missing device name")
		}
		if mount.VirtualName == "" && mount.Size == 0 {
			return nil, errors.New("must provide either a virtual name or an EBS size")
		}

		m := types.BlockDeviceMapping{
			DeviceName: aws.String(mount.DeviceName),
		}
		// Without a virtual name, this is EBS
		if mount.VirtualName == "" {
			m.Ebs = &types.EbsBlockDevice{
				DeleteOnTermination: aws.Bool(true),
				VolumeSize:          aws.Int32(mount.Size),
				VolumeType:          types.VolumeTypeGp2,
			}
			if mount.Iops != 0 {
				m.Ebs.Iops = aws.Int32(mount.Iops)
			}
			if mount.SnapshotID != "" {
				m.Ebs.SnapshotId = aws.String(mount.SnapshotID)
			}
			if mount.VolumeType != "" {
				m.Ebs.VolumeType = types.VolumeType(mount.VolumeType)
			}
			if mount.Throughput != 0 {
				//aws only allows values between 125 and 1000
				if mount.Throughput > 1000 || mount.Throughput < 125 {
					return nil, errors.New("throughput must be between 125 and 1000")
				}
				// This parameter is valid only for gp3 volumes.
				if m.Ebs.VolumeType != types.VolumeTypeGp3 {
					return nil, errors.Errorf("throughput is not valid for volume type '%s', it is only valid for gp3 volumes", m.Ebs.VolumeType)
				}
				m.Ebs.Throughput = aws.Int32(mount.Throughput)
			}
		} else { // With a virtual name, this is an instance store
			m.VirtualName = aws.String(mount.VirtualName)
		}
		mappings = append(mappings, m)
	}
	return mappings, nil
}

func makeBlockDeviceMappingsTemplate(mounts []MountPoint) ([]types.LaunchTemplateBlockDeviceMappingRequest, error) {
	if len(mounts) == 0 {
		return nil, nil
	}
	mappings := []types.LaunchTemplateBlockDeviceMappingRequest{}
	for _, mount := range mounts {
		if mount.DeviceName == "" {
			return nil, errors.New("missing device name")
		}
		if mount.VirtualName == "" && mount.Size == 0 {
			return nil, errors.New("must provide either a virtual name or an EBS size")
		}

		m := types.LaunchTemplateBlockDeviceMappingRequest{
			DeviceName: aws.String(mount.DeviceName),
		}
		// Without a virtual name, this is EBS
		if mount.VirtualName == "" {
			m.Ebs = &types.LaunchTemplateEbsBlockDeviceRequest{
				DeleteOnTermination: aws.Bool(true),
				VolumeSize:          aws.Int32(mount.Size),
				VolumeType:          types.VolumeTypeGp2,
			}
			if mount.Iops != 0 {
				m.Ebs.Iops = aws.Int32(mount.Iops)
			}
			if mount.SnapshotID != "" {
				m.Ebs.SnapshotId = aws.String(mount.SnapshotID)
			}
			if mount.VolumeType != "" {
				m.Ebs.VolumeType = types.VolumeType(mount.VolumeType)
			}
			if mount.Throughput != 0 {
				//aws only allows values between 125 and 1000
				if mount.Throughput > 1000 || mount.Throughput < 125 {
					return nil, errors.New("throughput must be between 125 and 1000")
				}
				// This parameter is valid only for gp3 volumes.
				if m.Ebs.VolumeType != types.VolumeTypeGp3 {
					return nil, errors.Errorf("throughput is not valid for volume type '%s', it is only valid for gp3 volumes", m.Ebs.VolumeType)
				}
				m.Ebs.Throughput = aws.Int32(mount.Throughput)
			}
		} else { // With a virtual name, this is an instance store
			m.VirtualName = aws.String(mount.VirtualName)
		}
		mappings = append(mappings, m)
	}
	return mappings, nil
}

func makeVolumeAttachments(devices []types.InstanceBlockDeviceMapping) []host.VolumeAttachment {
	attachments := []host.VolumeAttachment{}
	for _, device := range devices {
		if device.Ebs != nil && device.Ebs.VolumeId != nil && device.DeviceName != nil {
			attachments = append(attachments, host.VolumeAttachment{
				VolumeID:   *device.Ebs.VolumeId,
				DeviceName: *device.DeviceName,
			})
		}
	}
	return attachments
}

func ec2CreateFleetResponseContainsInstance(createFleetResponse *ec2.CreateFleetOutput) bool {
	if createFleetResponse == nil {
		return false
	}

	if len(createFleetResponse.Instances) == 0 || len(createFleetResponse.Instances[0].InstanceIds) == 0 {
		return false
	}

	return true
}

func validateEc2DescribeInstancesOutput(describeInstancesResponse *ec2.DescribeInstancesOutput) error {
	catcher := grip.NewBasicCatcher()
	for _, reservation := range describeInstancesResponse.Reservations {
		if len(reservation.Instances) == 0 {
			catcher.New("reservation missing instance")
		} else {
			instance := reservation.Instances[0]
			catcher.NewWhen(instance.InstanceId == nil, "instance missing instance ID")
			catcher.NewWhen(instance.State == nil || instance.State.Name == "", "instance missing state name")
		}
	}

	return catcher.Resolve()
}

func getEC2ManagerOptionsFromSettings(provider string, settings *EC2ProviderSettings) ManagerOpts {
	region := settings.Region
	if region == "" {
		region = evergreen.DefaultEC2Region
	}
	return ManagerOpts{
		Provider: provider,
		Region:   region,
	}
}

func validateEC2HostModifyOptions(h *host.Host, opts host.HostModifyOptions) error {
	if opts.InstanceType != "" && h.Status != evergreen.HostStopped {
		return errors.New("host must be stopped to modify instance type")
	}
	return nil
}

func ValidVolumeOptions(v *host.Volume, s *evergreen.Settings) error {
	catcher := grip.NewBasicCatcher()
	if !utility.StringSliceContains(ValidVolumeTypes, v.Type) {
		catcher.Errorf("invalid volume type '%s', valid EBS volume types are: %s", v.Type, ValidVolumeTypes)
	}

	_, err := getSubnetForZone(s.Providers.AWS.Subnets, v.AvailabilityZone)
	catcher.Add(err)
	return catcher.Resolve()
}

func getSubnetForZone(subnets []evergreen.Subnet, zone string) (string, error) {
	zones := []string{}
	for _, subnet := range subnets {
		if subnet.AZ == zone {
			return subnet.SubnetID, nil
		}
		zones = append(zones, subnet.AZ)
	}
	return "", errors.Errorf("invalid availability zone '%s', valid availability zones are: %s", zone, zones)
}

// addSSHKey adds an SSH key for the given client. If an SSH key already exists
// with the given name, this no-ops.
func addSSHKey(ctx context.Context, client AWSClient, pair evergreen.SSHKeyPair) error {
	if _, err := client.ImportKeyPair(ctx, &ec2.ImportKeyPairInput{
		KeyName:           aws.String(pair.Name),
		PublicKeyMaterial: []byte(pair.Public),
	}); err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == EC2DuplicateKeyPair {
			return nil
		}
		return errors.Wrap(err, "importing public SSH key")
	}
	return nil
}

func AttachVolumeBadRequest(err error) bool {
	for _, noRetryError := range []string{EC2VolumeNotFound, EC2InvalidParam} {
		if strings.Contains(err.Error(), noRetryError) {
			return true
		}
	}
	return false
}

func ModifyVolumeBadRequest(err error) bool {
	for _, noRetryError := range []string{EC2VolumeNotFound, EC2VolumeResizeRate} {
		if strings.Contains(err.Error(), noRetryError) {
			return true
		}
	}
	return false
}

// IsEC2InstanceID returns whether or not a host's ID is an EC2 instance ID or
// not.
func IsEC2InstanceID(id string) bool {
	return strings.HasPrefix(id, "i-")
}

// Gp2EquivalentThroughputForGp3 returns a throughput value for gp3 volumes that's at least
// equivalent to the throughput of gp2 volumes.
// See https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/general-purpose.html for more information.
func Gp2EquivalentThroughputForGp3(volumeSize int32) int32 {
	if volumeSize <= 170 {
		return 128
	}
	return 250
}

// Gp2EquivalentIOPSForGp3 returns an IOPS value for gp3 volumes that's at least
// equivalent to the IOPS of gp2 volumes.
// See https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/general-purpose.html for more information.
func Gp2EquivalentIOPSForGp3(volumeSize int32) int32 {
	iops := volumeSize * 3

	if volumeSize <= 1000 {
		iops = 3000
	}
	if iops >= 16000 {
		iops = 16000
	}

	return iops
}

// isEC2InstanceNotFound returns whether or not the given error is due to the
// EC2 instance not being found.
func isEC2InstanceNotFound(err error) bool {
	if err == noReservationError {
		return true
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == EC2ErrorNotFound {
		return true
	}
	return false
}
