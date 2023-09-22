package model

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionByMostRecentNonIgnored(t *testing.T) {
	assert.NoError(t, db.ClearCollections(VersionCollection))
	ts := time.Now()
	v1 := Version{
		Id:         "v1",
		Identifier: "proj",
		Requester:  evergreen.RepotrackerVersionRequester,
		CreateTime: ts.Add(-1 * time.Second),
	}
	v2 := Version{
		Id:         "v2",
		Identifier: "proj",
		Requester:  evergreen.RepotrackerVersionRequester,
		CreateTime: ts.Add(-2 * time.Minute), // created too early
	}
	v3 := Version{
		Id:         "v3",
		Identifier: "proj",
		Requester:  evergreen.RepotrackerVersionRequester,
		CreateTime: ts.Add(time.Minute), // created too late
	}
	// Should not get picked even though it is the most recent
	v4 := Version{
		Id:         "v4",
		Identifier: "proj",
		Requester:  evergreen.AdHocRequester,
		CreateTime: ts,
	}

	assert.NoError(t, db.InsertMany(VersionCollection, v1, v2, v3, v4))

	v, err := VersionFindOne(VersionByMostRecentNonIgnored("proj", ts))
	assert.NoError(t, err)
	assert.Equal(t, v.Id, "v1")
}

func TestRestartVersion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Insert data for the test paths
	assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection, task.OldCollection))
	defer func() {
		assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection, task.OldCollection))
	}()

	versionID := "version0"
	versions := []*Version{
		{Id: versionID},
	}
	buildID := "build0"
	builds := []*build.Build{
		{Id: buildID},
	}
	tasks := []*task.Task{
		{Id: "task0", Version: versionID, DisplayTaskId: utility.ToStringPtr(""), Aborted: false, Status: evergreen.TaskSucceeded, BuildId: buildID},
		{Id: "task1", Version: versionID, DisplayTaskId: utility.ToStringPtr(""), Aborted: false, Status: evergreen.TaskDispatched, BuildId: buildID},
		{Id: "display0", Version: versionID, DisplayTaskId: utility.ToStringPtr(""), Aborted: false, Status: evergreen.TaskStarted, BuildId: buildID},
		{Id: "exec00", Version: versionID, DisplayTaskId: utility.ToStringPtr("display0"), Aborted: false, Status: evergreen.TaskSucceeded, BuildId: buildID},
		{Id: "exec10", Version: versionID, DisplayTaskId: utility.ToStringPtr("display0"), Aborted: false, Status: evergreen.TaskStarted, BuildId: buildID},
		{Id: "display1", Version: versionID, DisplayTaskId: utility.ToStringPtr(""), Aborted: false, Status: evergreen.TaskFailed, BuildId: buildID},
		{Id: "exec01", Version: versionID, DisplayTaskId: utility.ToStringPtr("display1"), Aborted: false, Status: evergreen.TaskSucceeded, BuildId: buildID},
		{Id: "exec11", Version: versionID, DisplayTaskId: utility.ToStringPtr("display1"), Aborted: false, Status: evergreen.TaskFailed, BuildId: buildID},
	}
	for _, item := range versions {
		require.NoError(t, item.Insert())
	}
	for _, item := range tasks {
		require.NoError(t, item.Insert())
	}
	for _, item := range builds {
		require.NoError(t, item.Insert())
	}

	require.NoError(t, RestartVersion(ctx, versionID, nil, true, "caller"))

	// Check that completed non-execution tasks are reset.
	for _, taskID := range []string{"task0", "display1"} {
		tsk, err := task.FindOneId(taskID)
		require.NoError(t, err)

		assert.Equal(t, evergreen.TaskUndispatched, tsk.Status)
	}

	// Check that completed execution tasks are neither aborted nor reset.
	for _, taskID := range []string{"exec00", "exec01", "exec11"} {
		tsk, err := task.FindOneId(taskID)
		require.NoError(t, err)

		assert.Contains(t, evergreen.TaskCompletedStatuses, tsk.Status)
		assert.Empty(t, tsk.AbortInfo)
		assert.False(t, tsk.Aborted)
		assert.False(t, tsk.ResetWhenFinished)
	}

	// Check that in-progress tasks are aborted and marked for reset.
	for _, taskID := range []string{"task1", "display0", "exec00", "exec10"} {
		tsk, err := task.FindOneId(taskID)
		require.NoError(t, err)

		if !utility.StringSliceContains(evergreen.TaskCompletedStatuses, tsk.Status) {
			assert.Equal(t, task.AbortInfo{User: "caller"}, tsk.AbortInfo, taskID)
			assert.True(t, tsk.Aborted)
			assert.True(t, tsk.ResetWhenFinished)
		} else {
			// Check that completed execution tasks that are part
			// of an in-progress display task are not aborted or
			// reset.
			assert.Empty(t, tsk.AbortInfo)
			assert.False(t, tsk.Aborted)
			assert.False(t, tsk.ResetWhenFinished)
		}
	}

	// Build status for all builds containing the tasks that we touched
	// should be updated.
	b, err := build.FindOneId(buildID)
	require.NoError(t, err)
	assert.Equal(t, evergreen.BuildStarted, b.Status)
	assert.Equal(t, "caller", b.ActivatedBy)
}

func TestFindVersionByIdFail(t *testing.T) {
	// Finding a non-existent version should fail
	assert.NoError(t, db.ClearCollections(VersionCollection))
	v, err := VersionFindOneId("build3")
	assert.NoError(t, err)
	assert.Nil(t, v)
}

