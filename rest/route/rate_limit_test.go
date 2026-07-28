package route

import (
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserRateLimitGetHandlerSelfOnly(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T){
		"MismatchedUserIDIsForbidden": func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "http://example.com/api/rest/v2/users/other_user/rate_limit", nil)
			require.NoError(t, err)
			req = gimlet.SetURLVars(req, map[string]string{"user_id": "other_user"})

			handler := makeUserRateLimitGetHandler(nil)
			require.NoError(t, handler.Parse(t.Context(), req))

			ctx := gimlet.AttachUser(t.Context(), &user.DBUser{Id: "me"})
			resp := handler.Run(ctx)
			assert.Equal(t, http.StatusForbidden, resp.Status())
		},
		"MismatchedServiceUserIDIsForbidden": func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "http://example.com/api/rest/v2/users/other_service_user/rate_limit", nil)
			require.NoError(t, err)
			req = gimlet.SetURLVars(req, map[string]string{"user_id": "other_service_user"})

			handler := makeUserRateLimitGetHandler(nil)
			require.NoError(t, handler.Parse(t.Context(), req))

			ctx := gimlet.AttachUser(t.Context(), &user.DBUser{Id: "me", OnlyAPI: true})
			resp := handler.Run(ctx)
			assert.Equal(t, http.StatusForbidden, resp.Status())
		},
		"MatchingUserIDIsAllowed": func(t *testing.T) {
			env := setupRateLimitEnv(t, evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 5})

			req, err := http.NewRequest(http.MethodGet, "http://example.com/api/rest/v2/users/me/rate_limit", nil)
			require.NoError(t, err)
			req = gimlet.SetURLVars(req, map[string]string{"user_id": "me"})

			handler := makeUserRateLimitGetHandler(env)
			require.NoError(t, handler.Parse(t.Context(), req))

			ctx := gimlet.AttachUser(t.Context(), &user.DBUser{Id: "me"})
			resp := handler.Run(ctx)
			assert.Equal(t, http.StatusOK, resp.Status())
		},
	} {
		t.Run(testName, func(t *testing.T) {
			testCase(t)
		})
	}
}
