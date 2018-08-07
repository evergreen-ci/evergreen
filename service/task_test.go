package service

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
)

func TestCircularDependency(t *testing.T) {
	assert := assert.New(t)
	t1 := task.Task{
		Id:          "t1",
		DisplayName: "t1",
		Activated:   true,
		Status:      evergreen.TaskSucceeded,
		DependsOn: []task.Dependency{
			{TaskId: "t2", Status: evergreen.TaskSucceeded},
		},
	}
	t2 := task.Task{
		Id:          "t2",
		DisplayName: "t2",
		Activated:   true,
		Status:      evergreen.TaskSucceeded,
		DependsOn: []task.Dependency{
			{TaskId: "t1", Status: evergreen.TaskSucceeded},
		},
	}
	uiDeps := map[string]uiDep{
		t1.Id: uiDep{
			Id:             t2.Id,
			Name:           t2.DisplayName,
			Status:         evergreen.TaskSucceeded,
			RequiredStatus: evergreen.TaskSucceeded,
			Activated:      true,
		},
		t2.Id: uiDep{
			Id:             t1.Id,
			Name:           t1.DisplayName,
			Status:         evergreen.TaskSucceeded,
			RequiredStatus: evergreen.TaskSucceeded,
			Activated:      true,
		},
	}
	idToDep := map[string]task.Task{
		t1.Id: t2,
		t2.Id: t1,
	}
	assert.NotPanics(func() {
		_, err := setBlockedOrPending(t1, idToDep, uiDeps, map[string]bool{})
		assert.EqualError(err, "circular dependency containing task t2 encountered")
	})
}
