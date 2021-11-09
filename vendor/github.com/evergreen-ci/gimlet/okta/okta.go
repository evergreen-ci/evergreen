package okta

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/usercache"
	"github.com/evergreen-ci/gimlet/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	jwtverifier "github.com/okta/okta-jwt-verifier-golang"
	"github.com/pkg/errors"
)

// CreationOptions specify the options to create the manager.
type CreationOptions struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Issuer       string
	// Scopes define the user information to request when authorizing the user.
	// See https://developer.okta.com/docs/reference/api/oidc/#access-token-scopes-and-claims
	Scopes []string

	UserGroup string
	// If set, user can be reauthorized without needing to authenticate.
	AllowReauthorization bool
	// If set, authentication and reauthorization will validate the group for
	// the user matches UserGroup. Otherwise, it simply checks that the user
	// attempting to reauthorize has the same name as that returned by the ID
	// token. This validation is only possible when the issuer returns group
	// information from its endpoints, which requires the application to have
	// permission to request them as part of the scopes.
	ValidateGroups bool

	CookiePath   string
	CookieDomain string
	CookieTTL    time.Duration

	LoginCookieName string
	LoginCookieTTL  time.Duration

	UserCache     usercache.Cache
	ExternalCache *usercache.ExternalOptions

	GetHTTPClient func() *http.Client
	PutHTTPClient func(*http.Client)

	// ReconciliateID is only used for the purposes of reconciliating existing
	// user IDs with their Okta IDs.
	ReconciliateID func(id string) (newID string)
}

func (opts *CreationOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(opts.ClientID == "", "must specify client ID")
	catcher.NewWhen(opts.ClientSecret == "", "must specify client secret")
	catcher.NewWhen(opts.RedirectURI == "", "must specify redirect URI")
	catcher.NewWhen(opts.Issuer == "", "must specify issuer")
	if opts.ValidateGroups {
		catcher.NewWhen(opts.UserGroup == "", "must specify user group")
	}
	catcher.NewWhen(opts.CookiePath == "", "must specify cookie path")
	catcher.NewWhen(opts.LoginCookieName == "", "must specify login cookie name")
	if opts.LoginCookieTTL == time.Duration(0) {
		opts.LoginCookieTTL = 365 * time.Hour
	}
	catcher.NewWhen(opts.UserCache == nil && opts.ExternalCache == nil, "must specify one user cache")
	catcher.NewWhen(opts.UserCache != nil && opts.ExternalCache != nil, "must specify exactly one user cache")
	catcher.NewWhen(opts.GetHTTPClient == nil, "must specify function to get HTTP clients")
	catcher.NewWhen(opts.PutHTTPClient == nil, "must specify function to put HTTP clients")
	if opts.CookieTTL == time.Duration(0) {
		opts.CookieTTL = time.Hour
	}
	if opts.ReconciliateID == nil {
		opts.ReconciliateID = func(id string) string { return id }
	}
	return catcher.Resolve()
}

type userManager struct {
	clientID     string
	clientSecret string
	redirectURI  string
	issuer       string
	scopes       []string

	userGroup string

	validateGroups       bool
	allowReauthorization bool

	// Injected functions for validating tokens. Should only be set for mocking
	// purposes.
	doValidateIDToken     func(string, string) (*jwtverifier.Jwt, error)
	doValidateAccessToken func(string) error

	cookiePath   string
	cookieDomain string
	cookieTTL    time.Duration

	loginCookieName string
	loginCookieTTL  time.Duration

	cache usercache.Cache

	getHTTPClient func() *http.Client
	putHTTPClient func(*http.Client)

	reconciliateID func(id string) (newID string)
}

