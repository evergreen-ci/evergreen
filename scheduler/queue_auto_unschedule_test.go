package scheduler

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoUnscheduleLargeQueueOnlyUnschedulesCLIPatchTasks(t *testing.T) {
	ctx := t.Context()

	require.NoError(t, db.ClearCollections(task.Collection, build.Collection, model.VersionCollection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(task.Collection, build.Collection, model.VersionCollection))
	})

	newTask := func(id, requester string) task.Task {
		return task.Task{
			Id:        id,
			Status:    evergreen.TaskUndispatched,
			Activated: true,
			Requester: requester,
			DistroId:  "d",
			BuildId:   "b",
			Version:   "v",
		}
	}
	plan := []task.Task{
		newTask("mainline", evergreen.RepotrackerVersionRequester),
		newTask("patch", evergreen.PatchVersionRequester),
		newTask("github-pr", evergreen.GithubPRRequester),
		newTask("merge-queue", evergreen.GithubMergeRequester),
	}
	for _, tsk := range plan {
		require.NoError(t, tsk.Insert(ctx))
	}
	b := build.Build{
		Id:        "b",
		Activated: true,
		Status:    evergreen.BuildStarted,
		Version:   "v",
	}
	require.NoError(t, b.Insert(ctx))
	require.NoError(t, (&model.Version{Id: "v", Status: evergreen.VersionStarted}).Insert(ctx))

	assertActivated := func(expected func(task.Task) bool) {
		for _, tsk := range plan {
			found, err := task.FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			require.NotNil(t, found)
			assert.Equal(t, expected(tsk), found.Activated, "task '%s'", tsk.Id)
		}
	}

	require.NoError(t, autoUnscheduleLargeQueue(ctx, "d", plan, len(plan)+1))
	assertActivated(func(task.Task) bool { return true })

	require.NoError(t, autoUnscheduleLargeQueue(ctx, "d", plan, len(plan)))
	assertActivated(func(tsk task.Task) bool { return tsk.Id != "patch" })
}
