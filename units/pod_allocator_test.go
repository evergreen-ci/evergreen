package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/pod/dispatcher"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPodAllocatorJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, pod.Collection, dispatcher.Collection, event.AllLogCollection))
	}()

	var originalPodInit evergreen.PodInitConfig
	require.NoError(t, originalPodInit.Get(evergreen.GetEnvironment()))
	originalFlags, err := evergreen.GetServiceFlags()
	require.NoError(t, err)
	// Since the tests depend on modifying the global environment, reset it to
	// its initial state afterwards.
	defer func() {
		require.NoError(t, originalPodInit.Set())
		require.NoError(t, originalFlags.Set())
	}()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	env.EvergreenSettings.ServiceFlags.PodAllocatorDisabled = false
	require.NoError(t, env.EvergreenSettings.ServiceFlags.Set())
	env.EvergreenSettings.PodInit.MaxParallelPodRequests = 10
	require.NoError(t, env.EvergreenSettings.PodInit.Set())

	// Pod allocation uses a multi-document transaction, which requires the
	// collections to exist first before any documents can be inserted.
	require.NoError(t, db.CreateCollections(task.Collection, pod.Collection, dispatcher.Collection))

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *podAllocatorJob, tsk task.Task){
		"RunSucceeds": func(ctx context.Context, t *testing.T, j *podAllocatorJob, tsk task.Task) {
			j.task = &tsk
			require.NoError(t, tsk.Insert())

			j.Run(ctx)

			require.NoError(t, j.Error())

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.Equal(t, evergreen.TaskContainerAllocated, dbTask.Status)

			dbPod, err := pod.FindOne(db.Query(bson.M{}))
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusInitializing, dbPod.Status)

			dbDispatcher, err := dispatcher.FindOneByGroupID(dispatcher.GetGroupID(&tsk))
			require.NoError(t, err)
			require.NotZero(t, dbDispatcher)
			assert.Equal(t, []string{dbPod.ID}, dbDispatcher.PodIDs)
			assert.Equal(t, []string{tsk.Id}, dbDispatcher.TaskIDs)

			taskEvents, err := event.FindAllByResourceID(tsk.Id)
			require.NoError(t, err)
			require.Len(t, taskEvents, 1)
			assert.Equal(t, event.ContainerAllocated, taskEvents[0].EventType)
		},
		"RunFailsForTaskWhoseDBStatusHasChanged": func(ctx context.Context, t *testing.T, j *podAllocatorJob, tsk task.Task) {
			j.task = &tsk
			modified := tsk
			modified.Activated = false
			require.NoError(t, modified.Insert())

			j.Run(ctx)

			require.Error(t, j.Error())
			assert.True(t, j.RetryInfo().ShouldRetry())

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.Equal(t, evergreen.TaskContainerUnallocated, dbTask.Status)

			dbPod, err := pod.FindOne(db.Query(bson.M{}))
			assert.NoError(t, err)
			assert.Zero(t, dbPod)

			dbDispatcher, err := dispatcher.FindOne(db.Query(bson.M{}))
			assert.NoError(t, err)
			assert.Zero(t, dbDispatcher)

			taskEvents, err := event.FindAllByResourceID(tsk.Id)
			assert.NoError(t, err)
			assert.Empty(t, taskEvents)
		},
		"RunNoopsForTaskThatDoesNotNeedContainerAllocation": func(ctx context.Context, t *testing.T, j *podAllocatorJob, tsk task.Task) {
			tsk.Activated = false
			j.task = &tsk
			require.NoError(t, tsk.Insert())

			j.Run(ctx)

			require.NoError(t, j.Error())
			assert.False(t, j.RetryInfo().ShouldRetry())

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.False(t, dbTask.Activated)
			assert.Equal(t, evergreen.TaskContainerUnallocated, dbTask.Status)

			dbPod, err := pod.FindOne(db.Query(bson.M{}))
			assert.NoError(t, err)
			assert.Zero(t, dbPod)

			dbDispatcher, err := dispatcher.FindOne(db.Query(bson.M{}))
			assert.NoError(t, err)
			assert.Zero(t, dbDispatcher)

			taskEvents, err := event.FindAllByResourceID(tsk.Id)
			assert.NoError(t, err)
			assert.Empty(t, taskEvents)
		},
		"RunNoopsWhenPodAllocationIsDisabled": func(ctx context.Context, t *testing.T, j *podAllocatorJob, tsk task.Task) {
			originalFlags := env.EvergreenSettings.ServiceFlags
			defer func() {
				assert.NoError(t, originalFlags.Set())
			}()
			env.EvergreenSettings.ServiceFlags.PodAllocatorDisabled = true
			require.NoError(t, env.EvergreenSettings.ServiceFlags.Set())

			j.task = &tsk
			require.NoError(t, tsk.Insert())

			j.Run(ctx)
			assert.True(t, j.RetryInfo().ShouldRetry())

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.Equal(t, evergreen.TaskContainerUnallocated, dbTask.Status)
		},
		"RunNoopsWhenMaxParallelPodRequestLimitIsReached": func(ctx context.Context, t *testing.T, j *podAllocatorJob, tsk task.Task) {
			originalPodInit := env.EvergreenSettings.PodInit
			defer func() {
				assert.NoError(t, originalPodInit.Set())
			}()
			env.EvergreenSettings.PodInit.MaxParallelPodRequests = 1
			require.NoError(t, env.EvergreenSettings.PodInit.Set())

			initializing := getInitializingPod(t)
			require.NoError(t, initializing.Insert())

			j.task = &tsk
			require.NoError(t, tsk.Insert())

			j.Run(ctx)
			assert.True(t, j.RetryInfo().ShouldRetry())

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.Equal(t, evergreen.TaskContainerUnallocated, dbTask.Status, "container task should not have been allocated because of max parallel pod request limit")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 10*time.Second)
			defer tcancel()

			require.NoError(t, db.ClearCollections(task.Collection, pod.Collection, dispatcher.Collection, event.AllLogCollection))

			tsk := getTaskThatNeedsContainerAllocation()
			j := NewPodAllocatorJob(tsk.Id, utility.RoundPartOfMinute(0).Format(TSFormat))
			allocatorJob := j.(*podAllocatorJob)
			allocatorJob.env = env
			tCase(tctx, t, allocatorJob, tsk)
		})
	}
}

func TestPopulatePodAllocatorJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	getProjectRef := func() model.ProjectRef {
		return model.ProjectRef{
			Id:         utility.RandomString(),
			Identifier: utility.RandomString(),
			Enabled:    utility.TruePtr(),
		}
	}

	// Pod allocation uses a multi-document transaction, which requires
	// the collections to exist first before any documents can be
	// inserted.
	require.NoError(t, db.CreateCollections(task.Collection, pod.Collection, dispatcher.Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection, pod.Collection, dispatcher.Collection))
	}()

	var originalPodInit evergreen.PodInitConfig
	require.NoError(t, originalPodInit.Get(evergreen.GetEnvironment()))
	originalFlags, err := evergreen.GetServiceFlags()
	require.NoError(t, err)
	// Since the tests depend on modifying the global environment, reset it to
	// its initial state afterwards.
	defer func() {
		require.NoError(t, originalPodInit.Set())
		require.NoError(t, originalFlags.Set())
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment){
		"CreatesNoPodAllocatorsWithoutTasksNeedingContainerAllocation": func(ctx context.Context, t *testing.T, env *mock.Environment) {
			ref := getProjectRef()
			require.NoError(t, ref.Insert())
			doesNotNeedAllocation := getTaskThatNeedsContainerAllocation()
			doesNotNeedAllocation.Status = evergreen.TaskContainerAllocated
			doesNotNeedAllocation.Project = ref.Id
			require.NoError(t, doesNotNeedAllocation.Insert())

			require.NoError(t, PopulatePodAllocatorJobs(env)(ctx, env.Remote))

			assert.Zero(t, env.Remote.Stats(ctx))
		},
		"AllocatesTaskNeedingContainerAllocation": func(ctx context.Context, t *testing.T, env *mock.Environment) {
			ref := getProjectRef()
			require.NoError(t, ref.Insert())
			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			require.NoError(t, PopulatePodAllocatorJobs(env)(ctx, env.Remote))

			stats := env.Remote.Stats(ctx)
			assert.Equal(t, stats.Total, 1)

			require.True(t, amboy.WaitInterval(ctx, env.Remote, 100*time.Millisecond), "pod allocator should have finished running")

			dbTask, err := task.FindOneId(needsAllocation.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.Equal(t, evergreen.TaskContainerAllocated, dbTask.Status)

			pd, err := dispatcher.FindOneByGroupID(needsAllocation.Id)
			require.NoError(t, err)
			require.NotZero(t, pd, "pod dispatcher should have been allocated for task")
			require.Len(t, pd.TaskIDs, 1)
			assert.Equal(t, needsAllocation.Id, pd.TaskIDs[0])
			require.Len(t, pd.PodIDs, 1)

			p, err := pod.FindOneByID(pd.PodIDs[0])
			require.NoError(t, err)
			require.NotZero(t, p, "intent pod should have been allocated for task")
		},
		"StopsEnqueueingJobsWhenMaxParallelPodRequestLimitIsReached": func(ctx context.Context, t *testing.T, env *mock.Environment) {
			originalPodInit := env.EvergreenSettings.PodInit
			defer func() {
				require.NoError(t, originalPodInit.Set())
			}()
			env.EvergreenSettings.PodInit.MaxParallelPodRequests = 1
			require.NoError(t, env.EvergreenSettings.PodInit.Set())

			initializing := getInitializingPod(t)
			require.NoError(t, initializing.Insert())

			ref := getProjectRef()
			require.NoError(t, ref.Insert())
			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			require.NoError(t, PopulatePodAllocatorJobs(env)(ctx, env.Remote))

			assert.Zero(t, env.Remote.Stats(ctx), "should not enqueue more pod allocator jobs when max parallel pod request limit is reached")
		},
		"DoesNotEnqueueJobsWhenDisabled": func(ctx context.Context, t *testing.T, env *mock.Environment) {
			originalFlags := env.EvergreenSettings.ServiceFlags
			defer func() {
				assert.NoError(t, originalFlags.Set())
			}()
			env.EvergreenSettings.ServiceFlags.PodAllocatorDisabled = true
			require.NoError(t, env.EvergreenSettings.ServiceFlags.Set())

			ref := getProjectRef()
			require.NoError(t, ref.Insert())
			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			require.NoError(t, PopulatePodAllocatorJobs(env)(ctx, env.Remote))

			assert.Zero(t, env.Remote.Stats(ctx), "pod allocator job should not be created when max parallel pod requset limit is reached")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 10*time.Second)
			defer tcancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))

			env.EvergreenSettings.ServiceFlags.PodAllocatorDisabled = false
			require.NoError(t, env.EvergreenSettings.ServiceFlags.Set())
			env.EvergreenSettings.PodInit.MaxParallelPodRequests = 100
			require.NoError(t, env.EvergreenSettings.PodInit.Set())

			require.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection, pod.Collection, dispatcher.Collection))

			tCase(tctx, t, env)
		})
	}
}

func getTaskThatNeedsContainerAllocation() task.Task {
	return task.Task{
		Id:                utility.RandomString(),
		Activated:         true,
		ActivatedTime:     time.Now(),
		Status:            evergreen.TaskContainerUnallocated,
		ExecutionPlatform: task.ExecutionPlatformContainer,
	}
}

func getInitializingPod(t *testing.T) pod.Pod {
	initializing, err := pod.NewTaskIntentPod(pod.TaskIntentPodOptions{
		CPU:        1024,
		MemoryMB:   1024,
		OS:         pod.OSLinux,
		Arch:       pod.ArchAMD64,
		Image:      "image",
		WorkingDir: "/",
	})
	require.NoError(t, err)
	return *initializing
}
