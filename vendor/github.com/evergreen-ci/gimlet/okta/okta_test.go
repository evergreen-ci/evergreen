package okta

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/testutil"
	"github.com/evergreen-ci/gimlet/usercache"
	"github.com/mongodb/grip"
	jwtverifier "github.com/okta/okta-jwt-verifier-golang"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserManagerCreation(t *testing.T) {
	for testName, testCase := range map[string]struct {
		modifyOpts func(CreationOptions) CreationOptions
		shouldPass bool
	}{
		"AllOptionsSetWithUserCache": {
			modifyOpts: func(opts CreationOptions) CreationOptions { return opts },
			shouldPass: true,
		},
		"AllOptionsSetWithExternalCache": {
			modifyOpts: func(opts CreationOptions) CreationOptions {
				return mockCreationOptionsWithExternalCache()
			},
			shouldPass: true,
		},
		"BothUserCacheAndExternalCacheSet": {
			modifyOpts: func(opts CreationOptions) CreationOptions {
				opts.ExternalCache = mockExternalCacheOptions()
				return opts
			},
			shouldPass: false,
		},
		"MissingClientID": {
			modifyOpts: func(opts CreationOptions) CreationOptions { opts.ClientID = ""; return opts },
			shouldPass: false,
		},
		"MissingClientSecret": {
			modifyOpts: func(opts CreationOptions) CreationOptions { opts.ClientSecret = ""; return opts },
			shouldPass: false,
		},
		"MissingRedirectURI": {
			modifyOpts: func(opts CreationOptions) CreationOptions { opts.RedirectURI = ""; return opts },
			shouldPass: false,
		},
		"MissingIssuer": {
			modifyOpts: func(opts CreationOptions) CreationOptions { opts.Issuer = ""; return opts },
			shouldPass: false,
		},
		"NoValidateGroupMissingUserGroup": {
			modifyOpts: func(opts CreationOptions) CreationOptions { opts.UserGroup = ""; return opts },
			shouldPass: true,
		},
		"ValidateGroupsButMissingUserGroup": {
			modifyOpts: func(opts CreationOptions) CreationOptions {
				opts.ValidateGroups = true
				opts.UserGroup = ""
				return opts
			},
			shouldPass: false,
		},
		"MissingCookiePath": {
			modifyOpts: func(opts CreationOptions) CreationOptions { opts.CookiePath = ""; return opts },
			shouldPass: false,
		},
		"MissingCookieDomain": {
			modifyOpts: func(opts CreationOptions) CreationOptions { opts.CookieDomain = ""; return opts },
			shouldPass: true,
		},
		"MissingLoginCookieName": {
			modifyOpts: func(opts CreationOptions) CreationOptions { opts.LoginCookieName = ""; return opts },
			shouldPass: false,
		},
		"MissingLoginCookieTTL": {
			modifyOpts: func(opts CreationOptions) CreationOptions { opts.LoginCookieTTL = time.Duration(0); return opts },
			shouldPass: true,
		},
		"MissingUserCache": {
			modifyOpts: func(opts CreationOptions) CreationOptions { opts.UserCache = nil; return opts },
			shouldPass: false,
		},
		"MissingGetHTTPClient": {
			modifyOpts: func(opts CreationOptions) CreationOptions { opts.GetHTTPClient = nil; return opts },
			shouldPass: false,
		},
		"MissingPutHTTPClient": {
			modifyOpts: func(opts CreationOptions) CreationOptions { opts.PutHTTPClient = nil; return opts },
			shouldPass: false,
		},
		"MissingReconciliateID": {
			modifyOpts: func(opts CreationOptions) CreationOptions { opts.ReconciliateID = nil; return opts },
			shouldPass: true,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			um, err := NewUserManager(testCase.modifyOpts(mockCreationOptions()))
			if testCase.shouldPass {
				assert.NoError(t, err)
				assert.NotNil(t, um)
			} else {
				assert.Error(t, err)
				assert.Nil(t, um)
			}
		})
	}
}

// mockAuthorizationServer represents a server against which OIDC requests can
// be sent.
type mockAuthorizationServer struct {
	// AuthorizeParameters are parameters sent to the authorize endpoint.
	AuthorizeParameters url.Values
	// Authorizeheaders are the headers sent to the authorize endpoint.
	AuthorizeHeaders http.Header
	// AuthorizeResponse is returned from the authorize endpoint.
	AuthorizeResponse map[string]interface{}
	// TokenParameters are parameters sent to the token endpoint.
	TokenParameters url.Values
	// TokenHeaders are the headers sent to the token endpoint.
	TokenHeaders http.Header
	// TokenResponse is returned from the token endpoint.
	TokenResponse *tokenResponse
	// UserInfoHeaders are the headers sent to the userinfo endpoint.
	UserInfoHeaders http.Header
	// UserInfoResponse is returned from the userinfo endpoint.
	UserInfoResponse *userInfoResponse
	// IntrospectParameters are parameters sent to the introspect endpoint.
	IntrospectParameters url.Values
	// IntrospectParameters are headers sent to the introspect endpoint.
	IntrospectHeaders http.Header
	// IntrospectResponse is returned from the introspect endpoint.
	IntrospectResponse *introspectResponse
}

func (s *mockAuthorizationServer) app(port int) (*gimlet.APIApp, error) {
	app := gimlet.NewApp()
	if err := app.SetHost("localhost"); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := app.SetPort(port); err != nil {
		return nil, err
	}

	app.AddRoute("/").Version(1).Get().Handler(s.root)
	app.AddRoute("/v1/authorize").Version(1).Get().Handler(s.authorize)
	app.AddRoute("/v1/token").Version(1).Post().Handler(s.token)
	app.AddRoute("/v1/userinfo").Version(1).Get().Handler(s.userinfo)
	app.AddRoute("/v1/introspect").Version(1).Post().Handler(s.introspect)

	return app, nil
}

