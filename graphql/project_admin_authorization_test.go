package graphql

import (
	"bytes"
	"context"
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

	administeredProject := model.ProjectRef{
		Id:         "project_id",
		Identifier: "administered_project",
		Owner:      "administered_owner",
		Repo:       "administered_repo",
		Branch:     "main",
	}
	require.NoError(t, administeredProject.Insert(t.Context()))

	victimProject := model.ProjectRef{
		Id:         "victim_project_id",
		Identifier: "victim_project",
		Owner:      "victim_owner",
		Repo:       "victim_repo",
		Branch:     "main",
	}
	require.NoError(t, victimProject.Insert(t.Context()))

	const originalRevision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	_, err := model.GetNewRevisionOrderNumber(t.Context(), victimProject.Id)
	require.NoError(t, err)
	require.NoError(t, model.UpdateLastRevision(t.Context(), victimProject.Id, originalRevision))

	srv := handler.NewDefaultServer(NewExecutableSchema(New("/graphql")))
	runMutation := func(t *testing.T, query string, variables map[string]any) {
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
		runMutation(t, `mutation CreateProject($projectId: String!) {
			spoofed: deleteProject(projectId: $projectId)
		}`, map[string]any{"projectId": victimProject.Identifier})

		project, err := model.FindBranchProjectRef(t.Context(), victimProject.Id)
		require.NoError(t, err)
		require.NotNil(t, project)
		assert.False(t, project.IsHidden())
		assert.Equal(t, victimProject.Owner, project.Owner)
	})

	t.Run("CopyProject", func(t *testing.T) {
		const copiedProjectIdentifier = "copied_victim_project"
		runMutation(t, `mutation CreateProject($project: CopyProjectInput!) {
			spoofed: copyProject(project: $project) {
				id
			}
		}`, map[string]any{
			"project": map[string]any{
				"newProjectIdentifier": copiedProjectIdentifier,
				"projectIdToCopy":      victimProject.Identifier,
			},
		})

		project, err := model.FindBranchProjectRef(t.Context(), copiedProjectIdentifier)
		require.NoError(t, err)
		assert.Nil(t, project)
	})

	t.Run("SetLastRevision", func(t *testing.T) {
		runMutation(t, `mutation CreateProject($projectIdentifier: String!, $revision: String!) {
			spoofed: setLastRevision(opts: {
				projectIdentifier: $projectIdentifier,
				revision: $revision
			}) {
				mergeBaseRevision
			}
		}`, map[string]any{
			"projectIdentifier": victimProject.Identifier,
			"revision":          "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		})

		repository, err := model.FindRepository(t.Context(), victimProject.Id)
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
			"variables": map[string]any{"projectId": administeredProject.Identifier},
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

		project, err := model.FindBranchProjectRef(t.Context(), administeredProject.Id)
		require.NoError(t, err)
		require.NotNil(t, project)
		assert.True(t, project.IsHidden())
	})
}

func TestRequireProjectAccessRejectsProjectCreatePermissionWithoutProjectSettingsPermission(t *testing.T) {
	setupPermissions(t)

	usr := &user.DBUser{
		Id:          testUser,
		SystemRoles: []string{"superuser"},
	}
	require.NoError(t, usr.Insert(t.Context()))

	project := model.ProjectRef{
		Id:         "victim_project_id",
		Identifier: "victim_project",
	}
	require.NoError(t, project.Insert(t.Context()))

	config := New("/graphql")
	ctx := gimlet.AttachUser(t.Context(), usr)
	callCount := 0
	next := func(context.Context) (any, error) {
		callCount++
		return nil, nil
	}

	result, err := config.Directives.RequireProjectAccess(
		ctx,
		map[string]any{"projectIdentifier": project.Identifier},
		next,
		ProjectPermissionSettings,
		AccessLevelEdit,
	)
	assert.EqualError(t, err, "input: user 'test_user' does not have permission to 'edit project settings' for the project 'victim_project_id'")
	assert.Nil(t, result)
	assert.Equal(t, 0, callCount)
}
