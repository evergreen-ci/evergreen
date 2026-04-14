//go:build linux

package agent

import (
	"context"
	"os"
	"strings"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// activeMountsUnder returns bind mount points at or under dir.
func activeMountsUnder(ctx context.Context, dir string) []string {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		grip.Warning(ctx, message.WrapError(err, message.Fields{
			"message": "failed to read mountinfo to check for active mounts under task directory",
		}))
		return nil
	}

	var mounts []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		// The mount point is the fifth field.
		if len(fields) < 5 {
			continue
		}
		mountPoint := fields[4]
		if mountPoint == dir || strings.HasPrefix(mountPoint, dir+"/") {
			mounts = append(mounts, mountPoint)
		}
	}
	return mounts
}
