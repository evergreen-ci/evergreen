package task

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestGenerateTasksEstimations(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := evergreen.GetEnvironment().DB().Collection(Collection).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: TaskHistoricalDataIndex})
	assert.NoError(err)

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
	assert.NoError(t1.Insert(t.Context()))
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
	assert.NoError(t2.Insert(t.Context()))
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
	assert.NoError(t3.Insert(t.Context()))
	t4 := Task{
		GenerateTask: true,
		Id:           "t4",
		DisplayName:  displayName,
		BuildVariant: bv,
		Project:      project,
	}
	assert.NoError(t4.Insert(t.Context()))

	err = t4.SetGenerateTasksEstimations(ctx)
	assert.NoError(err)
	assert.Equal(2, utility.FromIntPtr(t4.EstimatedNumGeneratedTasks))
	assert.Equal(20, utility.FromIntPtr(t4.EstimatedNumActivatedGeneratedTasks))
}

func TestGenerateTasksEstimationsNoPreviousTasks(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := evergreen.GetEnvironment().DB().Collection(Collection).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: TaskHistoricalDataIndex})
	assert.NoError(err)

	t1 := Task{
		Id:           "t1",
		DisplayName:  "new task with no history",
		BuildVariant: "bv",
		Project:      "project",
		GenerateTask: true,
	}
	assert.NoError(t1.Insert(t.Context()))

	err = t1.SetGenerateTasksEstimations(ctx)
	assert.NoError(err)
	assert.Equal(0, utility.FromIntPtr(t1.EstimatedNumGeneratedTasks))
	assert.Equal(0, utility.FromIntPtr(t1.EstimatedNumActivatedGeneratedTasks))
}

func TestGenerateTasksEstimationsDoesNotRun(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1 := Task{
		Id:           "t1",
		DisplayName:  "not generate task",
		BuildVariant: "bv",
		Project:      "project",
		GenerateTask: false,
	}
	assert.NoError(t1.Insert(t.Context()))

	err := t1.SetGenerateTasksEstimations(ctx)
	assert.NoError(err)
	assert.Nil(t1.EstimatedNumGeneratedTasks)
	assert.Nil(t1.EstimatedNumActivatedGeneratedTasks)
}
