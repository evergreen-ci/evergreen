package data

import (
	"context"
	"net/http"
	"net/url"
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
	"go.mongodb.org/mongo-driver/bson/primitive"
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
	require.NoError(t, project.Insert(t.Context()))
	projectId, statusCode, err := GetProjectIdFromParams(ctx, map[string]string{"projectId": project.Id})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	require.Equal(t, projectId, project.Id)

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"projectIdentifier": project.Identifier})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	require.Equal(t, projectId, project.Id)

	// Parameters include taskId.
	task := &task.Task{
		Id:      "task_id",
		Project: project.Identifier,
	}
	require.NoError(t, task.Insert(t.Context()))
	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"taskId": task.Id})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	require.Equal(t, projectId, project.Id)

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"taskId": "does-not-exist"})
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, statusCode)
	require.Equal(t, "", projectId)

	// Parameters include versionId.
	version := &model.Version{
		Id:         "version_id",
		Identifier: project.Identifier,
	}
	require.NoError(t, version.Insert(t.Context()))
	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"versionId": version.Id})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	require.Equal(t, projectId, project.Id)

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"versionId": "does-not-exist"})
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, statusCode)
	require.Equal(t, "", projectId)

	// Parameters include patchId.
	patchId := bson.NewObjectId()
	patch := &patch.Patch{
		Id:      patchId,
		Project: project.Identifier,
	}
	require.NoError(t, patch.Insert(t.Context()))
	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"patchId": patchId.Hex()})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	require.Equal(t, projectId, project.Id)

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"patchId": "invalid-patch-id"})
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, statusCode)
	require.Equal(t, "", projectId)

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"patchId": primitive.NewObjectID().Hex()})
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, statusCode)
	require.Equal(t, "", projectId)

	// Parameters include buildId.
	build := &build.Build{
		Id:      "build_id",
		Project: project.Identifier,
	}
	require.NoError(t, build.Insert(t.Context()))
	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"buildId": build.Id})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	require.Equal(t, projectId, project.Id)

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"buildId": "does-not-exist"})
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, statusCode)
	require.Equal(t, "", projectId)

	// Parameters include logId.
	testLog := &testlog.TestLog{
		Id:   "test_log_id",
		Task: task.Id,
		Name: "this is a test",
	}
	require.NoError(t, testLog.Insert(t.Context()))
	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"logId": testLog.Id})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	require.Equal(t, projectId, project.Id)

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"logId": "does-not-exist"})
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, statusCode)
	require.Equal(t, "", projectId)

	// Parameters include repoId.
	repo := &model.RepoRef{
		ProjectRef: model.ProjectRef{
			Id: "repo_id",
		},
	}
	require.NoError(t, repo.Replace(t.Context()))
	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"repoId": repo.ProjectRef.Id})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	require.Equal(t, projectId, repo.ProjectRef.Id)

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"repoId": "does-not-exist"})
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, statusCode)
	require.Equal(t, "", projectId)

	// Parameters are empty.
	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{})
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, statusCode)
	require.Equal(t, "", projectId)
}

func TestBuildProjectParameterMapForGraphQL(t *testing.T) {
	paramsMap, err := BuildProjectParameterMapForGraphQL(map[string]any{
		projectIdToCopyKey: "project_to_copy",
	})
	require.NoError(t, err)
	require.Equal(t, "project_to_copy", paramsMap[projectIdentifierKey])
}

func TestBuildProjectParameterMapForLegacy(t *testing.T) {
	t.Run("QueryProjectIDIgnoredWhenBuildIDInPath", func(t *testing.T) {
		query := url.Values{"project_id": []string{"attacker_project"}}
		vars := map[string]string{"build_id": "victim_build_id"}

		paramsMap := BuildProjectParameterMapForLegacy(query, vars)

		require.Empty(t, paramsMap[projectIdKey])
		require.Equal(t, "victim_build_id", paramsMap[buildIdKey])
	})

	t.Run("QueryProjectIDIgnoredWhenTaskIDInPath", func(t *testing.T) {
		query := url.Values{"project_id": []string{"attacker_project"}}
		vars := map[string]string{"task_id": "victim_task_id"}

		paramsMap := BuildProjectParameterMapForLegacy(query, vars)

		require.Empty(t, paramsMap[projectIdKey])
		require.Equal(t, "victim_task_id", paramsMap[taskIdKey])
	})

	t.Run("QueryProjectIDIgnoredWhenVersionIDInPath", func(t *testing.T) {
		query := url.Values{"project_id": []string{"attacker_project"}}
		vars := map[string]string{"version_id": "victim_version_id"}

		paramsMap := BuildProjectParameterMapForLegacy(query, vars)

		require.Empty(t, paramsMap[projectIdKey])
		require.Equal(t, "victim_version_id", paramsMap[versionIdKey])
	})

	t.Run("QueryProjectIDIgnoredWhenPatchIDInPath", func(t *testing.T) {
		query := url.Values{"project_id": []string{"attacker_project"}}
		vars := map[string]string{"patch_id": "victim_patch_id"}

		paramsMap := BuildProjectParameterMapForLegacy(query, vars)

		require.Empty(t, paramsMap[projectIdKey])
		require.Equal(t, "victim_patch_id", paramsMap[patchIdKey])
	})
}
