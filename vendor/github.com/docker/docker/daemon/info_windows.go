package daemon // import "github.com/docker/docker/daemon"

import (
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/sysinfo"
)

// fillPlatformInfo fills the platform related info.
func (daemon *Daemon) fillPlatformInfo(v *types.Info, sysInfo *sysinfo.SysInfo) {
}

func (daemon *Daemon) fillPlatformVersion(v *types.Version) {}

func fillDriverWarnings(v *types.Info) {
}

// Rootless returns true if daemon is running in rootless mode
func (daemon *Daemon) Rootless() bool {
	return false
}
