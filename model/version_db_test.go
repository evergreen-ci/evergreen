package model

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
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
	versions := []*Version{
		{Id: "version3"},
	}
	tasks := []*task.Task{
		{Id: "task5", Version: "version3", Aborted: false, Status: evergreen.TaskSucceeded, BuildId: "build1"},
	}
	builds := []*build.Build{
		{Id: "build1"},
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

	versionId := "version3"
	err := RestartTasksInVersion(ctx, versionId, true, "caller3")
	assert.NoError(t, err)

	// When a version is restarted, all of its completed tasks should be reset.
	// (task.Status should be undispatched)
	t5, _ := task.FindOneId("task5")
	assert.Equal(t, versionId, t5.Version)
	assert.Equal(t, evergreen.TaskUndispatched, t5.Status)

	// Build status for all builds containing the tasks that we touched
	// should be updated.
	b1, _ := build.FindOneId("build1")
	assert.Equal(t, evergreen.BuildStarted, b1.Status)
	assert.Equal(t, "caller3", b1.ActivatedBy)
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
