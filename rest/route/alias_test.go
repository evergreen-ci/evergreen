package route

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAliasesHandler(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, h *aliasGetHandler){
		"ReturnsProjectLevelAliases": func(ctx context.Context, t *testing.T, h *aliasGetHandler) {
			r, err := http.NewRequest(http.MethodGet, "/alias/project_ref", nil)
			assert.NoError(t, err)
			assert.NoError(t, h.Parse(ctx, r))

			resp := h.Run(ctx)
			assert.Equal(t, http.StatusOK, resp.Status())

			payload := resp.Data().([]any)
			require.Len(t, payload, 1)
			foundAlias := payload[0].(model.APIProjectAlias)
			assert.Equal(t, "project_alias", utility.FromStringPtr(foundAlias.Alias))
		},
		"ReturnsRepoLevelAliases": func(ctx context.Context, t *testing.T, h *aliasGetHandler) {
			projectAliases, err := dbModel.FindAliasesForProjectFromDb("project_ref")
			require.NoError(t, err)
			require.Len(t, projectAliases, 1)
			require.NoError(t, dbModel.RemoveProjectAlias(ctx, projectAliases[0].ID.Hex()))

			r, err := http.NewRequest(http.MethodGet, "/alias/project_ref", nil)
			assert.NoError(t, err)
			assert.NoError(t, h.Parse(ctx, r))

			resp := h.Run(ctx)
			assert.Equal(t, http.StatusOK, resp.Status())

			payload := resp.Data().([]any)
			require.Len(t, payload, 1)
			foundAlias := payload[0].(model.APIProjectAlias)
			assert.Equal(t, "repo_alias", utility.FromStringPtr(foundAlias.Alias))
		},
		"ReturnsNoAliasesWithoutProjectConfigURLParam": func(ctx context.Context, t *testing.T, h *aliasGetHandler) {
			require.NoError(t, db.Clear(dbModel.ProjectAliasCollection))

			r, err := http.NewRequest(http.MethodGet, "/alias/project_ref", nil)
			assert.NoError(t, err)
			assert.NoError(t, h.Parse(ctx, r))

			resp := h.Run(ctx)
			assert.Equal(t, http.StatusOK, resp.Status())

			payload := resp.Data().([]any)
			require.Empty(t, payload)
		},
		"ReturnsConfigLevelAliases": func(ctx context.Context, t *testing.T, h *aliasGetHandler) {
			require.NoError(t, db.Clear(dbModel.ProjectAliasCollection))

			r, err := http.NewRequest(http.MethodGet, "/alias/project_ref?includeProjectConfig=true", nil)
			assert.NoError(t, err)
			assert.NoError(t, h.Parse(ctx, r))

			resp := h.Run(ctx)
			assert.Equal(t, http.StatusOK, resp.Status())

			payload := resp.Data().([]any)
			require.Len(t, payload, 1)
			foundAlias := payload[0].(model.APIProjectAlias)
			assert.Equal(t, "project_config_alias", utility.FromStringPtr(foundAlias.Alias))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			require.NoError(t, db.ClearCollections(
				dbModel.RepoRefCollection,
				dbModel.ProjectRefCollection,
				dbModel.ProjectAliasCollection,
				dbModel.ProjectConfigCollection,
			))

			repoRef := &dbModel.RepoRef{
				ProjectRef: dbModel.ProjectRef{
					Id:    "repo_ref",
					Repo:  "repo",
					Owner: "mongodb",
				},
			}
			projectRef := &dbModel.ProjectRef{
				Id:                    "project_ref",
				Repo:                  "repo",
				Owner:                 "mongodb",
				RepoRefId:             "repo_ref",
				VersionControlEnabled: utility.TruePtr(),
			}
			require.NoError(t, repoRef.Upsert())
			require.NoError(t, projectRef.Upsert())

			repoAlias := &dbModel.ProjectAlias{
				ProjectID: repoRef.Id,
				Alias:     "repo_alias",
				Variant:   "test_variant",
			}
			projectAlias := &dbModel.ProjectAlias{
				ProjectID: projectRef.Id,
				Alias:     "project_alias",
				Variant:   "test_variant",
			}
			require.NoError(t, repoAlias.Upsert())
			require.NoError(t, projectAlias.Upsert())

			projectConfig := &dbModel.ProjectConfig{
				Id:      "project-1",
				Project: "project_ref",
				ProjectConfigFields: dbModel.ProjectConfigFields{
					PatchAliases: []dbModel.ProjectAlias{
						{
							Alias:       "project_config_alias",
							Description: "from project config",
						},
					},
				}}
			require.NoError(t, projectConfig.Insert())

			rh, ok := makeFetchAliases().(*aliasGetHandler)
			require.True(t, ok)

			tCase(ctx, t, rh)
		})
	}
}
