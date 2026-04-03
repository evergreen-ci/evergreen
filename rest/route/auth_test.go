package route

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type AuthRouteSuite struct {
	suite.Suite
}

func TestAuthRouteSuite(t *testing.T) {
	s := new(AuthRouteSuite)
	suite.Run(t, s)
}

func (s *AuthRouteSuite) TestParse() {
	h := &authPermissionGetHandler{}
	urlString := "http://evergreen.mongodb.com/rest/v2/auth"
	urlString += "?resource=abc&resource_type=project&permission=read&required_level=10"
	req := &http.Request{Method: http.MethodGet}
	req.URL, _ = url.Parse(urlString)
	err := h.Parse(context.TODO(), req)
	s.Require().NoError(err)
	s.Equal("abc", h.resource)
	s.Equal("project", h.resourceType)
	s.Equal("read", h.permission)
	s.Equal(10, h.requiredLevel)
}

func (s *AuthRouteSuite) TestParseFail() {
	h := &authPermissionGetHandler{}
	urlString := "http://evergreen.mongodb.com/rest/v2/auth"
	urlString += "?resource=abc&resource_type=project&permission=read&required_level=notAnumber"
	req := &http.Request{Method: http.MethodGet}
	req.URL, _ = url.Parse(urlString)
	err := h.Parse(context.TODO(), req)
	s.Error(err)
	s.Equal("abc", h.resource)
	s.Equal("project", h.resourceType)
	s.Equal("read", h.permission)
	s.Equal(0, h.requiredLevel)
}

