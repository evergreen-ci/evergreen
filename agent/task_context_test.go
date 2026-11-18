package agent

import (
	"slices"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/mock"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

func TestMakeTaskConfigLoadsGitHubParentPRCheckoutForPatchRequesters(t *testing.T) {
	checkout := &patch.GitHubParentPRCheckout{
		PRNumber:  1234,
		HeadHash:  "abcdef0123456789abcdef0123456789abcdef01",
		ForModule: "evergreen",
	}
	for requester, expectCheckout := range map[string]bool{
		evergreen.PatchVersionRequester:       true,
		evergreen.GithubPRRequester:           true,
		evergreen.RepotrackerVersionRequester: false,
	} {
		t.Run(requester, func(t *testing.T) {
			comm := client.NewMock("url")
			comm.GetTaskResponse = &task.Task{
				Id:             "logging",
				BuildVariant:   "mock_build_variant",
				Requester:      requester,
				TaskOutputInfo: &task.TaskOutput{},
			}
			comm.GetTaskPatchResponse = &patch.Patch{GitHubParentPRCheckout: checkout}
			agt := &Agent{
				opts: Options{
					WorkingDirectory: t.TempDir(),
					Mode:             globals.HostMode,
				},
				comm:   comm,
				tracer: otel.GetTracerProvider().Tracer("noop_tracer"),
			}
			tc := &taskContext{
				task:       client.TaskData{ID: "logging", Secret: "task_secret"},
				oomTracker: &mock.OOMTracker{},
			}

			conf, err := agt.makeTaskConfig(t.Context(), tc)
			require.NoError(t, err)
			if expectCheckout {
				assert.Equal(t, checkout, conf.GitHubParentPRCheckout)
			} else {
				assert.Nil(t, conf.GitHubParentPRCheckout)
			}
		})
	}
}

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

	t.Run("MountPoints", func(t *testing.T) {
		tc := taskContext{}

		mountpoints := tc.getMountpoints()
		require.NotEmpty(t, mountpoints)

		partitions, err := disk.PartitionsWithContext(t.Context(), false)
		require.NoError(t, err)
		require.NotEmpty(t, partitions)

		// Count expected matches upfront
		expectedDeviceCount := 0
		for _, partition := range partitions {
			if slices.Contains(mountpoints, partition.Mountpoint) {
				_, valid := getDeviceName(partition.Device)
				if valid {
					expectedDeviceCount++
				}
			}
		}

		err = tc.getDeviceNames(t.Context())
		require.NoError(t, err)
		assert.Len(t, tc.diskDevices, expectedDeviceCount)
	})
}
