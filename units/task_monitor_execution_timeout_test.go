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
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestTaskExecutionTimeoutJob(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mp := cloud.GetMockProvider()

	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, pod.Collection, task.OldCollection, build.Collection, model.VersionCollection, model.ParserProjectCollection, model.ProjectRefCollection, host.Collection, event.EventCollection))
		mp.Reset()
	}()

	checkTaskRestarted := func(t *testing.T, taskID string, oldExecution int, description string) {
		archivedTask, err := task.FindOneIdAndExecution(taskID, oldExecution)
		require.NoError(t, err)
		require.NotZero(t, archivedTask)
		assert.True(t, archivedTask.Archived)
		assert.Equal(t, evergreen.TaskFailed, archivedTask.Status)
		assert.Equal(t, description, archivedTask.Details.Description)

		restartedTask, err := task.FindOneIdAndExecution(taskID, oldExecution+1)
		require.NoError(t, err)
		require.NotZero(t, restartedTask)
		assert.Equal(t, evergreen.TaskUndispatched, restartedTask.Status)
	}
	checkPodRunningTaskCleared := func(t *testing.T, podID string) {
		foundPod, err := pod.FindOneByID(podID)
		require.NoError(t, err)
		require.NotZero(t, foundPod)
		assert.Zero(t, foundPod.TaskRuntimeInfo)
	}

	const hostID = "host_id"
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *taskExecutionTimeoutJob, v model.Version){
		"RestartsStaleHostTask": func(ctx context.Context, t *testing.T, j *taskExecutionTimeoutJob, v model.Version) {
			require.NoError(t, j.task.Insert())
			require.NoError(t, v.Insert())

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
			require.NoError(t, v.Insert())
			require.NoError(t, j.task.Insert())
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
			require.NoError(t, otherTask.Insert())

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
			require.NoError(t, pp.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			checkTaskRestarted(t, j.task.Id, 0, evergreen.TaskDescriptionHeartbeat)

			dbOtherTask, err := task.FindOneId(otherTask.Id)
			require.NoError(t, err)
			require.NotZero(t, dbOtherTask)
			assert.Equal(t, evergreen.TaskUndispatched, dbOtherTask.Status)
			depsMet, err := dbOtherTask.DependenciesMet(map[string]task.Task{})
			require.NoError(t, err)
			assert.False(t, depsMet, "single host task group task should depend on first task succeeding")
		},
		"RestartsStaleContainerTask": func(ctx context.Context, t *testing.T, j *taskExecutionTimeoutJob, v model.Version) {
			j.task.ExecutionPlatform = task.ExecutionPlatformContainer
			j.task.HostId = ""
			p := pod.Pod{
				ID:     "pod_id",
				Status: pod.StatusRunning,
			}

			j.task.PodID = p.ID
			j.task.ContainerAllocated = true
			require.NoError(t, j.task.Insert())
			require.NoError(t, v.Insert())
			require.NoError(t, p.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			checkTaskRestarted(t, j.task.Id, 0, evergreen.TaskDescriptionHeartbeat)
		},
		"RestartsStaleContainerTaskAndChecksForUnhealthyPod": func(ctx context.Context, t *testing.T, j *taskExecutionTimeoutJob, v model.Version) {
			p := pod.Pod{
				ID:     "pod_id",
				Status: pod.StatusDecommissioned,
				TaskRuntimeInfo: pod.TaskRuntimeInfo{
					RunningTaskID:        j.task.Id,
					RunningTaskExecution: j.task.Execution,
				},
			}
			j.task.ExecutionPlatform = task.ExecutionPlatformContainer
			j.task.HostId = ""
			j.task.ContainerAllocated = true
			j.task.PodID = p.ID

			require.NoError(t, v.Insert())
			require.NoError(t, p.Insert())
			require.NoError(t, j.task.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			var foundHealthCheckJob bool
			for info := range j.env.RemoteQueue().JobInfo(ctx) {
				if info.Type.Name == podHealthCheckJobName {
					foundHealthCheckJob = true
					break
				}
			}
			assert.True(t, foundHealthCheckJob, "should have enqueued a pod health check job")

			checkTaskRestarted(t, j.task.Id, 0, evergreen.TaskDescriptionHeartbeat)
			checkPodRunningTaskCleared(t, p.ID)
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
			require.NoError(t, et0.Insert())
			et1 := task.Task{
				Id:            "another_execution_task",
				BuildId:       j.task.BuildId,
				Version:       j.task.Version,
				Project:       j.task.Project,
				DisplayTaskId: utility.ToStringPtr(displayTaskID),
				Status:        evergreen.TaskUndispatched,
			}
			require.NoError(t, et1.Insert())
			dt := task.Task{
				Id:             displayTaskID,
				BuildId:        j.task.BuildId,
				Version:        j.task.Version,
				Project:        j.task.Project,
				DisplayOnly:    true,
				ExecutionTasks: []string{j.task.Id, et0.Id, et1.Id},
			}
			require.NoError(t, dt.Insert())
			j.task.DisplayTaskId = utility.ToStringPtr(displayTaskID)
			require.NoError(t, j.task.Insert())
			require.NoError(t, v.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			checkTaskRestarted(t, j.task.Id, 0, evergreen.TaskDescriptionHeartbeat)

			for _, taskID := range []string{et0.Id, et1.Id, j.task.Id} {
				dbTask, err := task.FindOneId(taskID)
				require.NoError(t, err)
				assert.Equal(t, evergreen.TaskUndispatched, dbTask.Status, "execution task '%s' should be reset", dbTask.Id)
			}
		},
		"RestartsStaleHostTaskAndTerminatesExternallyTerminatedHost": func(ctx context.Context, t *testing.T, j *taskExecutionTimeoutJob, v model.Version) {
			mp.Set(hostID, cloud.MockInstance{
				Status: cloud.StatusTerminated,
			})

			require.NoError(t, v.Insert())
			require.NoError(t, j.task.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			require.True(t, amboy.WaitInterval(ctx, j.env.RemoteQueue(), 100*time.Millisecond))

			dbHost, err := host.FindOneId(ctx, hostID)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status, "externally terminated host should be terminated")

			checkTaskRestarted(t, j.task.Id, 0, evergreen.TaskDescriptionStranded)
		},
		"NoopsForActiveTask": func(ctx context.Context, t *testing.T, j *taskExecutionTimeoutJob, _ model.Version) {
			j.task.LastHeartbeat = time.Now()
			require.NoError(t, j.task.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			dbTask, err := task.FindOneIdAndExecution(j.task.Id, 0)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.False(t, dbTask.Archived, "active task should not be archived")
			assert.False(t, dbTask.IsFinished(), "active task should not be marked finished")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithCancel(ctx)
			defer tcancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, model.VersionCollection, model.ParserProjectCollection, model.ProjectRefCollection, pod.Collection, host.Collection, event.EventCollection))
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
			require.NoError(t, b.Insert())

			pRef := model.ProjectRef{
				Id:      "project_id",
				Enabled: true,
			}
			require.NoError(t, pRef.Insert())

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
				LastHeartbeat:     time.Now().Add(-time.Hour),
			}

			j, ok := NewTaskExecutionMonitorJob(tsk.Id, "").(*taskExecutionTimeoutJob)
			require.True(t, ok)
			j.task = &tsk
			j.env = env

			tCase(tctx, t, j, v)
		})
	}
}