func (s *mockAuthorizationServer) startMockServer(ctx context.Context) (port int, err error) {
tryToMakeServer:
	for {
		select {
		case <-ctx.Done():
			grip.Warning("timed out starting mock server")
			return -1, errors.WithStack(ctx.Err())
		default:
			port = testutil.GetPortNumber()
			app, err := s.app(port)
			if err != nil {
				grip.Warning(err)
				continue tryToMakeServer
			}

			go func() {
				grip.Warning(app.Run(ctx))
			}()

			timer := time.NewTimer(5 * time.Millisecond)
			defer timer.Stop()
			url := fmt.Sprintf("http://localhost:%d/v1", port)

			trials := 0
		checkServer:
			for {
				if trials > 5 {
					continue tryToMakeServer
				}

				select {
				case <-ctx.Done():
					return -1, errors.WithStack(ctx.Err())
				case <-timer.C:
					req, err := http.NewRequest(http.MethodGet, url, nil)
					if err != nil {
						timer.Reset(5 * time.Millisecond)
						trials++
						continue checkServer
					}
					rctx, cancel := context.WithTimeout(ctx, time.Second)
					defer cancel()
					req = req.WithContext(rctx)
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						timer.Reset(5 * time.Millisecond)
						trials++
						continue checkServer
					}
					if resp.StatusCode != http.StatusOK {
						timer.Reset(5 * time.Millisecond)
						trials++
						continue checkServer
					}

					return port, nil
				}
			}
		}
	}
}

func (s *mockAuthorizationServer) root(rw http.ResponseWriter, r *http.Request) {
	gimlet.WriteJSON(rw, struct{}{})
}

func (s *mockAuthorizationServer) authorize(rw http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		gimlet.WriteJSONError(rw, &tokenResponse{responseError: responseError{ErrorCode: "invalid_request"}})
		return
	}
	s.AuthorizeParameters, err = url.ParseQuery(string(body))
	if err != nil {
		gimlet.WriteJSONError(rw, &tokenResponse{responseError: responseError{ErrorCode: "invalid_request"}})
		return
	}
	s.AuthorizeHeaders = r.Header
	if s.AuthorizeResponse == nil {
		gimlet.WriteJSON(rw, struct{}{})
	}
	gimlet.WriteJSON(rw, s.AuthorizeResponse)
}

func (s *mockAuthorizationServer) token(rw http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		gimlet.WriteJSONError(rw, &tokenResponse{responseError: responseError{ErrorCode: "invalid_request"}})
		return
	}
	s.TokenParameters, err = url.ParseQuery(string(body))
	if err != nil {
		gimlet.WriteJSONError(rw, &tokenResponse{responseError: responseError{ErrorCode: "invalid_request"}})
		return
	}
	s.TokenHeaders = r.Header
	if s.TokenResponse == nil {
		gimlet.WriteJSON(rw, struct{}{})
		return
	}
	gimlet.WriteJSON(rw, s.TokenResponse)
}

func (s *mockAuthorizationServer) userinfo(rw http.ResponseWriter, r *http.Request) {
	s.UserInfoHeaders = r.Header
	if s.UserInfoResponse == nil {
		gimlet.WriteJSON(rw, struct{}{})
		return
	}
	gimlet.WriteJSON(rw, s.UserInfoResponse)
}

func (s *mockAuthorizationServer) introspect(rw http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		gimlet.WriteJSONError(rw, &introspectResponse{responseError: responseError{ErrorCode: "invalid_request"}})
		return
	}
	s.IntrospectParameters, err = url.ParseQuery(string(body))
	if err != nil {
		gimlet.WriteJSONError(rw, &introspectResponse{responseError: responseError{ErrorCode: "invalid_request"}})
		return
	}
	s.IntrospectHeaders = r.Header
	if s.IntrospectResponse == nil {
		gimlet.WriteJSON(rw, struct{}{})
		return
	}
	gimlet.WriteJSON(rw, s.IntrospectResponse)
}

func mapContains(t *testing.T, set, subset map[string][]string) {
	for k, v := range subset {
		checkVal, ok := set[k]
		if !assert.Truef(t, ok, "missing key '%s'", k) {
			continue
		}
		assert.ElementsMatch(t, v, checkVal)
	}
}

func TestBasic(t *testing.T) {
	um := &userManager{}
	assert.Implements(t, (*gimlet.UserManager)(nil), um)
	assert.True(t, um.IsRedirect())
}

