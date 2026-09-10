package route

import (
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubIntentProcessingError(t *testing.T) {
	testutil.NewEnvironment(t.Context(), t)
	require.NoError(t, db.ClearCollections(patch.GitHubIntentProcessingErrorCollection, evergreen.ScopeCollection, evergreen.RoleCollection))
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

	stored, err := patch.InsertGitHubIntentProcessingError(t.Context(), "project-id", "processing failed")
	require.NoError(t, err)

	scope := gimlet.Scope{ID: "project-scope", Type: evergreen.ProjectResourceType, Resources: []string{"project-id"}}
	require.NoError(t, db.Insert(t.Context(), evergreen.ScopeCollection, scope))
	role := gimlet.Role{
		ID:          "task-viewer",
		Scope:       scope.ID,
		Permissions: gimlet.Permissions{evergreen.PermissionTasks: evergreen.TasksView.Value},
	}
	require.NoError(t, db.Insert(t.Context(), evergreen.RoleCollection, role))

	t.Run("AuthorizedUserCanReadMessage", func(t *testing.T) {
		ctx := gimlet.AttachUser(t.Context(), &user.DBUser{Id: "authorized", SystemRoles: []string{role.ID}})
		handler := &githubIntentProcessingErrorHandler{errorID: stored.ID}
		resp := handler.Run(ctx)
		require.Equal(t, http.StatusOK, resp.Status())
		data, ok := resp.Data().(githubIntentProcessingErrorResponse)
		require.True(t, ok)
		assert.Equal(t, "processing failed", data.Message)
	})

	t.Run("UnauthorizedUserCannotReadMessage", func(t *testing.T) {
		ctx := gimlet.AttachUser(t.Context(), &user.DBUser{Id: "unauthorized"})
		handler := &githubIntentProcessingErrorHandler{errorID: stored.ID}
		resp := handler.Run(ctx)
		assert.Equal(t, http.StatusUnauthorized, resp.Status())
	})

	t.Run("UserWithPermissionForAnotherProjectCannotReadMessage", func(t *testing.T) {
		otherScope := gimlet.Scope{ID: "other-project-scope", Type: evergreen.ProjectResourceType, Resources: []string{"other-project"}}
		require.NoError(t, db.Insert(t.Context(), evergreen.ScopeCollection, otherScope))
		otherRole := gimlet.Role{
			ID:          "other-project-task-viewer",
			Scope:       otherScope.ID,
			Permissions: gimlet.Permissions{evergreen.PermissionTasks: evergreen.TasksView.Value},
		}
		require.NoError(t, db.Insert(t.Context(), evergreen.RoleCollection, otherRole))
		ctx := gimlet.AttachUser(t.Context(), &user.DBUser{Id: "other-project-user", SystemRoles: []string{otherRole.ID}})
		handler := &githubIntentProcessingErrorHandler{errorID: stored.ID}
		resp := handler.Run(ctx)
		assert.Equal(t, http.StatusUnauthorized, resp.Status())
	})

	t.Run("MissingErrorReturnsNotFound", func(t *testing.T) {
		ctx := gimlet.AttachUser(t.Context(), &user.DBUser{Id: "authorized", SystemRoles: []string{role.ID}})
		handler := &githubIntentProcessingErrorHandler{errorID: mgobson.NewObjectId()}
		resp := handler.Run(ctx)
		assert.Equal(t, http.StatusNotFound, resp.Status())
	})

	t.Run("InvalidIDReturnsBadRequest", func(t *testing.T) {
		handler := makeGitHubIntentProcessingError().(*githubIntentProcessingErrorHandler)
		req, err := http.NewRequest(http.MethodGet, "/github/intent-processing-errors/invalid", nil)
		require.NoError(t, err)
		req = gimlet.SetURLVars(req, map[string]string{"error_id": "invalid"})
		err = handler.Parse(t.Context(), req)
		require.Error(t, err)
		respErr, ok := err.(gimlet.ErrorResponse)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, respErr.StatusCode)
	})
}
