package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"gopkg.in/yaml.v3"
)

func TestTaskExecutionTimeoutJob(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	mp := cloud.GetMockProvider()
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, model.VersionCollection, model.ParserProjectCollection, model.ProjectRefCollection, host.Collection, event.EventCollection))
		mp.Reset()
	}()

	checkTaskRestarted := func(t *testing.T, taskID string, oldExecution int, description string) {
		archivedTask, err := task.FindOneIdAndExecution(ctx, taskID, oldExecution)
		require.NoError(t, err)
		require.NotZero(t, archivedTask)
		assert.True(t, archivedTask.Archived)
		assert.Equal(t, evergreen.TaskFailed, archivedTask.Status)
		assert.Equal(t, description, archivedTask.Details.Description)
		assert.WithinDuration(t, archivedTask.LastHeartbeat, archivedTask.FinishTime, 0, "task finish time should be when it stopped heartbeating")

		restartedTask, err := task.FindOneIdAndExecution(ctx, taskID, oldExecution+1)
		require.NoError(t, err)
		require.NotZero(t, restartedTask)
		assert.Equal(t, evergreen.TaskUndispatched, restartedTask.Status)
	}

	// restartAndDispatchTask inserts a new execution of the given task that's
	// dispatched to the given host.
	restartAndDispatchTask := func(ctx context.Context, t *testing.T, tsk task.Task, hostID string) task.Task {
		tsk.Execution++
		tsk.Status = evergreen.TaskDispatched
		tsk.HostId = hostID
		tsk.StartTime = time.Time{}
		require.NoError(t, tsk.Insert(ctx))
		return tsk
	}
	checkTaskStillDispatched := func(t *testing.T, expected task.Task) {
		dbTask, err := task.FindOneId(ctx, expected.Id)
		require.NoError(t, err)
		require.NotZero(t, dbTask)
		assert.Equal(t, expected.Execution, dbTask.Execution, "task should not be restarted again")
		assert.Equal(t, evergreen.TaskDispatched, dbTask.Status, "new execution should not be system failed")
		assert.Equal(t, expected.HostId, dbTask.HostId)
	}
	checkHostRunningTask := func(t *testing.T, hostID string, expected task.Task) {
		dbHost, err := host.FindOneId(ctx, hostID)
		require.NoError(t, err)
		require.NotZero(t, dbHost)
		assert.Equal(t, expected.Id, dbHost.RunningTask, "host should still be running the new execution")
		assert.Equal(t, expected.Execution, dbHost.RunningTaskExecution)
	}

	const hostID = "host_id"
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *taskExecutionTimeoutJob, v model.Version){
		"RestartsStaleHostTask": func(ctx context.Context, t *testing.T, j *taskExecutionTimeoutJob, v model.Version) {
			require.NoError(t, j.task.Insert(t.Context()))
			require.NoError(t, v.Insert(t.Context()))

			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, hostID)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status, "host should still be running")

			checkTaskRestarted(t, j.task.Id, 0, evergreen.TaskDescriptionHeartbeat)
		},
		"RestartsAllTaskGroupTasksForStaleHostTaskInSingleHostTaskGroup": func(ctx context.Context, t *testing.T, j *taskExecutionTimeoutJob, v model.Version) {
			const taskGroupName = "some_task_group"
			j.task.TaskGroup = taskGroupName
			j.task.TaskGroupMaxHosts = 1
			require.NoError(t, v.Insert(t.Context()))
			require.NoError(t, j.task.Insert(t.Context()))
			otherTask := task.Task{
				Id:                "another_task",
				BuildId:           j.task.BuildId,
				BuildVariant:      j.task.BuildVariant,
				Version:           j.task.Version,
				TaskGroup:         taskGroupName,
				TaskGroupMaxHosts: 1,
				DisplayName:       "display_name_for_another_task",
				Status:            evergreen.TaskUndispatched,
				DependsOn: []task.Dependency{
					{
						TaskId: j.task.Id,
						Status: evergreen.TaskSucceeded,
					},
				},
			}
			require.NoError(t, otherTask.Insert(t.Context()))

			p := model.Project{
				TaskGroups: []model.TaskGroup{
					{
						Name:     taskGroupName,
						MaxHosts: 1,
						Tasks:    []string{j.task.DisplayName, otherTask.DisplayName},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: j.task.BuildVariant,
						Tasks: []model.BuildVariantTaskUnit{
							{Name: taskGroupName, Variant: j.task.BuildVariant},
						},
					},
				},
				Tasks: []model.ProjectTask{
					{Name: j.task.DisplayName},
					{Name: otherTask.DisplayName},
				},
			}
			yml, err := yaml.Marshal(p)
			require.NoError(t, err)
			pp := &model.ParserProject{}
			err = util.UnmarshalYAMLWithFallback(yml, &pp)
			require.NoError(t, err)
			pp.Id = v.Id
			require.NoError(t, pp.Insert(t.Context()))

			j.Run(ctx)
			require.NoError(t, j.Error())

			checkTaskRestarted(t, j.task.Id, 0, evergreen.TaskDescriptionHeartbeat)

			dbOtherTask, err := task.FindOneId(ctx, otherTask.Id)
			require.NoError(t, err)
			require.NotZero(t, dbOtherTask)
			assert.Equal(t, evergreen.TaskUndispatched, dbOtherTask.Status)
			depsMet, err := dbOtherTask.DependenciesMet(ctx, map[string]task.Task{})
			require.NoError(t, err)
			assert.False(t, depsMet, "single host task group task should depend on first task succeeding")
		},
		"RestartsParentDisplayTaskForStaleHostExecutionTask": func(ctx context.Context, t *testing.T, j *taskExecutionTimeoutJob, v model.Version) {
			const displayTaskID = "display_task"
			et0 := task.Task{
				Id:            "some_other_execution_task",
				BuildId:       j.task.BuildId,
				Version:       j.task.Version,
				Project:       j.task.Project,
				DisplayTaskId: utility.ToStringPtr(displayTaskID),
				Status:        evergreen.TaskFailed,
			}
			require.NoError(t, et0.Insert(t.Context()))
			et1 := task.Task{
				Id:            "another_execution_task",
				BuildId:       j.task.BuildId,
				Version:       j.task.Version,
				Project:       j.task.Project,
				DisplayTaskId: utility.ToStringPtr(displayTaskID),
				Status:        evergreen.TaskUndispatched,
			}
			require.NoError(t, et1.Insert(t.Context()))
			dt := task.Task{
				Id:             displayTaskID,
				BuildId:        j.task.BuildId,
				Version:        j.task.Version,
				Project:        j.task.Project,
				DisplayOnly:    true,
				ExecutionTasks: []string{j.task.Id, et0.Id, et1.Id},
			}
			require.NoError(t, dt.Insert(t.Context()))
			j.task.DisplayTaskId = utility.ToStringPtr(displayTaskID)
			require.NoError(t, j.task.Insert(t.Context()))
			require.NoError(t, v.Insert(t.Context()))

			j.Run(ctx)
			require.NoError(t, j.Error())

			checkTaskRestarted(t, j.task.Id, 0, evergreen.TaskDescriptionHeartbeat)

			for _, taskID := range []string{et0.Id, et1.Id, j.task.Id} {
				dbTask, err := task.FindOneId(ctx, taskID)
				require.NoError(t, err)
				assert.Equal(t, evergreen.TaskUndispatched, dbTask.Status, "execution task '%s' should be reset", dbTask.Id)
			}
		},
		"RestartsStaleHostTaskAndTerminatesExternallyTerminatedHost": func(ctx context.Context, t *testing.T, j *taskExecutionTimeoutJob, v model.Version) {
			mp.Set(hostID, cloud.MockInstance{
				Status: cloud.StatusTerminated,
			})

			require.NoError(t, v.Insert(t.Context()))
			require.NoError(t, j.task.Insert(t.Context()))

			j.Run(ctx)
			require.NoError(t, j.Error())

			queue, err := j.env.RemoteQueueGroup().Get(ctx, terminateHostQueueGroup)
			require.NoError(t, err)
			require.True(t, amboy.WaitInterval(ctx, queue, 100*time.Millisecond),
				"failed while waiting for host termination job to complete")

			dbHost, err := host.FindOneId(ctx, hostID)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status, "externally terminated host should be terminated")

			checkTaskRestarted(t, j.task.Id, 0, evergreen.TaskDescriptionStranded)
		},
		"StaleCopyOfPreviousExecutionShouldNotFailNewExecutionOrClearNewHost": func(ctx context.Context, t *testing.T, j *taskExecutionTimeoutJob, v model.Version) {
			require.NoError(t, v.Insert(t.Context()))

			// Simulate another cleanup job having already cleared the original
			// host, restarted the task, and dispatched the new execution to a
			// different host, all after this job loaded its stale copy of
			// execution 0.
			oldHost, err := host.FindOneId(ctx, hostID)
			require.NoError(t, err)
			require.NotZero(t, oldHost)
			require.NoError(t, oldHost.ClearRunningTask(ctx))

			const newHostID = "new_host_id"
			newExecution := restartAndDispatchTask(ctx, t, *j.task, newHostID)
			newHost := host.Host{
				Id:                   newHostID,
				Distro:               distro.Distro{Id: "distro_id", Provider: evergreen.ProviderNameMock},
				Provider:             evergreen.ProviderNameMock,
				StartedBy:            evergreen.User,
				Status:               evergreen.HostRunning,
				RunningTask:          newExecution.Id,
				RunningTaskExecution: newExecution.Execution,
			}
			require.NoError(t, newHost.Insert(ctx))
			mp.Set(newHost.Id, cloud.MockInstance{Status: cloud.StatusRunning})

			j.Run(ctx)
			require.NoError(t, j.Error())

			checkTaskStillDispatched(t, newExecution)
			checkHostRunningTask(t, newHostID, newExecution)
		},
		"StaleCopyOfPreviousExecutionShouldNotClearSameHostRunningNewExecution": func(ctx context.Context, t *testing.T, j *taskExecutionTimeoutJob, v model.Version) {
			require.NoError(t, v.Insert(t.Context()))

			// Simulate the task having been restarted and the new execution
			// dispatched back to the same host after this job loaded its stale
			// copy of execution 0.
			newExecution := restartAndDispatchTask(ctx, t, *j.task, hostID)
			require.NoError(t, host.UpdateOne(ctx, host.ById(hostID), bson.M{"$set": bson.M{host.RunningTaskExecutionKey: newExecution.Execution}}))

			j.Run(ctx)
			require.NoError(t, j.Error())

			checkTaskStillDispatched(t, newExecution)
			checkHostRunningTask(t, hostID, newExecution)
		},
		"StaleCopyWithDeletedHostShouldNotUnscheduleRedispatchedTask": func(ctx context.Context, t *testing.T, j *taskExecutionTimeoutJob, v model.Version) {
			require.NoError(t, v.Insert(t.Context()))
			require.NoError(t, db.ClearCollections(host.Collection))

			redispatched := *j.task
			redispatched.Status = evergreen.TaskDispatched
			redispatched.HostId = "new_host_id"
			require.NoError(t, redispatched.Insert(t.Context()))

			j.Run(ctx)
			require.NoError(t, j.Error())

			checkTaskStillDispatched(t, redispatched)
		},
		"NoopsForActiveTask": func(ctx context.Context, t *testing.T, j *taskExecutionTimeoutJob, _ model.Version) {
			j.task.LastHeartbeat = time.Now()
			require.NoError(t, j.task.Insert(t.Context()))

			j.Run(ctx)
			require.NoError(t, j.Error())

			dbTask, err := task.FindOneIdAndExecution(ctx, j.task.Id, 0)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.False(t, dbTask.Archived, "active task should not be archived")
			assert.False(t, dbTask.IsFinished(), "active task should not be marked finished")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithCancel(ctx)
			defer tcancel()
			tctx = testutil.TestSpan(tctx, t)

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))
			require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, model.VersionCollection, model.ParserProjectCollection, model.ProjectRefCollection, host.Collection, event.EventCollection))
			mp.Reset()

			const taskID = "task_id"
			h := host.Host{
				Id: hostID,
				Distro: distro.Distro{
					Id:       "distro_id",
					Provider: evergreen.ProviderNameMock,
				},
				Provider:    evergreen.ProviderNameMock,
				StartedBy:   evergreen.User,
				Status:      evergreen.HostRunning,
				RunningTask: taskID,
			}
			require.NoError(t, h.Insert(ctx))
			mp.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			v := model.Version{
				Id:     "version",
				Status: evergreen.VersionStarted,
			}

			b := build.Build{
				Id:      "build",
				Version: v.Id,
				Status:  evergreen.BuildStarted,
			}
			require.NoError(t, b.Insert(t.Context()))

			pRef := model.ProjectRef{
				Id:      "project_id",
				Enabled: true,
			}
			require.NoError(t, pRef.Insert(t.Context()))

			tsk := task.Task{
				Id:                taskID,
				BuildId:           b.Id,
				BuildVariant:      "build_variant",
				Version:           v.Id,
				Project:           pRef.Id,
				DisplayName:       "display_name",
				ExecutionPlatform: task.ExecutionPlatformHost,
				Activated:         true,
				ActivatedTime:     time.Now().Add(-10 * time.Minute),
				Status:            evergreen.TaskStarted,
				HostId:            h.Id,
				StartTime:         time.Now().Add(-90 * time.Minute),
				LastHeartbeat:     time.Now().Add(-time.Hour),
			}

			j, ok := NewTaskExecutionMonitorJob(tsk.Id, "").(*taskExecutionTimeoutJob)
			require.True(t, ok)
			j.task = &tsk
			j.env = env

			tCase(tctx, t, j, v)
		})
	}

	t.Run("JobsForSameTaskInDifferentTimeIntervalsShouldShareEnqueueScope", func(t *testing.T) {
		first := NewTaskExecutionMonitorJob("task_id", "ts0")
		second := NewTaskExecutionMonitorJob("task_id", "ts1")
		require.NotEqual(t, first.ID(), second.ID())
		assert.Equal(t, []string{"task-execution-timeout.task_id"}, first.EnqueueScopes())
		assert.Equal(t, first.EnqueueScopes(), second.EnqueueScopes())
		assert.NotEqual(t, first.EnqueueScopes(), NewTaskExecutionMonitorJob("other_task_id", "ts0").EnqueueScopes())
	})
}
