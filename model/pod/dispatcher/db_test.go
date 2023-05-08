package dispatcher

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func init() {
	testutil.Setup()
}

func TestFindOne(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	for tName, tCase := range map[string]func(t *testing.T, pds []PodDispatcher){
		"ReturnsNilResultForNoMatches": func(t *testing.T, pds []PodDispatcher) {
			found, err := FindOne(db.Query(bson.M{}))
			assert.NoError(t, err)
			assert.Zero(t, found)
		},
		"ReturnsOneMatching": func(t *testing.T, pds []PodDispatcher) {
			for _, pd := range pds {
				require.NoError(t, pd.Insert())
			}
			found, err := FindOneByID(pds[0].ID)
			require.NoError(t, err)
			require.NotZero(t, found)
			assert.Equal(t, pds[0], *found)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))
			tCase(t, []PodDispatcher{
				NewPodDispatcher("group0", []string{"task0"}, []string{"pod0"}),
				NewPodDispatcher("group1", []string{"task1"}, []string{"pod1"}),
			})
		})
	}
}

func TestFind(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	for tName, tCase := range map[string]func(t *testing.T, pds []PodDispatcher){
		"ReturnsEmptyForNoMatches": func(t *testing.T, pds []PodDispatcher) {
			found, err := Find(db.Query(bson.M{}))
			assert.NoError(t, err)
			assert.Empty(t, found)
		},
		"ReturnsMatching": func(t *testing.T, pds []PodDispatcher) {
			for _, pd := range pds {
				require.NoError(t, pd.Insert())
			}
			found, err := Find(db.Query(bson.M{}))
			require.NoError(t, err)
			require.Len(t, found, 2)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))
			tCase(t, []PodDispatcher{
				NewPodDispatcher("group0", []string{"task0"}, []string{"pod0"}),
				NewPodDispatcher("group1", []string{"task1"}, []string{"pod1"}),
			})
		})
	}
}

func TestFindOneByGroupID(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	for tName, tCase := range map[string]func(t *testing.T, pds []PodDispatcher){
		"FindsMatchingGroupID": func(t *testing.T, pds []PodDispatcher) {
			for _, pd := range pds {
				require.NoError(t, pd.Insert())
			}
			found, err := FindOneByGroupID(pds[0].GroupID)
			require.NoError(t, err)
			assert.Equal(t, pds[0].ID, found.ID)
		},
		"ReturnsNoResultsWithUnmatchedGroupID": func(t *testing.T, pds []PodDispatcher) {
			for _, pd := range pds {
				require.NoError(t, pd.Insert())
			}
			found, err := FindOneByGroupID("foo")
			assert.NoError(t, err)
			assert.Zero(t, found)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))
			tCase(t, []PodDispatcher{
				NewPodDispatcher("group0", []string{"task0"}, []string{"pod0"}),
				NewPodDispatcher("group1", []string{"task1"}, []string{"pod1"}),
			})
		})
	}
}

func TestFindOneByPodID(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	dispatchers := []PodDispatcher{
		NewPodDispatcher("group", []string{"task0", "task1"}, []string{"pod0", "pod1"}),
		NewPodDispatcher("group", []string{"task2", "task3"}, []string{"pod2", "pod3"}),
	}
	for _, pd := range dispatchers {
		require.NoError(t, pd.Insert())
	}

	t.Run("FindsDispatcherWithMatchingPodID", func(t *testing.T) {
		for _, pd := range dispatchers {
			for _, podID := range pd.PodIDs {
				dbDisp, err := FindOneByPodID(podID)
				require.NoError(t, err)
				require.NotZero(t, dbDisp)
				assert.Equal(t, pd.ID, dbDisp.ID)
			}
		}
	})
	t.Run("DoesNotFindDispatcherWithoutMatchingPodID", func(t *testing.T) {
		pd, err := FindOneByPodID("foo")
		assert.NoError(t, err)
		assert.Zero(t, pd)
	})
}

