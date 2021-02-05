package route

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRepoIDGetHandler(t *testing.T) {
	require.NoError(t, db.ClearCollections(
		dbModel.RepoRefCollection,
		dbModel.ProjectVarsCollection,
		dbModel.ProjectAliasCollection,
	))

	repoRef := &dbModel.RepoRef{
		ProjectRef: dbModel.ProjectRef{
			Id:      "repo_ref",
			Repo:    "repo",
			Owner:   "mongodb",
			Enabled: utility.TruePtr(),
		},
	}
	require.NoError(t, repoRef.Insert())

	repoVars := &dbModel.ProjectVars{
		Id:   repoRef.Id,
		Vars: map[string]string{"a": "hello", "b": "world"},
	}
	_, err := repoVars.Upsert()
	require.NoError(t, err)

	repoAlias := &dbModel.ProjectAlias{
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

	repo := resp.Data().(*model.APIProjectRef)
	alias := model.APIProjectAlias{}
	err = alias.BuildFromService(repoAlias)
	assert.NoError(t, err)

	assert.Equal(t, repoRef.Id, utility.FromStringPtr(repo.Id))
	assert.Equal(t, repoRef.Repo, utility.FromStringPtr(repo.Repo))
	assert.Equal(t, repoRef.Owner, utility.FromStringPtr(repo.Owner))
	assert.Equal(t, repoRef.Enabled, repo.Enabled)
	assert.Len(t, repo.Aliases, 1)
	assert.Equal(t, alias, repo.Aliases[0])
	assert.Equal(t, repoVars.Vars, repo.Variables.Vars)
}