func TestRequestHelpers(t *testing.T) {
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, um *userManager, s *mockAuthorizationServer){
		"GetUserInfoSuccess": func(ctx context.Context, t *testing.T, um *userManager, s *mockAuthorizationServer) {
			s.UserInfoResponse = &userInfoResponse{
				Name:   "name",
				Email:  "email",
				Groups: []string{"group"},
			}
			userInfo, err := um.getUserInfo(ctx, "access_token")
			require.NoError(t, err)
			mapContains(t, s.UserInfoHeaders, map[string][]string{
				"Accept":        {"application/json"},
				"Authorization": {"Bearer access_token"},
			})
			require.NotNil(t, userInfo)
			assert.Equal(t, *s.UserInfoResponse, *userInfo)
		},
		"GetUserInfoError": func(ctx context.Context, t *testing.T, um *userManager, s *mockAuthorizationServer) {
			s.UserInfoResponse = &userInfoResponse{
				Name:   "name",
				Email:  "email",
				Groups: []string{"group"},
				responseError: responseError{
					ErrorCode:        "error_code",
					ErrorDescription: "error_description",
				},
			}
			userInfo, err := um.getUserInfo(ctx, "access_token")
			assert.Error(t, err)
			mapContains(t, s.UserInfoHeaders, map[string][]string{
				"Accept":        {"application/json"},
				"Authorization": {"Bearer access_token"},
			})
			require.NotNil(t, userInfo)
			assert.Equal(t, *s.UserInfoResponse, *userInfo)
		},
		"ExchangeCodeForTokensSuccess": func(ctx context.Context, t *testing.T, um *userManager, s *mockAuthorizationServer) {
			s.TokenResponse = &tokenResponse{
				AccessToken:  "access_token",
				IDToken:      "id_token",
				RefreshToken: "refresh_token",
				TokenType:    "token_type",
				ExpiresIn:    3600,
				Scope:        "scope",
			}
			code := "some_code"
			tokens, err := um.exchangeCodeForTokens(ctx, code)
			require.NoError(t, err)
			mapContains(t, s.TokenHeaders, map[string][]string{
				"Accept":        {"application/json"},
				"Content-Type":  {"application/x-www-form-urlencoded"},
				"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", um.clientID, um.clientSecret)))},
			})
			mapContains(t, s.TokenParameters, map[string][]string{
				"grant_type":   {"authorization_code"},
				"code":         {code},
				"redirect_uri": {um.redirectURI},
			})
			require.NotNil(t, tokens)
			assert.Equal(t, *s.TokenResponse, *tokens)
		},
		"ExchangeCodeForTokensError": func(ctx context.Context, t *testing.T, um *userManager, s *mockAuthorizationServer) {
			s.TokenResponse = &tokenResponse{
				AccessToken:  "access_token",
				IDToken:      "id_token",
				RefreshToken: "refresh_token",
				TokenType:    "token_type",
				ExpiresIn:    3600,
				Scope:        "scope",
				responseError: responseError{
					ErrorCode:        "error_code",
					ErrorDescription: "error_description",
				},
			}
			code := "some_code"
			tokens, err := um.exchangeCodeForTokens(ctx, code)
			assert.Error(t, err)
			mapContains(t, s.TokenHeaders, map[string][]string{
				"Accept":        {"application/json"},
				"Content-Type":  {"application/x-www-form-urlencoded"},
				"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", um.clientID, um.clientSecret)))},
			})
			mapContains(t, s.TokenParameters, map[string][]string{
				"grant_type":   {"authorization_code"},
				"code":         {code},
				"redirect_uri": {um.redirectURI},
			})
			require.NotNil(t, tokens)
			assert.Equal(t, *s.TokenResponse, *tokens)
		},
		"IntrospectSucceeds": func(ctx context.Context, t *testing.T, um *userManager, s *mockAuthorizationServer) {

		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			s := &mockAuthorizationServer{}
			port, err := s.startMockServer(ctx)
			require.NoError(t, err)
			opts := mockCreationOptions()
			opts.Issuer = fmt.Sprintf("http://localhost:%d/v1", port)
			um, err := NewUserManager(opts)
			require.NoError(t, err)
			impl, ok := um.(*userManager)
			require.True(t, ok)
			testCase(ctx, t, impl, s)
		})
	}
}

func mockCreationOptions() CreationOptions {
	return CreationOptions{
		ClientID:             "client_id",
		ClientSecret:         "client_secret",
		RedirectURI:          "redirect_uri",
		Issuer:               "issuer",
		Scopes:               []string{"openid", "email", "profile", "offline_access"},
		UserGroup:            "user_group",
		CookiePath:           "cookie_path",
		CookieDomain:         "example.com",
		LoginCookieName:      "login_cookie",
		LoginCookieTTL:       time.Hour,
		UserCache:            usercache.NewInMemory(context.Background(), time.Minute),
		GetHTTPClient:        func() *http.Client { return &http.Client{} },
		PutHTTPClient:        func(*http.Client) {},
		AllowReauthorization: false,
		ReconciliateID:       func(id string) string { return id },
	}
}

func mockCreationOptionsWithExternalCache() CreationOptions {
	opts := mockCreationOptions()
	opts.UserCache = nil
	opts.ExternalCache = mockExternalCacheOptions()
	return opts
}
func mockExternalCacheOptions() *usercache.ExternalOptions {
	return &usercache.ExternalOptions{
		GetUserByToken:  func(string) (gimlet.User, bool, error) { return nil, false, nil },
		GetUserByID:     func(string) (gimlet.User, bool, error) { return nil, false, nil },
		PutUserGetToken: func(gimlet.User) (string, error) { return "", nil },
		ClearUserToken:  func(gimlet.User, bool) error { return nil },
		GetOrCreateUser: func(gimlet.User) (gimlet.User, error) { return nil, nil },
	}
}

func TestMakeUserFromInfo(t *testing.T) {
	for testName, testCase := range map[string]struct {
		info             userInfoResponse
		expectedUsername string
		reconciliateID   func(string) string
		shouldPass       bool
	}{
		"Succeeds": {
			info: userInfoResponse{
				Email:  "foo@bar.com",
				Name:   "foo",
				Groups: []string{"group1"},
			},
			expectedUsername: "foo@bar.com",
			shouldPass:       true,
		},
		"SucceedsWithoutGroups": {
			info: userInfoResponse{
				Email: "foo@bar.com",
				Name:  "foo",
			},
			expectedUsername: "foo@bar.com",
			shouldPass:       true,
		},
		"FailsWithoutEmail": {
			info: userInfoResponse{
				Name: "foo",
			},
			shouldPass: false,
		},
		"FixIDChangesUsername": {
			info: userInfoResponse{
				Name:  "foo",
				Email: "foo@bar.com",
			},
			reconciliateID: func(id string) (newID string) {
				return strings.TrimSuffix(id, "@bar.com")
			},
			expectedUsername: "foo",
			shouldPass:       true,
		},
		"FixIDFailsIfReturnsEmpty": {
			info: userInfoResponse{
				Name:  "foo",
				Email: "foo@bar.com",
			},
			reconciliateID: func(string) string { return "" },
			shouldPass:     false,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			user, err := makeUserFromInfo(&testCase.info, "access_token", "refresh_token", testCase.reconciliateID)
			if testCase.shouldPass {
				require.NoError(t, err)
				require.NotNil(t, user)
				assert.NotEmpty(t, user.Username())
				assert.Equal(t, testCase.info.Name, user.DisplayName())
				assert.Equal(t, testCase.info.Email, user.Email())
				assert.ElementsMatch(t, testCase.info.Groups, user.Roles())
				assert.Equal(t, "access_token", user.GetAccessToken())
				assert.Equal(t, "refresh_token", user.GetRefreshToken())
			} else {
				assert.Error(t, err)
				assert.Nil(t, user)
			}
		})
	}
}

