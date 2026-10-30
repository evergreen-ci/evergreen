package units

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskStrandedCleanupJob(t *testing.T) {
	t.Run("VirtualTasks", func(t *testing.T) {
		for tName, tCase := range map[string]struct {
			runnerStatus   string
			expectStranded bool
		}{
			"SystemFailsVirtualTaskWithFinishedRunner": {
				runnerStatus:   evergreen.TaskFailed,
				expectStranded: true,
			},
			"LeavesVirtualTaskWithRunningRunner": {
				runnerStatus:   evergreen.TaskStarted,
				expectStranded: false,
			},
		} {
			t.Run(tName, func(t *testing.T) {
				env := &mock.Environment{}
				require.NoError(t, env.Configure(t.Context()))
				colls := []string{task.Collection, task.OldCollection, build.Collection, model.VersionCollection, host.Collection}
				require.NoError(t, db.ClearCollections(colls...))
				t.Cleanup(func() {
					assert.NoError(t, db.ClearCollections(colls...))
				})

				v := model.Version{
					Id:     "version",
					Status: evergreen.VersionStarted,
				}
				require.NoError(t, v.Insert(t.Context()))
				b := build.Build{
					Id:      "build",
					Version: v.Id,
					Status:  evergreen.BuildStarted,
				}
				require.NoError(t, b.Insert(t.Context()))

				runner := task.Task{
					Id:           "runner_task",
					BuildId:      b.Id,
					BuildVariant: "build_variant",
					Version:      v.Id,
					Status:       tCase.runnerStatus,
				}
				require.NoError(t, runner.Insert(t.Context()))

				vt := task.Task{
					Id:           "virtual_task",
					IsVirtual:    true,
					CompletedBy:  runner.Id,
					BuildId:      b.Id,
					BuildVariant: "build_variant",
					Version:      v.Id,
					Status:       evergreen.TaskUndispatched,
				}
				require.NoError(t, vt.Insert(t.Context()))

				j := NewStrandedTaskCleanupJob(env, "id")
				j.Run(t.Context())
				require.NoError(t, j.Error())

				dbVT, err := task.FindOneId(t.Context(), vt.Id)
				require.NoError(t, err)
				require.NotZero(t, dbVT)
				if tCase.expectStranded {
					assert.Equal(t, evergreen.TaskFailed, dbVT.Status)
					assert.Equal(t, evergreen.TaskDescriptionStranded, dbVT.Details.Description)
					assert.Equal(t, "runner task finished without setting a status for this virtual task", dbVT.Details.SystemFailureReason)
					assert.True(t, dbVT.IsFinished())
				} else {
					assert.Equal(t, evergreen.TaskUndispatched, dbVT.Status)
					assert.False(t, dbVT.IsFinished())
				}
			})
		}
	})
}
