package units

import (
	"context"
	"strings"
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
	for _, status := range []cloud.CloudStatus{
		cloud.StatusTerminated,
		cloud.StatusNonExistent,
		cloud.StatusStopped,
	} {
		t.Run("InstanceStatus"+strings.Title(status.String()), func(t *testing.T) {
			t.Run("TerminatesHostAndClearsTask", func(t *testing.T) {
				strings.Title(status.String())
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
					StartedBy:   evergreen.User,
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

				require.True(t, amboy.WaitInterval(ctx, env.RemoteQueue(), 100*time.Millisecond), "failed while waiting for host termination job to complete")

				dbHost, err := host.FindOneId(h.Id)
				require.NoError(t, err)
				require.NotZero(t, dbHost)

				assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
				assert.Zero(t, dbHost.RunningTask)
			})
		})
	}
	testCloudStatusTerminatesHostAndClearsTask := func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, status cloud.CloudStatus) {
		tsk := &task.Task{
			Id:      "t1",
			BuildId: "b1",
			HostId:  h.Id,
		}
		require.NoError(t, tsk.Insert())
		h.RunningTask = tsk.Id
		require.NoError(t, h.Insert())

		mockInstance := cloud.MockInstance{
			Status: status,
		}
		cloud.GetMockProvider().Set(h.Id, mockInstance)

		terminated, err := handleExternallyTerminatedHost(ctx, t.Name(), env, h)
		require.NoError(t, err)
		assert.True(t, terminated)

		require.True(t, amboy.WaitInterval(ctx, env.RemoteQueue(), 100*time.Millisecond), "failed while waiting for host termination job to complete")

		dbHost, err := host.FindOneId(h.Id)
		require.NoError(t, err)
		require.NotZero(t, dbHost)

		assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		assert.Zero(t, dbHost.RunningTask)
	}
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host){
		"TerminatedInstanceStatusTerminatesHostAndClearTask": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			testCloudStatusTerminatesHostAndClearsTask(ctx, t, env, h, cloud.StatusTerminated)
		},
		"NonexistentInstanceStatusTerminatesHostAndClearTask": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			testCloudStatusTerminatesHostAndClearsTask(ctx, t, env, h, cloud.StatusNonExistent)
		},
		"StoppedInstanceStatusTerminatesHostAndClearsTask": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			testCloudStatusTerminatesHostAndClearsTask(ctx, t, env, h, cloud.StatusStopped)
		},
		"NonexistentInstanceStatusTerminatesSpawnHost": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			h.UserHost = true
			h.StartedBy = "user"
			require.NoError(t, h.Insert())

			terminated, err := handleExternallyTerminatedHost(ctx, t.Name(), env, h)
			require.NoError(t, err)
			assert.True(t, terminated)

			require.True(t, amboy.WaitInterval(ctx, env.RemoteQueue(), 100*time.Millisecond), "failed while waiting for host termination job to complete")

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		},
		"StoppedInstanceStatusErrorsWithSpawnHost": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			h.UserHost = true
			h.StartedBy = "user"
			require.NoError(t, h.Insert())

			mockInstance := cloud.MockInstance{
				Status: cloud.StatusStopped,
			}
			cloud.GetMockProvider().Set(h.Id, mockInstance)

			terminated, err := handleExternallyTerminatedHost(ctx, t.Name(), env, h)
			assert.Error(t, err)
			assert.False(t, terminated)

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
		},
		"RunningInstanceNoops": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			require.NoError(t, h.Insert())

			mockInstance := cloud.MockInstance{
				Status: cloud.StatusRunning,
			}
			cloud.GetMockProvider().Set(h.Id, mockInstance)

			terminated, err := handleExternallyTerminatedHost(ctx, t.Name(), env, h)
			assert.NoError(t, err)
			assert.False(t, terminated)

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
		},
		"UnexpectedInstanceStatusErrors": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			require.NoError(t, h.Insert())

			mockInstance := cloud.MockInstance{
				Status: cloud.StatusUnknown,
			}
			cloud.GetMockProvider().Set(h.Id, mockInstance)

			terminated, err := handleExternallyTerminatedHost(ctx, t.Name(), env, h)
			assert.Error(t, err)
			assert.False(t, terminated)

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
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

			h := &host.Host{
				Id:        "h1",
				Status:    evergreen.HostRunning,
				Distro:    distro.Distro{Provider: evergreen.ProviderNameMock},
				StartedBy: evergreen.User,
				Provider:  evergreen.ProviderNameMock,
			}

			testCase(tctx, t, env, h)
		})
	}
}

