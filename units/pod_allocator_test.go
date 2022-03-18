package units

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
	"github.com/evergreen-ci/evergreen/model/pod/dispatcher"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPodAllocatorJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, pod.Collection, dispatcher.Collection, event.AllLogCollection))
	}()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

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
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 10*time.Second)
			defer tcancel()

			require.NoError(t, db.ClearCollections(task.Collection, pod.Collection, dispatcher.Collection, event.AllLogCollection))

			tsk := task.Task{
				Id:        "task0",
				Activated: true,
				Status:    evergreen.TaskContainerUnallocated,
			}
			j := NewPodAllocatorJob(tsk.Id, utility.RoundPartOfMinute(0).Format(TSFormat))
			allocatorJob := j.(*podAllocatorJob)
			allocatorJob.env = env
			tCase(tctx, t, allocatorJob, tsk)
		})
	}
}
