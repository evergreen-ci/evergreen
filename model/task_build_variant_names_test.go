package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
)

func TestFindUniqueBuildVariantNamesByTask(t *testing.T) {
	assert.NoError(t, db.ClearCollections(task.Collection, build.Collection))
	b1 := build.Build{
		Id:          "b1",
		DisplayName: "Ubuntu 16.04",
	}
	assert.NoError(t, b1.Insert())
	t1 := task.Task{
		Id:                  "t1",
		Status:              evergreen.TaskSucceeded,
		BuildVariant:        "ubuntu1604",
		DisplayName:         "test-agent",
		Project:             "evergreen",
		Requester:           evergreen.RepotrackerVersionRequester,
		BuildId:             "b1",
		CreateTime:          time.Now().Add(-time.Hour),
		RevisionOrderNumber: 1,
	}
	assert.NoError(t, t1.Insert())
	t2 := task.Task{
		Id:                      "t2",
		Status:                  evergreen.TaskSucceeded,
		BuildVariant:            "osx",
		BuildVariantDisplayName: "OSX",
		DisplayName:             "test-agent",
		Project:                 "evergreen",
		Requester:               evergreen.RepotrackerVersionRequester,
		BuildId:                 "b2",
		CreateTime:              time.Now().Add(-time.Hour),
		RevisionOrderNumber:     1,
	}
	assert.NoError(t, t2.Insert())
	t3 := task.Task{
		Id:                      "t3",
		Status:                  evergreen.TaskSucceeded,
		BuildVariant:            "windows",
		BuildVariantDisplayName: "Windows 64 bit",
		DisplayName:             "test-agent",
		Project:                 "evergreen",
		Requester:               evergreen.RepotrackerVersionRequester,
		BuildId:                 "b3",
		CreateTime:              time.Now().Add(-time.Hour),
		RevisionOrderNumber:     1,
	}
	assert.NoError(t, t3.Insert())
	b4 := build.Build{
		Id:          "b4",
		DisplayName: "Race Detector",
	}
	assert.NoError(t, b4.Insert())
	t4 := task.Task{
		Id:                      "t4",
		Status:                  evergreen.TaskSucceeded,
		BuildVariant:            "race-detector",
		BuildVariantDisplayName: "",
		DisplayName:             "test-agent",
		Project:                 "evergreen",
		Requester:               evergreen.RepotrackerVersionRequester,
		BuildId:                 "b4",
		CreateTime:              time.Now().Add(-time.Hour),
		RevisionOrderNumber:     1,
	}
	assert.NoError(t, t4.Insert())
	t5 := task.Task{
		Id:                      "t5",
		Status:                  evergreen.TaskSucceeded,
		BuildVariant:            "race-detector",
		BuildVariantDisplayName: "Race Detector",
		DisplayName:             "test-agent",
		Project:                 "evergreen",
		Requester:               evergreen.RepotrackerVersionRequester,
		BuildId:                 "b4",
		CreateTime:              time.Now().Add(-time.Hour),
		RevisionOrderNumber:     1,
	}
	assert.NoError(t, t5.Insert())
	taskBuildVariants, err := task.FindUniqueBuildVariantNamesByTask("evergreen", "test-agent", 1)
	assert.NoError(t, err)
	assert.Equal(t, []*task.BuildVariantTuple{
		{DisplayName: "OSX", BuildVariant: "osx"},
		{DisplayName: "Race Detector", BuildVariant: "race-detector"},
		{DisplayName: "Ubuntu 16.04", BuildVariant: "ubuntu1604"},
		{DisplayName: "Windows 64 bit", BuildVariant: "windows"},
	}, taskBuildVariants)
}
