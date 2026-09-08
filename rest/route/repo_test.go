package route

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoGetByID(t *testing.T) {
	collections := []string{
		model.RepoRefCollection,
		model.ProjectVarsCollection,
		model.ProjectAliasCollection,
		fakeparameter.Collection,
		event.SubscriptionsCollection,
	}
	require.NoError(t, db.ClearCollections(collections...))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(collections...))
	})

	repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
		Id:    "my-repo",
		Owner: "evergreen-ci",
		Repo:  "evergreen",
	}}
	require.NoError(t, repoRef.Replace(t.Context()))

	repoVars := model.ProjectVars{
		Id:   "my-repo",
		Vars: map[string]string{"key1": "val1"},
	}
	require.NoError(t, repoVars.Insert(t.Context()))

	repoAlias := model.ProjectAlias{
		ProjectID: "my-repo",
		Alias:     "__commit_queue",
		Variant:   "ubuntu",
		Task:      "lint",
	}
	require.NoError(t, repoAlias.Upsert(t.Context()))

	repoSubscription := event.Subscription{
		ID:           "subscription-1",
		Owner:        "my-repo",
		OwnerType:    event.OwnerTypeProject,
		ResourceType: event.ResourceTypeTask,
		Trigger:      event.TriggerOutcome,
		Subscriber: event.Subscriber{
			Type:   event.EmailSubscriberType,
			Target: "a@b.com",
		},
	}
	require.NoError(t, repoSubscription.Upsert(t.Context()))

	t.Run("NonexistentRepoReturnsError", func(t *testing.T) {
		h := &repoIDGetHandler{repoID: "nonexistent"}
		resp := h.Run(t.Context())
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusNotFound, resp.Status())
	})

	t.Run("ExistingRepoReturnsData", func(t *testing.T) {
		h := &repoIDGetHandler{repoID: "my-repo"}
		resp := h.Run(t.Context())
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusOK, resp.Status())

		apiRef, ok := resp.Data().(*restmodel.APIProjectRef)
		require.True(t, ok)
		assert.Equal(t, "my-repo", utility.FromStringPtr(apiRef.Id))
		assert.Equal(t, "evergreen-ci", utility.FromStringPtr(apiRef.Owner))
		assert.Equal(t, "evergreen", utility.FromStringPtr(apiRef.Repo))
	})

	t.Run("ReturnsVars", func(t *testing.T) {
		h := &repoIDGetHandler{repoID: "my-repo"}
		resp := h.Run(t.Context())
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status())

		apiRef, ok := resp.Data().(*restmodel.APIProjectRef)
		require.True(t, ok)
		assert.Contains(t, apiRef.Variables.Vars, "key1")
	})

	t.Run("ReturnsAliases", func(t *testing.T) {
		h := &repoIDGetHandler{repoID: "my-repo"}
		resp := h.Run(t.Context())
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status())

		apiRef, ok := resp.Data().(*restmodel.APIProjectRef)
		require.True(t, ok)
		require.Len(t, apiRef.Aliases, 1)
		assert.Equal(t, "__commit_queue", utility.FromStringPtr(apiRef.Aliases[0].Alias))
		assert.Equal(t, "ubuntu", utility.FromStringPtr(apiRef.Aliases[0].Variant))
		assert.Equal(t, "lint", utility.FromStringPtr(apiRef.Aliases[0].Task))
	})

	t.Run("ReturnsSubscriptions", func(t *testing.T) {
		h := &repoIDGetHandler{repoID: "my-repo"}
		resp := h.Run(t.Context())
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status())

		apiRef, ok := resp.Data().(*restmodel.APIProjectRef)
		require.True(t, ok)
		require.Len(t, apiRef.Subscriptions, 1)
		assert.Equal(t, utility.ToStringPtr("subscription-1"), apiRef.Subscriptions[0].ID)
	})
}

