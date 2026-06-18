package route

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLimitsFor(t *testing.T) {
	// Distinct values per field so a wrong-field bug is caught.
	cfg := evergreen.RateLimitConfig{
		RESTUserPerHour:       1,
		RESTUserBurst:         2,
		RESTServicePerHour:    3,
		RESTServiceBurst:      4,
		GraphQLUserPerHour:    5,
		GraphQLUserBurst:      6,
		GraphQLServicePerHour: 7,
		GraphQLServiceBurst:   8,
	}
	for testName, testCase := range map[string]struct {
		surface         evergreen.RateLimitSurface
		isService       bool
		expectedPerHour int
		expectedBurst   int
	}{
		"RESTUser":       {evergreen.RateLimitSurfaceREST, false, 1, 2},
		"RESTService":    {evergreen.RateLimitSurfaceREST, true, 3, 4},
		"GraphQLUser":    {evergreen.RateLimitSurfaceGraphQL, false, 5, 6},
		"GraphQLService": {evergreen.RateLimitSurfaceGraphQL, true, 7, 8},
		"UnknownSurface": {evergreen.RateLimitSurface("bogus"), false, 0, 0},
	} {
		t.Run(testName, func(t *testing.T) {
			perHour, burst := limitsFor(&cfg, testCase.surface, testCase.isService)
			assert.Equal(t, testCase.expectedPerHour, perHour)
			assert.Equal(t, testCase.expectedBurst, burst)
		})
	}
}

// setupRateLimitEnv builds a mock environment backed by an in-memory Redis
// (miniredis) with the given rate-limit config. It returns the env so tests
// can pass it directly to NewRateLimitMiddleware and adjust it further.
func setupRateLimitEnv(t *testing.T, cfg evergreen.RateLimitConfig) *mock.Environment {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { assert.NoError(t, rdb.Close()) })

	env := &mock.Environment{}
	require.NoError(t, env.Configure(t.Context()))
	env.SetRedisClient(rdb)

	require.NoError(t, db.ClearCollections(evergreen.ConfigCollection))
	require.NoError(t, cfg.Set(t.Context()))
	return env
}

// runRateLimit sends a single request with the given user attached through the
// middleware and reports the recorder plus whether the next handler ran.
func runRateLimit(t *testing.T, mw gimlet.Middleware, surfacePath string, u *user.DBUser) (*httptest.ResponseRecorder, bool) {
	r, err := http.NewRequest(http.MethodGet, surfacePath, nil)
	require.NoError(t, err)
	if u != nil {
		r = r.WithContext(gimlet.AttachUser(t.Context(), u))
	}
	rw := httptest.NewRecorder()
	ran := false
	mw.ServeHTTP(rw, r, func(http.ResponseWriter, *http.Request) { ran = true })
	return rw, ran
}

func TestRateLimitMiddlewareNilUserPassesThrough(t *testing.T) {
	env := setupRateLimitEnv(t, evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 1})
	mw := NewRateLimitMiddleware(env, evergreen.RateLimitSurfaceREST)

	rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", nil)
	assert.True(t, ran)
	assert.Equal(t, http.StatusOK, rw.Code)
	assert.Empty(t, rw.Header().Get(evergreen.RateLimitLimitHeader))
}

func TestRateLimitMiddlewareZeroLimitPassesThrough(t *testing.T) {
	env := setupRateLimitEnv(t, evergreen.RateLimitConfig{}) // all zero
	mw := NewRateLimitMiddleware(env, evergreen.RateLimitSurfaceREST)

	rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", &user.DBUser{Id: "u"})
	assert.True(t, ran)
	assert.Equal(t, http.StatusOK, rw.Code)
	assert.Empty(t, rw.Header().Get(evergreen.RateLimitLimitHeader))
}

func TestRateLimitMiddlewareUnderLimitAllowed(t *testing.T) {
	env := setupRateLimitEnv(t, evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 5})
	mw := NewRateLimitMiddleware(env, evergreen.RateLimitSurfaceREST)

	rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", &user.DBUser{Id: "u"})
	assert.True(t, ran)
	assert.Equal(t, http.StatusOK, rw.Code)
	assert.Equal(t, "100", rw.Header().Get(evergreen.RateLimitLimitHeader))
	assert.NotEmpty(t, rw.Header().Get(evergreen.RateLimitRemainingHeader))
}

func TestRateLimitMiddlewareOverLimitReturns429(t *testing.T) {
	env := setupRateLimitEnv(t, evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 1})
	mw := NewRateLimitMiddleware(env, evergreen.RateLimitSurfaceREST)
	u := &user.DBUser{Id: "u"}

	_, ran := runRateLimit(t, mw, "/rest/v2/hosts", u)
	require.True(t, ran)

	rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", u)
	assert.False(t, ran)
	assert.Equal(t, http.StatusTooManyRequests, rw.Code)
	assert.Equal(t, "true", rw.Header().Get(evergreen.RateLimitExceededHeader))
}

func TestRateLimitMiddlewareServiceTierUsesServiceLimits(t *testing.T) {
	env := setupRateLimitEnv(t, evergreen.RateLimitConfig{
		RESTUserPerHour:    100,
		RESTUserBurst:      1,
		RESTServicePerHour: 200,
		RESTServiceBurst:   5,
	})
	mw := NewRateLimitMiddleware(env, evergreen.RateLimitSurfaceREST)

	rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", &user.DBUser{Id: "svc", OnlyAPI: true})
	assert.True(t, ran)
	assert.Equal(t, "200", rw.Header().Get(evergreen.RateLimitLimitHeader))
}

