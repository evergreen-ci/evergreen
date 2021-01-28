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
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/amboy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostMonitoringCheckJob(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	testConfig := testutil.TestConfig()

	env := &mock.Environment{
		EvergreenSettings: testConfig,
	}

	mockCloud := cloud.GetMockProvider()
	mockCloud.Reset()

	// reset the db
	require.NoError(db.ClearCollections(host.Collection))

	m1 := cloud.MockInstance{
		IsUp:           true,
		IsSSHReachable: true,
		Status:         cloud.StatusTerminated,
	}
	mockCloud.Set("h1", m1)

	// this host should be picked up and updated to running
	h := &host.Host{
		Id:                    "h1",
		LastCommunicationTime: time.Now().Add(-15 * time.Minute),
		Status:                evergreen.HostRunning,
		Distro:                distro.Distro{Provider: evergreen.ProviderNameMock},
		StartedBy:             evergreen.User,
	}
	require.NoError(h.Insert())

	j := NewHostMonitorExternalStateJob(env, h, "one")
	assert.False(j.Status().Completed)

	j.Run(context.Background())

	assert.NoError(j.Error())
	assert.True(j.Status().Completed)

	host1, err := host.FindOne(host.ById("h1"))
	assert.NoError(err)
	assert.Equal(host1.Status, evergreen.HostTerminated)
}

func TestHandleExternallyTerminatedHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, h *host.Host, env *mock.Environment){
		"TerminatesHostWhoseInstanceWasTerminated": func(ctx context.Context, t *testing.T, h *host.Host, env *mock.Environment) {
			mockInstance := cloud.MockInstance{
				Status: cloud.StatusTerminated,
			}
			cloud.GetMockProvider().Set("h1", mockInstance)

			require.NoError(t, h.Insert())

			terminated, err := handleExternallyTerminatedHost(ctx, t.Name(), env, h)
			require.NoError(t, err)
			assert.True(t, terminated)

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)

			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
			assert.Zero(t, dbHost.RunningTask)
		},
		"TerminatesHostWhoseInstanceWasStopped": func(ctx context.Context, t *testing.T, h *host.Host, env *mock.Environment) {
			mockInstance := cloud.MockInstance{
				Status: cloud.StatusStopped,
			}
			cloud.GetMockProvider().Set("h1", mockInstance)

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
		},
	} {
		t.Run(testName, func(t *testing.T) {
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
			testCase(tctx, t, h, env)
		})
	}
}
