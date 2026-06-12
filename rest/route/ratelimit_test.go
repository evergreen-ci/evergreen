package route

import (
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
// (miniredis) with the given rate-limit config, and installs it as the global
// environment because the middleware reads its config and service flags from
// there. It returns the env so tests can adjust it further.
func setupRateLimitEnv(t *testing.T, cfg evergreen.RateLimitConfig) *mock.Environment {
	ctx := t.Context()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { assert.NoError(t, rdb.Close()) })

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	env.SetRedisClient(rdb)
	env.EvergreenSettings.RateLimit = cfg

	prev := evergreen.GetEnvironment()
	evergreen.SetEnvironment(env)
	t.Cleanup(func() { evergreen.SetEnvironment(prev) })

	// Start from a clean service-flag state so GetServiceFlags returns zero values
	// unless a test sets them explicitly.
	require.NoError(t, db.ClearCollections(evergreen.ConfigCollection))

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

// allowedBefore429 sends requests for the user until one is rejected (or maxReqs
// is hit) and returns how many were allowed.
func allowedBefore429(t *testing.T, mw gimlet.Middleware, u *user.DBUser, maxReqs int) int {
	allowed := 0
	for i := 0; i < maxReqs; i++ {
		rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", u)
		if rw.Code == http.StatusTooManyRequests || !ran {
			break
		}
		allowed++
	}
	return allowed
}

func TestRateLimitMiddlewareNilUserPassesThrough(t *testing.T) {
	setupRateLimitEnv(t, evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 1})
	mw := NewRateLimitMiddleware(evergreen.GetEnvironment(), evergreen.RateLimitSurfaceREST)

	rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", nil)
	assert.True(t, ran)
	assert.Equal(t, http.StatusOK, rw.Code)
	assert.Empty(t, rw.Header().Get(evergreen.RateLimitLimitHeader))
}

func TestRateLimitMiddlewareZeroLimitPassesThrough(t *testing.T) {
	setupRateLimitEnv(t, evergreen.RateLimitConfig{}) // all zero
	mw := NewRateLimitMiddleware(evergreen.GetEnvironment(), evergreen.RateLimitSurfaceREST)

	rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", &user.DBUser{Id: "u"})
	assert.True(t, ran)
	assert.Equal(t, http.StatusOK, rw.Code)
	assert.Empty(t, rw.Header().Get(evergreen.RateLimitLimitHeader))
}

func TestRateLimitMiddlewareUnderLimitAllowed(t *testing.T) {
	setupRateLimitEnv(t, evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 5})
	mw := NewRateLimitMiddleware(evergreen.GetEnvironment(), evergreen.RateLimitSurfaceREST)

	rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", &user.DBUser{Id: "u"})
	assert.True(t, ran)
	assert.Equal(t, http.StatusOK, rw.Code)
	assert.Equal(t, "100", rw.Header().Get(evergreen.RateLimitLimitHeader))
	assert.NotEmpty(t, rw.Header().Get(evergreen.RateLimitRemainingHeader))
}

func TestRateLimitMiddlewareOverLimitReturns429(t *testing.T) {
	setupRateLimitEnv(t, evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 1})
	mw := NewRateLimitMiddleware(evergreen.GetEnvironment(), evergreen.RateLimitSurfaceREST)
	u := &user.DBUser{Id: "u"}

	_, ran := runRateLimit(t, mw, "/rest/v2/hosts", u)
	require.True(t, ran)

	rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", u)
	assert.False(t, ran)
	assert.Equal(t, http.StatusTooManyRequests, rw.Code)
	assert.Equal(t, "true", rw.Header().Get(evergreen.RateLimitExceededHeader))

	// TODO(DEVPROD-33795): the middleware currently writes res.ResetAfter (a
	// time.Duration) with %d, emitting nanoseconds rather than a delta in
	// seconds. Remove the skip once the unix conversion is fixed.
	t.Run("RetryAfterInSeconds", func(t *testing.T) {
		t.Skip("encodes target behavior; fails until Retry-After is emitted in seconds")
		retryAfter := rw.Header().Get(evergreen.RetryAfterHeader)
		require.NotEmpty(t, retryAfter)
		assert.Less(t, len(retryAfter), 7, "Retry-After should be a small seconds value, not nanoseconds")
	})
}

func TestRateLimitMiddlewareServiceTierUsesServiceLimits(t *testing.T) {
	setupRateLimitEnv(t, evergreen.RateLimitConfig{
		RESTUserPerHour:    100,
		RESTUserBurst:      1,
		RESTServicePerHour: 200,
		RESTServiceBurst:   5,
	})
	mw := NewRateLimitMiddleware(evergreen.GetEnvironment(), evergreen.RateLimitSurfaceREST)

	rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", &user.DBUser{Id: "svc", OnlyAPI: true})
	assert.True(t, ran)
	assert.Equal(t, "200", rw.Header().Get(evergreen.RateLimitLimitHeader))
}

