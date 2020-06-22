package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixPatchInHistory(t *testing.T) {
	require.NoError(t, db.Clear(TaskJSONCollection))

	// no JSON exists for this task
	history := []TaskJSON{
		{TaskId: "t0"},
		{TaskId: "t1"},
		{TaskId: "t2"},
	}
	newHistory, err := fixPatchInHistory(&task.Task{Id: "t1_patch"}, &Version{RevisionOrderNumber: 1, Revision: "abc"}, history)
	assert.NoError(t, err)
	// returned as is
	assert.Equal(t, history, newHistory)

	// matching revision found
	patchJSON := TaskJSON{TaskId: "t1_patch", RevisionOrderNumber: 12}
	require.NoError(t, db.Insert(TaskJSONCollection, patchJSON))
	history = []TaskJSON{
		{TaskId: "t0"},
		{TaskId: "t1", Revision: "abc"},
		{TaskId: "t2"},
	}
	newHistory, err = fixPatchInHistory(&task.Task{Id: "t1_patch"}, &Version{RevisionOrderNumber: 1, Revision: "abc"}, history)
	assert.NoError(t, err)
	// replaced in the middle
	assert.Len(t, newHistory, 3)
	assert.Equal(t, "t1_patch", newHistory[1].TaskId)
	assert.Equal(t, 1, newHistory[1].RevisionOrderNumber)

	// no matching revision found
	history = []TaskJSON{
		{TaskId: "t0"},
		{TaskId: "t2"},
	}
	newHistory, err = fixPatchInHistory(&task.Task{Id: "t1_patch"}, &Version{RevisionOrderNumber: 1, Revision: "abc"}, history)
	assert.NoError(t, err)
	// appended to the end
	assert.Len(t, newHistory, 3)
	assert.Equal(t, "t1_patch", newHistory[2].TaskId)
}

func TestGetTaskJSONHistory(t *testing.T) {
	require.NoError(t, db.ClearCollections(TaskJSONCollection, VersionCollection))
	baseVersion := &Version{
		Revision:            "abc",
		Identifier:          "evergreen",
		RevisionOrderNumber: 1,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	require.NoError(t, baseVersion.Insert())

	taskJSON := []TaskJSON{
		{TaskId: "t0", RevisionOrderNumber: 0, ProjectId: "evergreen"},
		{TaskId: "t1", RevisionOrderNumber: 1, Revision: "abc", ProjectId: "evergreen"},
		{TaskId: "t1_patch", RevisionOrderNumber: 12, ProjectId: "evergreen", IsPatch: true},
		{TaskId: "t2", RevisionOrderNumber: 2, ProjectId: "evergreen"},
	}
	for _, json := range taskJSON {
		require.NoError(t, db.Insert(TaskJSONCollection, json))
	}

	patchTask := &task.Task{
		Id:                  "t1_patch",
		Requester:           evergreen.PatchVersionRequester,
		Revision:            "abc",
		RevisionOrderNumber: 123,
		Project:             "evergreen",
	}

	history, err := GetTaskJSONHistory(patchTask, "")
	assert.NoError(t, err)

	assert.Len(t, history, 3)
	for i, json := range history {
		assert.Equal(t, i, json.RevisionOrderNumber)
	}
	assert.Equal(t, "t1_patch", history[1].TaskId)
}
