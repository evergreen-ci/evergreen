package graphql

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAttachProjectToNewRepoRequiresTargetRepoAdmin verifies that project admin access alone is not
// sufficient to move a project onto a repo the user does not administer, and that the same request
// succeeds once the user is an admin of that repo.
func TestAttachProjectToNewRepoRequiresTargetRepoAdmin(t *testing.T) {
	setupPermissions(t)

	usr := &user.DBUser{
		Id:          testUser,
		SystemRoles: []string{"admin_project"},
	}
	require.NoError(t, usr.Insert(t.Context()))

	originRepoRef := model.RepoRef{ProjectRef: model.ProjectRef{
		Id:    "origin_repo_id",
		Owner: "old-owner",
		Repo:  "old-repo",
	}}
	require.NoError(t, originRepoRef.Replace(t.Context()))

	targetRepoRef := model.RepoRef{ProjectRef: model.ProjectRef{
		Id:    "target_repo_id",
		Owner: "new-owner",
		Repo:  "new-repo",
	}}
	require.NoError(t, targetRepoRef.Replace(t.Context()))

	projectRef := model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
		Owner:      originRepoRef.Owner,
		Repo:       originRepoRef.Repo,
		Branch:     "main",
		RepoRefId:  originRepoRef.Id,
	}
	require.NoError(t, projectRef.Insert(t.Context()))

	payload, err := json.Marshal(map[string]any{
		"operationName": "AttachProjectToNewRepo",
		"query": `mutation AttachProjectToNewRepo($project: MoveProjectInput!) {
			attachProjectToNewRepo(project: $project) {
				id
			}
		}`,
		"variables": map[string]any{
			"project": map[string]any{
				"projectId": projectRef.Id,
				"newOwner":  targetRepoRef.Owner,
				"newRepo":   targetRepoRef.Repo,
			},
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/graphql/query", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(gimlet.AttachUser(req.Context(), usr))
	recorder := httptest.NewRecorder()
	handler.NewDefaultServer(NewExecutableSchema(New("/graphql"))).ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Errors, 1)
	assert.Contains(t, response.Errors[0].Message, "is not an admin of repo 'new-owner/new-repo'")

	projectFromDB, err := model.FindBranchProjectRef(t.Context(), projectRef.Id)
	require.NoError(t, err)
	require.NotNil(t, projectFromDB)
	assert.Equal(t, originRepoRef.Owner, projectFromDB.Owner)
	assert.Equal(t, originRepoRef.Repo, projectFromDB.Repo)
	assert.Equal(t, originRepoRef.Id, projectFromDB.RepoRefId)

	// Making the user an admin of the target repo allows the same request to succeed.
	rm := evergreen.GetEnvironment().RoleManager()
	for _, repoRefID := range []string{originRepoRef.Id, targetRepoRef.Id} {
		require.NoError(t, rm.AddScope(t.Context(), gimlet.Scope{
			ID:        model.GetRepoAdminScope(repoRefID),
			Type:      evergreen.ProjectResourceType,
			Resources: []string{repoRefID},
		}))
		require.NoError(t, rm.AddScope(t.Context(), gimlet.Scope{
			ID:        model.GetUnrestrictedBranchProjectsScope(repoRefID),
			Type:      evergreen.ProjectResourceType,
			Resources: []string{repoRefID},
		}))
	}
	require.NoError(t, rm.UpdateRole(t.Context(), gimlet.Role{
		ID:          model.GetRepoAdminRole(targetRepoRef.Id),
		Scope:       model.GetRepoAdminScope(targetRepoRef.Id),
		Permissions: gimlet.Permissions{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value},
	}))
	require.NoError(t, usr.AddRole(t.Context(), model.GetRepoAdminRole(targetRepoRef.Id)))

	// AttachToNewRepo validates the new owner against the cached environment settings.
	settings := evergreen.GetEnvironment().Settings()
	originalOrgs := settings.GithubOrgs
	settings.GithubOrgs = []string{originRepoRef.Owner, targetRepoRef.Owner}
	t.Cleanup(func() { settings.GithubOrgs = originalOrgs })

	req = httptest.NewRequest(http.MethodPost, "/graphql/query", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(gimlet.AttachUser(req.Context(), usr))
	recorder = httptest.NewRecorder()
	handler.NewDefaultServer(NewExecutableSchema(New("/graphql"))).ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	response.Errors = nil
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Empty(t, response.Errors, "response body: %s", recorder.Body.String())

	projectFromDB, err = model.FindBranchProjectRef(t.Context(), projectRef.Id)
	require.NoError(t, err)
	require.NotNil(t, projectFromDB)
	assert.Equal(t, targetRepoRef.Owner, projectFromDB.Owner)
	assert.Equal(t, targetRepoRef.Repo, projectFromDB.Repo)
	assert.Equal(t, targetRepoRef.Id, projectFromDB.RepoRefId)
}