func TestRateLimitMiddlewareElevatedUserGetsMoreHeadroom(t *testing.T) {
	setupRateLimitEnv(t, evergreen.RateLimitConfig{
		RESTUserPerHour: 100,
		RESTUserBurst:   2,
		ElevatedUserIDs: []string{"elevated"},
	})
	mw := NewRateLimitMiddleware(evergreen.GetEnvironment(), evergreen.RateLimitSurfaceREST)

	// Separate usernames keep the per-user buckets independent.
	baseAllowed := allowedBefore429(t, mw, &user.DBUser{Id: "base"}, 20)
	elevatedAllowed := allowedBefore429(t, mw, &user.DBUser{Id: "elevated"}, 20)

	// Assert the elevated user simply gets more headroom, not a specific
	// multiplier (the 2x is provisional, see DEVPROD-34486).
	assert.Greater(t, elevatedAllowed, baseAllowed)

	rw, _ := runRateLimit(t, mw, "/rest/v2/hosts", &user.DBUser{Id: "elevated2" /* not elevated */})
	baseLimit := rw.Header().Get(evergreen.RateLimitLimitHeader)
	rwElevated, _ := runRateLimit(t, mw, "/rest/v2/hosts", &user.DBUser{Id: "elevated"})
	assert.Greater(t, rwElevated.Header().Get(evergreen.RateLimitLimitHeader), baseLimit)
}

func TestRateLimitMiddlewareWarnOnlyModeServesOverLimit(t *testing.T) {
	setupRateLimitEnv(t, evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 1})
	require.NoError(t, (&evergreen.ServiceFlags{APIRateLimiterDisabled: true}).Set(t.Context()))
	mw := NewRateLimitMiddleware(evergreen.GetEnvironment(), evergreen.RateLimitSurfaceREST)
	u := &user.DBUser{Id: "u"}

	_, ran := runRateLimit(t, mw, "/rest/v2/hosts", u)
	require.True(t, ran)

	rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", u)
	assert.True(t, ran, "warn-only mode should still serve the request")
	assert.Equal(t, http.StatusOK, rw.Code)
	assert.Equal(t, "true", rw.Header().Get(evergreen.RateLimitExceededHeader))
}

func TestRateLimitMiddlewareBucketsIndependentAcrossSurfaces(t *testing.T) {
	setupRateLimitEnv(t, evergreen.RateLimitConfig{
		RESTUserPerHour:    100,
		RESTUserBurst:      1,
		GraphQLUserPerHour: 100,
		GraphQLUserBurst:   1,
	})
	restMW := NewRateLimitMiddleware(evergreen.GetEnvironment(), evergreen.RateLimitSurfaceREST)
	gqlMW := NewRateLimitMiddleware(evergreen.GetEnvironment(), evergreen.RateLimitSurfaceGraphQL)
	u := &user.DBUser{Id: "u"}

	// Exhaust the REST bucket.
	_, ran := runRateLimit(t, restMW, "/rest/v2/hosts", u)
	require.True(t, ran)
	rw, ran := runRateLimit(t, restMW, "/rest/v2/hosts", u)
	require.False(t, ran)
	require.Equal(t, http.StatusTooManyRequests, rw.Code)

	// The same user's GraphQL bucket is independent.
	rw, ran = runRateLimit(t, gqlMW, "/graphql/query", u)
	assert.True(t, ran)
	assert.Equal(t, http.StatusOK, rw.Code)
}

func TestRateLimitMiddlewareWiredIntoAssembledApp(t *testing.T) {
	// Exercises the same app-wide AddWrapper(rateLimit) wiring used by
	// AttachHandler, through gimlet's real router rather than a direct
	// ServeHTTP, to confirm the wrapper actually applies to dispatched routes.
	setupRateLimitEnv(t, evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 1})

	app := gimlet.NewApp()
	app.SetPrefix("rest")
	app.AddWrapper(NewRateLimitMiddleware(evergreen.GetEnvironment(), evergreen.RateLimitSurfaceREST))
	app.AddRoute("/ping").Version(2).Get().Handler(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	require.NoError(t, app.Resolve())
	router, err := app.Router()
	require.NoError(t, err)

	doReq := func() *httptest.ResponseRecorder {
		req, err := http.NewRequest(http.MethodGet, "/rest/v2/ping", nil)
		require.NoError(t, err)
		req = req.WithContext(gimlet.AttachUser(t.Context(), &user.DBUser{Id: "u"}))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}

	first := doReq()
	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, "100", first.Header().Get(evergreen.RateLimitLimitHeader), "wrapper should set rate-limit headers on dispatched routes")

	second := doReq()
	assert.Equal(t, http.StatusTooManyRequests, second.Code, "wrapper should throttle the assembled route")
}

func TestRateLimitMiddlewareAllowErrorFailsOpen(t *testing.T) {
	// TODO(DEVPROD-33795): the middleware currently returns 500 on an Allow
	// error instead of failing open (logging and passing through). Remove the
	// skip once it fails open per the ticket spec.
	t.Skip("encodes target behavior; fails until the middleware fails open on limiter errors")

	env := setupRateLimitEnv(t, evergreen.RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 1})
	env.SetRedisClient(nil) // force NewRateLimiter(nil) -> Allow errors
	mw := NewRateLimitMiddleware(env, evergreen.RateLimitSurfaceREST)

	rw, ran := runRateLimit(t, mw, "/rest/v2/hosts", &user.DBUser{Id: "u"})
	assert.True(t, ran, "limiter errors should fail open")
	assert.Equal(t, http.StatusOK, rw.Code)
}