func TestGetVersionAuthorID(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(VersionCollection))
	}()

	for name, test := range map[string]func(*testing.T){
		"HasAuthorID": func(t *testing.T) {
			assert.NoError(t, (&Version{
				Id:       "v0",
				AuthorID: "me",
			}).Insert())
			author, err := GetVersionAuthorID("v0")
			assert.NoError(t, err)
			assert.Equal(t, "me", author)
		},
		"NoVersion": func(t *testing.T) {
			author, err := GetVersionAuthorID("v0")
			assert.Error(t, err)
			assert.Empty(t, author)
		},
		"EmptyAuthorID": func(t *testing.T) {
			assert.NoError(t, (&Version{
				Id: "v0",
			}).Insert())
			author, err := GetVersionAuthorID("v0")
			assert.NoError(t, err)
			assert.Empty(t, author)
		},
	} {
		assert.NoError(t, db.ClearCollections(VersionCollection))
		t.Run(name, test)
	}
}

func TestFindLatestRevisionForProject(t *testing.T) {
	for name, test := range map[string]func(*testing.T){
		"wrongProject": func(t *testing.T) {
			assert.NoError(t, (&Version{
				Id:         "v0",
				Identifier: "project1",
				Requester:  evergreen.RepotrackerVersionRequester,
				Revision:   "abc",
			}).Insert())
			revision, err := FindLatestRevisionForProject("project2")
			assert.Error(t, err)
			assert.Equal(t, "", revision)
		},
		"rightProject": func(t *testing.T) {
			assert.NoError(t, (&Version{
				Id:                  "v0",
				Identifier:          "project1",
				Requester:           evergreen.RepotrackerVersionRequester,
				Revision:            "abc",
				RevisionOrderNumber: 12,
			}).Insert())
			assert.NoError(t, (&Version{
				Id:                  "v1",
				Identifier:          "project1",
				Requester:           evergreen.RepotrackerVersionRequester,
				Revision:            "def",
				RevisionOrderNumber: 10,
			}).Insert())
			revision, err := FindLatestRevisionForProject("project1")
			assert.NoError(t, err)
			assert.Equal(t, "abc", revision)
		},
		"wrongRequester": func(t *testing.T) {
			assert.NoError(t, (&Version{
				Id:                  "v0",
				Identifier:          "project1",
				Requester:           evergreen.AdHocRequester,
				Revision:            "abc",
				RevisionOrderNumber: 12,
			}).Insert())
			assert.NoError(t, (&Version{
				Id:                  "v1",
				Identifier:          "project1",
				Requester:           evergreen.TriggerRequester,
				Revision:            "def",
				RevisionOrderNumber: 10,
			}).Insert())
			revision, err := FindLatestRevisionForProject("project1")
			assert.Error(t, err)
			assert.Equal(t, "", revision)
		},
		"emptyRevision": func(t *testing.T) {
			assert.NoError(t, (&Version{
				Id:         "v0",
				Identifier: "project1",
				Requester:  evergreen.RepotrackerVersionRequester,
				Revision:   "",
			}).Insert())
			revision, err := FindLatestRevisionForProject("project1")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "has no revision")
			assert.Equal(t, "", revision)
		},
	} {
		assert.NoError(t, db.ClearCollections(VersionCollection))
		t.Run(name, test)
	}
}

func TestFindBaseVersionForVersion(t *testing.T) {
	assert.NoError(t, db.ClearCollections(VersionCollection))

	patch0 := Version{
		Id:                  "v0",
		Identifier:          "project1",
		Requester:           evergreen.PatchVersionRequester,
		Revision:            "ghi",
		RevisionOrderNumber: 1,
	}
	patch1 := Version{
		Id:                  "v1",
		Identifier:          "project1",
		Requester:           evergreen.PatchVersionRequester,
		Revision:            "abc",
		RevisionOrderNumber: 2,
	}
	mainlineCommit1 := Version{
		Id:                  "project1_v1",
		Identifier:          "project1",
		Requester:           evergreen.RepotrackerVersionRequester,
		Revision:            "abc",
		RevisionOrderNumber: 1,
	}
	mainlineCommit2 := Version{
		Id:                  "project1_v2",
		Identifier:          "project1",
		Requester:           evergreen.RepotrackerVersionRequester,
		Revision:            "def",
		RevisionOrderNumber: 2,
	}

	assert.NoError(t, patch0.Insert())
	assert.NoError(t, patch1.Insert())
	assert.NoError(t, mainlineCommit1.Insert())
	assert.NoError(t, mainlineCommit2.Insert())
	// Test that it returns the base version mainline commit for a patch
	version, err := FindBaseVersionForVersion("v1")
	assert.NoError(t, err)
	assert.NotNil(t, version)
	assert.Equal(t, "project1_v1", version.Id)

	// test that it returns the previous mainline commit for a mainline commit
	version, err = FindBaseVersionForVersion("project1_v2")
	assert.NoError(t, err)
	assert.NotNil(t, version)
	assert.Equal(t, "project1_v1", version.Id)

	// Test that it returns an empty string if the previous version doesn't exist for a mainline commit
	version, err = FindBaseVersionForVersion("project1_v1")
	assert.NoError(t, err)
	assert.Nil(t, version)

	// Test that it returns an empty string if the base version doesn't exist for a patch
	version, err = FindBaseVersionForVersion("v0")
	assert.NoError(t, err)
	assert.Nil(t, version)

}
