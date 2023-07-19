package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
)

func TestFindUniqueBuildVariantNamesByTask(t *testing.T) {
	assert.NoError(t, db.ClearCollections(task.Collection))
	t1 := task.Task{
		Id:                      "t1",
		Status:                  evergreen.TaskSucceeded,
		BuildVariant:            "ubuntu1604",
		BuildVariantDisplayName: "Ubuntu 16.04",
		DisplayName:             "test-agent",
		Project:                 "evergreen",
		Requester:               evergreen.RepotrackerVersionRequester,
		BuildId:                 "b1",
		CreateTime:              time.Now().Add(-time.Hour),
		RevisionOrderNumber:     1,
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
	taskBuildVariants, err := task.FindUniqueBuildVariantNamesByTask("evergreen", "test-agent", 1)
	assert.NoError(t, err)
	assert.Equal(t, []*task.BuildVariantTuple{
		{DisplayName: "OSX", BuildVariant: "osx"},
		{DisplayName: "Ubuntu 16.04", BuildVariant: "ubuntu1604"},
		{DisplayName: "Windows 64 bit", BuildVariant: "windows"},
	}, taskBuildVariants)

}
