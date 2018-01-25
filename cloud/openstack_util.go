package cloud

import (
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

const (
	// OSStatusActive means the instance is currently running.
	OSStatusActive = "ACTIVE"
	// OSStatusInProgress means the instance is currently running and processing a request.
	OSStatusInProgress = "IN_PROGRESS"
	// OSStatusShutOff means the instance has been temporarily stopped.
	OSStatusShutOff = "SHUTOFF"
	// OSStatusBuilding means the instance is starting up.
	OSStatusBuilding = "BUILD"
)

func osStatusToEvgStatus(status string) CloudStatus {
	// Note: There is no equivalent to the 'terminated' cloud status since instances are no
	// longer detectable once they have been terminated.
	switch status {
	case OSStatusActive:
		return StatusRunning
	case OSStatusInProgress:
		return StatusRunning
	case OSStatusShutOff:
		return StatusStopped
	case OSStatusBuilding:
		return StatusInitializing
	default:
		return StatusUnknown
	}
}

func getSpawnOptions(h *host.Host, s *openStackSettings) servers.CreateOpts {
	return servers.CreateOpts{
		Name:           h.Id,
		ImageName:      s.ImageName,
		FlavorName:     s.FlavorName,
		SecurityGroups: []string{s.SecurityGroup},
		Metadata:       makeTags(h),
	}
}