func TestMakeUserFromIDToken(t *testing.T) {
	for testName, testCase := range map[string]struct {
		token            jwtverifier.Jwt
		reconciliateID   func(string) string
		expectedUsername string
		shouldPass       bool
	}{
		"Succeeds": {
			token: jwtverifier.Jwt{
				Claims: map[string]interface{}{
					"email": "foo@bar.com",
					"name":  "foo",
				},
			},
			expectedUsername: "foo@bar.com",
			shouldPass:       true,
		},
		"FailsWithoutEmail": {
			token: jwtverifier.Jwt{
				Claims: map[string]interface{}{
					"name": "foo",
				},
			},
			shouldPass: false,
		},
		"FixIDChangesUsername": {
			token: jwtverifier.Jwt{
				Claims: map[string]interface{}{
					"email": "foo@bar.com",
					"name":  "foo",
				},
			},
			reconciliateID: func(id string) (newID string) {
				return strings.TrimSuffix(id, "@bar.com")
			},
			expectedUsername: "foo",
			shouldPass:       true,
		},
		"FixIDFailsIfReturnsEmpty": {
			token: jwtverifier.Jwt{
				Claims: map[string]interface{}{
					"email": "foo@bar.com",
					"name":  "foo",
				},
			},
			expectedUsername: "foo@bar.com",
			reconciliateID:   func(string) string { return "" },
			shouldPass:       false,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			user, err := makeUserFromIDToken(&testCase.token, "access_token", "refresh_token", testCase.reconciliateID)
			if testCase.shouldPass {
				require.NoError(t, err)
				require.NotNil(t, user)
				assert.NotEmpty(t, user.Username())
				assert.Equal(t, testCase.expectedUsername, user.Username())
				assert.Equal(t, testCase.token.Claims["name"].(string), user.DisplayName())
				assert.Equal(t, testCase.token.Claims["email"], user.Email())
				assert.Equal(t, "access_token", user.GetAccessToken())
				assert.Equal(t, "refresh_token", user.GetRefreshToken())
				assert.Empty(t, user.Roles())
			} else {
				assert.Error(t, err)
				assert.Nil(t, user)
			}
		})
	}
}

func TestCreateUserToken(t *testing.T) {
	um, err := NewUserManager(mockCreationOptions())
	require.NoError(t, err)
	token, err := um.CreateUserToken("username", "password")
	assert.Error(t, err)
	assert.Empty(t, token)
}

func TestGetUserByID(t *testing.T) {
	opts, err := gimlet.NewBasicUserOptions("username")
	require.NoError(t, err)
	expectedUser := gimlet.NewBasicUser(opts.Name("name").Email("email").Password("password").Key("key").AccessToken("access_token").RefreshToken("refresh_token"))
	for testName, testCase := range map[string]struct {
		modifyOpts       func(CreationOptions) CreationOptions
		shouldPass       bool
		shouldReturnUser bool
	}{
		"Succeeds": {
			modifyOpts: func(opts CreationOptions) CreationOptions {
				opts.ExternalCache.GetUserByID = func(string) (gimlet.User, bool, error) { return expectedUser, true, nil }
				return opts
			},
			shouldPass:       true,
			shouldReturnUser: true,
		},
		"Errors": {
			modifyOpts: func(opts CreationOptions) CreationOptions {
				opts.ExternalCache.GetUserByID = func(string) (gimlet.User, bool, error) { return nil, false, errors.New("fail") }
				return opts
			},
			shouldPass:       false,
			shouldReturnUser: false,
		},
		"ErrorsForNilUser": {
			modifyOpts: func(opts CreationOptions) CreationOptions {
				opts.ExternalCache.GetUserByID = func(string) (gimlet.User, bool, error) { return nil, false, nil }
				return opts
			},
			shouldPass:       false,
			shouldReturnUser: false,
		},
		"FailsDueToInvalidUser": {
			modifyOpts: func(opts CreationOptions) CreationOptions {
				opts.ExternalCache.GetUserByID = func(string) (gimlet.User, bool, error) { return expectedUser, false, nil }
				return opts
			},
			shouldPass:       false,
			shouldReturnUser: true,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			opts := mockCreationOptions()
			opts.UserCache = nil
			um, err := NewUserManager(testCase.modifyOpts(mockCreationOptionsWithExternalCache()))
			require.NoError(t, err)
			user, err := um.GetUserByID(expectedUser.Username())
			if testCase.shouldPass {
				require.NoError(t, err)
				require.NotNil(t, user)
				assert.EqualValues(t, expectedUser, user)
			} else {
				assert.Error(t, err)
				if testCase.shouldReturnUser {
					assert.NotNil(t, user)
				} else {
					assert.Nil(t, user)
				}
			}
		})
	}

}

func TestGetOrCreateUser(t *testing.T) {
	opts, err := gimlet.NewBasicUserOptions("username")
	require.NoError(t, err)
	expectedUser := gimlet.NewBasicUser(opts.Name("name").Email("email").Password("password").Key("key").AccessToken("access_token").RefreshToken("refresh_token"))
	for testName, testCase := range map[string]struct {
		modifyOpts func(CreationOptions) CreationOptions
		shouldPass bool
	}{
		"Succeeds": {
			modifyOpts: func(opts CreationOptions) CreationOptions {
				opts.ExternalCache.GetOrCreateUser = func(gimlet.User) (gimlet.User, error) { return expectedUser, nil }
				return opts
			},
			shouldPass: true,
		},
		"Errors": {
			modifyOpts: func(opts CreationOptions) CreationOptions {
				opts.ExternalCache.GetOrCreateUser = func(gimlet.User) (gimlet.User, error) { return nil, errors.New("fail") }
				return opts
			},
			shouldPass: false,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			opts := mockCreationOptions()
			opts.UserCache = nil
			um, err := NewUserManager(testCase.modifyOpts(mockCreationOptionsWithExternalCache()))
			require.NoError(t, err)
			user, err := um.GetOrCreateUser(expectedUser)
			if testCase.shouldPass {
				require.NoError(t, err)
				require.NotNil(t, user)

			} else {
				assert.Error(t, err)
				assert.Nil(t, user)
			}
		})
	}
}

