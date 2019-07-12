package cloud

import (
	"context"
	"math"
	"os"
	"os/user"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
	ec2aws "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	spawnHostExpireDays = 30
	mciHostExpireDays   = 10
)

//Valid values for EC2 instance states:
//pending | running | shutting-down | terminated | stopping | stopped
//see http://goo.gl/3OrCGn
const (
	EC2StatusPending      = "pending"
	EC2StatusRunning      = "running"
	EC2StatusShuttingdown = "shutting-down"
	EC2StatusTerminated   = "terminated"
	EC2StatusStopped      = "stopped"
	EC2ErrorNotFound      = "InvalidInstanceID.NotFound"
)

type MountPoint struct {
	VirtualName string `mapstructure:"virtual_name" json:"virtual_name,omitempty" bson:"virtual_name,omitempty"`
	DeviceName  string `mapstructure:"device_name" json:"device_name,omitempty" bson:"device_name,omitempty"`
	Size        int64  `mapstructure:"size" json:"size,omitempty" bson:"size,omitempty"`
	Iops        int64  `mapstructure:"iops" json:"iops,omitempty" bson:"iops,omitempty"`
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
	// bson fields for the EC2SpotSettings struct
	BidPriceKey = bsonutil.MustHaveTag(EC2ProviderSettings{}, "BidPrice")
)

var (
	// bson fields for the MountPoint struct
	VirtualNameKey = bsonutil.MustHaveTag(MountPoint{}, "VirtualName")
	DeviceNameKey  = bsonutil.MustHaveTag(MountPoint{}, "DeviceName")
	SizeKey        = bsonutil.MustHaveTag(MountPoint{}, "Size")
	VolumeTypeKey  = bsonutil.MustHaveTag(MountPoint{}, "VolumeType")
)

// type/consts for price evaluation based on OS
type osType string

const (
	osLinux   osType = "Linux/UNIX"
	osSUSE    osType = "SUSE Linux"
	osWindows osType = "Windows"
)

// regionFullname takes the API ID of amazon region and returns the
// full region name. For instance, "us-west-1" becomes "US West (N. California)".
// This is necessary as the On Demand pricing endpoint uses the full name, unlike
// the rest of the API. THIS FUNCTION ONLY HANDLES U.S. REGIONS.
func regionFullname(region string) (string, error) {
	switch region {
	case "us-east-1":
		return "US East (N. Virginia)", nil
	case "us-west-1":
		return "US West (N. California)", nil
	case "us-west-2":
		return "US West (Oregon)", nil
	}
	return "", errors.Errorf("region %v not supported for On Demand cost calculation", region)
}

// azToRegion takes an availability zone and returns the region id.
func azToRegion(az string) string {
	// an amazon region is just the availability zone minus the final letter
	return az[:len(az)-1]
}

// returns the format of os name expected by EC2 On Demand billing data,
// bucking the normal AWS API naming scheme.
func osBillingName(os osType) string {
	if os == osLinux {
		return "Linux"
	}
	return string(os)
}

//ec2StatusToEvergreenStatus returns a "universal" status code based on EC2's
//provider-specific status codes.
func ec2StatusToEvergreenStatus(ec2Status string) CloudStatus {
	switch ec2Status {
	case EC2StatusPending:
		return StatusInitializing
	case EC2StatusRunning:
		return StatusRunning
	case EC2StatusShuttingdown:
		return StatusTerminated
	case EC2StatusTerminated:
		return StatusTerminated
	case EC2StatusStopped:
		return StatusStopped
	default:
		return StatusUnknown
	}
}

// expireInDays creates an expire-on string in the format YYYY-MM-DD for numDays days
// in the future.
func expireInDays(numDays int) string {
	return time.Now().AddDate(0, 0, numDays).Format("2006-01-02")
}

//makeTags populates a map of tags based on a host object, which contain keys
//for the user, owner, hostname, and if it's a spawnhost or not.
func makeTags(intentHost *host.Host) map[string]string {
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
	expireOn := expireInDays(mciHostExpireDays)
	if intentHost.UserHost {
		// If this is a spawn host, use a different expiration date.
		expireOn = expireInDays(spawnHostExpireDays)
	}

	tags := map[string]string{
		"name":              intentHost.Id,
		"distro":            intentHost.Distro.Id,
		"evergreen-service": hostname,
		"username":          username,
		"owner":             intentHost.StartedBy,
		"mode":              "production",
		"start-time":        intentHost.CreationTime.Format(evergreen.NameTimeFormat),
		"expire-on":         expireOn,
	}

	if intentHost.UserHost {
		tags["mode"] = "testing"
	}
	return tags
}

