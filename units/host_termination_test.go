package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostTerminationJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	checkTerminationEvent := func(t *testing.T, hostID, reason string) {
		hostEventOpts := event.HostEventsOpts{
			ID:      hostID,
			Tag:     "",
			Limit:   50,
			SortAsc: false,
		}
		events, err := event.Find(event.HostEvents(hostEventOpts))
		require.NoError(t, err)
		require.NotEmpty(t, events)
		var foundTerminationEvent bool
		for _, e := range events {
			if e.EventType != event.EventHostStatusChanged {
				continue
			}
			data, ok := e.Data.(*event.HostEventData)
			require.True(t, ok)
			if data.NewStatus != evergreen.HostTerminated {
				continue
			}

			assert.Equal(t, reason, data.Logs, "event log termination reason should match expected reason")

			foundTerminationEvent = true
		}
		assert.True(t, foundTerminationEvent, "expected host termination event to be logged")
	}

	reason := "some termination message"
	buildId := "b1"
	versionId := "v1"
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host){
		"TerminatesRunningHost": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			require.NoError(t, h.Insert(ctx))
			mcp.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOne(ctx, host.ById(h.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)

			checkTerminationEvent(t, h.Id, reason)

			cloudHost := mcp.Get(h.Id)
			require.NotZero(t, cloudHost)
			assert.Equal(t, cloud.StatusTerminated, cloudHost.Status)
		},
		"SkipsCloudHostTermination": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			require.NoError(t, h.Insert(ctx))
			mcp.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:          true,
				SkipCloudHostTermination: true,
				TerminationReason:        reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOne(ctx, host.ById(h.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)

			checkTerminationEvent(t, h.Id, reason)

			cloudHost := mcp.Get(h.Id)
			require.NotZero(t, cloudHost)
			assert.Equal(t, cloud.StatusRunning, cloudHost.Status, "cloud host should be unchanged because cloud host termination should be skipped")
		},
		"TerminatesStaticHosts": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Distro.Provider = evergreen.ProviderNameStatic
			h.Provider = evergreen.ProviderNameStatic
			require.NoError(t, h.Insert(ctx))

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: "foo",
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOne(ctx, host.ById(h.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		},
		"FailsWithNonexistentDBHost": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: "foo",
			})
			terminationJob, ok := j.(*hostTerminationJob)
			require.True(t, ok)
			terminationJob.host = nil

			j.Run(ctx)
			assert.Error(t, j.Error())
		},
		"ReterminatesCloudHostIfAlreadyMarkedTerminated": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert(ctx))
			mcp.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			const reason = "foo"
			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			// Don't check the host events for termination since the host is
			// already in a terminated state.

			mockInstance := mcp.Get(h.Id)
			assert.Equal(t, cloud.StatusTerminated, mockInstance.Status)
		},
		"TerminatesDBHostWithoutCloudHost": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			require.NoError(t, h.Insert(ctx))

			const reason = "foo"
			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			checkTerminationEvent(t, h.Id, reason)

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.NotEqual(t, evergreen.HostRunning, dbHost.Status)
		},
		"MarksUninitializedIntentHostAsTerminated": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Status = evergreen.HostUninitialized
			require.NoError(t, h.Insert(ctx))

			const reason = "foo"
			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			checkTerminationEvent(t, h.Id, reason)

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		},
		"MarksBuildingIntentHostAsTerminated": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Status = evergreen.HostBuilding
			require.NoError(t, h.Insert(ctx))

			const reason = "foo"
			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			checkTerminationEvent(t, h.Id, reason)

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		},
		"MarksBuildingFailedIntentHostAsTerminated": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Status = evergreen.HostBuildingFailed
			require.NoError(t, h.Insert(ctx))

			const reason = "foo"
			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			checkTerminationEvent(t, h.Id, reason)

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		},
		"NoopsWithAlreadyTerminatedIntentHost": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			// The ID must be a valid intent host ID.
			h.Id = h.Distro.GenerateName()
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert(ctx))

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: "foo",
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		},
		"TaskInTaskGroupDoesNotRestartIfFinished": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.LastGroup = "taskgroup"
			h.LastTask = "task2"
			require.NoError(t, h.Insert(ctx))
			mcp.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			task1 := task.Task{
				Id:                "task1",
				Status:            evergreen.TaskSucceeded,
				Activated:         true,
				BuildId:           buildId,
				Version:           versionId,
				Project:           "exists",
				HostId:            h.Id,
				TaskGroup:         "taskgroup",
				TaskGroupMaxHosts: 1,
			}
			task2 := task.Task{
				Id:                "task2",
				Status:            evergreen.TaskSucceeded,
				Activated:         true,
				BuildId:           buildId,
				Version:           versionId,
				Project:           "exists",
				TaskGroup:         "taskgroup",
				TaskGroupMaxHosts: 1,
				DependsOn: []task.Dependency{
					{
						TaskId: task1.Id,
						Status: evergreen.TaskSucceeded,
					},
				},
			}
			require.NoError(t, task1.Insert())
			require.NoError(t, task2.Insert())

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOne(ctx, host.ById(h.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)

			checkTerminationEvent(t, h.Id, reason)

			cloudHost := mcp.Get(h.Id)
			require.NotZero(t, cloudHost)
			assert.Equal(t, cloud.StatusTerminated, cloudHost.Status)

			resetTask, err := task.FindOneId("task2")
			require.NoError(t, err)
			assert.Equal(t, evergreen.TaskSucceeded, resetTask.Status)

			// Verify the single host task group did not reset
			tasks, err := task.Find(task.ByIds([]string{task1.Id, task2.Id}))
			require.NoError(t, err)
			require.Len(t, tasks, 2)
			for _, dbTask := range tasks {
				assert.Equal(t, evergreen.TaskSucceeded, dbTask.Status)
				assert.False(t, dbTask.ResetWhenFinished)
			}
		},
		"TaskInSingleHostTaskGroupBlocksAndRestartsTasks": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.LastGroup = "taskgroup"
			h.LastTask = "task2"
			require.NoError(t, h.Insert(ctx))

			mcp.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			task1 := task.Task{
				Id:                "task1",
				Status:            evergreen.TaskSucceeded,
				Activated:         true,
				BuildId:           buildId,
				Version:           versionId,
				Project:           "exists",
				HostId:            h.Id,
				TaskGroup:         "taskgroup",
				TaskGroupMaxHosts: 1,
			}
			task2 := task.Task{
				Id:                "task2",
				Status:            evergreen.TaskSucceeded,
				Activated:         true,
				BuildId:           buildId,
				Version:           versionId,
				Project:           "exists",
				TaskGroup:         "taskgroup",
				TaskGroupMaxHosts: 1,
				DependsOn: []task.Dependency{
					{
						TaskId: task1.Id,
						Status: evergreen.TaskSucceeded,
					},
				},
			}
			task3 := task.Task{
				Id:                "task3",
				Status:            evergreen.TaskUndispatched,
				Activated:         true,
				BuildId:           buildId,
				Version:           versionId,
				Project:           "exists",
				TaskGroup:         "taskgroup",
				TaskGroupMaxHosts: 1,
				DependsOn: []task.Dependency{
					{
						TaskId: task2.Id,
						Status: evergreen.TaskSucceeded,
					},
				},
			}
			task4 := task.Task{
				Id:                "task4",
				Status:            evergreen.TaskUndispatched,
				Activated:         true,
				BuildId:           buildId,
				Version:           versionId,
				Project:           "exists",
				TaskGroup:         "taskgroup",
				TaskGroupMaxHosts: 1,
				DependsOn: []task.Dependency{
					{
						TaskId: task3.Id,
						Status: evergreen.TaskSucceeded,
					},
				},
			}
			task5 := task.Task{
				Id:                "task5",
				Status:            evergreen.TaskUndispatched,
				Activated:         false,
				BuildId:           buildId,
				Version:           versionId,
				Project:           "exists",
				TaskGroup:         "taskgroup",
				TaskGroupMaxHosts: 1,
				DependsOn: []task.Dependency{
					{
						TaskId: task4.Id,
						Status: evergreen.TaskSucceeded,
					},
				},
			}
			require.NoError(t, task1.Insert())
			require.NoError(t, task2.Insert())
			require.NoError(t, task3.Insert())
			require.NoError(t, task4.Insert())
			require.NoError(t, task5.Insert())

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOne(ctx, host.ById(h.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)

			checkTerminationEvent(t, h.Id, reason)

			cloudHost := mcp.Get(h.Id)
			require.NotZero(t, cloudHost)
			assert.Equal(t, cloud.StatusTerminated, cloudHost.Status)

			// Verify the single host task group reset
			tasks, err := task.Find(task.ByIds([]string{task1.Id, task2.Id, task3.Id, task4.Id, task5.Id}))
			require.NoError(t, err)
			require.Len(t, tasks, 5)
			for _, dbTask := range tasks {
				assert.Equal(t, evergreen.TaskUndispatched, dbTask.Status)
			}
		},
		"TaskInTaskGroupAccountsForInactiveTasks": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			// If we have a partially activated task group, and the last one that is activated finishes
			// we should not restart the task group.
			h.LastGroup = "taskgroup"
			h.LastTask = "task2"
			require.NoError(t, h.Insert(ctx))

			mcp.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			task1 := task.Task{
				Id:                "task1",
				Status:            evergreen.TaskSucceeded,
				Activated:         true,
				BuildId:           buildId,
				Version:           versionId,
				Project:           "exists",
				HostId:            h.Id,
				TaskGroup:         "taskgroup",
				TaskGroupMaxHosts: 1,
			}
			task2 := task.Task{
				Id:                "task2",
				Status:            evergreen.TaskSucceeded,
				Activated:         true,
				BuildId:           buildId,
				Version:           versionId,
				Project:           "exists",
				TaskGroup:         "taskgroup",
				TaskGroupMaxHosts: 1,
				DependsOn: []task.Dependency{
					{
						TaskId: task1.Id,
						Status: evergreen.TaskSucceeded,
					},
				},
			}
			task3 := task.Task{
				Id:                "task3",
				Status:            evergreen.TaskUndispatched,
				Activated:         false,
				BuildId:           buildId,
				Version:           versionId,
				Project:           "exists",
				TaskGroup:         "taskgroup",
				TaskGroupMaxHosts: 1,
				DependsOn: []task.Dependency{
					{
						TaskId: task2.Id,
						Status: evergreen.TaskSucceeded,
					},
				},
			}
			nonTgTask := task.Task{
				Id:        "task4",
				Status:    evergreen.TaskSucceeded,
				Activated: true,
				BuildId:   buildId,
				Version:   versionId,
				Project:   "exists",
				DependsOn: []task.Dependency{
					{
						TaskId: task2.Id,
						Status: evergreen.TaskSucceeded,
					},
				},
			}
			require.NoError(t, task1.Insert())
			require.NoError(t, task2.Insert())
			require.NoError(t, task3.Insert())
			require.NoError(t, nonTgTask.Insert())

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOne(ctx, host.ById(h.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)

			checkTerminationEvent(t, h.Id, reason)

			cloudHost := mcp.Get(h.Id)
			require.NotZero(t, cloudHost)
			assert.Equal(t, cloud.StatusTerminated, cloudHost.Status)

			// Verify the task group has not been reset
			resetTask, err := task.FindOneId("task2")
			require.NoError(t, err)
			require.NotNil(t, resetTask)
			assert.Equal(t, evergreen.TaskSucceeded, resetTask.Status)

			dbTask, err := task.FindOneId(nonTgTask.Id)
			require.NoError(t, err)
			require.NotNil(t, dbTask)
			assert.False(t, dbTask.UnattainableDependency)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(host.Collection, event.EventCollection, task.Collection, model.ProjectRefCollection, build.Collection, model.VersionCollection))
			tctx := testutil.TestSpan(ctx, t)

			env := testutil.NewEnvironment(tctx, t)

			h := &host.Host{
				Id:          "i-12345",
				Status:      evergreen.HostRunning,
				Distro:      distro.Distro{Id: "d1", Provider: evergreen.ProviderNameMock},
				Provider:    evergreen.ProviderNameMock,
				Provisioned: true,
			}
			build := build.Build{
				Id:      "b1",
				Version: "v1",
			}
			require.NoError(t, build.Insert())
			version := model.Version{
				Id: "v1",
			}
			require.NoError(t, version.Insert())
			pref := &model.ProjectRef{
				Id:      "exists",
				Enabled: true,
			}
			require.NoError(t, pref.Insert())
			provider := cloud.GetMockProvider()
			provider.Reset()

			tCase(tctx, t, env, provider, h)
		})
	}
}
