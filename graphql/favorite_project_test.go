package graphql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFavoriteProjectMutationVariablesRequireTargetAccess(t *testing.T) {
	const identifier = "restricted-secret-project"
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
		for _, tc := range []struct {
			name      string
			projectID string
			forbidden bool
		}{
			{name: "OtherProjectTaskViewShouldBeForbidden", projectID: "project_id_2", forbidden: true},
			{name: "TargetTaskViewShouldSucceed", projectID: "project_id"},
		} {
			t.Run(mutation.name+"/"+tc.name, func(t *testing.T) {
				setupPermissions(t)
				project := model.ProjectRef{
					Id:         tc.projectID,
					Identifier: identifier,
					Restricted: utility.TruePtr(),
				}
				require.NoError(t, project.Insert(t.Context()))
				usr := &user.DBUser{
					Id:               testUser,
					SystemRoles:      []string{"view_task"},
					FavoriteProjects: []string{"existing-favorite"},
				}
				if mutation.remove {
					usr.FavoriteProjects = append(usr.FavoriteProjects, identifier)
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
				query := fmt.Sprintf(`mutation($opts: %s!) { %s(opts: $opts) { identifier } }`, mutation.inputType, field)
				variables := map[string]any{"opts": map[string]any{"projectIdentifier": identifier}}
				payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
				require.NoError(t, err)
				req := httptest.NewRequest(http.MethodPost, "/graphql/query", bytes.NewReader(payload))
				req.Header.Set("Content-Type", "application/json")
				req = req.WithContext(gimlet.AttachUser(t.Context(), usr))
				recorder := httptest.NewRecorder()
				srv.ServeHTTP(recorder, req)
				require.Equal(t, http.StatusOK, recorder.Code)
				var response gqlResponse
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
				expectedFavorites := []string{"existing-favorite"}
				if tc.forbidden {
					require.Len(t, response.Errors, 1)
					assert.Equal(t, "FORBIDDEN", response.Errors[0].Extensions.Code)
					assert.Nil(t, response.Data)
					expectedFavorites = originalFavorites
				} else {
					require.Empty(t, response.Errors)
					assert.JSONEq(t, `{"identifier": "restricted-secret-project"}`, string(response.Data[field]))
					if !mutation.remove {
						expectedFavorites = append(expectedFavorites, identifier)
					}
				}
				storedUser, err := user.FindOneById(t.Context(), usr.Id)
				require.NoError(t, err)
				require.NotNil(t, storedUser)
				assert.Equal(t, expectedFavorites, storedUser.FavoriteProjects)
			})
		}
	}
}
