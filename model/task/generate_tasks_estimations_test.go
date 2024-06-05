package task

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
)

func TestGenerateTasksEstimations(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bv := "bv"
	project := "proj"
	displayName := "display_name"

	t1 := Task{
		Id:                         "t1",
		DisplayName:                displayName,
		BuildVariant:               bv,
		Project:                    project,
		Status:                     evergreen.TaskFailed,
		Requester:                  evergreen.RepotrackerVersionRequester,
		NumGeneratedTasks:          1,
		NumActivatedGeneratedTasks: 11,
		StartTime:                  time.Now().Add(-1 * 10 * time.Hour),
		FinishTime:                 time.Now().Add(-1 * 9 * time.Hour),
		GeneratedTasks:             true,
	}
	assert.NoError(t1.Insert())
	t2 := Task{
		Id:                         "t2",
		DisplayName:                displayName,
		BuildVariant:               bv,
		Project:                    project,
		Status:                     evergreen.TaskSucceeded,
		Requester:                  evergreen.GithubPRRequester,
		NumGeneratedTasks:          2,
		NumActivatedGeneratedTasks: 20,
		StartTime:                  time.Now().Add(-1 * 20 * time.Hour),
		FinishTime:                 time.Now().Add(-1 * 19 * time.Hour),
		GeneratedTasks:             true,
	}
	assert.NoError(t2.Insert())
	t3 := Task{
		Id:                         "t3",
		DisplayName:                displayName,
		BuildVariant:               bv,
		Project:                    project,
		Status:                     evergreen.TaskFailed,
		Requester:                  evergreen.PatchVersionRequester,
		NumGeneratedTasks:          3,
		NumActivatedGeneratedTasks: 30,
		StartTime:                  time.Now().Add(-1 * 30 * time.Hour),
		FinishTime:                 time.Now().Add(-1 * 29 * time.Hour),
		GeneratedTasks:             true,
	}
	assert.NoError(t3.Insert())
	t4 := Task{
		GenerateTask:        true,
		Id:                  "t4",
		DisplayName:         displayName,
		BuildVariant:        bv,
		Project:             project,
		RevisionOrderNumber: 4,
	}
	assert.NoError(t4.Insert())

	err := t4.setGenerateTasksEstimations(ctx)
	assert.NoError(err)
	assert.Equal(2, utility.FromIntPtr(t4.EstimatedNumGeneratedTasks))
	assert.Equal(20, utility.FromIntPtr(t4.EstimatedNumActivatedGeneratedTasks))

	dbTask, err := FindOneId(t4.Id)
	assert.NoError(err)
	assert.Equal(2, utility.FromIntPtr(dbTask.EstimatedNumGeneratedTasks))
	assert.Equal(20, utility.FromIntPtr(dbTask.EstimatedNumActivatedGeneratedTasks))
}

func TestGenerateTasksEstimationsNoPreviousTasks(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1 := Task{
		Id:           "t1",
		DisplayName:  "new task with no history",
		BuildVariant: "bv",
		Project:      "project",
	}
	assert.NoError(t1.Insert())

	err := t1.setGenerateTasksEstimations(ctx)
	assert.NoError(err)
	assert.Equal(0, utility.FromIntPtr(t1.EstimatedNumGeneratedTasks))
	assert.Equal(0, utility.FromIntPtr(t1.EstimatedNumActivatedGeneratedTasks))

	dbTask, err := FindOneId(t1.Id)
	assert.NoError(err)
	assert.Equal(0, utility.FromIntPtr(dbTask.EstimatedNumGeneratedTasks))
	assert.Equal(0, utility.FromIntPtr(dbTask.EstimatedNumActivatedGeneratedTasks))
}
