package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestFindUniqueBuildVariantNamesByTask(t *testing.T) {
	Convey("Should return unique build variants for tasks", t, func() {
		assert.NoError(t, db.ClearCollections(task.Collection, build.Collection))
		b1 := build.Build{
			Id:          "b1",
			DisplayName: "Ubuntu 16.04",
		}
		assert.NoError(t, b1.Insert())
		t1 := task.Task{
			Id:           "t1",
			Status:       evergreen.TaskSucceeded,
			BuildVariant: "ubuntu1604",
			DisplayName:  "test-agent",
			Project:      "evergreen",
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildId:      "b1",
		}
		assert.NoError(t, t1.Insert())
		b2 := build.Build{
			Id:          "b2",
			DisplayName: "OSX",
		}
		assert.NoError(t, b2.Insert())
		t2 := task.Task{
			Id:           "t2",
			Status:       evergreen.TaskSucceeded,
			BuildVariant: "osx",
			DisplayName:  "test-agent",
			Project:      "evergreen",
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildId:      "b2",
		}
		assert.NoError(t, t2.Insert())
		b3 := build.Build{
			Id:          "b3",
			DisplayName: "Windows 64 bit",
		}
		assert.NoError(t, b3.Insert())
		t3 := task.Task{
			Id:           "t3",
			Status:       evergreen.TaskSucceeded,
			BuildVariant: "windows",
			DisplayName:  "test-agent",
			Project:      "evergreen",
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildId:      "b3",
		}
		assert.NoError(t, t3.Insert())
		b4 := build.Build{
			Id:          "b4",
			DisplayName: "Ubuntu 16.04",
		}
		assert.NoError(t, b4.Insert())
		t4 := task.Task{
			Id:           "t4",
			Status:       evergreen.TaskFailed,
			BuildVariant: "ubuntu1604",
			DisplayName:  "test-agent",
			Project:      "evergreen",
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildId:      "b4",
		}
		assert.NoError(t, t4.Insert())
		taskBuildVariants, err := task.FindUniqueBuildVariantNamesByTask("evergreen", "test-agent")
		assert.NoError(t, err)
		assert.Equal(t, []task.BuildVariantTuple{
			{DisplayName: "OSX", BuildVariant: "osx"},
			{DisplayName: "Ubuntu 16.04", BuildVariant: "ubuntu1604"},
			{DisplayName: "Windows 64 bit", BuildVariant: "windows"},
		}, taskBuildVariants)

	})
	Convey("Should only include tasks that appear on mainline commits", t, func() {
		assert.NoError(t, db.ClearCollections(build.Collection, task.Collection))
		b1 := build.Build{
			Id:          "b1",
			DisplayName: "Ubuntu 16.04",
		}
		assert.NoError(t, b1.Insert())
		t1 := task.Task{
			Id:           "t1",
			Status:       evergreen.TaskSucceeded,
			BuildVariant: "ubuntu1604",
			DisplayName:  "test-agent",
			Project:      "evergreen",
			Requester:    evergreen.PatchVersionRequester,
			BuildId:      "b1",
		}
		assert.NoError(t, t1.Insert())
		b2 := build.Build{
			Id:          "b2",
			DisplayName: "OSX",
		}
		assert.NoError(t, b2.Insert())
		t2 := task.Task{
			Id:           "t2",
			Status:       evergreen.TaskSucceeded,
			BuildVariant: "osx",
			DisplayName:  "test-agent",
			Project:      "evergreen",
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildId:      "b2",
		}
		assert.NoError(t, t2.Insert())
		b3 := build.Build{
			Id:          "b3",
			DisplayName: "Windows 64 bit",
		}
		assert.NoError(t, b3.Insert())
		t3 := task.Task{
			Id:           "t3",
			Status:       evergreen.TaskSucceeded,
			BuildVariant: "windows",
			DisplayName:  "test-agent",
			Project:      "evergreen",
			Requester:    evergreen.PatchVersionRequester,
			BuildId:      "b3",
		}
		assert.NoError(t, t3.Insert())
		b4 := build.Build{
			Id:          "b4",
			DisplayName: "Ubuntu 16.04",
		}
		assert.NoError(t, b4.Insert())
		t4 := task.Task{
			Id:           "t4",
			Status:       evergreen.TaskFailed,
			BuildVariant: "ubuntu1604",
			DisplayName:  "test-agent",
			Project:      "evergreen",
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildId:      "b4",
		}
		assert.NoError(t, t4.Insert())
		taskBuildVariants, err := task.FindUniqueBuildVariantNamesByTask("evergreen", "test-agent")
		assert.NoError(t, err)
		assert.Equal(t, []task.BuildVariantTuple{
			{
				BuildVariant: "osx",
				DisplayName:  "OSX",
			},
			{
				BuildVariant: "ubuntu1604",
				DisplayName:  "Ubuntu 16.04",
			},
		}, taskBuildVariants)
	})

}
