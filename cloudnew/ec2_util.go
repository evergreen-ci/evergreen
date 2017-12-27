package cloudnew

import (
	"math"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
)

const (
	HostExpireDays      = 10
	NameTimeFormat      = "20060102150405"
	SpawnHostExpireDays = 30
)

func makeBlockDeviceMappings(mounts []cloud.MountPoint) ([]*ec2.BlockDeviceMapping, error) {
	mappings := []*ec2.BlockDeviceMapping{}
	for i, mount := range mounts {
		if mount.DeviceName == "" {
			return nil, errors.Errorf("missing 'device_name': %+v", mount)
		}
		if mount.VirtualName == "" {
			return nil, errors.Errorf("missing 'virtual_name': %+v", mount)
		}
		mappings = append(mappings, &ec2.BlockDeviceMapping{
			DeviceName:  &mounts[i].DeviceName,
			VirtualName: &mounts[i].VirtualName,
		})
	}
	return mappings, nil
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
	expireOn := expireInDays(HostExpireDays)
	if intentHost.UserHost {
		// If this is a spawn host, use a different expiration date.
		expireOn = expireInDays(SpawnHostExpireDays)
	}

	tags := map[string]string{
		"name":              intentHost.Id,
		"distro":            intentHost.Distro.Id,
		"evergreen-service": hostname,
		"username":          username,
		"owner":             intentHost.StartedBy,
		"mode":              "production",
		"start-time":        intentHost.CreationTime.Format(NameTimeFormat),
		"expire-on":         expireOn,
	}

	if intentHost.UserHost {
		tags["mode"] = "testing"
	}
	return tags
}

// expireInDays creates an expire-on string in the format YYYY-MM-DD for numDays days
// in the future.
func expireInDays(numDays int) string {
	return time.Now().AddDate(0, 0, numDays).Format("2006-01-02")
}

//ec2StatusToEvergreenStatus returns a "universal" status code based on EC2's
//provider-specific status codes.
func ec2StatusToEvergreenStatus(ec2Status string) cloud.CloudStatus {
	switch ec2Status {
	case cloud.EC2StatusPending:
		return cloud.StatusInitializing
	case cloud.EC2StatusRunning:
		return cloud.StatusRunning
	case cloud.EC2StatusShuttingdown:
		return cloud.StatusTerminated
	case cloud.EC2StatusTerminated:
		return cloud.StatusTerminated
	case cloud.EC2StatusStopped:
		return cloud.StatusStopped
	default:
		return cloud.StatusUnknown
	}
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
