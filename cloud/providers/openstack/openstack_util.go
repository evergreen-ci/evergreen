package openstack

import (
	"os"
	"os/user"

	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

const (
	// NameTimeFormat is the format in which to log times like instance start time.
	NameTimeFormat = "20060102150405"
	// OSStatusActive means the instance is currently running.
	OSStatusActive = "ACTIVE"
	// OSStatusInProgress means the instance is currently running and processing a request.
	OSStatusInProgress = "IN_PROGRESS"
	// OSStatusShutOff means the instance has been temporarily stopped.
	OSStatusShutOff = "SHUTOFF"
	// OSStatusBuilding means the instance is starting up.
	OSStatusBuilding = "BUILD"
)

func osStatusToEvgStatus(status string) cloud.CloudStatus {
	// Note: There is no equivalent to the 'terminated' cloud status since instances are no
	// longer detectable once they have been terminated.
	switch status {
	case OSStatusActive:
		return cloud.StatusRunning
	case OSStatusInProgress:
		return cloud.StatusRunning
	case OSStatusShutOff:
		return cloud.StatusStopped
	case OSStatusBuilding:
		return cloud.StatusInitializing
	default:
		return cloud.StatusUnknown
	}
}

func getSpawnOptions(h *host.Host, s *ProviderSettings) servers.CreateOpts {
	return servers.CreateOpts{
		Name:           h.Id,
		ImageName:      s.ImageName,
		FlavorName:     s.FlavorName,
		SecurityGroups: []string{s.SecurityGroup},
		Metadata:       makeTags(h),
	}
}

func makeTags(intent *host.Host) map[string]string {
	// Get requester host name
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Get requester user name
	var username string
	user, err := user.Current()
	if err != nil {
		username = "unknown"
	} else {
		username = user.Name
	}

	tags := map[string]string{
		"distro":            intent.Distro.Id,
		"evergreen-service": hostname,
		"username":          username,
		"owner":             intent.StartedBy,
		"mode":              "production",
		"start-time":        intent.CreationTime.Format(NameTimeFormat),
	}

	if intent.UserHost {
		tags["mode"] = "testing"
	}
	return tags
}