// NewUserManager creates a manager that connects to Okta for user
// management services.
func NewUserManager(opts CreationOptions) (gimlet.UserManager, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid Okta manager options")
	}
	var cache usercache.Cache
	if opts.UserCache != nil {
		cache = opts.UserCache
	} else {
		var err error
		cache, err = usercache.NewExternal(*opts.ExternalCache)
		if err != nil {
			return nil, errors.Wrap(err, "problem creating external user cache")
		}
	}
	m := &userManager{
		cache:                cache,
		clientID:             opts.ClientID,
		clientSecret:         opts.ClientSecret,
		redirectURI:          opts.RedirectURI,
		issuer:               opts.Issuer,
		userGroup:            opts.UserGroup,
		scopes:               opts.Scopes,
		cookiePath:           opts.CookiePath,
		cookieDomain:         opts.CookieDomain,
		cookieTTL:            opts.CookieTTL,
		loginCookieName:      opts.LoginCookieName,
		loginCookieTTL:       opts.LoginCookieTTL,
		validateGroups:       opts.ValidateGroups,
		allowReauthorization: opts.AllowReauthorization,
		getHTTPClient:        opts.GetHTTPClient,
		putHTTPClient:        opts.PutHTTPClient,
		reconciliateID:       opts.ReconciliateID,
	}
	m.doValidateIDToken = m.validateIDToken
	m.doValidateAccessToken = m.validateAccessToken
	return m, nil
}

func (m *userManager) GetUserByToken(ctx context.Context, token string) (gimlet.User, error) {
	user, valid, err := m.cache.Get(token)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting cached user")
	}
	if user == nil {
		return nil, errors.New("user not found in cache")
	}
	if !valid {
		if m.allowReauthorization {
			if err := m.ReauthorizeUser(user); err != nil {
				return user, gimlet.ErrNeedsReauthentication
			}
			return user, nil
		}
		return user, gimlet.ErrNeedsReauthentication
	}
	return user, nil
}

// reauthorizeGroup attempts to reauthorize the user based on their groups.
func (m *userManager) reauthorizeGroup(accessToken, refreshToken string) error {
	catcher := grip.NewBasicCatcher()
	err := m.doValidateAccessToken(accessToken)
	catcher.Wrap(err, "invalid access token")
	if err == nil {
		user, err := m.generateUserFromInfo(accessToken, refreshToken)
		catcher.Wrap(err, "could not generate user from Okta user info")
		if err == nil {
			_, err = m.cache.Put(user)
			catcher.Wrap(err, "could not update reauthorized user in cache")
			if err == nil {
				return nil
			}
		}
	}
	return catcher.Resolve()
}

// reauthorizeID attempts to reauthorize the user based on their ID token.
// Reauthorization succeeds if the username derived from their ID token matches
// the given username.
func (m *userManager) reauthorizeID(username string, tokens *tokenResponse) error {
	idToken, err := m.doValidateIDToken(tokens.IDToken, "")
	if err != nil {
		return errors.Wrap(err, "invalid ID token")
	}
	email, ok := idToken.Claims["email"].(string)
	if ok {
		checkUsername := m.reconciliateID(email)
		if checkUsername != username {
			return errors.Errorf("user name from ID token '%s' did not match user '%s'", checkUsername, username)
		}
	} else {
		return errors.New("ID token is missing email claim")
	}
	catcher := grip.NewBasicCatcher()
	user, err := makeUserFromIDToken(idToken, tokens.AccessToken, tokens.RefreshToken, m.reconciliateID)
	catcher.Wrap(err, "could not generate user from Okta ID token")
	if err == nil {
		_, err = m.cache.Put(user)
		catcher.Wrapf(err, "could not update reauthorized user in cache")
		if err == nil {
			return nil
		}
	}
	return catcher.Resolve()
}

