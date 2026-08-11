package model

import (
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModifyVersionRestart(t *testing.T) {
	ctx := t.Context()
	require.NoError(t, db.ClearCollections(VersionCollection, task.Collection, build.Collection, task.OldCollection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(VersionCollection, task.Collection, build.Collection, task.OldCollection))
	})

	mergeQueueVersion := &Version{
		Id:        "merge-queue-version",
		Requester: evergreen.GithubMergeRequester,
	}
	require.NoError(t, mergeQueueVersion.Insert(ctx))

	patchVersion := &Version{
		Id:        "patch-version",
		Requester: evergreen.PatchVersionRequester,
	}
	require.NoError(t, patchVersion.Insert(ctx))

	u := user.DBUser{Id: "user"}

	t.Run("RestartRejectsTopLevelMergeQueueVersion", func(t *testing.T) {
		statusCode, err := ModifyVersion(ctx, *mergeQueueVersion, u, VersionModification{
			Action: evergreen.RestartAction,
			VersionsToRestart: []*VersionToRestart{
				{VersionId: utility.ToStringPtr(mergeQueueVersion.Id)},
			},
		})
		assert.Error(t, err)
		assert.Equal(t, http.StatusBadRequest, statusCode)
		assert.Contains(t, err.Error(), "merge queue patches cannot be manually restarted")
	})

	t.Run("RestartRejectsMergeQueueVersionInList", func(t *testing.T) {
		statusCode, err := ModifyVersion(ctx, *patchVersion, u, VersionModification{
			Action: evergreen.RestartAction,
			VersionsToRestart: []*VersionToRestart{
				{VersionId: utility.ToStringPtr(patchVersion.Id)},
				{VersionId: utility.ToStringPtr(mergeQueueVersion.Id)},
			},
		})
		assert.Error(t, err)
		assert.Equal(t, http.StatusBadRequest, statusCode)
		assert.Contains(t, err.Error(), "merge queue patches cannot be manually restarted")
	})

	t.Run("RestartAllowsNonMergeQueueVersions", func(t *testing.T) {
		statusCode, err := ModifyVersion(ctx, *patchVersion, u, VersionModification{
			Action: evergreen.RestartAction,
			VersionsToRestart: []*VersionToRestart{
				{VersionId: utility.ToStringPtr(patchVersion.Id)},
			},
		})
		assert.NoError(t, err)
		assert.Zero(t, statusCode)
	})
}

func TestModifyVersionSetActive(t *testing.T) {
	ctx := t.Context()
	require.NoError(t, db.ClearCollections(VersionCollection, task.Collection, build.Collection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(VersionCollection, task.Collection, build.Collection))
	})

	u := user.DBUser{Id: "user"}

	t.Run("SetActiveRejectsMergeQueueVersion", func(t *testing.T) {
		v := &Version{
			Id:        "merge-queue-version",
			Requester: evergreen.GithubMergeRequester,
		}
		require.NoError(t, v.Insert(ctx))
		statusCode, err := ModifyVersion(ctx, *v, u, VersionModification{
			Action: evergreen.SetActiveAction,
			Active: true,
		})
		assert.Error(t, err)
		assert.Equal(t, http.StatusBadRequest, statusCode)
		assert.Contains(t, err.Error(), "merge queue patches cannot be manually scheduled")
	})

	t.Run("SetActiveAllowsNonMergeQueueVersion", func(t *testing.T) {
		v := &Version{
			Id:        "patch-version",
			Requester: evergreen.PatchVersionRequester,
		}
		require.NoError(t, v.Insert(ctx))
		statusCode, err := ModifyVersion(ctx, *v, u, VersionModification{
			Action: evergreen.SetActiveAction,
			Active: true,
		})
		assert.NoError(t, err)
		assert.Zero(t, statusCode)
	})
}

func TestModifyVersionSetPriority(t *testing.T) {
	ctx := t.Context()
	require.NoError(t, db.ClearCollections(VersionCollection, task.Collection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(VersionCollection, task.Collection))
	})

	u := user.DBUser{Id: "user"}

	t.Run("SetPriorityRequiresProjectIdentifier", func(t *testing.T) {
		v := &Version{
			Id:        "version-no-project",
			Requester: evergreen.PatchVersionRequester,
		}
		require.NoError(t, v.Insert(ctx))
		statusCode, err := ModifyVersion(ctx, *v, u, VersionModification{
			Action:   evergreen.SetPriorityAction,
			Priority: 5,
		})
		assert.Error(t, err)
		assert.Equal(t, http.StatusNotFound, statusCode)
	})

	t.Run("SetPrioritySucceeds", func(t *testing.T) {
		v := &Version{
			Id:         "version-with-project",
			Requester:  evergreen.PatchVersionRequester,
			Identifier: "proj",
		}
		require.NoError(t, v.Insert(ctx))
		statusCode, err := ModifyVersion(ctx, *v, u, VersionModification{
			Action:   evergreen.SetPriorityAction,
			Priority: 5,
		})
		assert.NoError(t, err)
		assert.Zero(t, statusCode)
	})
}

func TestModifyVersionUnrecognizedAction(t *testing.T) {
	v := Version{Id: "v1"}
	u := user.DBUser{Id: "user"}
	statusCode, err := ModifyVersion(t.Context(), v, u, VersionModification{
		Action: "invalid",
	})
	assert.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)
	assert.Contains(t, err.Error(), "unrecognized action")
}
