package graphql

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectAdminOperationNameDoesNotBypassTargetAuthorization(t *testing.T) {
	setupPermissions(t)

	usr := &user.DBUser{
		Id:          testUser,
		SystemRoles: []string{"admin_project"},
	}
	require.NoError(t, usr.Insert(t.Context()))

	authorizedProject := model.ProjectRef{
		Id:         "project_id",
		Identifier: "authorized_project",
		Owner:      "authorized_owner",
		Repo:       "authorized_repo",
		Branch:     "main",
	}
	require.NoError(t, authorizedProject.Insert(t.Context()))

	unauthorizedProject := model.ProjectRef{
		Id:         "unauthorized_project_id",
		Identifier: "unauthorized_project",
		Owner:      "unauthorized_owner",
		Repo:       "unauthorized_repo",
		Branch:     "main",
	}
	require.NoError(t, unauthorizedProject.Insert(t.Context()))

	const originalRevision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	_, err := model.GetNewRevisionOrderNumber(t.Context(), unauthorizedProject.Id)
	require.NoError(t, err)
	require.NoError(t, model.UpdateLastRevision(t.Context(), unauthorizedProject.Id, originalRevision))

	srv := handler.NewDefaultServer(NewExecutableSchema(New("/graphql")))
	runMutationAndAssertUnauthorized := func(t *testing.T, query string, variables map[string]any) {
		payload, err := json.Marshal(map[string]any{
			"operationName": "CreateProject",
			"query":         query,
			"variables":     variables,
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/graphql/query", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(gimlet.AttachUser(req.Context(), usr))
		recorder := httptest.NewRecorder()
		srv.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		require.Len(t, response.Errors, 1)
		assert.Contains(t, response.Errors[0].Message, "does not have permission")
	}

	t.Run("DeleteProject", func(t *testing.T) {
		runMutationAndAssertUnauthorized(t, `mutation CreateProject($projectId: String!) {
			spoofed: deleteProject(projectId: $projectId)
		}`, map[string]any{"projectId": unauthorizedProject.Identifier})

		project, err := model.FindBranchProjectRef(t.Context(), unauthorizedProject.Id)
		require.NoError(t, err)
		require.NotNil(t, project)
		assert.False(t, project.IsHidden())
		assert.Equal(t, unauthorizedProject.Owner, project.Owner)
	})

	t.Run("CopyProject", func(t *testing.T) {
		const copiedProjectIdentifier = "copied_unauthorized_project"
		runMutationAndAssertUnauthorized(t, `mutation CreateProject($project: CopyProjectInput!) {
			spoofed: copyProject(project: $project) {
				id
			}
		}`, map[string]any{
			"project": map[string]any{
				"newProjectIdentifier": copiedProjectIdentifier,
				"projectIdToCopy":      unauthorizedProject.Identifier,
			},
		})

		project, err := model.FindBranchProjectRef(t.Context(), copiedProjectIdentifier)
		require.NoError(t, err)
		assert.Nil(t, project)
	})

	t.Run("SetLastRevision", func(t *testing.T) {
		runMutationAndAssertUnauthorized(t, `mutation CreateProject($projectIdentifier: String!, $revision: String!) {
			spoofed: setLastRevision(opts: {
				projectIdentifier: $projectIdentifier,
				revision: $revision
			}) {
				mergeBaseRevision
			}
		}`, map[string]any{
			"projectIdentifier": unauthorizedProject.Identifier,
			"revision":          "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		})

		repository, err := model.FindRepository(t.Context(), unauthorizedProject.Id)
		require.NoError(t, err)
		require.NotNil(t, repository)
		assert.Equal(t, originalRevision, repository.LastRevision)
	})

	t.Run("TargetAdminCanUseArbitraryOperationName", func(t *testing.T) {
		payload, err := json.Marshal(map[string]any{
			"operationName": "ArbitraryOperation",
			"query": `mutation ArbitraryOperation($projectId: String!) {
				aliasedDelete: deleteProject(projectId: $projectId)
			}`,
			"variables": map[string]any{"projectId": authorizedProject.Identifier},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/graphql/query", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(gimlet.AttachUser(req.Context(), usr))
		recorder := httptest.NewRecorder()
		srv.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Data struct {
				AliasedDelete bool `json:"aliasedDelete"`
			} `json:"data"`
			Errors []any `json:"errors"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		assert.Empty(t, response.Errors)
		assert.True(t, response.Data.AliasedDelete)

		project, err := model.FindBranchProjectRef(t.Context(), authorizedProject.Id)
		require.NoError(t, err)
		require.NotNil(t, project)
		assert.True(t, project.IsHidden())
	})
}
