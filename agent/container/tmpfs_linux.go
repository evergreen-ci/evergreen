//go:build linux

package container

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// provisionEnvTmpfs creates dir and mounts a tmpfs on it. The mount is owned
// by the current process's UID/GID with mode 0700 so only the agent can
// read and write it. If a stale mount from a prior incomplete Destroy is
// detected, it is cleared before mounting to prevent tmpfs stacking.
func provisionEnvTmpfs(dir string) error {
	if isMountpoint(dir) {
		if err := exec.Command("sudo", "umount", dir).Run(); err != nil {
			return errors.Wrapf(err, "clearing stale tmpfs mount at '%s'", dir)
		}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.Wrapf(err, "creating env tmpfs dir '%s'", dir)
	}
	uid := os.Getuid()
	gid := os.Getgid()
	opts := fmt.Sprintf("uid=%d,gid=%d,mode=0700", uid, gid)
	if err := exec.Command("sudo", "mount", "-t", "tmpfs", "-o", opts, "tmpfs", dir).Run(); err != nil {
		_ = os.Remove(dir)
		return errors.Wrapf(err, "mounting tmpfs at '%s'", dir)
	}
	return nil
}

// removeEnvTmpfs unmounts and removes the env tmpfs directory. If the
// unmount fails, the directory contents are still removed but the tmpfs
// mount itself may persist; a warning is logged in that case.
func removeEnvTmpfs(dir string) error {
	if dir == "" {
		return nil
	}
	if err := exec.Command("sudo", "umount", dir).Run(); err != nil {
		grip.Warningf(context.Background(), "unmounting env tmpfs at '%s' failed; directory contents will be removed but the mount may persist: %s", dir, err)
	}
	return errors.Wrapf(os.RemoveAll(dir), "removing env tmpfs dir '%s'", dir)
}

// isMountpoint returns true if dir is currently a mount point according to
// /proc/self/mountinfo.
func isMountpoint(dir string) bool {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		// Mount point is the fifth field (index 4) in mountinfo format.
		if len(fields) >= 5 && fields[4] == dir {
			return true
		}
	}
	return false
}