// ReauthorizeUser attempts to reauthorize the user.
// Reauthorization works as follows: if the user manager is configured to
// validate their groups, it tries checking their user groups using their access
// token. Otherwise, it just validates that the ID token user matches the
// current user.
// If reauthorization fails or the user manager is configured to always refresh
// the user's tokens that fails, it refreshes the tokens and attempts to
// reauthorize them again.
func (m *userManager) ReauthorizeUser(user gimlet.User) error {
	refreshToken := user.GetRefreshToken()
	catcher := grip.NewBasicCatcher()

	// This is just an optimization to try using the access token without
	// needing to make a request to refresh the tokens. Validating the groups
	// may fail if the access token is expired, in which case it has to refresh
	// the access token and try again.
	if m.validateGroups {
		accessToken := user.GetAccessToken()
		if accessToken == "" {
			return errors.Errorf("user '%s' cannot reauthorize because user is missing access token", user.Username())
		}
		err := m.reauthorizeGroup(accessToken, refreshToken)
		catcher.Wrap(err, "could not reauthorize user with current access token")
		if err == nil {
			return nil
		}
	}

	// Refresh the tokens and reauthenticate.
	if refreshToken == "" {
		return errors.Errorf("user '%s' cannot refresh tokens because refresh token is missing", user.Username())
	}
	tokens, err := m.refreshTokens(context.Background(), refreshToken)
	catcher.Wrap(err, "refreshing authorization tokens")
	if err == nil {
		if m.validateGroups {
			err = m.reauthorizeGroup(tokens.AccessToken, tokens.RefreshToken)
			catcher.Wrap(err, "reauthorizing user groups user after refreshing tokens")
			if err == nil {
				return nil
			}
		} else {
			err = m.reauthorizeID(user.Username(), tokens)
			catcher.Wrap(err, "reauthorizing user ID token after refreshing tokens")
			if err == nil {
				return nil
			}
		}
	}

	return catcher.Resolve()
}

// validateGroup checks that the user groups returned for this access token
// contains the expected user group.
func (m *userManager) validateGroup(groups []string) error {
	for _, group := range groups {
		if group == m.userGroup {
			return nil
		}
	}
	return errors.New("user is not in a valid group")
}

func (m *userManager) CreateUserToken(user string, password string) (string, error) {
	return "", errors.New("creating user tokens is not supported for Okta")
}

const (
	nonceCookieName      = "okta-nonce"
	stateCookieName      = "okta-state"
	requestURICookieName = "okta-original-request-uri"
)

func (m *userManager) GetLoginHandler(_ string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nonce, err := util.RandomString()
		if err != nil {
			err = errors.Wrap(err, "could not get login handler")
			grip.Critical(message.WrapError(err, message.Fields{
				"reason": "nonce could not be generated",
			}))
			writeError(w, err)
			return
		}
		state, err := util.RandomString()
		if err != nil {
			err = errors.Wrap(err, "could not get login handler")
			grip.Critical(message.WrapError(err, message.Fields{
				"reason": "state could not be generated",
			}))
			writeError(w, err)
			return
		}

		q := r.URL.Query()
		redirectURI := q.Get("redirect")
		if redirectURI == "" {
			redirectURI = "/"
		}

		// Allow users to silently reauthenticate as long as the request
		// presents a cookie containing a login token associated with an
		// existing user.
		var canSilentReauth bool
		var user gimlet.User
		for _, cookie := range r.Cookies() {
			if cookie.Name == m.loginCookieName {
				loginToken, err := url.QueryUnescape(cookie.Value)
				if err != nil {
					grip.Warning(errors.Wrapf(err, "could not decode login cookie '%s'", cookie.Value))
				}
				user, err = m.GetUserByToken(context.Background(), loginToken)
				if (err == nil || errors.Cause(err) == gimlet.ErrNeedsReauthentication) && user != nil {
					canSilentReauth = true
					break
				}
			}
		}
		var userName string
		if user != nil {
			userName = user.Username()
		}

		m.setTemporaryCookie(w, nonceCookieName, nonce)
		m.setTemporaryCookie(w, stateCookieName, state)
		m.setTemporaryCookie(w, requestURICookieName, redirectURI)
		grip.Debug(message.Fields{
			"message": "storing nonce and state cookies",
			"nonce":   nonce,
			"state":   state,
			"user":    userName,
			"context": "Okta",
		})

		q.Add("client_id", m.clientID)
		q.Add("response_type", "code")
		q.Add("response_mode", "query")
		q.Add("scope", m.requestScopes())
		if !canSilentReauth {
			q.Add("prompt", "login consent")
		}
		q.Add("redirect_uri", m.redirectURI)
		q.Add("state", state)
		q.Add("nonce", nonce)

		r.Header.Add("Cache-Control", "no-cache,no-store")
		http.Redirect(w, r, fmt.Sprintf("%s/v1/authorize?%s", m.issuer, q.Encode()), http.StatusFound)
	}
}

