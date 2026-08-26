package route

import (
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
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
		assert.Equal(t, http.StatusBadRequest, resp.Status())
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
