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

	assert.NoError(t, db.InsertMany(t.Context(), VersionCollection, v1, v2, v3, v4))

	v, err := VersionFindOne(t.Context(), VersionByMostRecentNonIgnored("proj", ts))
	assert.NoError(t, err)
	assert.Equal(t, "v1", v.Id)
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
		require.NoError(t, item.Insert(t.Context()))
	}
	for _, item := range tasks {
		require.NoError(t, item.Insert(t.Context()))
	}
	for _, item := range builds {
		require.NoError(t, item.Insert(ctx))
	}

	require.NoError(t, RestartVersion(ctx, versionID, nil, true, "caller"))

	// Check that completed non-execution tasks are reset.
	for _, taskID := range []string{"task0", "display1"} {
		tsk, err := task.FindOneId(ctx, taskID)
		require.NoError(t, err)

		assert.Equal(t, evergreen.TaskUndispatched, tsk.Status)
	}

	// Check that completed execution tasks are neither aborted nor reset.
	for _, taskID := range []string{"exec00", "exec01", "exec11"} {
		tsk, err := task.FindOneId(ctx, taskID)
		require.NoError(t, err)

		assert.Contains(t, evergreen.TaskCompletedStatuses, tsk.Status)
		assert.Empty(t, tsk.AbortInfo)
		assert.False(t, tsk.Aborted)
		assert.False(t, tsk.ResetWhenFinished)
	}

	// Check that in-progress tasks are aborted and marked for reset.
	for _, taskID := range []string{"task1", "display0", "exec00", "exec10"} {
		tsk, err := task.FindOneId(ctx, taskID)
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
	b, err := build.FindOneId(ctx, buildID)
	require.NoError(t, err)
	assert.Equal(t, evergreen.BuildStarted, b.Status)
	assert.Equal(t, "caller", b.ActivatedBy)
}

func TestFindVersionByIdFail(t *testing.T) {
	// Finding a non-existent version should fail
	assert.NoError(t, db.ClearCollections(VersionCollection))
	v, err := VersionFindOneId(t.Context(), "build3")
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
			}).Insert(t.Context()))
			author, err := GetVersionAuthorID(t.Context(), "v0")
			assert.NoError(t, err)
			assert.Equal(t, "me", author)
		},
		"NoVersion": func(t *testing.T) {
			author, err := GetVersionAuthorID(t.Context(), "v0")
			assert.Error(t, err)
			assert.Empty(t, author)
		},
		"EmptyAuthorID": func(t *testing.T) {
			assert.NoError(t, (&Version{
				Id: "v0",
			}).Insert(t.Context()))
			author, err := GetVersionAuthorID(t.Context(), "v0")
			assert.NoError(t, err)
			assert.Empty(t, author)
		},
	} {
		assert.NoError(t, db.ClearCollections(VersionCollection))
		t.Run(name, test)
	}
}

