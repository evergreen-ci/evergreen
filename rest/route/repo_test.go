package route

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRepoIDHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.ClearCollections(
		dbModel.RepoRefCollection,
		dbModel.ProjectVarsCollection,
		dbModel.ProjectAliasCollection,
	))

	repoRef := &dbModel.RepoRef{
		ProjectRef: dbModel.ProjectRef{
			Id:    "repo_ref",
			Repo:  "repo",
			Owner: "mongodb",
		},
	}
	require.NoError(t, repoRef.Upsert())

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

	h := repoIDGetHandler{}
	r, err := http.NewRequest(http.MethodGet, "/repos/repo_ref", nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"repo_id": "repo_ref"})
	assert.NoError(t, h.Parse(ctx, r))

	resp := h.Run(ctx)
	assert.Equal(t, resp.Status(), http.StatusOK)
	assert.NotNil(t, resp.Data())

	repo := resp.Data().(*model.APIProjectRef)
	alias := model.APIProjectAlias{}
	alias.BuildFromService(*repoAlias)
	assert.NoError(t, err)

	assert.Equal(t, repoRef.Id, utility.FromStringPtr(repo.Id))
	assert.Equal(t, repoRef.Repo, utility.FromStringPtr(repo.Repo))
	assert.Equal(t, repoRef.Owner, utility.FromStringPtr(repo.Owner))
	assert.Equal(t, false, utility.FromBoolPtr(repo.Enabled))
	assert.Len(t, repo.Aliases, 1)
	assert.Equal(t, alias, repo.Aliases[0])
	assert.Equal(t, repoVars.Vars, repo.Variables.Vars)
}

func TestPatchRepoIDHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.ClearCollections(dbModel.RepoRefCollection, dbModel.ProjectVarsCollection,
		dbModel.ProjectAliasCollection, commitqueue.Collection, dbModel.ProjectRefCollection, evergreen.GitHubAppCollection))

	repoRef := &dbModel.RepoRef{
		ProjectRef: dbModel.ProjectRef{
			Id:    "repo_ref",
			Owner: "mongodb",
			Repo:  "mongo",
		},
	}
	assert.NoError(t, repoRef.Upsert())

	installation := evergreen.GitHubAppInstallation{
		Owner:          repoRef.Owner,
		Repo:           repoRef.Repo,
		InstallationID: 1234,
	}
	assert.NoError(t, installation.Upsert(ctx))

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
		Enabled:             true,
		CommitQueue:         dbModel.CommitQueueParams{Enabled: utility.TruePtr()},
		GithubChecksEnabled: utility.TruePtr(),
	}
	branchProject := &dbModel.ProjectRef{
		Id:         "branch_project_id",
		Identifier: "branch",
		Owner:      repoRef.Owner,
		Repo:       repoRef.Repo,
		Branch:     "main",
		RepoRefId:  repoRef.Id,
		Enabled:    true,
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

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "the amazing Annie"})
	settings, err := evergreen.GetConfig(ctx)
	assert.NoError(t, err)
	settings.GithubOrgs = []string{repoRef.Owner}
	h := repoIDPatchHandler{
		settings: settings,
	}

	settings.AuthConfig = evergreen.AuthConfig{
		Github: &evergreen.GithubAuthConfig{
			AppId: 1234,
		},
	}
	settings.Expansions = map[string]string{
		"github_app_key": "key",
	}
	body := bytes.NewBuffer([]byte(`{"commit_queue": {"enabled": true}}`))
	r, err := http.NewRequest(http.MethodGet, "/repos/repo_ref", body)
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
	r, err = http.NewRequest(http.MethodGet, "/repos/repo_ref", body)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"repo_id": "repo_ref"})
	require.NoError(t, h.Parse(ctx, r))
	assert.True(t, h.newRepoRef.IsGithubChecksEnabled())
	resp = h.Run(ctx)
	require.Equal(t, http.StatusBadRequest, resp.Status())
	assert.Contains(t, resp.Data().(gimlet.ErrorResponse).Message, "if repo GitHub checks enabled, must have aliases")

	body = bytes.NewBuffer([]byte(`{"commit_queue": {"enabled": true}, 
		"aliases": [{"alias": "__commit_queue", "variant": ".*", "task": ".*"}],
		"variables": {"vars": {"new": "variable"}, "private_vars": {"a": true}, "vars_to_delete": ["b"]}}`))
	r, err = http.NewRequest(http.MethodGet, "/repos/repo_ref", body)
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

	aliases, err := dbModel.FindAliasesForRepo(repoRef.Id)
	assert.NoError(t, err)
	assert.Len(t, aliases, 2)

	body = bytes.NewBuffer([]byte(`{"owner_name": "10gen"}`))
	r, err = http.NewRequest(http.MethodGet, "/repos/repo_ref", body)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"repo_id": "repo_ref"})
	require.NoError(t, h.Parse(ctx, r))
	assert.Equal(t, "10gen", h.newRepoRef.Owner)
	resp = h.Run(ctx)
	assert.Equal(t, http.StatusBadRequest, resp.Status())
	assert.Equal(t, resp.Data().(gimlet.ErrorResponse).Message, "owner not authorized")

	h.settings.GithubOrgs = append(h.settings.GithubOrgs, "10gen")
	resp = h.Run(ctx)
	assert.Equal(t, http.StatusBadRequest, resp.Status())
	assert.Contains(t, resp.Data().(gimlet.ErrorResponse).Message, "must enable GitHub webhooks first")

	installation = evergreen.GitHubAppInstallation{
		Owner:          "10gen",
		Repo:           repoRef.Repo,
		InstallationID: 1234,
	}
	assert.NoError(t, installation.Upsert(ctx))

	resp = h.Run(ctx)
	assert.Equal(t, http.StatusOK, resp.Status())

	repoRef, err = dbModel.FindOneRepoRef(repoRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, repoRef)
	assert.Equal(t, "10gen", repoRef.Owner)

	pRefs, err := dbModel.FindMergedEnabledProjectRefsByRepoAndBranch("10gen", "mongo", "main")
	assert.NoError(t, err)
	require.Len(t, pRefs, 1)
	assert.Equal(t, branchProject.Id, pRefs[0].Id)
}