func TestRepoPatchByID(t *testing.T) {
	ctx := t.Context()
	env := testutil.NewEnvironment(ctx, t)
	settings := env.Settings()
	settings.GithubOrgs = []string{"evergreen-ci"}

	collections := []string{
		model.RepoRefCollection,
		model.ProjectVarsCollection,
		model.ProjectAliasCollection,
		fakeparameter.Collection,
		event.SubscriptionsCollection,
		event.EventCollection,
		evergreen.ScopeCollection,
		evergreen.RoleCollection,
		user.Collection,
		evergreen.ConfigCollection,
	}
	require.NoError(t, db.ClearCollections(collections...))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(collections...))
	})

	testUser := user.DBUser{Id: "me"}
	require.NoError(t, testUser.Insert(ctx))

	repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
		Id:         "my-repo",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		Admins:     []string{"me"},
		Restricted: utility.FalsePtr(),
	}}
	require.NoError(t, repoRef.Replace(ctx))

	repoVars := model.ProjectVars{
		Id:   "my-repo",
		Vars: map[string]string{"key1": "val1"},
	}
	require.NoError(t, repoVars.Insert(ctx))

	rm := env.RoleManager()
	repoScope := gimlet.Scope{
		ID:        "repo_scope",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"my-repo"},
	}
	require.NoError(t, rm.AddScope(ctx, repoScope))

	repoRole := gimlet.Role{
		ID:    model.GetRepoAdminRole("my-repo"),
		Scope: repoScope.ID,
		Permissions: gimlet.Permissions{
			evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
		},
	}
	require.NoError(t, rm.UpdateRole(ctx, repoRole))

	makeRequest := func(t *testing.T, jsonBody string) gimlet.Responder {
		t.Helper()
		h := makePatchRepoByID(settings).(*repoIDPatchHandler)
		ctx := gimlet.AttachUser(t.Context(), &user.DBUser{Id: "me"})
		req, err := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/repos/my-repo", bytes.NewBufferString(jsonBody))
		require.NoError(t, err)
		req = gimlet.SetURLVars(req, map[string]string{"repo_id": "my-repo"})
		require.NoError(t, h.Parse(ctx, req))
		return h.Run(ctx)
	}

	t.Run("NonexistentRepoReturnsError", func(t *testing.T) {
		h := makePatchRepoByID(settings).(*repoIDPatchHandler)
		ctx := gimlet.AttachUser(t.Context(), &user.DBUser{Id: "me"})
		req, err := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/repos/nonexistent", bytes.NewBufferString(`{}`))
		require.NoError(t, err)
		req = gimlet.SetURLVars(req, map[string]string{"repo_id": "nonexistent"})
		err = h.Parse(ctx, req)
		require.Error(t, err)
		apiErr, ok := err.(gimlet.ErrorResponse)
		require.True(t, ok)
		assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	})

	t.Run("CannotChangeRepoID", func(t *testing.T) {
		h := makePatchRepoByID(settings).(*repoIDPatchHandler)
		ctx := gimlet.AttachUser(t.Context(), &user.DBUser{Id: "me"})
		req, err := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/repos/my-repo",
			bytes.NewBufferString(`{"id": "different-id"}`))
		require.NoError(t, err)
		req = gimlet.SetURLVars(req, map[string]string{"repo_id": "my-repo"})
		err = h.Parse(ctx, req)
		require.Error(t, err)
		apiErr, ok := err.(gimlet.ErrorResponse)
		require.True(t, ok)
		assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	})

	t.Run("InvalidOwnerReturnsError", func(t *testing.T) {
		h := makePatchRepoByID(settings).(*repoIDPatchHandler)
		ctx := gimlet.AttachUser(t.Context(), &user.DBUser{Id: "me"})
		req, err := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/repos/my-repo",
			bytes.NewBufferString(`{"owner_name": "not-allowed-org"}`))
		require.NoError(t, err)
		req = gimlet.SetURLVars(req, map[string]string{"repo_id": "my-repo"})
		require.NoError(t, h.Parse(ctx, req))
		resp := h.Run(ctx)
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusBadRequest, resp.Status())
	})

	t.Run("ValidUpdateSucceeds", func(t *testing.T) {
		resp := makeRequest(t, `{"display_name": "New Display Name", "task_ownership": { "default_mothra_team": "my-mothra-team" } }`)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status())

		updated, err := model.FindOneRepoRef(t.Context(), "my-repo")
		require.NoError(t, err)
		require.NotNil(t, updated)
		assert.Equal(t, "New Display Name", updated.DisplayName)
		assert.Equal(t, "my-mothra-team", updated.TaskOwnership.DefaultMothraTeam)
	})

	t.Run("UpdateAdmins", func(t *testing.T) {
		newAdmin := user.DBUser{Id: "new-admin"}
		require.NoError(t, newAdmin.Insert(t.Context()))

		resp := makeRequest(t, `{"admins": ["new-admin"]}`)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status())

		updated, err := model.FindOneRepoRef(t.Context(), "my-repo")
		require.NoError(t, err)
		require.NotNil(t, updated)
		assert.Contains(t, updated.Admins, "me")
		assert.Contains(t, updated.Admins, "new-admin")
	})

	t.Run("DeleteAdmins", func(t *testing.T) {
		resp := makeRequest(t, `{"delete_admins": ["new-admin"]}`)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status())

		updated, err := model.FindOneRepoRef(t.Context(), "my-repo")
		require.NoError(t, err)
		require.NotNil(t, updated)
		assert.NotContains(t, updated.Admins, "new-admin")
		assert.Contains(t, updated.Admins, "me")
	})

	t.Run("UpdateParsleyFiltersInvalidExpressionReturnsError", func(t *testing.T) {
		resp := makeRequest(t, `{"parsley_filters": [{"expression": "", "case_sensitive": true, "exact_match": false}]}`)
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusBadRequest, resp.Status())
	})

	t.Run("UpdateParsleyFiltersValidExpression", func(t *testing.T) {
		resp := makeRequest(t, `{"parsley_filters": [{"expression": "filter1", "case_sensitive": true, "exact_match": false}]}`)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status())

		updated, err := model.FindOneRepoRef(t.Context(), "my-repo")
		require.NoError(t, err)
		require.NotNil(t, updated)
		require.Len(t, updated.ParsleyFilters, 1)
		assert.Equal(t, "filter1", updated.ParsleyFilters[0].Expression)
	})

	t.Run("UpdateVariables", func(t *testing.T) {
		resp := makeRequest(t, `{"variables": { "vars": {"key2": "val2"} } }`)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status())

		vars, err := model.FindOneProjectVars(t.Context(), "my-repo")
		require.NoError(t, err)
		require.NotNil(t, vars)
		assert.Contains(t, vars.Vars, "key1")
		assert.Contains(t, vars.Vars, "key2")

		resp = makeRequest(t, `{ "variables" : { "vars_to_delete": ["key1"] } }`)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status())

		vars, err = model.FindOneProjectVars(t.Context(), "my-repo")
		require.NoError(t, err)
		require.NotNil(t, vars)
		assert.NotContains(t, vars.Vars, "key1")
		assert.Contains(t, vars.Vars, "key2")
	})

	t.Run("ModificationEventIsLogged", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(event.EventCollection))
		resp := makeRequest(t, `{"display_name": "Event Test"}`)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status())

		events, err := model.MostRecentProjectEvents(t.Context(), "my-repo", 1)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, event.EventTypeProjectModified, events[0].EventType)
	})
}