func TestAllocate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection, pod.Collection, task.Collection, event.EventCollection))
	}()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	// Running a multi-document transaction requires the collections to exist
	// first before any documents can be inserted.
	require.NoError(t, db.CreateCollections(Collection, task.Collection, pod.Collection))

	checkTaskAllocated := func(t *testing.T, tsk *task.Task) {
		dbTask, err := task.FindOneId(tsk.Id)
		require.NoError(t, err)
		require.NotZero(t, dbTask)
		assert.True(t, dbTask.ContainerAllocated)
		assert.NotZero(t, dbTask.ContainerAllocatedTime)
	}
	checkDispatcherUpdated := func(t *testing.T, tsk *task.Task, pd *PodDispatcher) {
		dbDispatcher, err := FindOneByGroupID(GetGroupID(tsk))
		require.NoError(t, err)
		require.NotZero(t, dbDispatcher)
		assert.Equal(t, pd.PodIDs, dbDispatcher.PodIDs)
		assert.Equal(t, pd.TaskIDs, dbDispatcher.TaskIDs)
		assert.Equal(t, pd.ModificationCount, dbDispatcher.ModificationCount)
		assert.False(t, utility.IsZeroTime(dbDispatcher.LastModified))
		assert.Equal(t, pd.LastModified, dbDispatcher.LastModified)
	}
	checkEventLogged := func(t *testing.T, tsk *task.Task) {
		dbEvents, err := event.FindAllByResourceID(tsk.Id)
		require.NoError(t, err)
		require.Len(t, dbEvents, 1)
		assert.Equal(t, event.ContainerAllocated, dbEvents[0].EventType)
	}
	checkAllocated := func(t *testing.T, tsk *task.Task, p *pod.Pod, pd *PodDispatcher) {
		dbPod, err := pod.FindOneByID(p.ID)
		require.NoError(t, err)
		require.NotZero(t, dbPod)
		assert.Equal(t, p.Type, dbPod.Type)
		assert.Equal(t, p.Status, dbPod.Status)

		checkTaskAllocated(t, tsk)
		checkDispatcherUpdated(t, tsk, pd)
		checkEventLogged(t, tsk)
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment, tsk *task.Task, p *pod.Pod){
		"SucceedsWithNewPodDispatcher": func(ctx context.Context, t *testing.T, env evergreen.Environment, tsk *task.Task, p *pod.Pod) {
			require.NoError(t, tsk.Insert())

			newDispatcher, err := Allocate(ctx, env, tsk, p)
			require.NoError(t, err)

			assert.Equal(t, GetGroupID(tsk), newDispatcher.GroupID)
			assert.Equal(t, []string{p.ID}, newDispatcher.PodIDs)
			assert.Equal(t, []string{tsk.Id}, newDispatcher.TaskIDs)
			assert.True(t, newDispatcher.ModificationCount > 0)

			checkAllocated(t, tsk, p, newDispatcher)
		},
		"SucceedsWithExistingPodDispatcherForGroup": func(ctx context.Context, t *testing.T, env evergreen.Environment, tsk *task.Task, p *pod.Pod) {
			pd := &PodDispatcher{
				GroupID:           GetGroupID(tsk),
				TaskIDs:           []string{tsk.Id},
				ModificationCount: 1,
			}
			require.NoError(t, pd.Insert())
			require.NoError(t, tsk.Insert())

			updatedDispatcher, err := Allocate(ctx, env, tsk, p)
			require.NoError(t, err)

			assert.Equal(t, pd.GroupID, updatedDispatcher.GroupID)
			left, right := utility.StringSliceSymmetricDifference(append(pd.PodIDs, p.ID), updatedDispatcher.PodIDs)
			assert.Empty(t, left)
			assert.Empty(t, right)
			assert.Equal(t, pd.TaskIDs, updatedDispatcher.TaskIDs)
			assert.True(t, updatedDispatcher.ModificationCount > pd.ModificationCount)
			assert.False(t, utility.IsZeroTime(updatedDispatcher.LastModified))

			checkAllocated(t, tsk, p, updatedDispatcher)
		},
		"SucceedsWithAlreadySufficientPodsAllocatedRunContainers": func(ctx context.Context, t *testing.T, env evergreen.Environment, tsk *task.Task, p *pod.Pod) {
			pd := &PodDispatcher{
				GroupID:           GetGroupID(tsk),
				PodIDs:            []string{utility.RandomString()},
				TaskIDs:           []string{tsk.Id},
				ModificationCount: 1,
			}
			require.NoError(t, pd.Insert())
			require.NoError(t, tsk.Insert())

			updatedDispatcher, err := Allocate(ctx, env, tsk, p)
			require.NoError(t, err)

			assert.Equal(t, pd.GroupID, updatedDispatcher.GroupID)
			assert.Equal(t, pd.PodIDs, updatedDispatcher.PodIDs)
			assert.Equal(t, pd.TaskIDs, updatedDispatcher.TaskIDs)
			assert.True(t, updatedDispatcher.ModificationCount > pd.ModificationCount)
			assert.False(t, utility.IsZeroTime(updatedDispatcher.LastModified))

			checkTaskAllocated(t, tsk)
			checkDispatcherUpdated(t, tsk, updatedDispatcher)
			checkEventLogged(t, tsk)
		},
		"FailsWithoutMatchingTaskStatus": func(ctx context.Context, t *testing.T, env evergreen.Environment, tsk *task.Task, p *pod.Pod) {
			pd, err := Allocate(ctx, env, tsk, p)
			require.Error(t, err)
			assert.Zero(t, pd)

			dbPod, err := pod.FindOneByID(p.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbPod)

			dbTask, err := task.FindOneId(tsk.Id)
			assert.NoError(t, err)
			assert.Zero(t, dbTask)

			dbDispatcher, err := FindOneByGroupID(GetGroupID(tsk))
			assert.NoError(t, err)
			assert.Zero(t, dbDispatcher)

			dbEvents, err := event.FindAllByResourceID(tsk.Id)
			assert.NoError(t, err)
			assert.Empty(t, dbEvents)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 10*time.Second)
			defer tcancel()

			require.NoError(t, db.ClearCollections(Collection, pod.Collection, task.Collection, event.EventCollection))

			p, err := pod.NewTaskIntentPod(evergreen.ECSConfig{
				AllowedImages: []string{
					"image",
				},
			}, pod.TaskIntentPodOptions{
				ID:                  primitive.NewObjectID().Hex(),
				CPU:                 256,
				MemoryMB:            512,
				OS:                  pod.OSLinux,
				Arch:                pod.ArchARM64,
				Image:               "image",
				WorkingDir:          "/",
				PodSecretExternalID: "pod_secret_external_id",
				PodSecretValue:      "pod_secret_value",
			})
			require.NoError(t, err)
			tCase(tctx, t, env, &task.Task{
				Id:                 "task",
				Status:             evergreen.TaskUndispatched,
				ExecutionPlatform:  task.ExecutionPlatformContainer,
				ContainerAllocated: false,
				Activated:          true,
			}, p)
		})
	}
}
