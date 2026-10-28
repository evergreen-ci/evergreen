package graphql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFavoriteProjectMutationsRequireTargetAccess(t *testing.T) {
	const selection = `{
		identifier restricted owner repo
		taskAnnotationSettings { fileTicketWebhook { endpoint secret } }
		workstationConfig { gitClone setupCommands { command directory } }
	}`
	for _, mutation := range []struct {
		name      string
		inputType string
		remove    bool
	}{
		{name: "AddFavoriteProject", inputType: "AddFavoriteProjectInput"},
		{name: "RemoveFavoriteProject", inputType: "RemoveFavoriteProjectInput", remove: true},
	} {
		field := "addFavoriteProject"
		if mutation.remove {
			field = "removeFavoriteProject"
		}
		for _, input := range []string{"InlineInput", "VariableInput"} {
			for _, tc := range []struct {
				name       string
				projectID  string
				identifier string
				errorCode  string
			}{
				{name: "OtherProjectTaskViewShouldBeForbidden", projectID: "project_id_2", identifier: "restricted-secret-project", errorCode: "FORBIDDEN"},
				{name: "TargetTaskViewShouldSucceed", projectID: "project_id", identifier: "restricted-secret-project"},
				{name: "MissingProjectShouldReturnNotFound", projectID: "project_id", identifier: "i-dont-exist", errorCode: "RESOURCE_NOT_FOUND"},
			} {
				t.Run(mutation.name+"/"+input+"/"+tc.name, func(t *testing.T) {
					setupPermissions(t)
					project := model.ProjectRef{
						Id:         tc.projectID,
						Identifier: "restricted-secret-project",
						Restricted: utility.TruePtr(),
						Owner:      "private-owner",
						Repo:       "private-repo",
						TaskAnnotationSettings: evergreen.AnnotationsSettings{
							FileTicketWebhook: evergreen.WebHook{Endpoint: "https://example.com/file-ticket", Secret: "EVG_SECRET_CANARY_4F2A"},
						},
						WorkstationConfig: model.WorkstationConfig{
							GitClone:      utility.TruePtr(),
							SetupCommands: []model.WorkstationSetupCommand{{Command: "private-setup-command", Directory: "private-directory"}},
						},
					}
					require.NoError(t, project.Insert(t.Context()))
					usr := &user.DBUser{
						Id:               testUser,
						SystemRoles:      []string{"view_task"},
						FavoriteProjects: []string{"existing-favorite"},
					}
					if mutation.remove {
						usr.FavoriteProjects = append(usr.FavoriteProjects, tc.identifier)
					}
					require.NoError(t, usr.Insert(t.Context()))
					originalFavorites := append([]string{}, usr.FavoriteProjects...)
					srv := handler.NewDefaultServer(NewExecutableSchema(NewConfig()))
					type gqlResponse struct {
						Data   map[string]json.RawMessage `json:"data"`
						Errors []struct {
							Extensions struct {
								Code string `json:"code"`
							} `json:"extensions"`
						} `json:"errors"`
					}
					runQuery := func(query string, variables map[string]any) gqlResponse {
						payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
						require.NoError(t, err)
						req := httptest.NewRequest(http.MethodPost, "/graphql/query", bytes.NewReader(payload))
						req.Header.Set("Content-Type", "application/json")
						req = req.WithContext(gimlet.AttachUser(t.Context(), usr))
						recorder := httptest.NewRecorder()
						srv.ServeHTTP(recorder, req)
						require.Equal(t, http.StatusOK, recorder.Code)
						assert.NotContains(t, recorder.Body.String(), project.TaskAnnotationSettings.FileTicketWebhook.Secret)
						var response gqlResponse
						require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
						return response
					}
					query := fmt.Sprintf(`mutation { %s(opts: {projectIdentifier: %q}) %s }`, field, tc.identifier, selection)
					var variables map[string]any
					if input == "VariableInput" {
						query = fmt.Sprintf(`mutation($opts: %s!) { %s(opts: $opts) %s }`, mutation.inputType, field, selection)
						variables = map[string]any{"opts": map[string]any{"projectIdentifier": tc.identifier}}
					}
					response := runQuery(query, variables)
					expectedFavorites := []string{"existing-favorite"}
					if tc.errorCode != "" {
						require.Len(t, response.Errors, 1)
						assert.Equal(t, tc.errorCode, response.Errors[0].Extensions.Code)
						assert.Nil(t, response.Data)
						expectedFavorites = originalFavorites
					} else {
						require.Empty(t, response.Errors)
						assert.JSONEq(t, fmt.Sprintf(`{
							"identifier": "restricted-secret-project", "restricted": true,
							"owner": "private-owner", "repo": "private-repo",
							"taskAnnotationSettings": {"fileTicketWebhook": {"endpoint": "https://example.com/file-ticket", "secret": %q}},
							"workstationConfig": {"gitClone": true, "setupCommands": [{"command": "private-setup-command", "directory": "private-directory"}]}
						}`, evergreen.RedactedValue), string(response.Data[field]))
						if !mutation.remove {
							expectedFavorites = append(expectedFavorites, tc.identifier)
						}
					}
					storedUser, err := user.FindOneById(t.Context(), usr.Id)
					require.NoError(t, err)
					require.NotNil(t, storedUser)
					assert.Equal(t, expectedFavorites, storedUser.FavoriteProjects)
					storedProject, err := model.FindBranchProjectRef(t.Context(), project.Id)
					require.NoError(t, err)
					require.NotNil(t, storedProject)
					assert.Equal(t, project.TaskAnnotationSettings, storedProject.TaskAnnotationSettings)
					assert.Equal(t, project.WorkstationConfig, storedProject.WorkstationConfig)
					if tc.errorCode == "FORBIDDEN" {
						control := runQuery(fmt.Sprintf(`query { project(projectIdentifier: %q) { identifier } }`, tc.identifier), nil)
						require.Len(t, control.Errors, 1)
						assert.Equal(t, "FORBIDDEN", control.Errors[0].Extensions.Code)
						assert.Nil(t, control.Data)
					}
				})
			}
		}
	}
}
