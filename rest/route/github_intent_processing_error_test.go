package route

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubIntentProcessingError(t *testing.T) {
	env := testutil.NewEnvironment(t.Context(), t)
	require.NoError(t, db.ClearCollections(patch.GitHubIntentInfoCollection, model.ProjectRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection))
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	require.NoError(t, (&model.ProjectRef{Id: "project-id"}).Insert(t.Context()))

	stored, err := patch.InsertGitHubIntentInfo(t.Context(), "project-id", "intent-id", "processing failed")
	require.NoError(t, err)

	scope := gimlet.Scope{ID: "project-scope", Type: evergreen.ProjectResourceType, Resources: []string{"project-id"}}
	require.NoError(t, db.Insert(t.Context(), evergreen.ScopeCollection, scope))
	role := gimlet.Role{
		ID:          "task-viewer",
		Scope:       scope.ID,
		Permissions: gimlet.Permissions{evergreen.PermissionTasks: evergreen.TasksView.Value},
	}
	require.NoError(t, db.Insert(t.Context(), evergreen.RoleCollection, role))
	adminProjectAccessRole := gimlet.Role{
		ID:          evergreen.SuperUserProjectAccessRole,
		Scope:       scope.ID,
		Permissions: gimlet.Permissions{evergreen.PermissionTasks: evergreen.TasksBasic.Value},
	}
	require.NoError(t, db.Insert(t.Context(), evergreen.RoleCollection, adminProjectAccessRole))
	userManager, err := gimlet.NewBasicUserManager(nil, env.RoleManager())
	require.NoError(t, err)
	authHandler := gimlet.NewAuthenticationHandler(gimlet.NewBasicAuthenticator(nil, nil), userManager)

	runRoute := func(t *testing.T, usr *user.DBUser, errorID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/github/intent-processing-errors/"+errorID, nil)
		req = gimlet.SetURLVars(req, map[string]string{"error_id": errorID})
		req = req.WithContext(gimlet.AttachUser(req.Context(), usr))
		recorder := httptest.NewRecorder()
		authHandler.ServeHTTP(recorder, req, func(rw http.ResponseWriter, r *http.Request) {
			newGitHubIntentProcessingErrorContextMiddleware().ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
				RequiresProjectPermission(evergreen.PermissionTasks, evergreen.TasksView).ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
					gimlet.WriteResponse(r.Context(), rw, makeGitHubIntentProcessingError().(*githubIntentProcessingErrorHandler).Run(r.Context()))
				})
			})
		})
		return recorder
	}

	t.Run("AuthorizedUserCanReadMessage", func(t *testing.T) {
		resp := runRoute(t, &user.DBUser{Id: "authorized", SystemRoles: []string{role.ID}}, stored.ID.Hex())
		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
		data := githubIntentProcessingErrorResponse{}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &data))
		assert.Equal(t, "processing failed", data.Message)
	})

	t.Run("AdminProjectAccessCanReadMessage", func(t *testing.T) {
		resp := runRoute(t, &user.DBUser{Id: "admin", SystemRoles: []string{adminProjectAccessRole.ID}}, stored.ID.Hex())
		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	})

	t.Run("UnauthorizedUserCannotReadMessage", func(t *testing.T) {
		resp := runRoute(t, &user.DBUser{Id: "unauthorized"}, stored.ID.Hex())
		assert.Equal(t, http.StatusUnauthorized, resp.Code)
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
		resp := runRoute(t, &user.DBUser{Id: "other-project-user", SystemRoles: []string{otherRole.ID}}, stored.ID.Hex())
		assert.Equal(t, http.StatusUnauthorized, resp.Code)
	})

	t.Run("MissingErrorReturnsNotFound", func(t *testing.T) {
		resp := runRoute(t, &user.DBUser{Id: "authorized", SystemRoles: []string{role.ID}}, "0123456789abcdef01234567")
		assert.Equal(t, http.StatusNotFound, resp.Code)
	})

	t.Run("InvalidIDReturnsBadRequest", func(t *testing.T) {
		resp := runRoute(t, &user.DBUser{Id: "authorized", SystemRoles: []string{role.ID}}, "invalid")
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}