func TestHandleTerminatedHostSpawnedByTask(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(host.Collection, task.Collection))
	}()

	for name, testCase := range map[string]struct {
		t                *task.Task
		h                *host.Host
		newIntentCreated bool
		hostDetailsSet   bool
	}{
		"TaskStillRunning": {
			t: &task.Task{
				Id:        "t0",
				Execution: 0,
				Status:    evergreen.TaskStarted,
			},
			h: &host.Host{
				Id: "h0",
				SpawnOptions: host.SpawnOptions{
					SpawnedByTask:       true,
					TaskID:              "t0",
					TaskExecutionNumber: 0,
					Respawns:            1,
				},
				Status: evergreen.HostStarting,
			},
			newIntentCreated: true,
			hostDetailsSet:   false,
		},
		"TaskAborted": {
			t: &task.Task{
				Id:        "t0",
				Execution: 0,
				Status:    evergreen.TaskStarted,
				Aborted:   true,
			},
			h: &host.Host{
				Id: "h0",
				SpawnOptions: host.SpawnOptions{
					SpawnedByTask:       true,
					TaskID:              "t0",
					TaskExecutionNumber: 0,
					Respawns:            1,
				},
				Status: evergreen.HostStarting,
			},
			newIntentCreated: false,
			hostDetailsSet:   true,
		},
		"HostAlreadyRunning": {
			t: &task.Task{
				Id:        "t0",
				Execution: 0,
				Status:    evergreen.TaskStarted,
				Aborted:   true,
			},
			h: &host.Host{
				Id: "h0",
				SpawnOptions: host.SpawnOptions{
					SpawnedByTask:       true,
					TaskID:              "t0",
					TaskExecutionNumber: 0,
					Respawns:            1,
				},
				Status: evergreen.HostRunning,
			},
			newIntentCreated: false,
			hostDetailsSet:   true,
		},
		"HostNotSpawnedByTask": {
			t: &task.Task{
				Id: "t0",
			},
			h: &host.Host{
				Id: "h0",
				SpawnOptions: host.SpawnOptions{
					SpawnedByTask: false,
				},
			},
			newIntentCreated: false,
			hostDetailsSet:   false,
		},
		"HostOutOfRespawns": {
			t: &task.Task{
				Id:        "t0",
				Execution: 0,
				Status:    evergreen.TaskStarted,
			},
			h: &host.Host{
				Id: "h0",
				SpawnOptions: host.SpawnOptions{
					SpawnedByTask:       true,
					TaskID:              "t0",
					TaskExecutionNumber: 0,
				},
				Status: evergreen.HostStarting,
			},
			newIntentCreated: false,
			hostDetailsSet:   true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(host.Collection, task.Collection))
			require.NoError(t, testCase.t.Insert())

			assert.NoError(t, handleTerminatedHostSpawnedByTask(testCase.h))

			intent, err := host.FindOne(db.Query(nil))
			require.NoError(t, err)
			if testCase.newIntentCreated {
				require.NotNil(t, intent)
				assert.True(t, host.IsIntentHostId(intent.Id))
				assert.Equal(t, evergreen.HostUninitialized, intent.Status)
			} else {
				assert.Nil(t, intent)
			}

			t0, err := task.FindOneId(testCase.t.Id)
			require.NoError(t, err)
			require.NotNil(t, t0)
			if testCase.hostDetailsSet {
				require.Len(t, t0.HostCreateDetails, 1)
				assert.Equal(t, testCase.h.Id, t0.HostCreateDetails[0].HostId)
			} else {
				assert.Nil(t, t0.HostCreateDetails)
			}
		})
	}
}