func (m *userManager) setLoginCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.loginCookieName,
		Path:     m.cookiePath,
		Value:    value,
		HttpOnly: true,
		Expires:  time.Now().Add(m.loginCookieTTL),
		Domain:   m.cookieDomain,
	})
}

// setTemporaryCookie sets a short-lived cookie that is required for login to
// succeed via Okta.
func (m *userManager) setTemporaryCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Path:     m.cookiePath,
		Value:    value,
		HttpOnly: true,
		Expires:  time.Now().Add(m.cookieTTL),
		Domain:   m.cookieDomain,
	})
}

// unsetTemporaryCookie unsets a temporary cookie used for login.
func (m *userManager) unsetTemporaryCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:   name,
		Path:   m.cookiePath,
		Domain: m.cookieDomain,
		Value:  "",
		MaxAge: -1,
	})
}

func writeError(w http.ResponseWriter, err error) {
	gimlet.WriteResponse(w, gimlet.MakeTextErrorResponder(gimlet.ErrorResponse{
		StatusCode: http.StatusInternalServerError,
		Message:    err.Error(),
	}))
}

func (m *userManager) GetLoginCallbackHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if errCode := r.URL.Query().Get("error"); errCode != "" {
			desc := r.URL.Query().Get("error_description")
			err := fmt.Errorf("%s: %s", errCode, desc)
			err = errors.Wrap(err, "callback handler received error from Okta")
			grip.Error(err)
			writeError(w, err)
			return
		}

		nonce, state, requestURI, err := getCookies(r)
		if err != nil {
			err = errors.Wrap(err, "failed to get Okta nonce and state from user")
			grip.Error(err)
			writeError(w, err)
			return
		}
		checkState := r.URL.Query().Get("state")
		if state != checkState {
			err = errors.New("state value received from Okta did not match expected state")
			grip.Error(message.WrapError(err, message.Fields{
				"expected_state": state,
				"actual_state":   checkState,
			}))
			writeError(w, err)
			return
		}

		tokens, idToken, err := m.getUserTokens(r.URL.Query().Get("code"), nonce)
		if err != nil {
			writeError(w, err)
			return
		}

		var user gimlet.User
		if m.validateGroups {
			user, err = m.generateUserFromInfo(tokens.AccessToken, tokens.RefreshToken)
			if err != nil {
				grip.Error(err)
				writeError(w, err)
				return
			}
		} else {
			user, err = makeUserFromIDToken(idToken, tokens.AccessToken, tokens.RefreshToken, m.reconciliateID)
			if err != nil {
				grip.Error(err)
				writeError(w, err)
				return
			}
		}

		user, err = m.GetOrCreateUser(user)
		if err != nil {
			err = errors.Wrap(err, "failed to get or create cached user")
			grip.Error(err)
			gimlet.MakeTextErrorResponder(err)
			return
		}

		loginToken, err := m.cache.Put(user)
		if err != nil {
			err = errors.Wrapf(err, "failed to cache user %s", user.Username())
			grip.Error(err)
			writeError(w, err)
			return
		}

		m.unsetTemporaryCookie(w, nonceCookieName)
		m.unsetTemporaryCookie(w, stateCookieName)
		m.unsetTemporaryCookie(w, requestURICookieName)
		m.setLoginCookie(w, loginToken)

		http.Redirect(w, r, requestURI, http.StatusFound)
	}
}

