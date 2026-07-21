package data

import (
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestGetProjectIdFromParams(t *testing.T) {
	ctx := t.Context()

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

	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{"projectId": project.Id, "repoId": repo.ProjectRef.Id})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	require.Equal(t, project.Id, projectId, "project ID should take precedence over repo ID")

	// Parameters are empty.
	projectId, statusCode, err = GetProjectIdFromParams(ctx, map[string]string{})
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, statusCode)
	require.Equal(t, "", projectId)
}

func TestBuildProjectParameterMapForLegacy(t *testing.T) {
	// When an object ID is in the path, the query string must not be able to
	// override any of the resolved IDs, since the route handler acts on the
	// path-bound object. When no object ID is in the path, query-string IDs are
	// honored. Each expected entry asserts a single resolved param.
	for name, tc := range map[string]struct {
		query    url.Values
		vars     map[string]string
		expected map[string]string
	}{
		"QueryProjectIDIgnoredWhenBuildIDInPath": {
			query:    url.Values{"project_id": []string{"unrelated_project"}},
			vars:     map[string]string{"build_id": "target_build_id"},
			expected: map[string]string{projectIdKey: "", buildIdKey: "target_build_id"},
		},
		"QueryProjectIDIgnoredWhenTaskIDInPath": {
			query:    url.Values{"project_id": []string{"unrelated_project"}},
			vars:     map[string]string{"task_id": "target_task_id"},
			expected: map[string]string{projectIdKey: "", taskIdKey: "target_task_id"},
		},
		"QueryProjectIDIgnoredWhenVersionIDInPath": {
			query:    url.Values{"project_id": []string{"unrelated_project"}},
			vars:     map[string]string{"version_id": "target_version_id"},
			expected: map[string]string{projectIdKey: "", versionIdKey: "target_version_id"},
		},
		"QueryProjectIDIgnoredWhenPatchIDInPath": {
			query:    url.Values{"project_id": []string{"unrelated_project"}},
			vars:     map[string]string{"patch_id": "target_patch_id"},
			expected: map[string]string{projectIdKey: "", patchIdKey: "target_patch_id"},
		},
		"QueryVersionIDIgnoredWhenTaskIDInPath": {
			query:    url.Values{"version_id": []string{"unrelated_version_id"}},
			vars:     map[string]string{"task_id": "target_task_id"},
			expected: map[string]string{versionIdKey: "", taskIdKey: "target_task_id"},
		},
		"QueryPatchIDIgnoredWhenTaskIDInPath": {
			query:    url.Values{"patch_id": []string{"unrelated_patch_id"}},
			vars:     map[string]string{"task_id": "target_task_id"},
			expected: map[string]string{patchIdKey: "", taskIdKey: "target_task_id"},
		},
		"QueryBuildIDIgnoredWhenTaskIDInPath": {
			query:    url.Values{"build_id": []string{"unrelated_build_id"}},
			vars:     map[string]string{"task_id": "target_task_id"},
			expected: map[string]string{buildIdKey: "", taskIdKey: "target_task_id"},
		},
		"QueryTaskIDIgnoredWhenPatchIDInPath": {
			query:    url.Values{"task_id": []string{"unrelated_task_id"}},
			vars:     map[string]string{"patch_id": "target_patch_id"},
			expected: map[string]string{taskIdKey: "", patchIdKey: "target_patch_id"},
		},
		"QueryLogIDIgnoredWhenTaskIDInPath": {
			query:    url.Values{"log_id": []string{"unrelated_log_id"}},
			vars:     map[string]string{"task_id": "target_task_id"},
			expected: map[string]string{logIdKey: "", taskIdKey: "target_task_id"},
		},
		"QueryProjectIDIgnoredWhenProjectIDInPath": {
			query:    url.Values{"project_id": []string{"unrelated_project"}},
			vars:     map[string]string{"project_id": "target_project"},
			expected: map[string]string{projectIdKey: "target_project"},
		},
		"QueryRepoIDIgnoredWhenTaskIDInPath": {
			query:    url.Values{"repo_id": []string{"unrelated_repo_id"}},
			vars:     map[string]string{"task_id": "target_task_id"},
			expected: map[string]string{repoIdKey: "", taskIdKey: "target_task_id"},
		},
		"QueryRepoIDIgnoredWhenRepoIDInPath": {
			query:    url.Values{"repo_id": []string{"unrelated_repo_id"}},
			vars:     map[string]string{"repo_id": "target_repo_id"},
			expected: map[string]string{repoIdKey: "target_repo_id"},
		},
		"QueryObjectIDsUsedWhenNoObjectIDInPath": {
			query:    url.Values{"version_id": []string{"query_version_id"}},
			vars:     map[string]string{},
			expected: map[string]string{versionIdKey: "query_version_id"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			paramsMap := BuildProjectParameterMapForLegacy(tc.query, tc.vars)
			for key, expectedValue := range tc.expected {
				assert.Equal(t, expectedValue, paramsMap[key])
			}
		})
	}
}
