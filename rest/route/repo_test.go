package route

import (
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoGetByID(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.RepoRefCollection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(model.RepoRefCollection))
	})

	repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
		Id:    "my-repo",
		Owner: "evergreen-ci",
		Repo:  "evergreen",
	}}
	require.NoError(t, repoRef.Insert(t.Context()))

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
}
