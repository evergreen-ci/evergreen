package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostMonitoringCheckJob(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)

	env := &mock.Environment{}
	require.NoError(env.Configure(ctx))

	require.NoError(db.ClearCollections(host.Collection))
	defer func() {
		assert.NoError(db.ClearCollections(host.Collection))
	}()
	h := &host.Host{
		Id:                    "h1",
		LastCommunicationTime: time.Now().Add(-15 * time.Minute),
		Status:                evergreen.HostRunning,
		Distro:                distro.Distro{Provider: evergreen.ProviderNameMock},
		Provider:              evergreen.ProviderNameMock,
		StartedBy:             evergreen.User,
	}
	require.NoError(h.Insert())

	mockInstance := cloud.MockInstance{
		IsUp:           true,
		IsSSHReachable: true,
		Status:         cloud.StatusTerminated,
	}
	mockCloud := cloud.GetMockProvider()
	mockCloud.Reset()
	mockCloud.Set(h.Id, mockInstance)

	j := NewHostMonitorExternalStateJob(env, h, "one")

	j.Run(ctx)

	assert.NoError(j.Error())
	assert.True(j.Status().Completed)

	require.True(amboy.WaitInterval(ctx, env.RemoteQueue(), 100*time.Millisecond))

	dbHost, err := host.FindOneId(h.Id)
	require.NoError(err)
	require.NotZero(t, dbHost)
	assert.Equal(evergreen.HostTerminated, dbHost.Status)
}

func TestHandleExternallyTerminatedHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for statusName, status := range map[string]cloud.CloudStatus{
		"Terminated": cloud.StatusTerminated,
		"Stopped":    cloud.StatusStopped,
	} {
		t.Run("InstanceStatus"+statusName, func(t *testing.T) {
			t.Run("TerminatesHostAndClearsTask", func(t *testing.T) {
				tctx, tcancel := context.WithTimeout(ctx, 5*time.Second)
				defer tcancel()

				env := &mock.Environment{}
				require.NoError(t, env.Configure(tctx))

				require.NoError(t, db.ClearCollections(host.Collection, task.Collection))
				defer func() {
					assert.NoError(t, db.ClearCollections(host.Collection, task.Collection))
				}()

				mockCloud := cloud.GetMockProvider()
				mockCloud.Reset()
				defer func() {
					mockCloud.Reset()
				}()

				tsk := &task.Task{
					Id:      "t1",
					BuildId: "b1",
				}
				require.NoError(t, tsk.Insert())

				h := &host.Host{
					Id:          "h1",
					Status:      evergreen.HostRunning,
					Distro:      distro.Distro{Provider: evergreen.ProviderNameMock},
					Provider:    evergreen.ProviderNameMock,
					RunningTask: tsk.Id,
				}
				mockInstance := cloud.MockInstance{
					Status: status,
				}
				cloud.GetMockProvider().Set(h.Id, mockInstance)

				require.NoError(t, h.Insert())

				terminated, err := handleExternallyTerminatedHost(ctx, t.Name(), env, h)
				require.NoError(t, err)
				assert.True(t, terminated)

				require.True(t, amboy.WaitInterval(ctx, env.RemoteQueue(), 100*time.Millisecond))

				dbHost, err := host.FindOneId(h.Id)
				require.NoError(t, err)
				require.NotZero(t, dbHost)

				assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
				assert.Zero(t, dbHost.RunningTask)
			})
		})
	}
}
