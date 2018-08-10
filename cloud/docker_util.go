// +build go1.7

package cloud

import (
	"github.com/docker/docker/api/types"
)

// toEvgStatus converts a container state to an Evergreen cloud provider status.
func toEvgStatus(s *types.ContainerState) CloudStatus {
	if s.Running {
		return StatusRunning
	} else if s.Paused {
		return StatusStopped
	} else if s.Restarting {
		return StatusInitializing
	} else if s.OOMKilled || s.Dead {
		return StatusTerminated
	}
	return StatusUnknown
}