// getUserTokens redeems the authorization code for tokens and validates the
// received tokens.
func (m *userManager) getUserTokens(code, nonce string) (*tokenResponse, *jwtverifier.Jwt, error) {
	tokens, err := m.exchangeCodeForTokens(context.Background(), code)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not redeem authorization code for tokens")
	}
	idToken, err := m.doValidateIDToken(tokens.IDToken, nonce)
	if err != nil {
		return tokens, nil, errors.Wrap(err, "invalid ID token from Okta")
	}
	if !m.validateGroups {
		return tokens, idToken, nil
	}
	if err := m.doValidateAccessToken(tokens.AccessToken); err != nil {
		return tokens, idToken, errors.Wrap(err, "invalid access token from Okta")
	}
	return tokens, idToken, nil
}

// generateUserFromInfo creates a user based on information from the userinfo
// endpoint.
func (m *userManager) generateUserFromInfo(accessToken, refreshToken string) (gimlet.User, error) {
	userInfo, err := m.getUserInfo(context.Background(), accessToken)
	if err != nil {
		return nil, errors.Wrap(err, "could not retrieve user info from Okta")
	}
	if m.validateGroups {
		if err := m.validateGroup(userInfo.Groups); err != nil {
			err = errors.Wrap(err, "could not authorize user")
			grip.Error(message.WrapError(err, message.Fields{
				"expected_group": m.userGroup,
				"actual_groups":  userInfo.Groups,
			}))
			return nil, err
		}
	}
	return makeUserFromInfo(userInfo, accessToken, refreshToken, m.reconciliateID)
}

// getCookies gets the nonce and the state required in the redirect callback as
// well as the originally requested URI from the cookies.
func getCookies(r *http.Request) (nonce, state, requestURI string, err error) {
	catcher := grip.NewBasicCatcher()
	for _, cookie := range r.Cookies() {
		if cookie.Name == nonceCookieName {
			nonce, err = url.QueryUnescape(cookie.Value)
			if err != nil {
				catcher.Wrap(err, "could not decode nonce cookie")
			}
		}
		if cookie.Name == stateCookieName {
			state, err = url.QueryUnescape(cookie.Value)
			if err != nil {
				catcher.Wrap(err, "could not decode state cookie")
			}
		}
		if cookie.Name == requestURICookieName {
			requestURI, err = url.QueryUnescape(cookie.Value)
			if err != nil {
				catcher.Wrap(err, "could not decode requestURI cookie")
			}
		}
	}
	catcher.NewWhen(nonce == "", "nonce could not be retrieved from cookies")
	catcher.NewWhen(state == "", "state could not be retrieved from cookies")
	grip.NoticeWhen(requestURI == "", "request URI could not be retrieved from cookies")
	if requestURI == "" {
		requestURI = "/"
	}
	return nonce, state, requestURI, catcher.Resolve()
}

func (m *userManager) IsRedirect() bool { return true }

func (m *userManager) GetUserByID(id string) (gimlet.User, error) {
	user, valid, err := m.cache.Find(id)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting user by ID")
	}
	if user == nil {
		return nil, errors.New("user not found in cache")
	}
	if !valid {
		if m.allowReauthorization {
			if err := m.ReauthorizeUser(user); err != nil {
				return user, gimlet.ErrNeedsReauthentication
			}
			return user, nil
		}
		return user, gimlet.ErrNeedsReauthentication
	}
	return user, nil
}

func (m *userManager) GetOrCreateUser(user gimlet.User) (gimlet.User, error) {
	return m.cache.GetOrCreate(user)
}

func (m *userManager) ClearUser(user gimlet.User, all bool) error {
	return m.cache.Clear(user, all)
}

func (m *userManager) GetGroupsForUser(user string) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (m *userManager) addAuthHeader(r *http.Request) {
	r.Header.Add("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", m.clientID, m.clientSecret))))
}

// validateIDToken verifies that the ID token is valid and returns it.
func (m *userManager) validateIDToken(token, nonce string) (*jwtverifier.Jwt, error) {
	verifier := jwtverifier.JwtVerifier{
		Issuer: m.issuer,
		ClaimsToValidate: map[string]string{
			"aud":   m.clientID,
			"nonce": nonce,
		},
	}
	return verifier.New().VerifyIdToken(token)
}

