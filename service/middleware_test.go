package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupComplexityUIServer installs a mock environment with the given GraphQL
// complexity limit as the global environment (the middleware reads its config
// and service flags from there) and returns a minimal UIServer to exercise.
func setupComplexityUIServer(t *testing.T, limit int) *UIServer {
	ctx := t.Context()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	env.EvergreenSettings.RateLimit = evergreen.RateLimitConfig{GraphQLComplexityLimit: limit}

	prev := evergreen.GetEnvironment()
	evergreen.SetEnvironment(env)
	t.Cleanup(func() { evergreen.SetEnvironment(prev) })

	require.NoError(t, db.ClearCollections(evergreen.ConfigCollection))

	return &UIServer{env: env, Settings: *env.Settings()}
}

// runComplexity invokes the complexity middleware with the given complexity
// header (omitted when empty) and user, returning the recorder and whether the
// next handler ran.
func runComplexity(t *testing.T, uis *UIServer, complexityHeader string, u *user.DBUser) (*httptest.ResponseRecorder, bool) {
	r, err := http.NewRequest(http.MethodPost, "/graphql/query", nil)
	require.NoError(t, err)
	if u != nil {
		r = r.WithContext(gimlet.AttachUser(t.Context(), u))
	}
	if complexityHeader != "" {
		r.Header.Set("complexity", complexityHeader)
	}
	rw := httptest.NewRecorder()
	ran := false
	uis.complexityLimit(func(http.ResponseWriter, *http.Request) { ran = true })(rw, r)
	return rw, ran
}

func TestComplexityLimitNilUserPassesThrough(t *testing.T) {
	uis := setupComplexityUIServer(t, 100)
	rw, ran := runComplexity(t, uis, "150", nil)
	assert.True(t, ran)
	assert.Equal(t, http.StatusOK, rw.Code)
}

func TestComplexityLimitZeroLimitPassesThrough(t *testing.T) {
	uis := setupComplexityUIServer(t, 0)
	rw, ran := runComplexity(t, uis, "150", &user.DBUser{Id: "u"})
	assert.True(t, ran)
	assert.Equal(t, http.StatusOK, rw.Code)
}

func TestComplexityLimitMissingHeaderPassesThrough(t *testing.T) {
	// Documents current behavior: with no complexity header the request always
	// passes through. This is the symptom of the score-source gap (the score is
	// computed server-side, not sent as a request header) — see TODO at
	// service/middleware.go:167 and TestComplexityLimitEnforcesOnComputedScore.
	uis := setupComplexityUIServer(t, 100)
	rw, ran := runComplexity(t, uis, "", &user.DBUser{Id: "u"})
	assert.True(t, ran)
	assert.Equal(t, http.StatusOK, rw.Code)
}

func TestComplexityLimitInvalidHeaderReturns400(t *testing.T) {
	uis := setupComplexityUIServer(t, 100)
	rw, ran := runComplexity(t, uis, "not-a-number", &user.DBUser{Id: "u"})
	assert.False(t, ran)
	assert.Equal(t, http.StatusBadRequest, rw.Code)
}

func TestComplexityLimitUnderLimitAllowed(t *testing.T) {
	uis := setupComplexityUIServer(t, 100)
	rw, ran := runComplexity(t, uis, "50", &user.DBUser{Id: "u"})
	assert.True(t, ran)
	assert.Equal(t, http.StatusOK, rw.Code)
	assert.Equal(t, "50", rw.Header().Get(evergreen.GraphQLComplexityHeader))
}

func TestComplexityLimitOverLimitReturns400(t *testing.T) {
	uis := setupComplexityUIServer(t, 100)
	rw, ran := runComplexity(t, uis, "150", &user.DBUser{Id: "u"})
	assert.False(t, ran)
	assert.Equal(t, http.StatusBadRequest, rw.Code)
	assert.Equal(t, "true", rw.Header().Get(evergreen.GraphQLComplexityExceededHeader))
}

func TestComplexityLimitWarnOnlyModeServesOverLimit(t *testing.T) {
	uis := setupComplexityUIServer(t, 100)
	require.NoError(t, (&evergreen.ServiceFlags{GraphQLComplexityLimiterDisabled: true}).Set(t.Context()))

	rw, ran := runComplexity(t, uis, "150", &user.DBUser{Id: "u"})
	assert.True(t, ran, "warn-only mode should still serve the request")
	assert.NotEqual(t, http.StatusBadRequest, rw.Code)
	assert.Equal(t, "true", rw.Header().Get(evergreen.GraphQLComplexityExceededHeader))
}

func TestComplexityLimitEnforcesOnComputedScore(t *testing.T) {
	// TODO(DEVPROD-33795): the complexity score should come from the
	// server-side computation (graphql.SplunkTracing), not an inbound
	// "complexity" request header. Until that is wired, a genuinely complex
	// query carries no header and is never rejected. Remove the skip and drive
	// a real complex query through the GraphQL handler once the score source is
	// fixed.
	t.Skip("encodes target behavior; blocked on routing the computed complexity score to the middleware")
}
