package service

import (
	"fmt"
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

func TestGetVersionsAndVariants(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.VersionCollection, build.Collection, task.Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(model.VersionCollection, build.Collection, task.Collection))
	}()

	firstVersionID := "first_version"
	firstBuildID := "first_build"
	firstTaskID := "first_task"
	firstCreationTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	require.NoError(t, (&model.Version{
		Id:         firstVersionID,
		Requester:  evergreen.RepotrackerVersionRequester,
		CreateTime: firstCreationTime,
	}).Insert())

	require.NoError(t, (&build.Build{
		Id:          firstBuildID,
		Version:     firstVersionID,
		Tasks:       []build.TaskCache{{Id: firstTaskID}},
		DisplayName: "old-bv",
	}).Insert())

	firstTask := &task.Task{
		Id:        firstTaskID,
		Version:   firstVersionID,
		BuildId:   firstBuildID,
		Activated: true,
	}
	require.NoError(t, firstTask.Insert())

	for x := 0; x < model.MaxMainlineCommitVersionLimit; x++ {
		require.NoError(t, (&model.Version{
			Id:         fmt.Sprintf("version_%d", x),
			Requester:  evergreen.RepotrackerVersionRequester,
			CreateTime: firstCreationTime.Add(time.Duration(x) * time.Second),
		}).Insert())

		require.NoError(t, (&build.Build{
			Id:      fmt.Sprintf("build_%d", x),
			Version: fmt.Sprintf("version_%d", x),
			Tasks:   []build.TaskCache{{Id: fmt.Sprintf("task_%d", x)}},
		}).Insert())

		require.NoError(t, (&task.Task{
			Id:        fmt.Sprintf("task_%d", x),
			Version:   fmt.Sprintf("version_%d", x),
			BuildId:   fmt.Sprintf("build_%d", x),
			Activated: true,
		}).Insert())
	}

	t.Run("NoFilter", func(t *testing.T) {
		data, err := getVersionsAndVariants(0, 10, &model.Project{}, "", true)
		assert.NoError(t, err)
		assert.Len(t, data.Versions, 10)
		for _, v := range data.Versions {
			assert.False(t, v.RolledUp)
		}
	})

	t.Run("RequestOverMax", func(t *testing.T) {
		data, err := getVersionsAndVariants(0, model.MaxMainlineCommitVersionLimit+1, &model.Project{}, "", true)
		assert.NoError(t, err)
		assert.Len(t, data.Versions, model.MaxMainlineCommitVersionLimit)
		for _, v := range data.Versions {
			assert.False(t, v.RolledUp)
		}
	})

	t.Run("BVNotWithinMax", func(t *testing.T) {
		data, err := getVersionsAndVariants(0, 10, &model.Project{}, "old-bv", true)
		assert.NoError(t, err)
		require.Len(t, data.Versions, 1)
		assert.NotContains(t, data.Versions[0].Ids, "firstVersionID")
	})
}
