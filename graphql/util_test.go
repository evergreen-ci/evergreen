package graphql

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
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

func TestCanRestartTask(t *testing.T) {
	blockedTask := &task.Task{
		Id: "t1",
		DependsOn: []task.Dependency{
			{TaskId: "testDepends1", Status: "*", Unattainable: true},
			{TaskId: "testDepends2", Status: "*", Unattainable: false},
		},
		Status:        evergreen.TaskUndispatched,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	canRestart := canRestartTask(blockedTask)
	assert.Equal(t, canRestart, false)

	blockedDisplayTask := &task.Task{
		Id:             "t4",
		Status:         evergreen.TaskUndispatched,
		DisplayStatus:  evergreen.TaskStatusBlocked,
		DisplayTaskId:  utility.ToStringPtr(""),
		DisplayOnly:    true,
		ExecutionTasks: []string{"exec1", "exec2"},
	}
	canRestart = canRestartTask(blockedDisplayTask)
	assert.Equal(t, canRestart, true)

	executionTask := &task.Task{
		Id:            "t2",
		Status:        evergreen.TaskUndispatched,
		DisplayTaskId: utility.ToStringPtr("display task"),
	}
	canRestart = canRestartTask(executionTask)
	assert.Equal(t, canRestart, false)

	runningTask := &task.Task{
		Id:            "t3",
		Status:        evergreen.TaskStarted,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	canRestart = canRestartTask(runningTask)
	assert.Equal(t, canRestart, false)

	finishedTask := &task.Task{
		Id:            "t5",
		Status:        evergreen.TaskSucceeded,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	canRestart = canRestartTask(finishedTask)
	assert.Equal(t, canRestart, true)

	abortedTask := &task.Task{
		Id:            "t6",
		Status:        evergreen.TaskUndispatched,
		DisplayTaskId: utility.ToStringPtr(""),
		Aborted:       true,
	}
	canRestart = canRestartTask(abortedTask)
	assert.Equal(t, canRestart, false)
}

func TestCanScheduleTask(t *testing.T) {
	abortedTask := &task.Task{
		Id:            "t1",
		Status:        evergreen.TaskUndispatched,
		DisplayTaskId: utility.ToStringPtr(""),
		Aborted:       true,
	}
	canSchedule := canScheduleTask(abortedTask)
	assert.Equal(t, canSchedule, false)

	executionTask := &task.Task{
		Id:            "t2",
		Status:        evergreen.TaskUndispatched,
		DisplayStatus: evergreen.TaskUnscheduled,
		DisplayTaskId: utility.ToStringPtr("display task"),
	}
	canSchedule = canScheduleTask(executionTask)
	assert.Equal(t, canSchedule, false)

	finishedTask := &task.Task{
		Id:            "t4",
		Status:        evergreen.TaskSucceeded,
		DisplayStatus: evergreen.TaskSucceeded,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	canSchedule = canScheduleTask(finishedTask)
	assert.Equal(t, canSchedule, false)

	unscheduledTask := &task.Task{
		Id:            "t3",
		Status:        evergreen.TaskUndispatched,
		DisplayStatus: evergreen.TaskUnscheduled,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	canSchedule = canScheduleTask(unscheduledTask)
	assert.Equal(t, canSchedule, true)
}

func TestGetDisplayStatus(t *testing.T) {
	assert.NoError(t, db.ClearCollections(model.VersionCollection, patch.Collection))
	patchId := bson.NewObjectId()
	childPatchId := bson.NewObjectId()
	version := &model.Version{
		Id:        patchId.Hex(),
		Aborted:   true,
		Status:    evergreen.VersionSucceeded,
		Requester: evergreen.PatchVersionRequester,
	}

	assert.NoError(t, version.Insert())

	p := &patch.Patch{
		Id:     patchId,
		Status: evergreen.VersionSucceeded,
		Triggers: patch.TriggerInfo{
			ChildPatches: []string{childPatchId.Hex()},
		},
	}
	assert.NoError(t, p.Insert())

	cv := model.Version{
		Id:      childPatchId.Hex(),
		Aborted: true,
		Status:  evergreen.VersionFailed,
	}
	assert.NoError(t, cv.Insert())

	cp := &patch.Patch{
		Id:     childPatchId,
		Status: evergreen.VersionFailed,
	}
	assert.NoError(t, cp.Insert())

	status, err := getDisplayStatus(version)
	require.NoError(t, err)
	assert.Equal(t, evergreen.VersionAborted, status)
}
