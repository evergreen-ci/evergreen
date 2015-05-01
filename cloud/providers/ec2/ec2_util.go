package ec2

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
	"math"
	"math/rand"
	"os"
	"os/user"
	"time"
)

const (
	OnDemandProviderName = "ec2"
	SpotProviderName     = "ec2-spot"
	NameTimeFormat       = "20060102150405"
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
	BidPriceKey = bsonutil.MustHaveTag(EC2SpotSettings{}, "BidPrice")
)

var (
	// bson fields for the MountPoint struct
	VirtualNameKey = bsonutil.MustHaveTag(MountPoint{}, "VirtualName")
	DeviceNameKey  = bsonutil.MustHaveTag(MountPoint{}, "DeviceName")
	SizeKey        = bsonutil.MustHaveTag(MountPoint{}, "Size")
)

//Utility func to create a create a temporary instance name for a host
func generateName(distroId string) string {
	return "evg_" + distroId + "_" + time.Now().Format(NameTimeFormat) +
		fmt.Sprintf("_%v", rand.New(rand.NewSource(time.Now().UnixNano())).Int())
}

//makeBlockDeviceMapping takes the mount_points settings defined in the distro,
//and converts them to ec2.BlockDeviceMapping structs which are usable by goamz.
//It returns a non-nil error if any of the fields appear invalid.
func makeBlockDeviceMappings(mounts []MountPoint) ([]ec2.BlockDeviceMapping, error) {
	mappings := []ec2.BlockDeviceMapping{}
	for _, mount := range mounts {
		if mount.DeviceName == "" {
			return nil, fmt.Errorf("missing 'device_name': %#v", mount)
		}
		if mount.VirtualName == "" {
			if mount.Size <= 0 {
				return nil, fmt.Errorf("invalid 'size': %#v", mount)
			}
			// EBS Storage - device name but no virtual name
			mappings = append(mappings, ec2.BlockDeviceMapping{
				DeviceName:          mount.DeviceName,
				VolumeSize:          int64(mount.Size),
				DeleteOnTermination: true,
			})
		} else {
			//Instance Storage - virtual name but no size
			mappings = append(mappings, ec2.BlockDeviceMapping{
				DeviceName:  mount.DeviceName,
				VirtualName: mount.VirtualName,
			})
		}
	}
	return mappings, nil
}

//helper function for getting an EC2 handle at US east
func getUSEast(creds aws.Auth) *ec2.EC2 {
	return ec2.New(creds, aws.USEast)
}

func getEC2KeyOptions(h *host.Host, keyPath string) ([]string, error) {
	if keyPath == "" {
		return []string{}, fmt.Errorf("No key specified for EC2 host")
	}
	opts := []string{"-i", keyPath}
	for _, opt := range h.Distro.SSHOptions {
		opts = append(opts, "-o", opt)
	}
	return opts, nil
}

//getInstanceInfo returns the full ec2 instance info for the given instance ID.
//Note that this is the *instance* id, not the spot request ID, which is different.
func getInstanceInfo(ec2Handle *ec2.EC2, instanceId string) (*ec2.Instance, error) {
	amis := []string{instanceId}
	resp, err := ec2Handle.Instances(amis, nil)
	if err != nil {
		return nil, err
	}

	reservation := resp.Reservations
	if len(reservation) < 1 {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "No reservation found for "+
			"instance id: %v", instanceId)
	}

	instances := reservation[0].Instances
	if len(instances) < 1 {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "'%v' was not found in "+
			"reservation '%v'", instanceId, resp.Reservations[0].ReservationId)
	}
	return &instances[0], nil
}

//ec2StatusToEvergreenStatus returns a "universal" status code based on EC2's
//provider-specific status codes.
func ec2StatusToEvergreenStatus(ec2Status string) cloud.CloudStatus {
	switch ec2Status {
	case EC2StatusPending:
		return cloud.StatusInitializing
	case EC2StatusRunning:
		return cloud.StatusRunning
	case EC2StatusShuttingdown:
		return cloud.StatusTerminated
	case EC2StatusTerminated:
		return cloud.StatusTerminated
	case EC2StatusStopped:
		return cloud.StatusStopped
	default:
		return cloud.StatusUnknown
	}
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

	tags := map[string]string{
		"Name":       intentHost.Id,
		"distro":     intentHost.Distro.Id,
		"hostname":   hostname,
		"username":   username,
		"owner":      intentHost.StartedBy,
		"mode":       "production",
		"start-time": intentHost.CreationTime.Format(NameTimeFormat),
	}

	if intentHost.UserHost {
		tags["mode"] = "testing"
	}
	return tags
}

//attachTags makes a call to EC2 to attach the given map of tags to a resource.
func attachTags(ec2Handle *ec2.EC2,
	tags map[string]string, instance string) error {

	tagSlice := []ec2.Tag{}
	for tag, value := range tags {
		tagSlice = append(tagSlice, ec2.Tag{tag, value})
	}

	_, err := ec2Handle.CreateTags([]string{instance}, tagSlice)
	return err
}

// determine how long until a payment is due for the specified host. since ec2
// bills per full hour the host has been up this number is just how long until,
// the host has been up the next round number of hours
func timeTilNextEC2Payment(host *host.Host) time.Duration {

	now := time.Now()

	// the time since the host was created
	timeSinceCreation := now.Sub(host.CreationTime)

	// the hours since the host was created, rounded up
	hoursRoundedUp := time.Duration(math.Ceil(timeSinceCreation.Hours()))

	// the next round number of hours the host will have been up - the time
	// that the next payment will be due
	nextPaymentTime := host.CreationTime.Add(hoursRoundedUp)

	return nextPaymentTime.Sub(now)

}
