package scheduler

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlushOversizedQueueOnlyUnschedulesStaleOverflowingPatchTasks(t *testing.T) {
	ctx := t.Context()

	require.NoError(t, db.ClearCollections(task.Collection, build.Collection, model.VersionCollection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(task.Collection, build.Collection, model.VersionCollection))
	})

	stale := time.Now().Add(-2 * oversizedQueueGracePeriod)
	newTask := func(id, requester string, activatedTime time.Time) task.Task {
		return task.Task{
			Id:            id,
			Status:        evergreen.TaskUndispatched,
			Activated:     true,
			ActivatedTime: activatedTime,
			Requester:     requester,
			DistroId:      "d",
			BuildId:       "b",
			Version:       "v",
		}
	}
	// The plan is in scheduler-sorted order; with a threshold of 2, everything from
	// "stale-patch" on is overflow.
	plan := []task.Task{
		newTask("under-threshold-mainline", evergreen.RepotrackerVersionRequester, stale),
		newTask("under-threshold-patch", evergreen.PatchVersionRequester, stale),
		newTask("stale-patch", evergreen.PatchVersionRequester, stale),
		newTask("fresh-patch", evergreen.PatchVersionRequester, time.Now()),
		newTask("stale-mainline", evergreen.RepotrackerVersionRequester, stale),
		newTask("stale-merge-queue", evergreen.GithubMergeRequester, stale),
	}
	for _, tsk := range plan {
		require.NoError(t, tsk.Insert(ctx))
	}
	require.NoError(t, (&build.Build{Id: "b", Activated: true, Status: evergreen.BuildStarted, Version: "v"}).Insert(ctx))
	require.NoError(t, (&model.Version{Id: "v", Status: evergreen.VersionStarted}).Insert(ctx))

	require.NoError(t, flushOversizedQueue(ctx, "d", plan, len(plan)))
	require.NoError(t, flushOversizedQueue(ctx, "d", plan, 0))
	require.NoError(t, flushOversizedQueue(ctx, "d", plan, 2))

	for _, tsk := range plan {
		found, err := task.FindOneId(ctx, tsk.Id)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, tsk.Id != "stale-patch", found.Activated, "task '%s'", tsk.Id)
	}
}