// validateAccessToken verifies that the access token is valid.
// TODO (kim): figure out why this does not validate with the same jwt verifier
// library as used for the ID token.
func (m *userManager) validateAccessToken(token string) error {
	info, err := m.getTokenInfo(context.Background(), token, "access_token")
	if err != nil {
		return errors.Wrap(err, "could not check if token is valid")
	}
	if !info.Active || info.ExpiresUnix != 0 && int64(info.ExpiresUnix) <= time.Now().Unix() {
		return errors.New("access token is inactive, so authorization is not possible")
	}
	return nil
}

// tokenResponse represents a response received from the token endpoint.
type tokenResponse struct {
	responseError
	AccessToken  string `json:"access_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// responseError is the common section of the Okta endpoint responses that
// contains error information.
type responseError struct {
	ErrorCode        string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

func (re *responseError) Error() string {
	var msg string
	if re.ErrorCode != "" {
		msg += re.ErrorCode
		if re.ErrorDescription != "" {
			msg += ": "
		}
	}
	if re.ErrorDescription != "" {
		msg += re.ErrorDescription
	}
	return msg
}

// exchangeCodeForTokens exchanges the given code to redeem tokens from the
// token endpoint.
func (m *userManager) exchangeCodeForTokens(ctx context.Context, code string) (*tokenResponse, error) {
	q := url.Values{}
	q.Set("grant_type", "authorization_code")
	q.Set("code", code)
	q.Set("redirect_uri", m.redirectURI)
	return m.redeemTokens(ctx, q.Encode())
}

// refreshTokens exchanges the given refresh token to redeem tokens from the
// token endpoint.
func (m *userManager) refreshTokens(ctx context.Context, refreshToken string) (*tokenResponse, error) {
	q := url.Values{}
	q.Set("grant_type", "refresh_token")
	q.Set("refresh_token", refreshToken)
	q.Set("scope", m.requestScopes())
	return m.redeemTokens(ctx, q.Encode())
}

// requestScopes returns the necessary scopes that Okta must return.
func (m *userManager) requestScopes() string {
	return strings.Join(m.scopes, " ")
}

// redeemTokens sends the request to redeem tokens with the required client
// credentials.
func (m *userManager) redeemTokens(ctx context.Context, query string) (*tokenResponse, error) {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/v1/token", m.issuer), bytes.NewBufferString(query))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	req = req.WithContext(ctx)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Connection", "close")
	m.addAuthHeader(req)

	client := m.getHTTPClient()
	defer m.putHTTPClient(client)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	start := time.Now()
	resp, err := client.Do(req)
	grip.Info(message.Fields{
		"endpoint":    "token",
		"duration_ms": int64(time.Since(start) / time.Millisecond),
		"context":     "Okta user manager",
	})
	if err != nil {
		return nil, errors.Wrap(err, "request to redeem token returned error")
	}
	tokens := &tokenResponse{}
	return tokens, readResp(resp, tokens)
}

// userInfo represents a response received from the userinfo endpoint.
type userInfoResponse struct {
	responseError
	Name   string   `json:"name"`
	Email  string   `json:"email"`
	Groups []string `json:"groups"`
}

// getUserInfo uses the access token to retrieve user information from the
// userinfo endpoint.
func (m *userManager) getUserInfo(ctx context.Context, accessToken string) (*userInfoResponse, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/userinfo", m.issuer), nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	req = req.WithContext(ctx)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", "Bearer "+accessToken)
	req.Header.Add("Connection", "close")

	client := m.getHTTPClient()
	defer m.putHTTPClient(client)
	start := time.Now()
	resp, err := client.Do(req)
	grip.Info(message.Fields{
		"endpoint":    "userinfo",
		"duration_ms": int64(time.Since(start) / time.Millisecond),
		"context":     "Okta user manager",
	})
	if err != nil {
		return nil, errors.Wrap(err, "error during request for user info")
	}
	userInfo := &userInfoResponse{}
	return userInfo, readResp(resp, userInfo)
}

// introspectResponse represents a response received from the introspect
// endpoint.
type introspectResponse struct {
	responseError
	Active          bool   `json:"active,omitempty"`
	Audience        string `json:"aud,omitempty"`
	ClientID        string `json:"client_id"`
	DeviceID        string `json:"device_id"`
	ExpiresUnix     int    `json:"exp"`
	IssuedAtUnix    int    `json:"iat"`
	Issuer          string `json:"iss"`
	TokenIdentifier string `json:"jti"`
	NotBeforeUnix   int    `json:"nbf"`
	Scopes          string `json:"scope"`
	Subject         string `json:"sub"`
	TokenType       string `json:"token_type"`
	UserID          string `json:"uid"`
	UserName        string `json:"username"`
}

// getTokenInfo returns information about the given token.
func (m *userManager) getTokenInfo(ctx context.Context, token, tokenType string) (*introspectResponse, error) {
	q := url.Values{}
	q.Add("token", token)
	q.Add("token_type_hint", tokenType)
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/v1/introspect", m.issuer), strings.NewReader(q.Encode()))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	req = req.WithContext(ctx)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Connection", "close")
	m.addAuthHeader(req)

	client := m.getHTTPClient()
	defer m.putHTTPClient(client)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	start := time.Now()
	resp, err := client.Do(req)
	grip.Info(message.Fields{
		"endpoint":    "introspect",
		"duration_ms": int64(time.Since(start) / time.Millisecond),
		"context":     "Okta user manager",
	})
	if err != nil {
		return nil, errors.Wrap(err, "request to introspect token returned error")
	}
	tokenInfo := &introspectResponse{}
	return tokenInfo, readResp(resp, tokenInfo)
}

// readResp verifies that an Okta response is OK and reads an Okta response body
// in JSON format to the given output out.
func readResp(resp *http.Response, out error) error {
	catcher := grip.NewBasicCatcher()
	if resp.StatusCode != http.StatusOK {
		catcher.Errorf("unexpected status code %d", resp.StatusCode)
	}
	if err := gimlet.GetJSONUnlimited(resp.Body, out); err != nil {
		catcher.Wrap(err, "reading JSON response body")
		return catcher.Resolve()
	}

	if errMsg := out.Error(); len(errMsg) != 0 {
		catcher.New(errMsg)
	}
	return catcher.Resolve()
}

// makeUserFromInfo returns a user based on information from a userinfo request.
func makeUserFromInfo(info *userInfoResponse, accessToken, refreshToken string, reconciliateID func(string) string) (gimlet.User, error) {
	id := info.Email
	if reconciliateID != nil {
		id = reconciliateID(id)
	}
	opts, err := gimlet.NewBasicUserOptions(id)
	if err != nil {
		return nil, errors.Wrap(err, "could not create user")
	}
	return gimlet.NewBasicUser(opts.Name(info.Name).Email(info.Email).AccessToken(accessToken).RefreshToken(refreshToken).Roles(info.Groups...)), nil
}

// makeUserFromIDToken returns a user based on information from an ID token.
func makeUserFromIDToken(idToken *jwtverifier.Jwt, accessToken, refreshToken string, reconciliateID func(string) string) (gimlet.User, error) {
	email, ok := idToken.Claims["email"].(string)
	if !ok {
		return nil, errors.New("user is missing email")
	}
	id := email
	if reconciliateID != nil {
		id = reconciliateID(id)
	}
	if id == "" {
		return nil, errors.New("could not create user ID from email")
	}
	name, ok := idToken.Claims["name"].(string)
	if !ok {
		return nil, errors.New("user is missing name")
	}
	opts, err := gimlet.NewBasicUserOptions(id)
	if err != nil {
		return nil, errors.Wrap(err, "could not create user")
	}
	return gimlet.NewBasicUser(opts.Name(name).Email(email).AccessToken(accessToken).RefreshToken(refreshToken)), nil
}