func TestTokenExchangeCallbackParse(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		h := &tokenExchangeCallbackHandler{}
		req := &http.Request{Method: http.MethodGet}
		req.URL, _ = url.Parse("http://example.com/callback?code=auth-code-123&state=state-abc")
		err := h.Parse(context.TODO(), req)
		require.NoError(t, err)
		assert.Equal(t, "auth-code-123", h.code)
		assert.Equal(t, "state-abc", h.state)
	})

	t.Run("MissingCode", func(t *testing.T) {
		h := &tokenExchangeCallbackHandler{}
		req := &http.Request{Method: http.MethodGet}
		req.URL, _ = url.Parse("http://example.com/callback?state=state-abc")
		err := h.Parse(context.TODO(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing 'code'")
	})

	t.Run("MissingState", func(t *testing.T) {
		h := &tokenExchangeCallbackHandler{}
		req := &http.Request{Method: http.MethodGet}
		req.URL, _ = url.Parse("http://example.com/callback?code=auth-code-123")
		err := h.Parse(context.TODO(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing 'state'")
	})
}

func TestTokenExchangeCallbackRejectsMismatchedState(t *testing.T) {
	testutil.NewEnvironment(t.Context(), t)
	require.NoError(t, db.ClearCollections(user.Collection))

	u := &user.DBUser{Id: "test-user"}
	require.NoError(t, u.Insert(t.Context()))
	require.NoError(t, u.UpdateTokenExchangeState(t.Context(), "expected-state", "verifier"))

	env := &mock.Environment{}
	require.NoError(t, env.Configure(t.Context()))
	env.EvergreenSettings.AuthConfig.Okta = &evergreen.OktaConfig{
		ClientID: "client",
		Issuer:   "https://example.okta.com",
	}

	h := &tokenExchangeCallbackHandler{
		env:   env,
		code:  "auth-code",
		state: "wrong-state",
	}
	ctx := gimlet.AttachUser(t.Context(), u)

	resp := h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusInternalServerError, resp.Status())

	dbUser, err := user.FindOneById(t.Context(), u.Id)
	require.NoError(t, err)
	require.NotNil(t, dbUser)
	require.NotNil(t, dbUser.TokenExchangeState, "state should remain so the user can retry the flow")
	assert.Equal(t, "expected-state", dbUser.TokenExchangeState.State)
}

func TestTokenExchangeAuthorizeHandler(t *testing.T) {
	setupEnv := func(t *testing.T, oktaDiscoverySrv *httptest.Server) *mock.Environment {
		env := &mock.Environment{}
		require.NoError(t, env.Configure(t.Context()))
		env.EvergreenSettings.AuthConfig.Okta = &evergreen.OktaConfig{
			ClientID: "test-client-id",
			Issuer:   oktaDiscoverySrv.URL,
			Scopes:   []string{"openid", "email"},
		}
		env.EvergreenSettings.OktaServiceConfig = evergreen.OktaServiceConfig{
			ClientID:     "test-service-client-id",
			ClientSecret: "test-service-client-secret",
			Audience:     "test-service-audience",
			Issuer:       oktaDiscoverySrv.URL,
			Scopes:       []string{"openid", "email", "profile", "offline_access"},
		}
		env.EvergreenSettings.Ui.Url = "https://evergreen.example.com"
		return env
	}

	newDiscoveryServer := func() *httptest.Server {
		var srvURL string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]string{
				"issuer":                 srvURL,
				"authorization_endpoint": srvURL + "/v1/authorize",
				"token_endpoint":         srvURL + "/v1/token",
			}))
		}))
		srvURL = srv.URL
		return srv
	}

	t.Run("MissingOktaConfig", func(t *testing.T) {
		env := &mock.Environment{}
		require.NoError(t, env.Configure(t.Context()))
		env.EvergreenSettings.AuthConfig.Okta = nil

		handler := tokenExchangeAuthorizeHandler(env)
		req := httptest.NewRequest(http.MethodGet, "/rest/v2/auth/token_exchange/authorize", nil)
		ctx := gimlet.AttachUser(req.Context(), &user.DBUser{Id: "test-user"})
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.Contains(t, rr.Body.String(), "Okta auth config is not set")
	})

	t.Run("UnauthenticatedUser", func(t *testing.T) {
		discoverySrv := newDiscoveryServer()
		defer discoverySrv.Close()
		env := setupEnv(t, discoverySrv)

		handler := tokenExchangeAuthorizeHandler(env)
		req := httptest.NewRequest(http.MethodGet, "/rest/v2/auth/token_exchange/authorize", nil)

		rr := httptest.NewRecorder()
		assert.Panics(t, func() {
			handler.ServeHTTP(rr, req)
		})
	})

	t.Run("RedirectsToOkta", func(t *testing.T) {
		testutil.NewEnvironment(t.Context(), t)
		require.NoError(t, db.ClearCollections(user.Collection))

		u := &user.DBUser{Id: "test-user"}
		require.NoError(t, u.Insert(t.Context()))

		discoverySrv := newDiscoveryServer()
		defer discoverySrv.Close()
		env := setupEnv(t, discoverySrv)

		handler := tokenExchangeAuthorizeHandler(env)
		req := httptest.NewRequest(http.MethodGet, "/rest/v2/auth/token_exchange/authorize", nil)
		ctx := gimlet.AttachUser(req.Context(), u)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusTemporaryRedirect, rr.Code)
		location := rr.Header().Get("Location")
		require.NotEmpty(t, location)

		parsed, err := url.Parse(location)
		require.NoError(t, err)
		assert.Equal(t, "test-client-id", parsed.Query().Get("client_id"))
		assert.Equal(t, "code", parsed.Query().Get("response_type"))
		assert.Equal(t, "S256", parsed.Query().Get("code_challenge_method"))
		assert.NotEmpty(t, parsed.Query().Get("state"))
		assert.NotEmpty(t, parsed.Query().Get("code_challenge"))
		assert.Contains(t, parsed.Query().Get("redirect_uri"), "/rest/v2/auth/token_exchange/callback")

		dbUser, err := user.FindOneById(t.Context(), u.Id)
		require.NoError(t, err)
		require.NotNil(t, dbUser)
		require.NotNil(t, dbUser.TokenExchangeState)
		assert.NotEmpty(t, dbUser.TokenExchangeState.State)
		assert.NotEmpty(t, dbUser.TokenExchangeState.CodeVerifier)
	})
}