func TestClearUser(t *testing.T) {
	opts, err := gimlet.NewBasicUserOptions("username")
	require.NoError(t, err)
	expectedUser := gimlet.NewBasicUser(opts.Name("name").Email("email").Password("password").Key("key").AccessToken("access_token").RefreshToken("refresh_token"))
	for testName, testCase := range map[string]struct {
		modifyOpts func(CreationOptions) CreationOptions
		shouldPass bool
	}{
		"Succeeds": {
			modifyOpts: func(opts CreationOptions) CreationOptions {
				opts.ExternalCache.ClearUserToken = func(gimlet.User, bool) error {
					return nil
				}
				return opts
			},
			shouldPass: true,
		},
		"Errors": {
			modifyOpts: func(opts CreationOptions) CreationOptions {
				opts.ExternalCache.ClearUserToken = func(gimlet.User, bool) error {
					return errors.New("fail")
				}
				return opts
			},
			shouldPass: false,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			opts := mockCreationOptions()
			opts.UserCache = nil
			um, err := NewUserManager(testCase.modifyOpts(mockCreationOptionsWithExternalCache()))
			require.NoError(t, err)
			err = um.ClearUser(expectedUser, false)
			if testCase.shouldPass {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func cookieMap(cookies []*http.Cookie) (map[string]string, error) {
	m := map[string]string{}
	for _, cookie := range cookies {
		var err error
		m[cookie.Name], err = url.QueryUnescape(cookie.Value)
		if err != nil {
			return m, errors.Wrapf(err, "could not decode cookie %s", cookie.Name)
		}
	}
	return m, nil
}

func TestLoginHandler(t *testing.T) {
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, um *userManager, s *mockAuthorizationServer){
		"Succeeds": func(ctx context.Context, t *testing.T, um *userManager, s *mockAuthorizationServer) {
			rw := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/login?redirect=/redirect", nil)
			require.NoError(t, err)
			um.GetLoginHandler("")(rw, req)

			resp := rw.Result()
			assert.NoError(t, resp.Body.Close())

			assert.Equal(t, http.StatusFound, resp.StatusCode)

			cookies, err := cookieMap(resp.Cookies())
			require.NoError(t, err)
			assert.Contains(t, cookies, nonceCookieName)
			assert.Contains(t, cookies, stateCookieName)
			redirectURI, ok := cookies[requestURICookieName]
			assert.True(t, ok)
			assert.Equal(t, "/redirect", redirectURI)

			loc, ok := resp.Header["Location"]
			assert.True(t, ok)
			require.Len(t, loc, 1)
			parsed, err := url.Parse(loc[0])
			require.NoError(t, err)
			q := parsed.Query()
			mapContains(t, q, map[string][]string{
				"client_id":     []string{"client_id"},
				"response_type": []string{"code"},
				"response_mode": []string{"query"},
				"redirect_uri":  []string{"redirect_uri"},
			})
			scope := q.Get("scope")
			for _, requestedScope := range mockCreationOptions().Scopes {
				assert.Contains(t, scope, requestedScope)
			}
		},
		"SucceedsWithoutRedirectURI": func(ctx context.Context, t *testing.T, um *userManager, s *mockAuthorizationServer) {
			rw := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/login", nil)
			require.NoError(t, err)
			um.GetLoginHandler("")(rw, req)

			resp := rw.Result()
			assert.NoError(t, resp.Body.Close())

			assert.Equal(t, http.StatusFound, resp.StatusCode)

			cookies, err := cookieMap(resp.Cookies())
			require.NoError(t, err)
			assert.Contains(t, cookies, nonceCookieName)
			assert.Contains(t, cookies, stateCookieName)
			redirectURI, ok := cookies[requestURICookieName]
			assert.True(t, ok)
			assert.Equal(t, "/", redirectURI)

			loc, ok := resp.Header["Location"]
			assert.True(t, ok)
			require.Len(t, loc, 1)
			parsed, err := url.Parse(loc[0])
			require.NoError(t, err)
			q := parsed.Query()
			mapContains(t, q, map[string][]string{
				"client_id":     {"client_id"},
				"response_type": {"code"},
				"response_mode": {"query"},
				"redirect_uri":  {"redirect_uri"},
			})
			scope := q.Get("scope")
			for _, requestedScope := range mockCreationOptions().Scopes {
				assert.Contains(t, scope, requestedScope)
			}
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			s := &mockAuthorizationServer{}
			port, err := s.startMockServer(ctx)
			require.NoError(t, err)
			opts := mockCreationOptions()
			opts.Issuer = fmt.Sprintf("http://localhost:%d/v1", port)
			um, err := NewUserManager(opts)
			require.NoError(t, err)
			impl, ok := um.(*userManager)
			require.True(t, ok)
			testCase(ctx, t, impl, s)
		})
	}
}

func TestLoginHandlerCallback(t *testing.T) {
	makeQuery := func(state, code, nonce string) url.Values {
		q := url.Values{}
		q.Add("state", state)
		q.Add("code", code)
		q.Add("nonce", nonce)
		return q
	}
	makeCookieHeader := func(nonce, state, redirect string) []string {
		cookieHeader := make([]string, 0, 3)
		cookieHeader = append(cookieHeader, fmt.Sprintf("%s=%s", nonceCookieName, nonce))
		cookieHeader = append(cookieHeader, fmt.Sprintf("%s=%s", stateCookieName, state))
		cookieHeader = append(cookieHeader, fmt.Sprintf("%s=%s", requestURICookieName, redirect))
		return cookieHeader
	}
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, um *userManager, s *mockAuthorizationServer){
		"Succeeds": func(ctx context.Context, t *testing.T, um *userManager, s *mockAuthorizationServer) {
			email := "email"
			name := "name"

			um.doValidateIDToken = func(string, string) (*jwtverifier.Jwt, error) {
				return &jwtverifier.Jwt{
					Claims: map[string]interface{}{
						"email": email,
						"name":  name,
					},
				}, nil
			}
			um.doValidateAccessToken = func(string) error { return nil }

			s.TokenResponse = &tokenResponse{
				AccessToken: "access_token",
				IDToken:     "id_token",
			}
			s.UserInfoResponse = &userInfoResponse{
				Name:  name,
				Email: email,
			}

			state := "some_state"
			nonce := "some_nonce"
			redirect := "/redirect"
			code := "some_code"

			rw := httptest.NewRecorder()
			q := makeQuery(state, code, nonce)
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/login/callback?%s", q.Encode()), strings.NewReader(q.Encode()))
			require.NoError(t, err)
			req.Header["Cookie"] = makeCookieHeader(nonce, state, redirect)

			um.GetLoginCallbackHandler()(rw, req)

			resp := rw.Result()
			assert.NoError(t, resp.Body.Close())

			assert.Equal(t, http.StatusFound, resp.StatusCode)
			assert.Equal(t, []string{redirect}, resp.Header["Location"])

			mapContains(t, s.TokenHeaders, map[string][]string{
				"Accept":        {"application/json"},
				"Content-Type":  {"application/x-www-form-urlencoded"},
				"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", um.clientID, um.clientSecret)))},
			})
			mapContains(t, s.TokenParameters, map[string][]string{
				"grant_type":   {"authorization_code"},
				"code":         {code},
				"redirect_uri": {um.redirectURI},
			})
			assert.Empty(t, s.UserInfoHeaders)

			cookies, err := cookieMap(resp.Cookies())
			require.NoError(t, err)
			loginToken, ok := cookies[um.loginCookieName]
			assert.True(t, ok)
			assert.NotEmpty(t, loginToken)

			user, err := um.GetUserByID(email)
			require.NoError(t, err)
			require.NotNil(t, user)
			assert.Equal(t, email, user.Email())
			assert.Empty(t, user.Roles())

			checkUser, err := um.GetUserByToken(ctx, loginToken)
			require.NoError(t, err)
			assert.Equal(t, user, checkUser)
		},
		"SucceedsWithGroupValidation": func(ctx context.Context, t *testing.T, um *userManager, s *mockAuthorizationServer) {
			email := "email"
			name := "name"
			groups := []string{"user_group"}

			um.validateGroups = true

			um.doValidateIDToken = func(string, string) (*jwtverifier.Jwt, error) {
				return &jwtverifier.Jwt{
					Claims: map[string]interface{}{
						"email": email,
						"name":  name,
					},
				}, nil
			}
			um.doValidateAccessToken = func(string) error { return nil }

			s.TokenResponse = &tokenResponse{
				AccessToken: "access_token",
				IDToken:     "id_token",
			}
			s.UserInfoResponse = &userInfoResponse{
				Name:   name,
				Email:  email,
				Groups: groups,
			}

			state := "some_state"
			nonce := "some_nonce"
			redirect := "/redirect"
			code := "some_code"

			rw := httptest.NewRecorder()
			q := makeQuery(state, code, nonce)
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/login/callback?%s", q.Encode()), strings.NewReader(q.Encode()))
			require.NoError(t, err)
			req.Header["Cookie"] = makeCookieHeader(nonce, state, redirect)

			um.GetLoginCallbackHandler()(rw, req)

			resp := rw.Result()
			assert.NoError(t, resp.Body.Close())

			assert.Equal(t, http.StatusFound, resp.StatusCode)
			assert.Equal(t, []string{redirect}, resp.Header["Location"])

			mapContains(t, s.TokenHeaders, map[string][]string{
				"Accept":        {"application/json"},
				"Content-Type":  {"application/x-www-form-urlencoded"},
				"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", um.clientID, um.clientSecret)))},
			})
			mapContains(t, s.TokenParameters, map[string][]string{
				"grant_type":   {"authorization_code"},
				"code":         {code},
				"redirect_uri": {um.redirectURI},
			})

			mapContains(t, s.UserInfoHeaders, map[string][]string{
				"Accept":        {"application/json"},
				"Authorization": {"Bearer access_token"},
			})

			cookies, err := cookieMap(resp.Cookies())
			require.NoError(t, err)
			loginToken, ok := cookies[um.loginCookieName]
			assert.True(t, ok)
			assert.NotEmpty(t, loginToken)

			user, err := um.GetUserByID(email)
			require.NoError(t, err)
			require.NotNil(t, user)
			assert.Equal(t, email, user.Email())
			assert.ElementsMatch(t, groups, user.Roles())

			checkUser, err := um.GetUserByToken(ctx, loginToken)
			require.NoError(t, err)
			assert.Equal(t, user, checkUser)
		},
		"FailsForReturnedErrors": func(ctx context.Context, t *testing.T, um *userManager, s *mockAuthorizationServer) {
			name := "name"
			email := "email"

			um.doValidateIDToken = func(string, string) (*jwtverifier.Jwt, error) {
				return &jwtverifier.Jwt{
					Claims: map[string]interface{}{
						"email": email,
						"name":  name,
					},
				}, nil
			}
			um.doValidateAccessToken = func(string) error { return nil }

			s.UserInfoResponse = &userInfoResponse{
				Name:   name,
				Email:  email,
				Groups: []string{"user_group"},
			}

			state := "some_state"
			nonce := "some_nonce"
			redirect := "/redirect"
			code := "some_code"

			rw := httptest.NewRecorder()
			q := makeQuery(state, code, nonce)
			q.Add("error", "some_error")
			q.Add("error_description", "some_error_description")
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/login/callback?%s", q.Encode()), strings.NewReader(q.Encode()))
			require.NoError(t, err)
			req.Header["Cookie"] = makeCookieHeader(nonce, state, redirect)

			um.GetLoginCallbackHandler()(rw, req)

			resp := rw.Result()
			assert.NoError(t, resp.Body.Close())
			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		},
		"FailsForMismatchedState": func(ctx context.Context, t *testing.T, um *userManager, s *mockAuthorizationServer) {
			s.UserInfoResponse = &userInfoResponse{
				Name:   "name",
				Email:  "email",
				Groups: []string{"user_group"},
			}

			state := "some_state"
			nonce := "some_nonce"
			redirect := "/redirect"
			code := "some_code"

			rw := httptest.NewRecorder()
			q := makeQuery("bad_state", code, redirect)
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/login/callback?%s", q.Encode()), strings.NewReader(q.Encode()))
			require.NoError(t, err)
			req.Header["Cookie"] = makeCookieHeader(nonce, state, redirect)

			um.GetLoginCallbackHandler()(rw, req)

			resp := rw.Result()
			assert.NoError(t, resp.Body.Close())
			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			s := &mockAuthorizationServer{}
			port, err := s.startMockServer(ctx)
			require.NoError(t, err)
			opts := mockCreationOptions()
			opts.Issuer = fmt.Sprintf("http://localhost:%d/v1", port)
			opts.ReconciliateID = func(id string) string { return id }
			// opts.SkipGroupPopulation = false
			um, err := NewUserManager(opts)
			require.NoError(t, err)
			impl, ok := um.(*userManager)
			require.True(t, ok)
			testCase(ctx, t, impl, s)
		})
	}
}

