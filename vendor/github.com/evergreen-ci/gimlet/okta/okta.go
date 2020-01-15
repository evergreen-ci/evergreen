package okta

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	verifier "github.com/okta/okta-jwt-verifier-golang"
	"github.com/pkg/errors"
)

// CreationOptions specify the options to create the manager.
type CreationOptions struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Issuer       string
	CookiePath   string
	CookieDomain string
	CookieTTL    time.Duration
}

func (opts CreationOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(opts.ClientID == "", "must specify client ID")
	catcher.NewWhen(opts.ClientSecret == "", "must specify client secret")
	catcher.NewWhen(opts.RedirectURI == "", "must specify redirect URI")
	catcher.NewWhen(opts.Issuer == "", "must specify issuer")
	catcher.NewWhen(opts.CookiePath == "", "must specify cookie path")
	catcher.NewWhen(opts.CookieDomain == "", "must specify cookie domain")
	if opts.CookieTTL == time.Duration(0) {
		opts.CookieTTL = time.Hour
	}
	return catcher.Resolve()
}

type userManager struct {
	clientID     string
	clientSecret string
	redirectURI  string
	issuer       string
	cookiePath   string
	cookieDomain string
	cookieTTL    time.Duration

	// TODO (kim): token caching functions
}

// NewUserManager creates a manager that connects to Okta for user
// management services.
func NewUserManager(opts CreationOptions) (gimlet.UserManager, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid Okta manager options")
	}
	m := &userManager{
		clientID:     opts.ClientID,
		clientSecret: opts.ClientSecret,
		redirectURI:  opts.RedirectURI,
		issuer:       opts.Issuer,
		cookiePath:   opts.CookiePath,
		cookieDomain: opts.CookieDomain,
		cookieTTL:    opts.CookieTTL,
	}
	return m, nil
}

func (m *userManager) GetUserByToken(ctx context.Context, token string) (gimlet.User, error) {
	return nil, errors.New("not implemented")
}

func (m *userManager) CreateUserToken(user string, password string) (string, error) {
	return "", errors.New("not implemented")
}

const (
	nonceCookieName = "okta-nonce"
	stateCookieName = "okta-state"
)

func (m *userManager) GetLoginHandler(callbackURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO (kim): persist nonce and state.
		nonce, err := util.RandomString()
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not get login handler",
			}))
			gimlet.WriteResponse(w, gimlet.MakeTextErrorResponder(errors.Wrap(err, "could not get login handler")))
			return
		}
		state, err := util.RandomString()
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not get login handler",
			}))
			gimlet.WriteResponse(w, gimlet.MakeTextErrorResponder(errors.Wrap(err, "could not get login handler")))
			return
		}

		q := r.URL.Query()
		q.Add("client_id", m.clientID)
		q.Add("response_type", "code")
		q.Add("response_mode", "query")
		q.Add("scope", "openid")
		q.Add("redirect_uri", m.redirectURI)
		q.Add("state", state)
		q.Add("nonce", nonce)

		http.SetCookie(w, &http.Cookie{
			Name:     nonceCookieName,
			Path:     m.cookiePath,
			Value:    nonce,
			HttpOnly: true,
			Expires:  time.Now().Add(m.cookieTTL),
			Domain:   m.cookieDomain,
		})
		http.SetCookie(w, &http.Cookie{
			Name:     stateCookieName,
			Path:     m.cookiePath,
			Value:    state,
			HttpOnly: true,
			Expires:  time.Now().Add(m.cookieTTL),
			Domain:   m.cookieDomain,
		})

		http.Redirect(w, r, fmt.Sprintf("%s/oauth2/v1/authorize?%s", m.issuer, q.Encode()), http.StatusMovedPermanently)
	}
}

