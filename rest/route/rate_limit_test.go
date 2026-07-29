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

// runUserRateLimitHandler parses and runs the handler for a GET on
// /users/{userID}/rate_limit, authenticated as userID.
func runUserRateLimitHandler(t *testing.T, env evergreen.Environment, userID string) gimlet.Responder {
	req, err := http.NewRequest(http.MethodGet, "http://example.com/api/rest/v2/users/"+userID+"/rate_limit", nil)
	require.NoError(t, err)
	req = gimlet.SetURLVars(req, map[string]string{"user_id": userID})

	handler := makeUserRateLimitGetHandler(env)
	require.NoError(t, handler.Parse(t.Context(), req))

	ctx := gimlet.AttachUser(t.Context(), &user.DBUser{Id: userID})
	return handler.Run(ctx)
}

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
		"MatchingUserIDIsAllowed": func(t *testing.T) {
			env := setupRateLimitEnv(t, evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 5})

			resp := runUserRateLimitHandler(t, env, "me")
			assert.Equal(t, http.StatusOK, resp.Status())
		},
	} {
		t.Run(testName, func(t *testing.T) {
			testCase(t)
		})
	}
}

// TestUserRateLimitGetHandlerReportsNoLimit verifies that cases where the caller
// has no enforceable limit (zero configured limit, the rate limiter disabled, or
// an uninitializable limiter) report a nil status with a 200, rather than a 500.
func TestUserRateLimitGetHandlerReportsNoLimit(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T){
		"UnconfiguredRESTLimitReportsNoLimit": func(t *testing.T) {
			env := setupRateLimitEnv(t, evergreen.RateLimitConfig{}) // all zero

			resp := runUserRateLimitHandler(t, env, "me")
			assert.Equal(t, http.StatusOK, resp.Status())
			assert.Nil(t, resp.Data())
		},
		"DisabledRateLimiterReportsNoLimit": func(t *testing.T) {
			env := setupRateLimitEnv(t, evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 5})
			require.NoError(t, (&evergreen.ServiceFlags{APIRateLimiterDisabled: true}).Set(t.Context()))

			resp := runUserRateLimitHandler(t, env, "me")
			assert.Equal(t, http.StatusOK, resp.Status())
			assert.Nil(t, resp.Data())
		},
		"NilRedisClientReportsNoLimitInsteadOfError": func(t *testing.T) {
			env := setupRateLimitEnv(t, evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 5})
			env.SetRedisClient(nil) // NewRateLimiter will fail; the route should still return 200.

			resp := runUserRateLimitHandler(t, env, "me")
			assert.Equal(t, http.StatusOK, resp.Status())
			assert.Nil(t, resp.Data())
		},
	} {
		t.Run(testName, func(t *testing.T) {
			testCase(t)
		})
	}
}
