package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/cocoa"
	cocoaMock "github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/pod/dispatcher"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPodAllocatorJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	defer func() {
		cocoaMock.ResetGlobalSecretCache()

		assert.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection, pod.Collection, dispatcher.Collection, event.EventCollection))
	}()

	var originalPodLifecycleConf evergreen.PodLifecycleConfig
	require.NoError(t, originalPodLifecycleConf.Get(ctx))
	originalFlags, err := evergreen.GetServiceFlags(ctx)
	require.NoError(t, err)
	// Since the tests depend on modifying the global environment, reset it to
	// its initial state afterwards.
	defer func() {
		require.NoError(t, originalPodLifecycleConf.Set(ctx))
		require.NoError(t, originalFlags.Set(ctx))
	}()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	env.EvergreenSettings.ServiceFlags.PodAllocatorDisabled = false
	require.NoError(t, env.EvergreenSettings.ServiceFlags.Set(ctx))
	env.EvergreenSettings.PodLifecycle.MaxParallelPodRequests = 10
	require.NoError(t, env.EvergreenSettings.PodLifecycle.Set(ctx))
	require.NoError(t, env.EvergreenSettings.Providers.Set(ctx))

	// Pod allocation uses a multi-document transaction, which requires the
	// collections to exist first before any documents can be inserted.
	require.NoError(t, db.CreateCollections(task.Collection, pod.Collection, dispatcher.Collection))

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *podAllocatorJob, v cocoa.Vault, tsk task.Task, pRef model.ProjectRef){
		"RunSucceeds": func(ctx context.Context, t *testing.T, j *podAllocatorJob, v cocoa.Vault, tsk task.Task, pRef model.ProjectRef) {
			require.NoError(t, tsk.Insert())

			j.Run(ctx)

			require.NoError(t, j.Error())

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.True(t, dbTask.ContainerAllocated)

			dbPod, err := pod.FindOne(db.Query(bson.M{}))
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusInitializing, dbPod.Status)

			podSecret, err := dbPod.GetSecret()
			require.NoError(t, err)
			assert.Equal(t, pRef.ContainerSecrets[0].ExternalID, podSecret.ExternalID)
			storedPodSecret, err := v.GetValue(ctx, pRef.ContainerSecrets[0].ExternalID)
			require.NoError(t, err)
			assert.Equal(t, storedPodSecret, podSecret.Value)

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
		"RunSucceedsAndPopulatesRepoCreds": func(ctx context.Context, t *testing.T, j *podAllocatorJob, v cocoa.Vault, tsk task.Task, pRef model.ProjectRef) {
			pRef.ContainerSecrets = append(pRef.ContainerSecrets, model.ContainerSecret{
				Name:         "repo_creds_name",
				ExternalName: "repo_creds_external_name",
				Type:         model.ContainerSecretRepoCreds,
			})
			require.NoError(t, pRef.Upsert())

			_, err := v.CreateSecret(ctx, *cocoa.NewNamedSecret().SetName(pRef.ContainerSecrets[1].ExternalName).SetValue("repo_creds_value"))
			require.NoError(t, err)

			dbProjRef, err := model.FindBranchProjectRef(pRef.Id)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
			pRef = *dbProjRef

			tsk.ContainerOpts.RepoCredsName = pRef.ContainerSecrets[1].Name
			require.NoError(t, tsk.Insert())

			j.Run(ctx)

			require.NoError(t, j.Error())

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.True(t, dbTask.ContainerAllocated)

			dbPod, err := pod.FindOne(db.Query(bson.M{}))
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusInitializing, dbPod.Status)

			podSecret, err := dbPod.GetSecret()
			require.NoError(t, err)
			assert.Equal(t, pRef.ContainerSecrets[0].ExternalID, podSecret.ExternalID)
			storedPodSecret, err := v.GetValue(ctx, pRef.ContainerSecrets[0].ExternalID)
			require.NoError(t, err)
			assert.Equal(t, storedPodSecret, podSecret.Value)

			assert.Equal(t, pRef.ContainerSecrets[1].ExternalID, dbPod.TaskContainerCreationOpts.RepoCredsExternalID)

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
		"RunFailsForTaskWhoseDBStatusHasChanged": func(ctx context.Context, t *testing.T, j *podAllocatorJob, v cocoa.Vault, tsk task.Task, pRef model.ProjectRef) {
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
			assert.False(t, dbTask.ContainerAllocated)

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
		"RunNoopsForTaskThatDoesNotNeedContainerAllocation": func(ctx context.Context, t *testing.T, j *podAllocatorJob, v cocoa.Vault, tsk task.Task, pRef model.ProjectRef) {
			tsk.Activated = false
			require.NoError(t, tsk.Insert())

			j.Run(ctx)

			require.NoError(t, j.Error())
			assert.False(t, j.RetryInfo().ShouldRetry())

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.False(t, dbTask.ContainerAllocated)

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
		"RunNoopsWhenPodAllocationIsDisabled": func(ctx context.Context, t *testing.T, j *podAllocatorJob, v cocoa.Vault, tsk task.Task, pRef model.ProjectRef) {
			originalFlags := env.EvergreenSettings.ServiceFlags
			defer func() {
				assert.NoError(t, originalFlags.Set(ctx))
			}()
			env.EvergreenSettings.ServiceFlags.PodAllocatorDisabled = true
			require.NoError(t, env.EvergreenSettings.ServiceFlags.Set(ctx))

			require.NoError(t, tsk.Insert())

			j.Run(ctx)
			assert.True(t, j.RetryInfo().ShouldRetry())

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.False(t, dbTask.ContainerAllocated)
		},
		"RunNoopsWhenMaxParallelPodRequestLimitIsReached": func(ctx context.Context, t *testing.T, j *podAllocatorJob, v cocoa.Vault, tsk task.Task, pRef model.ProjectRef) {
			originalPodLifecycle := env.EvergreenSettings.PodLifecycle
			defer func() {
				assert.NoError(t, originalPodLifecycle.Set(ctx))
			}()
			env.EvergreenSettings.PodLifecycle.MaxParallelPodRequests = 1
			require.NoError(t, env.EvergreenSettings.PodLifecycle.Set(ctx))

			initializing := getInitializingPod(t)
			require.NoError(t, initializing.Insert())

			require.NoError(t, tsk.Insert())

			j.Run(ctx)
			assert.True(t, j.RetryInfo().ShouldRetry())

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.False(t, dbTask.ContainerAllocated)
		},
		"RunNoopsWhenProjectDoesNotAllowDispatching": func(ctx context.Context, t *testing.T, j *podAllocatorJob, v cocoa.Vault, tsk task.Task, pRef model.ProjectRef) {
			pRef.Enabled = false
			require.NoError(t, pRef.Upsert())
			require.NoError(t, tsk.Insert())

			j.Run(ctx)

			require.NoError(t, j.Error())

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.False(t, dbTask.ContainerAllocated)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 10*time.Second)
			defer tcancel()
			tctx = testutil.TestSpan(tctx, t)

			cocoaMock.ResetGlobalSecretCache()

			require.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection, pod.Collection, dispatcher.Collection, event.EventCollection))

			tsk := getTaskThatNeedsContainerAllocation()
			pRef := model.ProjectRef{
				Id:         "project_id",
				Identifier: "project_identifier",
				Enabled:    true,
				ContainerSecrets: []model.ContainerSecret{
					{
						Name:         "pod_secret",
						ExternalName: "pod_secret_external_name",
						Type:         model.ContainerSecretPodSecret,
					},
				},
			}
			require.NoError(t, pRef.Insert())
			tsk.Project = pRef.Id

			smClient := &cocoaMock.SecretsManagerClient{}
			v, err := cloud.MakeSecretsManagerVault(smClient)
			require.NoError(t, err)
			mv := cocoaMock.NewVault(v)

			_, err = mv.CreateSecret(ctx, *cocoa.NewNamedSecret().
				SetName(pRef.ContainerSecrets[0].ExternalName).
				SetValue("super_secret_string"))
			require.NoError(t, err)

			// Re-find the project ref because creating the secret will update
			// the container secret.
			dbProjRef, err := model.FindBranchProjectRef(pRef.Id)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
			pRef = *dbProjRef
			require.NotZero(t, dbProjRef.ContainerSecrets[0].ExternalID, "creating the container secret should have set the external ID")

			j := NewPodAllocatorJob(tsk.Id, utility.RoundPartOfMinute(0).Format(TSFormat))
			allocatorJob := j.(*podAllocatorJob)
			allocatorJob.env = env

			allocatorJob.smClient = smClient
			allocatorJob.vault = mv

			env.EvergreenSettings.Providers.AWS.Pod.ECS.AllowedImages = []string{
				"rhel",
			}
			require.NoError(t, env.EvergreenSettings.Providers.Set(tctx))
			tCase(tctx, t, allocatorJob, mv, tsk, pRef)
		})
	}
}

func TestPopulatePodAllocatorJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	getProjectRef := func() model.ProjectRef {
		return model.ProjectRef{
			Id:         utility.RandomString(),
			Identifier: utility.RandomString(),
			Enabled:    true,
		}
	}

	// Pod allocation uses a multi-document transaction, which requires
	// the collections to exist first before any documents can be
	// inserted.
	require.NoError(t, db.CreateCollections(task.Collection, pod.Collection, dispatcher.Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection, pod.Collection, dispatcher.Collection))
	}()

	var originalPodLifecycleConf evergreen.PodLifecycleConfig
	require.NoError(t, originalPodLifecycleConf.Get(ctx))
	originalFlags, err := evergreen.GetServiceFlags(ctx)
	require.NoError(t, err)
	// Since the tests depend on modifying the global environment, reset it to
	// its initial state afterwards.
	defer func() {
		require.NoError(t, originalPodLifecycleConf.Set(ctx))
		require.NoError(t, originalFlags.Set(ctx))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment){
		"CreatesNoPodAllocatorsWithoutTasksNeedingContainerAllocation": func(ctx context.Context, t *testing.T, env *mock.Environment) {
			ref := getProjectRef()
			require.NoError(t, ref.Insert())
			doesNotNeedAllocation := getTaskThatNeedsContainerAllocation()
			doesNotNeedAllocation.ContainerAllocated = true
			doesNotNeedAllocation.Project = ref.Id
			require.NoError(t, doesNotNeedAllocation.Insert())

			jobs, err := podAllocatorJobs(ctx, time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
			require.NoError(t, err)
			require.NoError(t, env.Remote.PutMany(ctx, jobs))
			assert.Zero(t, env.Remote.Stats(ctx))
		},
		"MarksStaleContainerTasksAsNoLongerNeedingAllocation": func(ctx context.Context, t *testing.T, env *mock.Environment) {
			ref := getProjectRef()
			require.NoError(t, ref.Insert())
			staleNeedsAllocation := getTaskThatNeedsContainerAllocation()
			staleNeedsAllocation.ActivatedTime = time.Now().Add(-1000 * 24 * time.Hour)
			staleNeedsAllocation.Project = ref.Id
			require.NoError(t, staleNeedsAllocation.Insert())

			jobs, err := podAllocatorJobs(ctx, time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
			require.NoError(t, err)
			require.NoError(t, env.Remote.PutMany(ctx, jobs))
			assert.Zero(t, env.Remote.Stats(ctx))

			dbTask, err := task.FindOneId(staleNeedsAllocation.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.False(t, dbTask.ShouldAllocateContainer())
		},
		"StopsEnqueueingJobsWhenMaxParallelPodRequestLimitIsReached": func(ctx context.Context, t *testing.T, env *mock.Environment) {
			originalPodLifecycleConf := env.EvergreenSettings.PodLifecycle
			defer func() {
				require.NoError(t, originalPodLifecycleConf.Set(ctx))
			}()
			env.EvergreenSettings.PodLifecycle.MaxParallelPodRequests = 1
			require.NoError(t, env.EvergreenSettings.PodLifecycle.Set(ctx))

			initializing := getInitializingPod(t)
			require.NoError(t, initializing.Insert())

			ref := getProjectRef()
			require.NoError(t, ref.Insert())
			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			jobs, err := podAllocatorJobs(ctx, time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
			require.NoError(t, err)
			require.NoError(t, env.Remote.PutMany(ctx, jobs))
			assert.Zero(t, env.Remote.Stats(ctx), "should not enqueue more pod allocator jobs when max parallel pod request limit is reached")
		},
		"DoesNotEnqueueJobsWhenDisabled": func(ctx context.Context, t *testing.T, env *mock.Environment) {
			originalFlags := env.EvergreenSettings.ServiceFlags
			defer func() {
				assert.NoError(t, originalFlags.Set(ctx))
			}()
			env.EvergreenSettings.ServiceFlags.PodAllocatorDisabled = true
			require.NoError(t, env.EvergreenSettings.ServiceFlags.Set(ctx))

			ref := getProjectRef()
			require.NoError(t, ref.Insert())
			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			jobs, err := podAllocatorJobs(ctx, time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
			require.NoError(t, err)
			require.NoError(t, env.Remote.PutMany(ctx, jobs))
			assert.Zero(t, env.Remote.Stats(ctx), "pod allocator job should not be created when max parallel pod requset limit is reached")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 10*time.Second)
			defer tcancel()
			tctx = testutil.TestSpan(tctx, t)

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))

			env.EvergreenSettings.ServiceFlags.PodAllocatorDisabled = false
			require.NoError(t, env.EvergreenSettings.ServiceFlags.Set(tctx))
			env.EvergreenSettings.PodLifecycle.MaxParallelPodRequests = 100
			require.NoError(t, env.EvergreenSettings.PodLifecycle.Set(tctx))

			require.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection, pod.Collection, dispatcher.Collection))

			tCase(tctx, t, env)
		})
	}
}