func timeTilNextEC2Payment(h *host.Host) time.Duration {
	if usesHourlyBilling(h) {
		return timeTilNextHourlyPayment(h)
	}

	upTime := time.Since(h.StartTime)
	if upTime < time.Minute {
		return time.Minute - upTime
	}

	return time.Second
}

func usesHourlyBilling(h *host.Host) bool { return !strings.Contains(h.Distro.Arch, "linux") }

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

func getEc2SSHOptions(h *host.Host, keyName string) ([]string, error) {
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

func getKey(ctx context.Context, client AWSClient, credentials *credentials.Credentials, h *host.Host) (string, error) {
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

	newKey, err := makeNewKey(ctx, client, credentials, t.Project, h)
	if err != nil {
		return "", errors.Wrap(err, "problem creating new key")
	}
	return newKey, nil
}

func makeNewKey(ctx context.Context, client AWSClient, credentials *credentials.Credentials, project string, h *host.Host) (string, error) {
	r, err := getRegion(h)
	if err != nil {
		return "", errors.Wrap(err, "problem getting region from host")
	}
	if err = client.Create(credentials, r); err != nil {
		return "", errors.Wrap(err, "error creating client")
	}
	defer client.Close()

	name := "evg_auto_" + project
	_, err = client.DeleteKeyPair(ctx, &ec2.DeleteKeyPairInput{KeyName: aws.String(name)})
	if err != nil { // error does not indicate a problem, but log anyway for debugging
		grip.Debug(message.WrapError(err, message.Fields{
			"message":  "problem deleting key",
			"key_name": name,
		}))
	}
	resp, err := client.CreateKeyPair(ctx, &ec2.CreateKeyPairInput{KeyName: aws.String(name)})
	if err != nil {
		return "", errors.Wrap(err, "problem creating key pair")
	}

	if err := model.SetAWSKeyForProject(project, &model.AWSSSHKey{Name: name, Value: *resp.KeyMaterial}); err != nil {
		return "", errors.Wrap(err, "problem setting key")
	}

	return *resp.KeyMaterial, nil
}

func setTags(ctx context.Context, resources []*string, h *host.Host, client AWSClient, credentials *credentials.Credentials) error {
	r, err := getRegion(h)
	if err != nil {
		return errors.Wrap(err, "problem getting region from host")
	}
	if err = client.Create(credentials, r); err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer client.Close()

	tags := makeTags(h)
	tagSlice := []*ec2.Tag{}
	for tag := range tags {
		key := tag
		val := tags[tag]
		tagSlice = append(tagSlice, &ec2.Tag{Key: &key, Value: &val})
	}
	if _, err = client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: resources,
		Tags:      tagSlice,
	}); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error attaching tags",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return errors.Wrapf(err, "failed to attach tags for %s", h.Id)
	}

	grip.Debug(message.Fields{
		"message":       "attached tags for host",
		"host":          h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
	})

	return nil
}

func expandUserData(userData string, expansions map[string]string) (string, error) {
	exp := util.NewExpansions(expansions)
	expanded, err := exp.ExpandString(userData)
	if err != nil {
		return "", errors.Wrap(err, "error expanding userdata script")
	}
	return expanded, nil
}

// ebsRegex extracts EBS Price JSON data from Amazon's UI.
var ebsRegex = regexp.MustCompile(`(?s)callback\((.*)\)`)

// odInfo is an internal type for keying hosts by the attributes that affect billing.
type odInfo struct {
	os       string
	instance string
	region   string
}

// Terms is an internal type for loading price API results into.
type Terms struct {
	OnDemand map[string]map[string]struct {
		PriceDimensions map[string]struct {
			PricePerUnit struct {
				USD string
			}
		}
	}
}

func makeBlockDeviceMappings(mounts []MountPoint) ([]*ec2aws.BlockDeviceMapping, error) {
	if len(mounts) == 0 {
		return nil, nil
	}
	mappings := []*ec2aws.BlockDeviceMapping{}
	for _, mount := range mounts {
		if mount.DeviceName == "" {
			return nil, errors.New("missing device name")
		}
		if mount.VirtualName == "" && mount.Size == 0 {
			return nil, errors.New("must provide either a virtual name or an EBS size")
		}

		m := &ec2aws.BlockDeviceMapping{
			DeviceName: aws.String(mount.DeviceName),
		}
		// Without a virtual name, this is EBS
		if mount.VirtualName == "" {
			m.Ebs = &ec2aws.EbsBlockDevice{
				DeleteOnTermination: aws.Bool(true),
				VolumeSize:          aws.Int64(mount.Size),
				VolumeType:          aws.String(ec2aws.VolumeTypeGp2),
			}
			if mount.Iops != 0 {
				m.Ebs.Iops = aws.Int64(mount.Iops)
			}
			if mount.SnapshotID != "" {
				m.Ebs.SnapshotId = aws.String(mount.SnapshotID)
			}
			if mount.VolumeType != "" {
				m.Ebs.VolumeType = aws.String(mount.VolumeType)
			}
		} else { // With a virtual name, this is an instance store
			m.VirtualName = aws.String(mount.VirtualName)
		}
		mappings = append(mappings, m)
	}
	return mappings, nil
}

