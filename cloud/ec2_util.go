package cloud

import (
	"math"
	"os"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"time"

	ec2aws "github.com/aws/aws-sdk-go/service/ec2"
	gcec2 "github.com/dynport/gocloud/aws/ec2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/anser/bsonutil"
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
	Size        int    `mapstructure:"size" json:"size,omitempty" bson:"size,omitempty"`
}

var (
	// bson fields for the EC2ProviderSettings struct
	AMIKey           = bsonutil.MustHaveTag(EC2ProviderSettings{}, "AMI")
	InstanceTypeKey  = bsonutil.MustHaveTag(EC2ProviderSettings{}, "InstanceType")
	SecurityGroupKey = bsonutil.MustHaveTag(EC2ProviderSettings{}, "SecurityGroup")
	KeyNameKey       = bsonutil.MustHaveTag(EC2ProviderSettings{}, "KeyName")
	MountPointsKey   = bsonutil.MustHaveTag(EC2ProviderSettings{}, "MountPoints")
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
)

// type/consts for price evaluation based on OS
type osType string

const (
	osLinux   osType = gcec2.DESC_LINUX_UNIX
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

// skuPrice digs through the incredibly verbose Amazon price data format
// for the USD dollar amount of an SKU. The for loops are for traversing
// maps of size one with an unknown key, which is simple to do in a
// language like python but really ugly here.
func (t Terms) skuPrice(sku string) float64 {
	for _, v := range t.OnDemand[sku] {
		for _, p := range v.PriceDimensions {
			// parse -- ignoring errors
			val, _ := strconv.ParseFloat(p.PricePerUnit.USD, 64)
			return val
		}
	}
	return 0
}

func makeBlockDeviceMappings(mounts []MountPoint) ([]*ec2aws.BlockDeviceMapping, error) {
	if len(mounts) == 0 {
		return nil, nil
	}
	mappings := []*ec2aws.BlockDeviceMapping{}
	for i, mount := range mounts {
		if mount.DeviceName == "" {
			return nil, errors.Errorf("missing 'device_name': %+v", mount)
		}
		if mount.VirtualName == "" {
			if mount.Size <= 0 {
				return nil, errors.Errorf("must provide either a virtual name or an EBS size")
			}
			// EBS - size but no virtual name
			mappings = append(mappings, &ec2aws.BlockDeviceMapping{
				DeviceName:  &mounts[i].DeviceName,
				VirtualName: &mounts[i].VirtualName,
				Ebs: &ec2aws.EbsBlockDevice{
					DeleteOnTermination: makeBoolPtr(true),
				},
			})
		}
		// instance store - virtual name but no size
		mappings = append(mappings, &ec2aws.BlockDeviceMapping{
			DeviceName:  &mounts[i].DeviceName,
			VirtualName: &mounts[i].VirtualName,
		})
	}
	return mappings, nil
}

// makeInt64Ptr is necessary because Go does not allow you to write `&int64(1)`.
func makeInt64Ptr(i int64) *int64 {
	return &i
}

// makeStringPtr is necessary because Go does not allow you to write `&"foo"`.
func makeStringPtr(s string) *string {
	return &s
}

// makeBoolPtr is necessary because Go does not allow you to write `&true`.
func makeBoolPtr(b bool) *bool {
	return &b
}

// makeTimePtr is necessary because Go does not allow you to write `&time.Time`.
func makeTimePtr(t time.Time) *time.Time {
	return &t
}