func getTaskThatNeedsContainerAllocation() task.Task {
	return task.Task{
		Id:                 utility.RandomString(),
		Activated:          true,
		ActivatedTime:      time.Now(),
		Status:             evergreen.TaskUndispatched,
		ContainerAllocated: false,
		ExecutionPlatform:  task.ExecutionPlatformContainer,
		Container:          "container",
		ContainerOpts: task.ContainerOptions{
			Image:          "rhel",
			CPU:            256,
			MemoryMB:       1024,
			OS:             evergreen.WindowsOS,
			Arch:           evergreen.ArchAMD64,
			WindowsVersion: evergreen.Windows2022,
			WorkingDir:     "/data",
		},
	}
}

func getInitializingPod(t *testing.T) pod.Pod {
	initializing, err := pod.NewTaskIntentPod(evergreen.ECSConfig{
		AllowedImages: []string{"rhel"},
	}, pod.TaskIntentPodOptions{
		Image:               "rhel",
		CPU:                 256,
		MemoryMB:            1024,
		OS:                  pod.OSWindows,
		Arch:                pod.ArchAMD64,
		WindowsVersion:      pod.WindowsVersionServer2022,
		WorkingDir:          "/data",
		PodSecretExternalID: "pod_secret_external_id",
		PodSecretValue:      "pod_secret_value",
	})
	require.NoError(t, err)
	return *initializing
}