func TestRateLimitMiddlewareElevatedUserGetsMoreHeadroom(t *testing.T) {
	env := setupRateLimitEnv(t, evergreen.RateLimitConfig{
		RESTUserPerHour: 100,
		RESTUserBurst:   1,
		ElevatedUserIDs: []string{"elevated"},
	})
	mw := NewRateLimitMiddleware(env, evergreen.RateLimitSurfaceREST)

	// Elevated users get a higher configured limit (currently 2x; see DEVPROD-34486).
	rw, _ := runRateLimit(t, mw, "/rest/v2/hosts", &user.DBUser{Id: "base"})
	assert.Equal(t, "100", rw.Header().Get(evergreen.RateLimitLimitHeader))

	rw, _ = runRateLimit(t, mw, "/rest/v2/hosts", &user.DBUser{Id: "elevated"})
	assert.Equal(t, "200", rw.Header().Get(evergreen.RateLimitLimitHeader))

	// Elevated headroom is finite: burst=1 doubles to 2, so a third request is denied.
	_, ran := runRateLimit(t, mw, "/rest/v2/hosts", &user.DBUser{Id: "elevated"})
	assert.True(t, ran)
	rw, ran = runRateLimit(t, mw, "/rest/v2/hosts", &user.DBUser{Id: "elevated"})
	assert.False(t, ran)
	assert.Equal(t, http.StatusTooManyRequests, rw.Code)
}

func TestRateLimitMiddlewareWarnOnlyModeServesOverLimit(t *testing.T) {
	env := setupRateLimitEnv(t, evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 1})
	require.NoError(t, (&evergreen.ServiceFlags{APIRateLimiterDisabled: true}).Set(t.Context()))
	mw := NewRateLimitMiddleware(env, evergreen.RateLimitSurfaceREST)
	u := &user.DBUser{Id: "u"}

	_, ran := runRateLimit(t, mw, "/rest/v2/hosts", u)
	require.True(t, ran)

	rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", u)
	assert.True(t, ran, "warn-only mode should still serve the request")
	assert.Equal(t, http.StatusOK, rw.Code)
	assert.Equal(t, "true", rw.Header().Get(evergreen.RateLimitExceededHeader))
}

func TestRateLimitMiddlewareRemainingHeaderDecrements(t *testing.T) {
	env := setupRateLimitEnv(t, evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 5})
	mw := NewRateLimitMiddleware(env, evergreen.RateLimitSurfaceREST)
	u := &user.DBUser{Id: "u"}

	var prev int
	for i := 0; i < 5; i++ {
		rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", u)
		require.True(t, ran)
		require.Equal(t, http.StatusOK, rw.Code)

		remaining := rw.Header().Get(evergreen.RateLimitRemainingHeader)
		require.NotEmpty(t, remaining)
		var cur int
		require.NoError(t, func() error {
			_, err := fmt.Sscanf(remaining, "%d", &cur)
			return err
		}())
		if i > 0 {
			assert.Less(t, cur, prev, "remaining should decrease with each request")
		}
		prev = cur
	}
}

// Verifies that rate limiting is also applied to GraphQL endpoints, non-exhaustively since
// GraphQL and REST use the same underlying limiter logic.
func TestRateLimitMiddlewareGraphQLSurfaceEnforcesLimit(t *testing.T) {
	env := setupRateLimitEnv(t, evergreen.RateLimitConfig{GraphQLUserPerHour: 100, GraphQLUserBurst: 1})
	mw := NewRateLimitMiddleware(env, evergreen.RateLimitSurfaceGraphQL)
	u := &user.DBUser{Id: "u"}

	_, ran := runRateLimit(t, mw, "/graphql/query", u)
	require.True(t, ran)

	rw, ran := runRateLimit(t, mw, "/graphql/query", u)
	assert.False(t, ran)
	assert.Equal(t, http.StatusTooManyRequests, rw.Code)
	assert.Equal(t, "100", rw.Header().Get(evergreen.RateLimitLimitHeader))
}

// Verifies that if the limiter fails to initialize (e.g. due to the Redis client being nil),
// the middleware allows requests to proceed rather than erroring out.
func TestRateLimitMiddlewareNilRedisClientPassesThrough(t *testing.T) {
	env := setupRateLimitEnv(t, evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 1})
	env.SetRedisClient(nil) // NewRateLimiter will fail; request should still be served
	mw := NewRateLimitMiddleware(env, evergreen.RateLimitSurfaceREST)

	rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", &user.DBUser{Id: "u"})
	assert.True(t, ran)
	assert.Equal(t, http.StatusOK, rw.Code)
}

// Verifies that if Redis becomes unavailable after the limiter is already initialized,
// the middleware allows requests to proceed rather than erroring out.
func TestRateLimitMiddlewareRedisDropPassesThrough(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { assert.NoError(t, rdb.Close()) })

	env := &mock.Environment{}
	require.NoError(t, env.Configure(t.Context()))
	env.SetRedisClient(rdb)
	require.NoError(t, db.ClearCollections(evergreen.ConfigCollection))
	cfg := evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 1}
	require.NoError(t, cfg.Set(t.Context()))

	mw := NewRateLimitMiddleware(env, evergreen.RateLimitSurfaceREST)

	mr.Close() // limiter is initialized; simulate Redis dropping mid-request

	rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", &user.DBUser{Id: "u"})
	assert.True(t, ran)
	assert.Equal(t, http.StatusOK, rw.Code)
}