func TestReauthorization(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, um *userManager, s *mockAuthorizationServer){
		"SucceedsWithValidAccessTokenAndGroupValidation": func(t *testing.T, um *userManager, s *mockAuthorizationServer) {
			um.validateGroups = true

			accessToken := "access_token"
			opts, err := gimlet.NewBasicUserOptions("foo")
			require.NoError(t, err)
			user := gimlet.NewBasicUser(opts.Name("foo").Email("foo@bar.com").Password("password").Key("key").AccessToken(accessToken))
			_, err = um.cache.GetOrCreate(user)
			require.NoError(t, err)

			s.IntrospectResponse = &introspectResponse{
				Active:      true,
				ExpiresUnix: int(time.Now().Add(time.Hour).Unix()),
			}
			s.UserInfoResponse = &userInfoResponse{Name: "foo", Email: "foo@bar.com", Groups: []string{um.userGroup}}

			require.NoError(t, um.ReauthorizeUser(user))

			mapContains(t, s.IntrospectParameters, map[string][]string{
				"token":           {accessToken},
				"token_type_hint": {"access_token"},
			})
			mapContains(t, s.IntrospectHeaders, map[string][]string{
				"Content-Type": {"application/x-www-form-urlencoded"},
				"Accept":       {"application/json"},
			})
			mapContains(t, s.UserInfoHeaders, map[string][]string{
				"Accept":        {"application/json"},
				"Authorization": {"Bearer " + accessToken},
			})
			assert.Empty(t, s.TokenParameters, "reauthorization should succeed without requesting new tokens")
			assert.Empty(t, s.TokenHeaders, "reauthorization should succeed without requesting new tokens")

			cachedUser, _, err := um.cache.Find(user.Username())
			require.NoError(t, err)
			assert.Equal(t, user.GetAccessToken(), cachedUser.GetAccessToken())
			assert.Equal(t, user.GetRefreshToken(), cachedUser.GetRefreshToken())
		},
		"SucceedsWithoutGroupValidation": func(t *testing.T, um *userManager, s *mockAuthorizationServer) {
			refreshToken := "refresh_token"
			opts, err := gimlet.NewBasicUserOptions("foo")
			require.NoError(t, err)
			user := gimlet.NewBasicUser(opts.Name("foo").Email("foo@bar.com").Password("password").Key("key").RefreshToken(refreshToken))
			_, err = um.cache.GetOrCreate(user)
			require.NoError(t, err)

			newAccessToken := "new_access_token"
			newRefreshToken := "new_refresh_token"

			um.doValidateIDToken = func(string, string) (*jwtverifier.Jwt, error) {
				return &jwtverifier.Jwt{
					Claims: map[string]interface{}{
						"name":  user.Username(),
						"email": user.Email(),
					},
				}, nil
			}

			s.IntrospectResponse = &introspectResponse{
				Active:      true,
				ExpiresUnix: int(time.Now().Add(time.Hour).Unix()),
			}
			s.UserInfoResponse = &userInfoResponse{Name: "foo", Email: "foo@bar.com", Groups: []string{um.userGroup}}
			s.TokenResponse = &tokenResponse{
				AccessToken:  newAccessToken,
				RefreshToken: newRefreshToken,
				IDToken:      "new_id_token",
				TokenType:    "token_type",
				ExpiresIn:    3600,
				Scope:        "scope",
			}

			require.NoError(t, um.ReauthorizeUser(user))

			mapContains(t, s.TokenParameters, map[string][]string{
				"grant_type":    {"refresh_token"},
				"refresh_token": {refreshToken},
				"scope":         {um.requestScopes()},
			})
			mapContains(t, s.TokenHeaders, map[string][]string{
				"Content-Type": {"application/x-www-form-urlencoded"},
				"Accept":       {"application/json"},
			})
			assert.Empty(t, s.IntrospectParameters, "should not introspect access token if not validating groups")
			assert.Empty(t, s.IntrospectHeaders, "should not introspect access token if not validating groups")
			assert.Empty(t, s.UserInfoHeaders, "should not get user info if not validating groups")

			cachedUser, _, err := um.cache.Find(user.Username())
			require.NoError(t, err)
			assert.Equal(t, newAccessToken, cachedUser.GetAccessToken())
			assert.Equal(t, newRefreshToken, cachedUser.GetRefreshToken())
		},
		"FailsIfAccessTokenAndRefreshTokensMissingAndValidatingGroups": func(t *testing.T, um *userManager, s *mockAuthorizationServer) {
			um.validateGroups = true
			opts, err := gimlet.NewBasicUserOptions("foo")
			require.NoError(t, err)
			user := gimlet.NewBasicUser(opts.Name("foo").Email("foo@bar.com").Password("password").Key("key"))

			require.Error(t, um.ReauthorizeUser(user))
			assert.Empty(t, s.TokenHeaders, "should not refresh tokens if refresh token missing")
			assert.Empty(t, s.TokenParameters, "should not refresh tokens if refresh token missing")
			assert.Empty(t, s.IntrospectHeaders, "should not check access token if missing access and refresh tokens")
			assert.Empty(t, s.IntrospectParameters, "should not check access token if missing access and refresh tokens")
			assert.Empty(t, s.UserInfoHeaders, "should not get user info if missing access and refresh tokens")
		},
		"FailsIfRefreshTokensMissingAndNotValidatingGroups": func(t *testing.T, um *userManager, s *mockAuthorizationServer) {
			opts, err := gimlet.NewBasicUserOptions("foo")
			require.NoError(t, err)
			user := gimlet.NewBasicUser(opts.Name("foo").Email("foo@bar.com").Password("password").Key("key").AccessToken("access_token"))

			require.Error(t, um.ReauthorizeUser(user))
			assert.Empty(t, s.TokenHeaders, "should not refresh tokens if refresh token missing")
			assert.Empty(t, s.TokenParameters, "should not refresh tokens if refresh token missing")
			assert.Empty(t, s.IntrospectHeaders, "should not check access token if not validating groups")
			assert.Empty(t, s.IntrospectParameters, "should not check access token if not validating groups")
			assert.Empty(t, s.UserInfoHeaders, "should not get user info if not validating groups")
		},
		"FailsIfAccessTokenAndTokensCannotRefresh": func(t *testing.T, um *userManager, s *mockAuthorizationServer) {
			um.validateGroups = true

			accessToken := "access_token"
			refreshToken := "refresh_token"
			opts, err := gimlet.NewBasicUserOptions("foo")
			require.NoError(t, err)
			user := gimlet.NewBasicUser(opts.Name("foo").Email("foo@bar.com").Password("password").Key("key").AccessToken(accessToken).RefreshToken(refreshToken))

			_, err = um.cache.GetOrCreate(user)
			require.NoError(t, err)

			s.IntrospectResponse = &introspectResponse{
				Active:      false,
				ExpiresUnix: int(time.Now().Add(-time.Hour).Unix()),
			}
			s.TokenResponse = &tokenResponse{
				responseError: responseError{
					ErrorCode:        "error",
					ErrorDescription: "error_description",
				},
			}

			assert.Error(t, um.ReauthorizeUser(user))

			mapContains(t, s.IntrospectParameters, map[string][]string{
				"token":           {accessToken},
				"token_type_hint": {"access_token"},
			})
			mapContains(t, s.IntrospectHeaders, map[string][]string{
				"Content-Type": {"application/x-www-form-urlencoded"},
				"Accept":       {"application/json"},
			})
			assert.Empty(t, s.UserInfoHeaders)

			cachedUser, _, err := um.cache.Find(user.Username())
			require.NoError(t, err)
			assert.Equal(t, user.GetAccessToken(), cachedUser.GetAccessToken())
			assert.Equal(t, user.GetRefreshToken(), cachedUser.GetRefreshToken())
		},
		"FailsForInvalidGroups": func(t *testing.T, um *userManager, s *mockAuthorizationServer) {
			um.validateGroups = true
			accessToken := "access_token"
			refreshToken := "refresh_token"
			newAccessToken := "new_access_token"
			s.IntrospectResponse = &introspectResponse{
				Active:      true,
				ExpiresUnix: int(time.Now().Add(time.Hour).Unix()),
			}
			s.TokenResponse = &tokenResponse{
				AccessToken:  newAccessToken,
				IDToken:      "new_id_token",
				RefreshToken: "new_refresh_token",
				TokenType:    "token_type",
				ExpiresIn:    3600,
				Scope:        "scope",
			}
			s.UserInfoResponse = &userInfoResponse{Name: "foo", Email: "foo@bar.com", Groups: []string{"invalid_group"}}
			opts, err := gimlet.NewBasicUserOptions("id")
			require.NoError(t, err)
			user := gimlet.NewBasicUser(opts.Name("name").Email("email").Password("password").Key("key").AccessToken(accessToken).RefreshToken(refreshToken))
			_, err = um.cache.GetOrCreate(user)
			require.NoError(t, err)

			assert.Error(t, um.ReauthorizeUser(user))

			mapContains(t, s.IntrospectParameters, map[string][]string{
				"token":           {newAccessToken},
				"token_type_hint": {"access_token"},
			})
			mapContains(t, s.IntrospectHeaders, map[string][]string{
				"Content-Type": {"application/x-www-form-urlencoded"},
				"Accept":       {"application/json"},
			})
			mapContains(t, s.TokenParameters, map[string][]string{
				"grant_type":    {"refresh_token"},
				"refresh_token": {refreshToken},
				"scope":         {um.requestScopes()},
			})
			mapContains(t, s.TokenHeaders, map[string][]string{
				"Content-Type": {"application/x-www-form-urlencoded"},
				"Accept":       {"application/json"},
			})
			mapContains(t, s.UserInfoHeaders, map[string][]string{
				"Accept":        {"application/json"},
				"Authorization": {"Bearer " + newAccessToken},
			})

			cachedUser, _, err := um.cache.Find(user.Username())
			require.NoError(t, err)
			assert.Equal(t, user.GetAccessToken(), cachedUser.GetAccessToken())
			assert.Equal(t, user.GetRefreshToken(), cachedUser.GetRefreshToken())
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s := &mockAuthorizationServer{}
			port, err := s.startMockServer(ctx)
			require.NoError(t, err)
			opts := mockCreationOptions()
			opts.Issuer = fmt.Sprintf("http://localhost:%d/v1", port)
			opts.AllowReauthorization = true
			um, err := NewUserManager(opts)
			require.NoError(t, err)
			impl, ok := um.(*userManager)
			impl.reconciliateID = func(id string) string {
				index := strings.LastIndex(id, "@")
				if index == -1 {
					return id
				}
				return id[:index]
			}

			require.True(t, ok)
			testCase(t, impl, s)
		})
	}
}