func (m *userManager) GetLoginCallbackHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var nonce, state string
		for _, cookie := range r.Cookies() {
			var err error
			if cookie.Name == nonceCookieName {
				nonce, err = url.QueryUnescape(cookie.Value)
				if err != nil {
					err = errors.Wrap(err, "could not get Okta cookie for nonce")
					grip.Error(err)
					gimlet.WriteResponse(w, gimlet.MakeTextErrorResponder(err))
					return
				}
			}
			if cookie.Name == stateCookieName {
				state, err = url.QueryUnescape(cookie.Value)
				if err != nil {
					err = errors.Wrap(err, "could not get Okta cookie for state")
					grip.Error(err)
					gimlet.WriteResponse(w, gimlet.MakeTextErrorResponder(err))
					return
				}
			}
		}
		checkState := r.URL.Query().Get("state")

		if state != checkState {
			grip.Error(message.Fields{
				"message":        "mismatched states during authentication",
				"context":        "Okta",
				"expected_state": state,
				"actual_state":   checkState,
			})
			gimlet.WriteResponse(w, gimlet.MakeTextErrorResponder(errors.New("mismatched states during authentication")))
			return
		}

		if errCode := r.URL.Query().Get("error"); errCode != "" {
			desc := r.URL.Query().Get("error_description")
			err := fmt.Errorf("%s: %s", errCode, desc)
			grip.Error(message.WrapError(errors.WithStack(err), message.Fields{
				"message": "failure in callback handler redirect",
				"op":      "GetLoginCallbackHandler",
				"auth":    "Okta",
			}))
			gimlet.WriteResponse(w, gimlet.MakeTextErrorResponder(errors.Wrap(err, "could not get login callback handler")))
			return
		}

		resp, err := m.getToken(r.URL.Query().Get("code"))
		if err != nil {
			err = errors.Wrap(err, "could not get ID token")
			gimlet.WriteResponse(w, gimlet.MakeTextErrorResponder(err))
			grip.Error(message.WrapError(err, message.Fields{
				"message": "failed to get token from Okta",
				"op":      "GetLoginCallbackHandler",
				"auth":    "Okta",
			}))
			return
		}
		if err := m.validateToken(resp.IDToken, nonce); err != nil {
			err = errors.Wrap(err, "could not validate ID token from Okta")
			gimlet.WriteResponse(w, gimlet.MakeTextErrorResponder(err))
			grip.Error(message.WrapError(err, message.Fields{
				"message": "failed to validate ID token",
				"op":      "GetLoginCallbackHandler",
				"auth":    "Okta",
			}))
		}
		// TODO (kim): persist token.
		grip.Info(message.Fields{
			"message":  "successfully authenticated user and validated ID token",
			"op":       "GetLoginCallbackHandler",
			"context":  "Okta",
			"response": fmt.Sprintf("%+v", resp),
		})
		http.Redirect(w, r, m.redirectURI, http.StatusFound)
	}
}

// getToken exchanges the given code to redeem tokens from the endpoint.
func (m *userManager) getToken(code string) (*authResponse, error) {
	q := url.Values{}
	q.Set("grant_type", "authorization_code")
	q.Set("code", code)
	q.Set("redirect_uri", m.redirectURI)
	resp, err := m.doRequest(context.Background(), http.MethodPost, fmt.Sprintf("%s/oauth2/v1/token?%s", m.issuer, q.Encode()), nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	authResp := &authResponse{}
	if err := gimlet.GetJSONUnlimited(resp.Body, authResp); err != nil {
		return nil, errors.WithStack(err)
	}
	if authResp.ErrorCode != "" {
		return authResp, errors.Errorf("%s: %s", authResp.ErrorCode, authResp.ErrorDescription)
	}
	return authResp, nil
}

func (m *userManager) validateToken(token, nonce string) error {
	validator := verifier.JwtVerifier{
		Issuer: m.issuer,
		ClaimsToValidate: map[string]string{
			"nonce": nonce,
			"aud":   m.clientID,
		},
	}
	res, err := validator.New().VerifyIdToken(token)
	if err != nil {
		return errors.Wrap(err, "could not verify ID token")
	}
	if res == nil {
		return errors.New("token validation returned empty result")
	}
	return nil
}

func (m *userManager) IsRedirect() bool { return true }

func (m *userManager) GetUserByID(user string) (gimlet.User, error) {
	return nil, errors.New("not implemented")
}

func (m *userManager) GetOrCreateUser(user gimlet.User) (gimlet.User, error) {
	return nil, errors.New("not implemented")
}

func (m *userManager) ClearUser(user gimlet.User, all bool) error {
	return errors.New("not implemented")
}

func (m *userManager) GetGroupsForUser(user string) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (m *userManager) client() (*http.Client, error) {
	// TODO (kim): need to acquire an HTTP client at this point but this should
	// come from the application HTTP client pool.
	return &http.Client{}, nil
}

type authResponse struct {
	AccessToken      string `json:"access_token,omitempty"`
	TokenType        string `json:"token_type,omitempty"`
	ExpiresIn        int    `json:"expires_in,omitempty"`
	Scope            string `json:"scope,omitempty"`
	IDToken          string `json:"id_token,omitempty"`
	ErrorCode        string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// doRequest sends the request with the required client credentials.
func (m *userManager) doRequest(ctx context.Context, method string, url string, data interface{}) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	req = req.WithContext(ctx)
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		req.Body = ioutil.NopCloser(bytes.NewReader(b))
		req.Header.Add("Content-Length", strconv.Itoa(len(b)))
	} else {
		req.Header.Add("Content-Length", "0")
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "application/json")
	authHeader := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", m.clientID, m.clientSecret)))
	req.Header.Add("Authorization", fmt.Sprintf("Basic "+authHeader))
	req.Header.Add("Connection", "close")

	client, err := m.client()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return client.Do(req)
}
