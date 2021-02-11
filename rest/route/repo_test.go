package route

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/mgo.v2/bson"
)

func TestGetRepoIDHandler(t *testing.T) {
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

func TestPatchRepoIDHandler(t *testing.T) {
	require.NoError(t, db.ClearCollections(dbModel.RepoRefCollection, dbModel.ProjectVarsCollection,
		dbModel.ProjectAliasCollection, dbModel.GithubHooksCollection, commitqueue.Collection,
		dbModel.ProjectRefCollection))

	repoRef := &dbModel.RepoRef{
		ProjectRef: dbModel.ProjectRef{
			Id:      "repo_ref",
			Owner:   "mongodb",
			Repo:    "mongo",
			Enabled: utility.TruePtr(),
		},
	}
	assert.NoError(t, repoRef.Insert())
	hook := dbModel.GithubHook{
		HookID: 1,
		Owner:  repoRef.Owner,
		Repo:   repoRef.Repo,
	}
	assert.NoError(t, hook.Insert())
	repoVars := &dbModel.ProjectVars{
		Id:          repoRef.Id,
		Vars:        map[string]string{"a": "hello", "b": "world"},
		PrivateVars: map[string]bool{},
	}
	assert.NoError(t, repoVars.Insert())

	independentProject := &dbModel.ProjectRef{
		Id:                  "other_project_id",
		Identifier:          "independent",
		Owner:               repoRef.Owner,
		Repo:                repoRef.Repo,
		Branch:              "main",
		Enabled:             utility.TruePtr(),
		CommitQueue:         dbModel.CommitQueueParams{Enabled: utility.TruePtr()},
		GithubChecksEnabled: utility.TruePtr(),
	}
	branchProject := &dbModel.ProjectRef{
		Id:              "branch_project_id",
		Identifier:      "branch",
		Owner:           repoRef.Owner,
		Repo:            repoRef.Repo,
		Branch:          "main",
		UseRepoSettings: true,
		RepoRefId:       repoRef.Id,
	}
	assert.NoError(t, independentProject.Insert())
	assert.NoError(t, branchProject.Insert())

	repoAlias := &dbModel.ProjectAlias{
		ID:        bson.NewObjectId(),
		ProjectID: repoRef.Id,
		Alias:     evergreen.GithubPRAlias,
		Variant:   ".*",
		Task:      ".*",
	}
	assert.NoError(t, repoAlias.Upsert())
	independentAlias := &dbModel.ProjectAlias{
		ID:        bson.NewObjectId(),
		ProjectID: independentProject.Id,
		Alias:     evergreen.CommitQueueAlias,
		Variant:   ".*",
		Task:      ".*",
	}
	assert.NoError(t, independentAlias.Upsert())
	independentAlias.ID = bson.NewObjectId()
	independentAlias.Alias = evergreen.GithubChecksAlias
	assert.NoError(t, independentAlias.Upsert())

	ctx := gimlet.AttachUser(context.Background(), &user.DBUser{Id: "the amazing Annie"})
	settings, err := evergreen.GetConfig()
	assert.NoError(t, err)
	settings.GithubOrgs = []string{repoRef.Owner}
	h := repoIDPatchHandler{
		sc:       &data.DBConnector{},
		settings: settings,
	}
	body := bytes.NewBuffer([]byte(`{"commit_queue": {"enabled": true}}`))
	r, err := http.NewRequest("GET", "/repos/repo_ref", body)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"repo_id": "repo_ref"})
	require.NoError(t, h.Parse(ctx, r))
	assert.True(t, h.newRepoRef.CommitQueue.IsEnabled())
	resp := h.Run(ctx)
	require.Equal(t, http.StatusBadRequest, resp.Status())
	assert.Contains(t, resp.Data().(gimlet.ErrorResponse).Message, "commit queue is enabled in multiple projects")

	independentProject.CommitQueue.Enabled = nil
	assert.NoError(t, independentProject.Upsert())
	resp = h.Run(ctx)
	require.Equal(t, http.StatusBadRequest, resp.Status())
	assert.Contains(t, resp.Data().(gimlet.ErrorResponse).Message, "if repo commit queue enabled, must have aliases")

	body = bytes.NewBuffer([]byte(`{"github_checks_enabled": true}`))
	r, err = http.NewRequest("GET", "/repos/repo_ref", body)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"repo_id": "repo_ref"})
	require.NoError(t, h.Parse(ctx, r))
	assert.True(t, h.newRepoRef.IsGithubChecksEnabled())
	resp = h.Run(ctx)
	require.Equal(t, http.StatusBadRequest, resp.Status())
	assert.Contains(t, resp.Data().(gimlet.ErrorResponse).Message, "if repo github checks enabled, must have aliases")

	body = bytes.NewBuffer([]byte(`{"commit_queue": {"enabled": true}, 
		"aliases": [{"alias": "__commit_queue", "variant": ".*", "task": ".*"}],
		"variables": {"vars": {"new": "variable"}, "private_vars": {"a": true}, "vars_to_delete": ["b"]}}`))
	r, err = http.NewRequest("GET", "/repos/repo_ref", body)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"repo_id": "repo_ref"})
	require.NoError(t, h.Parse(ctx, r))
	assert.True(t, h.newRepoRef.CommitQueue.IsEnabled())
	resp = h.Run(ctx)
	assert.Equal(t, http.StatusOK, resp.Status())

	repoVars, err = dbModel.FindOneProjectVars(repoRef.Id)
	assert.NoError(t, err)
	require.NotNil(t, repoVars)
	assert.NotContains(t, repoVars.Vars, "b")
	assert.Contains(t, repoVars.Vars, "new")
	assert.Contains(t, repoVars.Vars, "a")
	assert.Contains(t, repoVars.PrivateVars, "a")

	aliases, err := dbModel.FindAliasesForProject(repoRef.Id)
	assert.NoError(t, err)
	assert.Len(t, aliases, 2)

	body = bytes.NewBuffer([]byte(`{"owner_name": "10gen"}`))
	r, err = http.NewRequest("GET", "/repos/repo_ref", body)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"repo_id": "repo_ref"})
	require.NoError(t, h.Parse(ctx, r))
	assert.Equal(t, "10gen", h.newRepoRef.Owner)
	resp = h.Run(ctx)
	assert.Equal(t, http.StatusBadRequest, resp.Status())
	assert.Equal(t, resp.Data().(gimlet.ErrorResponse).Message, "owner not authorized")

	h.settings.GithubOrgs = append(h.settings.GithubOrgs, "10gen")
	resp = h.Run(ctx)
	assert.Equal(t, http.StatusOK, resp.Status())

	repoRef, err = dbModel.FindOneRepoRef(repoRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, repoRef)
	assert.Equal(t, "10gen", repoRef.Owner)

	pRefs, err := dbModel.FindMergedProjectRefsByRepoAndBranch("10gen", "mongo", "main")
	assert.NoError(t, err)
	require.Len(t, pRefs, 1)
	assert.Equal(t, branchProject.Id, pRefs[0].Id)
}