func TestPatchHandlersWithRestricted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	require.NoError(t, db.ClearCollections(dbModel.RepoRefCollection, dbModel.ProjectVarsCollection,
		dbModel.ProjectAliasCollection, commitqueue.Collection, user.Collection, dbModel.ProjectRefCollection,
		evergreen.ScopeCollection, evergreen.RoleCollection, evergreen.ConfigCollection, evergreen.GitHubAppCollection))

	independentProject := &dbModel.ProjectRef{
		Id:                  "branch1",
		Identifier:          "branch1_iden",
		Owner:               "owner",
		Repo:                "repo",
		Branch:              "main",
		Enabled:             true,
		Restricted:          utility.TruePtr(),
		Admins:              []string{"branch1_admin"},
		RepotrackerDisabled: utility.TruePtr(),
	}
	branchProject := &dbModel.ProjectRef{
		Id:                  "branch2",
		Identifier:          "branch2_iden",
		Owner:               "owner",
		Repo:                "repo",
		Branch:              "main",
		Enabled:             true,
		Admins:              []string{"branch2_admin", "the amazing Annie"},
		RepotrackerDisabled: utility.TruePtr(),
	}
	assert.NoError(t, independentProject.Insert())
	assert.NoError(t, branchProject.Insert())

	installation := evergreen.GitHubAppInstallation{
		Owner:          branchProject.Owner,
		Repo:           branchProject.Repo,
		InstallationID: 1234,
	}
	assert.NoError(t, installation.Upsert(ctx))

	u := &user.DBUser{Id: "branch1_admin"}
	assert.NoError(t, u.Insert())
	u = &user.DBUser{Id: "branch2_admin"}
	assert.NoError(t, u.Insert())
	u = &user.DBUser{Id: "the amazing Annie"}
	assert.NoError(t, u.Insert())
	ctx = gimlet.AttachUser(context.Background(), u)

	rm := env.RoleManager()
	allProjectsScope := &gimlet.Scope{
		ID:        evergreen.AllProjectsScope,
		Resources: []string{},
	}
	assert.NoError(t, rm.AddScope(*allProjectsScope))
	restrictedScope := &gimlet.Scope{
		ID:          evergreen.RestrictedProjectsScope,
		Resources:   []string{"branch1"},
		ParentScope: evergreen.AllProjectsScope,
	}
	assert.NoError(t, rm.AddScope(*restrictedScope))
	unrestrictedScope := &gimlet.Scope{
		ID:          evergreen.UnrestrictedProjectsScope,
		Resources:   []string{"branch2"},
		ParentScope: evergreen.AllProjectsScope,
	}
	assert.NoError(t, rm.AddScope(*unrestrictedScope))
	// Verify that the all projects scope has both branches
	allProjectsScope, err := rm.GetScope(ctx, evergreen.AllProjectsScope)
	assert.NoError(t, err)
	assert.Len(t, allProjectsScope.Resources, 2)
	assert.Contains(t, allProjectsScope.Resources, "branch1")
	assert.Contains(t, allProjectsScope.Resources, "branch2")

	settings, err := evergreen.GetConfig(ctx)
	assert.NoError(t, err)
	settings.GithubOrgs = []string{branchProject.Owner}
	assert.NoError(t, settings.Set(ctx))
	attachProjectHandler := attachProjectToRepoHandler{}
	// Test that turning on repo settings doesn't impact existing restricted values
	req, _ := http.NewRequest(http.MethodPost, "rest/v2/projects/branch2/attach_to_repo", nil)
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "branch2"})

	assert.NoError(t, attachProjectHandler.Parse(ctx, req))
	resp := attachProjectHandler.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, resp.Status(), http.StatusOK)
	pRefs, err := dbModel.FindMergedEnabledProjectRefsByRepoAndBranch("owner", "repo", "main")
	assert.NoError(t, err)
	require.Len(t, pRefs, 2)
	repoId := ""
	for _, branch := range pRefs {
		if branch.Id == "branch1" {
			assert.True(t, branch.IsRestricted(), fmt.Sprintf("branch '%s' should be restricted", branch.Id))
		} else {
			assert.False(t, branch.IsRestricted(), fmt.Sprintf("branch '%s' shouldn't be restricted", branch.Id))
			assert.NotEmpty(t, branch.RepoRefId)
			repoId = branch.RepoRefId
		}
	}
	// Shouldn't impact the scopes restricted/unrestricted scopes.
	restrictedScope, err = rm.GetScope(ctx, evergreen.RestrictedProjectsScope)
	assert.NoError(t, err)
	assert.Equal(t, restrictedScope.Resources, []string{"branch1"})
	unrestrictedScope, err = rm.GetScope(ctx, evergreen.UnrestrictedProjectsScope)
	assert.NoError(t, err)
	assert.Equal(t, unrestrictedScope.Resources, []string{"branch2"})

	// Should be added to the all project scope however.
	allProjectsScope, err = rm.GetScope(ctx, evergreen.AllProjectsScope)
	assert.NoError(t, err)
	assert.Contains(t, allProjectsScope.Resources, repoId)

	// Branch admin that didn't turn on repo settings should still have view access
	u, err = user.FindOneById("branch2_admin")
	assert.NoError(t, err)
	assert.NotNil(t, u)
	assert.Contains(t, u.Roles(), dbModel.GetViewRepoRole(repoId))

	repoRef, err := dbModel.FindOneRepoRef(repoId)
	assert.NoError(t, err)
	assert.NotNil(t, repoRef)
	assert.Nil(t, repoRef.Restricted)

	scope, err := rm.GetScope(ctx, dbModel.GetUnrestrictedBranchProjectsScope(repoId))
	assert.NoError(t, err)
	assert.NotNil(t, scope)
	assert.Equal(t, scope.Resources, []string{"branch2"})

	// Test that setting repo to restricted impacts the branch project
	body := bytes.NewBuffer([]byte(`{"restricted": true}`))
	req, _ = http.NewRequest(http.MethodPatch, fmt.Sprintf("rest/v2/repos/%s", repoId), body)
	req = gimlet.SetURLVars(req, map[string]string{"repo_id": repoId})

	repoHandler := repoIDPatchHandler{
		settings: settings,
	}
	assert.NoError(t, repoHandler.Parse(ctx, req))
	resp = repoHandler.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, resp.Status(), http.StatusOK)

	repoRef, err = dbModel.FindOneRepoRef(repoId)
	assert.NoError(t, err)
	assert.NotNil(t, repoRef)
	assert.True(t, repoRef.IsRestricted())
	// now both branches should be restricted
	pRefs, err = dbModel.FindMergedEnabledProjectRefsByRepoAndBranch("owner", "repo", "main")
	assert.NoError(t, err)
	require.Len(t, pRefs, 2)
	for _, branch := range pRefs {
		assert.True(t, branch.IsRestricted(), fmt.Sprintf("branch '%s' should be restricted", branch.Id))
	}
	restrictedScope, err = rm.GetScope(ctx, evergreen.RestrictedProjectsScope)
	assert.NoError(t, err)
	assert.NotNil(t, restrictedScope)
	assert.Len(t, restrictedScope.Resources, 2)
	unrestrictedScope, err = rm.GetScope(ctx, evergreen.UnrestrictedProjectsScope)
	assert.NoError(t, err)
	assert.NotNil(t, unrestrictedScope)
	assert.Len(t, unrestrictedScope.Resources, 0)
	scope, err = rm.GetScope(ctx, dbModel.GetUnrestrictedBranchProjectsScope(repoId))
	assert.NoError(t, err)
	assert.NotNil(t, scope)
	assert.Empty(t, scope.Resources)
	// branch user should no longer be able to see repo settings, since it's restricted
	u, err = user.FindOneById("branch2_admin")
	assert.NoError(t, err)
	assert.NotNil(t, u)
	assert.NotContains(t, u.Roles(), dbModel.GetViewRepoRole(repoId))

	// test that setting branch explicitly not-restricted impacts that branch, even though it's using repo settings
	req, _ = http.NewRequest(http.MethodPost, "rest/v2/projects/branch1/attach_to_repo", nil)
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "branch1"})

	assert.NoError(t, attachProjectHandler.Parse(ctx, req))
	resp = attachProjectHandler.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, resp.Status(), http.StatusOK)

	body = bytes.NewBuffer([]byte(`{"restricted": false}`))
	req, _ = http.NewRequest(http.MethodPatch, "rest/v2/projects/branch1", body)
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "branch1"})

	projectHandler := projectIDPatchHandler{
		settings: settings,
	}
	assert.NoError(t, projectHandler.Parse(ctx, req))
	resp = projectHandler.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, resp.Status(), http.StatusOK)

	pRefs, err = dbModel.FindMergedEnabledProjectRefsByRepoAndBranch("owner", "repo", "main")
	assert.NoError(t, err)
	require.Len(t, pRefs, 2)
	for _, branch := range pRefs {
		assert.NotEmpty(t, branch.RepoRefId)
		if branch.Id == "branch2" {
			assert.True(t, branch.IsRestricted(), fmt.Sprintf("branch %s should be restricted", branch.Id))
		} else {
			assert.False(t, branch.IsRestricted(), fmt.Sprintf("branch '%s' shouldn't be restricted", branch.Id))
		}
	}
	restrictedScope, err = rm.GetScope(ctx, evergreen.RestrictedProjectsScope)
	assert.NoError(t, err)
	assert.NotNil(t, restrictedScope)
	assert.Equal(t, restrictedScope.Resources, []string{"branch2"})
	unrestrictedScope, err = rm.GetScope(ctx, evergreen.UnrestrictedProjectsScope)
	assert.NoError(t, err)
	assert.NotNil(t, unrestrictedScope)
	assert.Equal(t, unrestrictedScope.Resources, []string{"branch1"})
	scope, err = rm.GetScope(ctx, dbModel.GetUnrestrictedBranchProjectsScope(repoId))
	assert.NoError(t, err)
	assert.NotNil(t, scope)
	assert.Equal(t, scope.Resources, []string{"branch1"})
	// Verify that setting branch unrestricted doesn't give view settings to restricted repo
	u, err = user.FindOneById("branch1_admin")
	assert.NoError(t, err)
	assert.NotNil(t, u)
	assert.NotContains(t, u.Roles(), dbModel.GetViewRepoRole(repoId))

	// Test that setting branch to null uses the repo default (which is restricted)
	body = bytes.NewBuffer([]byte(`{"restricted": null}`))
	req, _ = http.NewRequest(http.MethodPatch, "rest/v2/projects/branch1", body)
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "branch1"})

	assert.NoError(t, projectHandler.Parse(ctx, req))
	resp = projectHandler.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, resp.Status(), http.StatusOK)

	pRefs, err = dbModel.FindMergedEnabledProjectRefsByRepoAndBranch("owner", "repo", "main")
	assert.NoError(t, err)
	require.Len(t, pRefs, 2)
	for _, branch := range pRefs {
		assert.NotEmpty(t, branch.RepoRefId)
		assert.True(t, branch.IsRestricted(), fmt.Sprintf("branch %s should be restricted", branch.Id))
	}

	restrictedScope, err = rm.GetScope(ctx, evergreen.RestrictedProjectsScope)
	assert.NoError(t, err)
	assert.NotNil(t, restrictedScope)
	assert.Contains(t, restrictedScope.Resources, "branch1")
	assert.Contains(t, restrictedScope.Resources, "branch2")
	unrestrictedScope, err = rm.GetScope(ctx, evergreen.UnrestrictedProjectsScope)
	assert.NoError(t, err)
	assert.NotNil(t, unrestrictedScope)
	assert.Empty(t, unrestrictedScope.Resources)
	scope, err = rm.GetScope(ctx, dbModel.GetUnrestrictedBranchProjectsScope(repoId))
	assert.NoError(t, err)
	assert.NotNil(t, scope)
	assert.Empty(t, scope.Resources)

	// Test that setting repo back to not restricted gives the branch admins view access again
	body = bytes.NewBuffer([]byte(`{"restricted": false}`))
	req, _ = http.NewRequest(http.MethodPatch, fmt.Sprintf("rest/v2/repos/%s", repoId), body)
	req = gimlet.SetURLVars(req, map[string]string{"repo_id": repoId})

	assert.NoError(t, repoHandler.Parse(ctx, req))
	resp = repoHandler.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, resp.Status(), http.StatusOK)
	u, err = user.FindOneById("branch1_admin")
	assert.NoError(t, err)
	assert.NotNil(t, u)
	assert.Contains(t, u.Roles(), dbModel.GetViewRepoRole(repoId))
	// Verify that setting branch unrestricted doesn't give view settings to restricted repo
	u, err = user.FindOneById("branch2_admin")
	assert.NoError(t, err)
	assert.NotNil(t, u)
	assert.Contains(t, u.Roles(), dbModel.GetViewRepoRole(repoId))
}
