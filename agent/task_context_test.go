package agent

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/mock"
	"github.com/stretchr/testify/assert"
)

func TestGetOomTrackerInfo(t *testing.T) {
	tc := taskContext{oomTracker: jasper.NewOOMTracker()}
	info := tc.getOomTrackerInfo()
	assert.Nil(t, info)

	tc.oomTracker = &mock.OOMTracker{Lines: []string{"line1", "line2", "line3"}, PIDs: []int{1, 2, 3}}
	info = tc.getOomTrackerInfo()
	assert.NotNil(t, info)
	assert.True(t, info.Detected)
	assert.Equal(t, []int{1, 2, 3}, info.Pids)
}

func TestGetDeviceNames(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("EmptyTaskConfig", func(t *testing.T) {
		tc := taskContext{}
		assert.NoError(t, tc.getDeviceNames(ctx))
		assert.Len(t, tc.diskDevices, 0)
	})

	t.Run("EmptyDistro", func(t *testing.T) {
		tc := taskContext{taskConfig: &internal.TaskConfig{}}
		assert.NoError(t, tc.getDeviceNames(ctx))
		assert.Len(t, tc.diskDevices, 0)
	})

	t.Run("EmptyMountpoints", func(t *testing.T) {
		tc := taskContext{taskConfig: &internal.TaskConfig{Distro: &apimodels.DistroView{}}}
		assert.NoError(t, tc.getDeviceNames(ctx))
		assert.Len(t, tc.diskDevices, 0)
	})

	t.Run("RootMountpoint", func(t *testing.T) {
		tc := taskContext{taskConfig: &internal.TaskConfig{Distro: &apimodels.DistroView{Mountpoints: []string{"/"}}}}
		assert.NoError(t, tc.getDeviceNames(ctx))
		assert.Len(t, tc.diskDevices, 1)
	})
}
