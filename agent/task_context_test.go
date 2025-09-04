package agent

import (
	"context"
	"slices"
	"testing"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/mock"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOomTrackerReport(t *testing.T) {
	tc := taskContext{oomTracker: jasper.NewOOMTracker()}
	lines, pids := tc.oomTracker.Report()
	assert.Empty(t, lines)
	assert.Empty(t, pids)

	tc.oomTracker = &mock.OOMTracker{Lines: []string{"line1", "line2", "line3"}, PIDs: []int{1, 2, 3}}
	lines, pids = tc.oomTracker.Report()
	assert.NotEmpty(t, lines)
	assert.NotEmpty(t, pids)
	assert.Equal(t, []int{1, 2, 3}, pids)
}

func TestGetDeviceNames(t *testing.T) {
	ctx := context.Background()

	t.Run("MountPoints", func(t *testing.T) {
		tc := taskContext{}

		mountpoints := tc.getMountpoints()
		require.NotEmpty(t, mountpoints)

		partitions, err := disk.PartitionsWithContext(ctx, false)
		require.NoError(t, err)
		require.NotEmpty(t, partitions)

		// Count expected matches upfront
		expectedDeviceCount := 0
		for _, partition := range partitions {
			if slices.Contains(mountpoints, partition.Mountpoint) {
				if getDeviceName(partition.Device) != "" {
					expectedDeviceCount++
				}
			}
		}

		err = tc.getDeviceNames(ctx)
		assert.NoError(t, err)
		assert.Len(t, tc.diskDevices, expectedDeviceCount)
	})
}
