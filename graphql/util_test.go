package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	testutil.Setup()
}

func TestFilterGeneralSubscriptions(t *testing.T) {
	usr := &user.DBUser{}
	usr.Settings.Notifications = user.NotificationPreferences{
		PatchFinishID: "patch_finish_id",
		CommitQueueID: "commit_queue_id",
	}

	t.Run("NoGeneralSubscriptions", func(t *testing.T) {
		subs := []event.Subscription{
			{ID: "123455"},
			{ID: "abcdef"},
		}
		filteredSubIDs := removeGeneralSubscriptions(usr, subs)
		assert.ElementsMatch(t, []string{"123455", "abcdef"}, filteredSubIDs)
	})

	t.Run("OnlyGeneralSubscriptions", func(t *testing.T) {
		subs := []event.Subscription{
			{ID: "patch_finish_id"},
			{ID: "commit_queue_id"},
		}
		filteredSubIDs := removeGeneralSubscriptions(usr, subs)
		assert.Empty(t, filteredSubIDs)
	})

	t.Run("MixGeneralSubscriptions", func(t *testing.T) {
		subs := []event.Subscription{
			{ID: "patch_finish_id"},
			{ID: "123456"},
		}
		filteredSubIDs := removeGeneralSubscriptions(usr, subs)
		assert.ElementsMatch(t, []string{"123456"}, filteredSubIDs)
	})
}

func TestCollectiveStatusArray(t *testing.T) {
	assert.NoError(t, db.ClearCollections(model.VersionCollection, patch.Collection))
	patchId := bson.NewObjectId()
	version := &restModel.APIVersion{
		Id:        utility.ToStringPtr(patchId.Hex()),
		Aborted:   utility.ToBoolPtr(true),
		Status:    utility.ToStringPtr(evergreen.PatchFailed),
		Requester: utility.ToStringPtr("patch_request"),
	}

	assert.NoError(t, db.Insert(model.VersionCollection, version))

	p := &patch.Patch{
		Id:     patchId,
		Status: evergreen.PatchFailed,
	}
	require.NoError(t, p.Insert())

	statusArray, err := getCollectiveStatusArray(*version)
	require.NoError(t, err)
	assert.Equal(t, evergreen.PatchAborted, statusArray[0])
}

func TestCanRestartTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blockedTask := &restModel.APITask{
		Id: utility.ToStringPtr("t1"),
		DependsOn: []restModel.APIDependency{
			{TaskId: "testDepends1", Status: "*", Unattainable: true},
			{TaskId: "testDepends2", Status: "*", Unattainable: false},
		},
		Status: utility.ToStringPtr(evergreen.TaskUndispatched),
	}
	canRestart, err := canRestartTask(ctx, blockedTask)
	require.NoError(t, err)
	assert.Equal(t, canRestart, false)

	executionTask := &restModel.APITask{
		Id:           utility.ToStringPtr("t2"),
		ParentTaskId: "a display task",
		Status:       utility.ToStringPtr(evergreen.TaskUndispatched),
	}
	canRestart, err = canRestartTask(ctx, executionTask)
	require.NoError(t, err)
	assert.Equal(t, canRestart, false)

	runningTask := &restModel.APITask{
		Id:     utility.ToStringPtr("t3"),
		Status: utility.ToStringPtr(evergreen.TaskStarted),
	}
	canRestart, err = canRestartTask(ctx, runningTask)
	require.NoError(t, err)
	assert.Equal(t, canRestart, false)

	finishedTask := &restModel.APITask{
		Id:     utility.ToStringPtr("t4"),
		Status: utility.ToStringPtr(evergreen.TaskSucceeded),
	}
	canRestart, err = canRestartTask(ctx, finishedTask)
	require.NoError(t, err)
	assert.Equal(t, canRestart, true)
}

func TestCanScheduleTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	abortedTask := &restModel.APITask{
		Id:      utility.ToStringPtr("t1"),
		Status:  utility.ToStringPtr(evergreen.TaskUndispatched),
		Aborted: true,
	}
	canSchedule, err := canScheduleTask(ctx, abortedTask)
	require.NoError(t, err)
	assert.Equal(t, canSchedule, false)

	executionTask := &restModel.APITask{
		Id:            utility.ToStringPtr("t2"),
		ParentTaskId:  "a display task",
		Status:        utility.ToStringPtr(evergreen.TaskUndispatched),
		DisplayStatus: utility.ToStringPtr(evergreen.TaskUnscheduled),
		Aborted:       false,
	}
	canSchedule, err = canScheduleTask(ctx, executionTask)
	require.NoError(t, err)
	assert.Equal(t, canSchedule, false)

	finishedTask := &restModel.APITask{
		Id:            utility.ToStringPtr("t4"),
		Status:        utility.ToStringPtr(evergreen.TaskSucceeded),
		DisplayStatus: utility.ToStringPtr(evergreen.TaskSucceeded),
		Aborted:       false,
	}
	canSchedule, err = canScheduleTask(ctx, finishedTask)
	require.NoError(t, err)
	assert.Equal(t, canSchedule, false)

	unscheduledTask := &restModel.APITask{
		Id:            utility.ToStringPtr("t3"),
		Status:        utility.ToStringPtr(evergreen.TaskUndispatched),
		DisplayStatus: utility.ToStringPtr(evergreen.TaskUnscheduled),
		Aborted:       false,
	}
	canSchedule, err = canScheduleTask(ctx, unscheduledTask)
	require.NoError(t, err)
	assert.Equal(t, canSchedule, true)

}