func TestFindLatestRevisionAndAuthorForProject(t *testing.T) {
	for name, test := range map[string]func(*testing.T){
		"wrongProject": func(t *testing.T) {
			assert.NoError(t, (&Version{
				Id:         "v0",
				Identifier: "project1",
				Requester:  evergreen.RepotrackerVersionRequester,
				Revision:   "abc",
			}).Insert(t.Context()))
			revision, author, err := FindLatestRevisionAndAuthorForProject(t.Context(), "project2")
			assert.Error(t, err)
			assert.Empty(t, revision)
			assert.Empty(t, author)
		},
		"rightProject": func(t *testing.T) {
			assert.NoError(t, (&Version{
				Id:                  "v0",
				Identifier:          "project1",
				Requester:           evergreen.RepotrackerVersionRequester,
				Revision:            "abc",
				Author:              "anna",
				AuthorID:            "banana",
				RevisionOrderNumber: 12,
			}).Insert(t.Context()))
			assert.NoError(t, (&Version{
				Id:                  "v1",
				Identifier:          "project1",
				Requester:           evergreen.RepotrackerVersionRequester,
				Revision:            "def",
				RevisionOrderNumber: 10,
			}).Insert(t.Context()))
			revision, author, err := FindLatestRevisionAndAuthorForProject(t.Context(), "project1")
			assert.NoError(t, err)
			assert.Equal(t, "abc", revision)
			assert.Equal(t, "banana", author)
		},
		"wrongRequester": func(t *testing.T) {
			assert.NoError(t, (&Version{
				Id:                  "v0",
				Identifier:          "project1",
				Requester:           evergreen.AdHocRequester,
				Revision:            "abc",
				RevisionOrderNumber: 12,
			}).Insert(t.Context()))
			assert.NoError(t, (&Version{
				Id:                  "v1",
				Identifier:          "project1",
				Requester:           evergreen.TriggerRequester,
				Revision:            "def",
				RevisionOrderNumber: 10,
			}).Insert(t.Context()))
			revision, author, err := FindLatestRevisionAndAuthorForProject(t.Context(), "project1")
			assert.Error(t, err)
			assert.Empty(t, revision)
			assert.Empty(t, author)
		},
		"emptyRevision": func(t *testing.T) {
			assert.NoError(t, (&Version{
				Id:         "v0",
				Identifier: "project1",
				Requester:  evergreen.RepotrackerVersionRequester,
				Revision:   "",
			}).Insert(t.Context()))
			revision, author, err := FindLatestRevisionAndAuthorForProject(t.Context(), "project1")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "has no revision")
			assert.Empty(t, revision)
			assert.Empty(t, author)
		},
		"emptyAuthor": func(t *testing.T) {
			assert.NoError(t, (&Version{
				Id:         "v0",
				Identifier: "project1",
				Requester:  evergreen.RepotrackerVersionRequester,
				Revision:   "mystery",
			}).Insert(t.Context()))
			revision, author, err := FindLatestRevisionAndAuthorForProject(t.Context(), "project1")
			require.NoError(t, err)
			assert.Equal(t, "mystery", revision)
			assert.Empty(t, author)
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

	assert.NoError(t, patch0.Insert(t.Context()))
	assert.NoError(t, patch1.Insert(t.Context()))
	assert.NoError(t, mainlineCommit1.Insert(t.Context()))
	assert.NoError(t, mainlineCommit2.Insert(t.Context()))
	// Test that it returns the base version mainline commit for a patch
	version, err := FindBaseVersionForVersion(t.Context(), "v1")
	assert.NoError(t, err)
	assert.NotNil(t, version)
	assert.Equal(t, "project1_v1", version.Id)

	// test that it returns the previous mainline commit for a mainline commit
	version, err = FindBaseVersionForVersion(t.Context(), "project1_v2")
	assert.NoError(t, err)
	assert.NotNil(t, version)
	assert.Equal(t, "project1_v1", version.Id)

	// Test that it returns an empty string if the previous version doesn't exist for a mainline commit
	version, err = FindBaseVersionForVersion(t.Context(), "project1_v1")
	assert.NoError(t, err)
	assert.Nil(t, version)

	// Test that it returns an empty string if the base version doesn't exist for a patch
	version, err = FindBaseVersionForVersion(t.Context(), "v0")
	assert.NoError(t, err)
	assert.Nil(t, version)

}

func TestVersionByProjectIdAndRevisionPrefix(t *testing.T) {
	assert.NoError(t, db.ClearCollections(VersionCollection))
	ts := time.Now()
	v1 := Version{
		Id:                  "v1",
		Identifier:          "proj",
		Requester:           evergreen.RepotrackerVersionRequester,
		CreateTime:          ts.Add(-1 * utility.Day),
		RevisionOrderNumber: 5,
	}
	v2 := Version{
		Id:                  "v2",
		Identifier:          "proj_2",
		Requester:           evergreen.RepotrackerVersionRequester,
		CreateTime:          ts.Add(-2 * utility.Day),
		RevisionOrderNumber: 4,
	}
	v3 := Version{
		Id:                  "v3",
		Identifier:          "proj",
		Requester:           evergreen.RepotrackerVersionRequester,
		CreateTime:          ts.Add(-3 * utility.Day).Add(30 * time.Minute),
		RevisionOrderNumber: 3,
	}
	v4 := Version{
		Id:                  "v4",
		Identifier:          "proj",
		Requester:           evergreen.GitTagRequester,
		CreateTime:          ts.Add(-3 * utility.Day),
		RevisionOrderNumber: 2,
	}
	v5 := Version{
		Id:                  "v5",
		Identifier:          "proj",
		Requester:           evergreen.PatchVersionRequester,
		CreateTime:          ts.Add(-5 * utility.Day),
		RevisionOrderNumber: 1,
	}

	assert.NoError(t, db.InsertMany(t.Context(), VersionCollection, v1, v2, v3, v4, v5))

	v, err := VersionFindOne(t.Context(), VersionByProjectIdAndCreateTime("proj", ts))
	assert.NoError(t, err)
	assert.Equal(t, "v1", v.Id)

	v, err = VersionFindOne(t.Context(), VersionByProjectIdAndCreateTime("proj", ts.Add(-3*utility.Day)))
	assert.NoError(t, err)
	assert.Equal(t, "v4", v.Id)

	v, err = VersionFindOne(t.Context(), VersionByProjectIdAndCreateTime("proj", ts.Add(-2*utility.Day).Add(-30*time.Minute)))
	assert.NoError(t, err)
	assert.Equal(t, "v3", v.Id)

	v, err = VersionFindOne(t.Context(), VersionByProjectIdAndCreateTime("proj", ts.Add(-2*utility.Day)))
	assert.NoError(t, err)
	assert.Equal(t, "v3", v.Id)

	v, err = VersionFindOne(t.Context(), VersionByProjectIdAndCreateTime("proj_2", ts.Add(-2*utility.Day)))
	assert.NoError(t, err)
	assert.Equal(t, "v2", v.Id)

	// Does not match on patch requester
	v, err = VersionFindOne(t.Context(), VersionByProjectIdAndCreateTime("proj", ts.Add(-5*utility.Day)))
	assert.NoError(t, err)
	assert.Nil(t, v)
}

func TestRemoveGitTagFromVersions(t *testing.T) {
	for name, run := range map[string]func(t *testing.T){
		"RemovesTagFromMultipleVersions": func(t *testing.T) {
			require.NoError(t, RemoveGitTagFromVersions(t.Context(), "owner", "repo", GitTag{Tag: "v1.0.1"}))

			v, err := VersionFindOne(t.Context(), VersionById("one"))
			require.NoError(t, err)
			require.Len(t, v.GitTags, 1)
			require.Equal(t, "v1.0.0", v.GitTags[0].Tag)

			v, err = VersionFindOne(t.Context(), VersionById("two"))
			require.NoError(t, err)
			require.Len(t, v.GitTags, 1)
			require.Equal(t, "v1.0.2", v.GitTags[0].Tag)

			v, err = VersionFindOne(t.Context(), VersionById("three"))
			require.NoError(t, err)
			require.Len(t, v.GitTags, 3)
		},
		"RemovesTagFromSingleVersion": func(t *testing.T) {
			require.NoError(t, RemoveGitTagFromVersions(t.Context(), "owner", "repo", GitTag{Tag: "v1.0.0"}))

			v, err := VersionFindOne(t.Context(), VersionById("one"))
			require.NoError(t, err)
			require.Len(t, v.GitTags, 1)
			require.Equal(t, "v1.0.1", v.GitTags[0].Tag)

			v, err = VersionFindOne(t.Context(), VersionById("two"))
			require.NoError(t, err)
			require.Len(t, v.GitTags, 2)

			v, err = VersionFindOne(t.Context(), VersionById("three"))
			require.NoError(t, err)
			require.Len(t, v.GitTags, 3)
		},
		"DoesNotRemoveAnyTagsIfNoneMatch": func(t *testing.T) {
			require.NoError(t, RemoveGitTagFromVersions(t.Context(), "owner", "repo", GitTag{Tag: "v2.0.0"}))

			v, err := VersionFindOne(t.Context(), VersionById("one"))
			require.NoError(t, err)
			require.Len(t, v.GitTags, 2)

			v, err = VersionFindOne(t.Context(), VersionById("two"))
			require.NoError(t, err)
			require.Len(t, v.GitTags, 2)

			v, err = VersionFindOne(t.Context(), VersionById("three"))
			require.NoError(t, err)
			require.Len(t, v.GitTags, 3)
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(VersionCollection))

			oneVersion := Version{
				Id:    "one",
				Owner: "owner",
				Repo:  "repo",
				GitTags: []GitTag{
					{Tag: "v1.0.0"},
					{Tag: "v1.0.1"},
				},
			}
			require.NoError(t, oneVersion.Insert(t.Context()))
			twoVersion := Version{
				Id:    "two",
				Owner: "owner",
				Repo:  "repo",
				GitTags: []GitTag{
					{Tag: "v1.0.1"},
					{Tag: "v1.0.2"},
				},
			}
			require.NoError(t, twoVersion.Insert(t.Context()))
			otherRepoVersion := Version{
				Id:    "three",
				Owner: "owner",
				Repo:  "other_repo",
				GitTags: []GitTag{
					{Tag: "v1.0.0"},
					{Tag: "v1.0.1"},
					{Tag: "v1.0.2"},
				},
			}
			require.NoError(t, otherRepoVersion.Insert(t.Context()))

			run(t)
		})
	}
}
