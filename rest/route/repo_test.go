package route

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRepoIDGetHandler(t *testing.T) {
	require.NoError(t, db.ClearCollections(
		model.RepoRefCollection,
		model.ProjectVarsCollection,
		model.ProjectAliasCollection,
	))

	repoRef := &model.RepoRef{model.ProjectRef{
		Id:      "repo_ref",
		Repo:    "repo",
		Owner:   "mongodb",
		Enabled: true,
	}}
	require.NoError(t, repoRef.Insert())

	repoVars := &model.ProjectVars{
		Id:   repoRef.Id,
		Vars: map[string]string{"a": "hello", "b": "world"},
	}
	_, err := repoVars.Upsert()
	require.NoError(t, err)

	repoAlias := &model.ProjectAlias{
		ProjectID: repoRef.Id,
		Alias:     "test_alias",
		Variant:   "test_variant",
	}
	require.NoError(t, repoAlias.Upsert())

	ctx := context.Background()
	h := repoIDGetHandler{
		sc: &data.DBConnector{},
	}
	r, err := http.NewRequest("GET", "/repos/repo_ref", nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"repo_id": "repo_ref"})
	assert.NoError(t, h.Parse(ctx, r))

	resp := h.Run(ctx)
	assert.Equal(t, resp.Status(), http.StatusOK)
	assert.NotNil(t, resp.Data())

	repo := resp.Data().(*restModel.APIProjectRef)
	alias := restModel.APIProjectAlias{}
	err = alias.BuildFromService(repoAlias)
	assert.NoError(t, err)

	assert.Equal(t, repoRef.Id, restModel.FromStringPtr(repo.Id))
	assert.Equal(t, repoRef.Repo, restModel.FromStringPtr(repo.Repo))
	assert.Equal(t, repoRef.Owner, restModel.FromStringPtr(repo.Owner))
	assert.Equal(t, repoRef.Enabled, repo.Enabled)
	assert.Equal(t, 1, len(repo.Aliases))
	assert.Equal(t, alias, repo.Aliases[0])
	assert.Equal(t, repoVars.Vars, repo.Variables.Vars)
}
