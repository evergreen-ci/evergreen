package data

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/require"
)

func TestGetProjectIdFromParams(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(task.Collection, model.VersionCollection, model.ProjectRefCollection,
		model.RepoRefCollection, patch.Collection, build.Collection, testlog.TestLogCollection))
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})

	// Parameters include projectId or projectIdentifier.
	project := &model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
	}
	require.NoError(t, project.Insert())
	projectId, statusCode, err := GetProjectIdFromParams(ctx, map[string]string{"projectId": project.Id})
	require.Nil(t, err)
	require.Equal(t, statusCode, http.StatusOK)
	require.Equal(t, projectId, project.Id)

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"projectIdentifier": project.Identifier})
	require.Nil(t, err)
	require.Equal(t, statusCode, http.StatusOK)
	require.Equal(t, projectId, project.Id)

	// Parameters include taskId.
	task := &task.Task{
		Id:      "task_id",
		Project: project.Identifier,
	}
	require.NoError(t, task.Insert())
	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"taskId": task.Id})
	require.Nil(t, err)
	require.Equal(t, statusCode, http.StatusOK)
	require.Equal(t, projectId, project.Id)

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"taskId": "does-not-exist"})
	require.NotNil(t, err)
	require.Equal(t, statusCode, http.StatusNotFound)
	require.Equal(t, projectId, "")

	// Parameters include versionId.
	version := &model.Version{
		Id:         "version_id",
		Identifier: project.Identifier,
	}
	require.NoError(t, version.Insert())
	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"versionId": version.Id})
	require.Nil(t, err)
	require.Equal(t, statusCode, http.StatusOK)
	require.Equal(t, projectId, project.Id)

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"versionId": "does-not-exist"})
	require.NotNil(t, err)
	require.Equal(t, statusCode, http.StatusNotFound)
	require.Equal(t, projectId, "")

	// Parameters include patchId.
	patchId := bson.NewObjectId()
	patch := &patch.Patch{
		Id:      patchId,
		Project: project.Identifier,
	}
	require.NoError(t, patch.Insert())
	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"patchId": patchId.Hex()})
	require.Nil(t, err)
	require.Equal(t, statusCode, http.StatusOK)
	require.Equal(t, projectId, project.Id)

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"patchId": "invalid-patch-id"})
	require.NotNil(t, err)
	require.Equal(t, statusCode, http.StatusBadRequest)
	require.Equal(t, projectId, "")

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"patchId": bson.NewObjectId().Hex()})
	require.NotNil(t, err)
	require.Equal(t, statusCode, http.StatusNotFound)
	require.Equal(t, projectId, "")

	// Parameters include buildId.
	build := &build.Build{
		Id:      "build_id",
		Project: project.Identifier,
	}
	require.NoError(t, build.Insert())
	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"buildId": build.Id})
	require.Nil(t, err)
	require.Equal(t, statusCode, http.StatusOK)
	require.Equal(t, projectId, project.Id)

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"buildId": "does-not-exist"})
	require.NotNil(t, err)
	require.Equal(t, statusCode, http.StatusNotFound)
	require.Equal(t, projectId, "")

	// Parameters include logId.
	testLog := &testlog.TestLog{
		Id:   "test_log_id",
		Task: task.Id,
		Name: "this is a test",
	}
	require.NoError(t, testLog.Insert())
	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"logId": testLog.Id})
	require.Nil(t, err)
	require.Equal(t, statusCode, http.StatusOK)
	require.Equal(t, projectId, project.Id)

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"logId": "does-not-exist"})
	require.NotNil(t, err)
	require.Equal(t, statusCode, http.StatusNotFound)
	require.Equal(t, projectId, "")

	// Parameters include repoId.
	repo := &model.RepoRef{
		ProjectRef: model.ProjectRef{
			Id: "repo_id",
		},
	}
	require.NoError(t, repo.Upsert())
	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"repoId": repo.ProjectRef.Id})
	require.Nil(t, err)
	require.Equal(t, statusCode, http.StatusOK)
	require.Equal(t, projectId, repo.ProjectRef.Id)

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"repoId": "does-not-exist"})
	require.NotNil(t, err)
	require.Equal(t, statusCode, http.StatusNotFound)
	require.Equal(t, projectId, "")

	// Parameters are empty.
	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{})
	require.NotNil(t, err)
	require.Equal(t, statusCode, http.StatusNotFound)
	require.Equal(t, projectId, "")
}