func makeBlockDeviceMappingsTemplate(mounts []MountPoint) ([]*ec2aws.LaunchTemplateBlockDeviceMappingRequest, error) {
	if len(mounts) == 0 {
		return nil, nil
	}
	mappings := []*ec2aws.LaunchTemplateBlockDeviceMappingRequest{}
	for _, mount := range mounts {
		if mount.DeviceName == "" {
			return nil, errors.New("missing device name")
		}
		if mount.VirtualName == "" && mount.Size == 0 {
			return nil, errors.New("must provide either a virtual name or an EBS size")
		}

		m := &ec2aws.LaunchTemplateBlockDeviceMappingRequest{
			DeviceName: aws.String(mount.DeviceName),
		}
		// Without a virtual name, this is EBS
		if mount.VirtualName == "" {
			m.Ebs = &ec2aws.LaunchTemplateEbsBlockDeviceRequest{
				DeleteOnTermination: aws.Bool(true),
				VolumeSize:          aws.Int64(mount.Size),
				VolumeType:          aws.String(ec2aws.VolumeTypeGp2),
			}
			if mount.Iops != 0 {
				m.Ebs.Iops = aws.Int64(mount.Iops)
			}
			if mount.SnapshotID != "" {
				m.Ebs.SnapshotId = aws.String(mount.SnapshotID)
			}
			if mount.VolumeType != "" {
				m.Ebs.VolumeType = aws.String(mount.VolumeType)
			}
		} else { // With a virtual name, this is an instance store
			m.VirtualName = aws.String(mount.VirtualName)
		}
		mappings = append(mappings, m)
	}
	return mappings, nil
}

func validateEc2CreateTemplateResponse(createTemplateResponse *ec2aws.CreateLaunchTemplateOutput) error {
	if createTemplateResponse == nil || createTemplateResponse.LaunchTemplate == nil {
		return errors.New("create template response launch template is nil")
	}

	catcher := grip.NewBasicCatcher()
	if createTemplateResponse.LaunchTemplate.LaunchTemplateId == nil || len(*createTemplateResponse.LaunchTemplate.LaunchTemplateId) == 0 {
		catcher.Add(errors.New("create template response has no template identifier"))
	}

	if createTemplateResponse.LaunchTemplate.LatestVersionNumber == nil {
		catcher.Add(errors.New("create template response has no latest version"))
	}

	return catcher.Resolve()
}

func validateEc2CreateFleetResponse(createFleetResponse *ec2aws.CreateFleetOutput) error {
	if createFleetResponse == nil {
		return errors.New("create fleet response is nil")
	}

	if len(createFleetResponse.Instances) == 0 || len(createFleetResponse.Instances[0].InstanceIds) == 0 {
		return errors.New("no instance ID in create fleet response")
	}

	return nil
}

func validateEc2DescribeInstancesOutput(describeInstancesResponse *ec2aws.DescribeInstancesOutput) error {
	catcher := grip.NewBasicCatcher()
	for _, reservation := range describeInstancesResponse.Reservations {
		if len(reservation.Instances) == 0 {
			catcher.Add(errors.New("reservation missing instance"))
		} else {
			if reservation.Instances[0].InstanceId == nil {
				catcher.Add(errors.New("instance missing instance id"))
			}
			if reservation.Instances[0].State == nil || reservation.Instances[0].State.Name == nil || len(*reservation.Instances[0].State.Name) == 0 {
				catcher.Add(errors.New("instance missing state name"))
			}
		}

	}

	return catcher.Resolve()
}

func validateEc2InstanceInfoResponse(instance *ec2aws.Instance) error {
	if instance == nil {
		return errors.New("instance is nil")
	}

	catcher := grip.NewBasicCatcher()
	if instance.Placement == nil || instance.Placement.AvailabilityZone == nil || len(*instance.Placement.AvailabilityZone) == 0 {
		catcher.Add(errors.New("AZ is missing"))
	}
	if instance.LaunchTime == nil {
		catcher.Add(errors.New("launch time is nil"))
	}
	if instance.PublicDnsName == nil || len(*instance.PublicDnsName) == 0 {
		catcher.Add(errors.New("dns name is missing"))
	}
	if instance.State == nil || instance.State.Name == nil || len(*instance.State.Name) == 0 {
		catcher.Add(errors.New("state name is missing"))
	}

	return catcher.Resolve()
}
