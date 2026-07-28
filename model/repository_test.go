package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_           fmt.Stringer = nil
	projectName              = "mongodb-mongo-testing"
)

func TestGetNewRevisionOrderNumber(t *testing.T) {
	Convey("When requesting a new commit order number...", t, func() {

		Convey("The returned commit order number should be 1 for a new"+
			" project", func() {
			ron, err := GetNewRevisionOrderNumber(t.Context(), projectName)
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 1)
		})

		Convey("The returned commit order number should be 1 for monotonically"+
			" incremental on a new project", func() {
			ron, err := GetNewRevisionOrderNumber(t.Context(), projectName)
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 1)
			ron, err = GetNewRevisionOrderNumber(t.Context(), projectName)
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 2)
		})

		Convey("The returned commit order number should be 1 for monotonically"+
			" incremental within (but not across) projects", func() {
			ron, err := GetNewRevisionOrderNumber(t.Context(), projectName)
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 1)
			ron, err = GetNewRevisionOrderNumber(t.Context(), projectName)
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 2)
			ron, err = GetNewRevisionOrderNumber(t.Context(), projectName+"-12")
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 1)
			ron, err = GetNewRevisionOrderNumber(t.Context(), projectName+"-12")
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 2)
		})

		Reset(func() {
			So(db.Clear(RepositoriesCollection), ShouldBeNil)
		})

	})
}

func TestUpdateLastRevision(t *testing.T) {
	for name, test := range map[string]func(*testing.T, string, string){
		"InvalidProject": func(t *testing.T, project string, revision string) {
			assert.Error(t, UpdateLastRevision(t.Context(), project, revision))
		},
		"ValidProject": func(t *testing.T, project string, revision string) {
			_, err := GetNewRevisionOrderNumber(t.Context(), project)
			assert.NoError(t, err)
			assert.NoError(t, UpdateLastRevision(t.Context(), project, revision))
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.Clear(RepositoriesCollection))
			defer func() {
				assert.NoError(t, db.Clear(RepositoriesCollection))
			}()
			test(t, "my-project", "my-revision")
		})
	}
}

func TestFindLatestRepositoryRevisionByIngestTime(t *testing.T) {
	require.NoError(t, db.ClearCollections(RepositoriesCollection, RepositoryRevisionsHistoryCollection))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(RepositoriesCollection, RepositoryRevisionsHistoryCollection))
	})

	ingestTime := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	require.NoError(t, UpsertRepositoryRevision(t.Context(), "owner", "repo", "branch", "r1", ingestTime))
	require.NoError(t, UpsertRepositoryRevision(t.Context(), "owner", "repo", "branch", "r2", ingestTime.Add(time.Minute)))
	require.NoError(t, UpsertRepositoryRevision(t.Context(), "owner", "repo", "branch", "r3", ingestTime.Add(3*time.Minute)))
	require.NoError(t, UpsertRepositoryRevision(t.Context(), "other", "other-repo", "other-branch", "other-r1", ingestTime.Add(2*time.Minute)))
	require.NoError(t, UpsertRepositoryRevision(t.Context(), "owner", "repo", "branch", "r2", ingestTime.Add(4*time.Minute)))

	t.Run("ReturnsRevisionIngestedAtVersionTime", func(t *testing.T) {
		revision, err := FindLatestRepositoryRevisionByIngestTime(t.Context(), "owner", "repo", "branch", ingestTime.Add(2*time.Minute))
		require.NoError(t, err)
		require.NotNil(t, revision)
		assert.Equal(t, "r2", revision.Revision)
		assert.Equal(t, ingestTime.Add(time.Minute), revision.IngestTime)
	})

	t.Run("ReturnsLatestRevisionAfterLaterIngest", func(t *testing.T) {
		revision, err := FindLatestRepositoryRevisionByIngestTime(t.Context(), "owner", "repo", "branch", ingestTime.Add(4*time.Minute))
		require.NoError(t, err)
		require.NotNil(t, revision)
		assert.Equal(t, "r3", revision.Revision)
		assert.Equal(t, ingestTime.Add(3*time.Minute), revision.IngestTime)
	})

	t.Run("ReturnsNilBeforeFirstIngest", func(t *testing.T) {
		revision, err := FindLatestRepositoryRevisionByIngestTime(t.Context(), "owner", "repo", "branch", ingestTime.Add(-time.Minute))
		require.NoError(t, err)
		assert.Nil(t, revision)
	})
}
