package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlePoisonedHost(t *testing.T) {
	ctx := context.Background()
	env := &mock.Environment{}

	for testCase, test := range map[string]func(*testing.T){
		"parent with a container running a task": func(t *testing.T) {
			t1 := &task.Task{
				Id:     "t1",
				Status: evergreen.TaskStarted,
			}
			require.NoError(t, t1.Insert())

			parent := &host.Host{
				Id:            "parent",
				HasContainers: true,
				Status:        evergreen.HostRunning,
			}
			container1 := &host.Host{
				Id:       "container1",
				Status:   evergreen.HostRunning,
				ParentID: parent.Id,
			}
			container2 := &host.Host{
				Id:          "container2",
				Status:      evergreen.HostRunning,
				ParentID:    parent.Id,
				RunningTask: t1.Id,
			}

			require.NoError(t, parent.Insert())
			require.NoError(t, container1.Insert())
			require.NoError(t, container2.Insert())

			assert.NoError(t, HandlePoisonedHost(ctx, env, container1, ""))

			parent, err := host.FindOneId(parent.Id)
			assert.NoError(t, err)
			assert.Equal(t, evergreen.HostDecommissioned, parent.Status)
			container1, err = host.FindOneId(container1.Id)
			assert.NoError(t, err)
			assert.Equal(t, evergreen.HostDecommissioned, container1.Status)
			container2, err = host.FindOneId(container2.Id)
			assert.NoError(t, err)
			assert.Equal(t, evergreen.HostDecommissioned, container2.Status)

			t1, err = task.FindOneId(t1.Id)
			assert.NoError(t, err)
			assert.Equal(t, evergreen.TaskFailed, t1.Status)
		},
		"running task": func(t *testing.T) {
			t1 := &task.Task{
				Id:     "t1",
				Status: evergreen.TaskStarted,
			}
			require.NoError(t, t1.Insert())

			hostRunningTask := &host.Host{
				Id:          "runningTask",
				Status:      evergreen.HostRunning,
				RunningTask: t1.Id,
			}
			require.NoError(t, hostRunningTask.Insert())

			assert.NoError(t, HandlePoisonedHost(ctx, env, hostRunningTask, ""))
			hostRunningTask, err := host.FindOneId(hostRunningTask.Id)
			assert.NoError(t, err)
			assert.Equal(t, evergreen.HostDecommissioned, hostRunningTask.Status)

			t1, err = task.FindOneId(t1.Id)
			assert.NoError(t, err)
			assert.Equal(t, evergreen.TaskFailed, t1.Status)
		},
		"static host": func(t *testing.T) {
			static := &host.Host{
				Id:       "static",
				Status:   evergreen.HostRunning,
				Provider: evergreen.ProviderNameStatic,
			}
			require.NoError(t, static.Insert())

			assert.NoError(t, HandlePoisonedHost(ctx, env, static, ""))
			static, err := host.FindOneId(static.Id)
			assert.NoError(t, err)
			assert.Equal(t, evergreen.HostQuarantined, static.Status)
		},
		"already decommissioned": func(t *testing.T) {
			decommissioned := &host.Host{
				Id:     "decommissioned",
				Status: evergreen.HostDecommissioned,
			}
			require.NoError(t, decommissioned.Insert())

			assert.NoError(t, HandlePoisonedHost(ctx, env, decommissioned, ""))
			decommissioned, err := host.FindOneId(decommissioned.Id)
			assert.NoError(t, err)
			assert.Equal(t, evergreen.HostDecommissioned, decommissioned.Status)
		},
		"already terminated": func(t *testing.T) {
			terminated := &host.Host{
				Id:     "terminated",
				Status: evergreen.HostTerminated,
			}
			require.NoError(t, terminated.Insert())

			assert.NoError(t, HandlePoisonedHost(ctx, env, terminated, ""))
			terminated, err := host.FindOneId(terminated.Id)
			assert.NoError(t, err)
			assert.Equal(t, evergreen.HostTerminated, terminated.Status)
		},
	} {
		require.NoError(t, db.ClearCollections(host.Collection, task.Collection))
		require.NoError(t, env.Configure(ctx))

		t.Run(testCase, test)
	}

}
