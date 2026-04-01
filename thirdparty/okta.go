package thirdparty

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

// TokenTypeAccessToken is the RFC 8693 URN for an OAuth access token used as subject_token_type.
const TokenTypeAccessToken = "urn:ietf:params:oauth:token-type:access_token"

const (
	tokenExchangeGrantType = "urn:ietf:params:oauth:grant-type:token-exchange"

	// oauthASMetadataPath is the well-known path for OAuth 2.0 Authorization
	// Server Metadata (RFC 8414).
	oauthASMetadataPath = "/.well-known/oauth-authorization-server"
)

// cachedTokenURL stores the discovered token endpoint so discovery
// doesn't need to run on every call.
var cachedTokenURL atomic.Value

// oauthASMetadata represents the relevant fields from an OAuth 2.0
// Authorization Server Metadata response (RFC 8414).
type oauthASMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

func getOktaCachedTokenURL() string {
	v, _ := cachedTokenURL.Load().(string)
	return v
}

// OktaTokenExchangeOptions configures the parameters for an OAuth 2.0
// Token Exchange (RFC 8693) against an Okta authorization server.
type OktaTokenExchangeOptions struct {
	// Issuer is the OAuth 2.0 Authorization Server issuer URL. Used to
	// discover the token endpoint via RFC 8414 metadata when the process-wide
	// token URL cache is empty.
	Issuer string
	// ClientID is the OAuth2 client identifier.
	ClientID string
	// ClientSecret is the OAuth2 client secret.
	ClientSecret string
	// SubjectToken is the existing token to exchange.
	SubjectToken string
	// SubjectTokenType identifies the type of SubjectToken (RFC 8693), e.g. TokenTypeAccessToken.
	SubjectTokenType string
	// Audience is the target audience for the requested token. Optional.
	Audience string
	// Scopes is the set of scopes to request on the new token. Optional.
	Scopes []string
}

// tokenExchangeResponse represents the JSON response from an RFC 8693 token
// exchange as defined in Section 2.2 of the spec.
type tokenExchangeResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

type tokenExchangeErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// ExchangeOktaToken performs an OAuth 2.0 Token Exchange (RFC 8693) against
// Okta. It resolves the token endpoint from the process-wide cache or via
// OAuth Authorization Server Metadata discovery (RFC 8414), then exchanges the
// provided subject token for a new token using client credentials.
func ExchangeOktaToken(ctx context.Context, opts OktaTokenExchangeOptions) (*oauth2.Token, error) {
	if err := opts.validate(); err != nil {
		return nil, errors.Wrap(err, "invalid token exchange options")
	}

	tokenURL, err := opts.resolveTokenURL(ctx)
	if err != nil {
		return nil, err
	}

	return doTokenExchange(ctx, tokenURL, opts)
}

func (o *OktaTokenExchangeOptions) validate() error {
	if o.ClientID == "" {
		return errors.New("client ID is required")
	}
	if o.ClientSecret == "" {
		return errors.New("client secret is required")
	}
	if o.SubjectToken == "" {
		return errors.New("subject token is required")
	}
	if o.SubjectTokenType == "" {
		return errors.New("subject token type is required")
	}
	return nil
}

// resolveTokenURL returns the token endpoint. It checks, in order:
//  1. The process-wide cached value (from a prior discovery)
//  2. OAuth 2.0 AS Metadata discovery on the Issuer (result is cached for subsequent calls)
func (o *OktaTokenExchangeOptions) resolveTokenURL(ctx context.Context) (string, error) {
	if cached := getOktaCachedTokenURL(); cached != "" {
		return cached, nil
	}

	metadata, err := discoverOAuthASMetadata(ctx, o.Issuer)
	if err != nil {
		return "", err
	}

	if metadata.TokenEndpoint == "" {
		return "", fmt.Errorf("OAuth AS metadata for issuer '%s' returned no token endpoint", o.Issuer)
	}

	cachedTokenURL.Store(metadata.TokenEndpoint)

	return metadata.TokenEndpoint, nil
}

// discoverOAuthASMetadata fetches the OAuth 2.0 Authorization Server Metadata
// document (RFC 8414) from {issuer}/.well-known/oauth-authorization-server.
func discoverOAuthASMetadata(ctx context.Context, issuer string) (*oauthASMetadata, error) {
	metadataURL := strings.TrimRight(issuer, "/") + oauthASMetadataPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "building OAuth AS metadata request for issuer '%s'", issuer)
	}
	req.Header.Set("Accept", "application/json")

	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "fetching OAuth AS metadata for issuer '%s'", issuer)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading OAuth AS metadata response")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("OAuth AS metadata request for issuer '%s' returned HTTP %d: %s", issuer, resp.StatusCode, string(body))
	}

	var metadata oauthASMetadata
	if err = json.Unmarshal(body, &metadata); err != nil {
		return nil, errors.Wrap(err, "unmarshalling OAuth AS metadata response")
	}

	return &metadata, nil
}

// doTokenExchange posts the RFC 8693 token exchange request and parses the
// response into an oauth2.Token.
func doTokenExchange(ctx context.Context, tokenURL string, opts OktaTokenExchangeOptions) (*oauth2.Token, error) {
	form := url.Values{
		"grant_type":         {tokenExchangeGrantType},
		"subject_token":      {opts.SubjectToken},
		"subject_token_type": {opts.SubjectTokenType},
	}
	if opts.Audience != "" {
		form.Set("audience", opts.Audience)
	}
	if len(opts.Scopes) > 0 {
		form.Set("scope", strings.Join(opts.Scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, errors.Wrap(err, "building token exchange request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(opts.ClientID, opts.ClientSecret)

	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "sending token exchange request")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading token exchange response body")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseTokenExchangeError(resp.StatusCode, body)
	}

	var tokenResp tokenExchangeResponse
	if err = json.Unmarshal(body, &tokenResp); err != nil {
		return nil, errors.Wrap(err, "unmarshalling token exchange response")
	}

	token := &oauth2.Token{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		RefreshToken: tokenResp.RefreshToken,
	}
	if tokenResp.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return token, nil
}

func parseTokenExchangeError(statusCode int, body []byte) error {
	var errResp tokenExchangeErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != "" {
		return errors.Errorf("token exchange failed (HTTP %d): %s: %s",
			statusCode, errResp.Error, errResp.ErrorDescription)
	}
	return errors.Errorf("token exchange failed with HTTP %d: %s", statusCode, string(body))
}

// OktaAuthorizeOptions configures the parameters for building an OAuth 2.0
// Authorization Code + PKCE authorization URL.
type OktaAuthorizeOptions struct {
	Issuer      string
	ClientID    string
	RedirectURI string
	Scopes      []string
}

// OktaAuthorizeResult contains the authorization URL to redirect the user to,
// along with the state and PKCE code verifier generated for this request.
type OktaAuthorizeResult struct {
	AuthorizeURL string
	State        string
	CodeVerifier string
}

// BuildAuthorizeURL discovers the Okta authorization endpoint, generates PKCE
// and state parameters, and returns the full authorize URL along with the
// state and code verifier that the caller should persist for the callback.
func BuildAuthorizeURL(ctx context.Context, opts OktaAuthorizeOptions) (*OktaAuthorizeResult, error) {
	if opts.Issuer == "" {
		return nil, errors.New("issuer is required")
	}
	if opts.ClientID == "" {
		return nil, errors.New("client ID is required")
	}
	if opts.RedirectURI == "" {
		return nil, errors.New("redirect URI is required")
	}
	if len(opts.Scopes) == 0 {
		return nil, errors.New("scopes are required")
	}

	metadata, err := discoverOAuthASMetadata(ctx, opts.Issuer)
	if err != nil {
		return nil, errors.Wrap(err, "discovering OAuth AS metadata")
	}
	if metadata.AuthorizationEndpoint == "" {
		return nil, errors.Errorf("OAuth AS metadata for issuer '%s' returned no authorization endpoint", opts.Issuer)
	}

	codeVerifierBytes, err := createCodeVerifier()
	if err != nil {
		return nil, errors.Wrap(err, "creating code verifier")
	}
	codeVerifier := base64.RawURLEncoding.EncodeToString(codeVerifierBytes)

	h := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(h[:])

	state := rand.Text()

	params := url.Values{
		"client_id":             {opts.ClientID},
		"response_type":         {"code"},
		"scope":                 {strings.Join(opts.Scopes, " ")},
		"redirect_uri":          {opts.RedirectURI},
		"state":                 {state},
		"code_challenge_method": {"S256"},
		"code_challenge":        {codeChallenge},
	}

	return &OktaAuthorizeResult{
		AuthorizeURL: metadata.AuthorizationEndpoint + "?" + params.Encode(),
		State:        state,
		CodeVerifier: codeVerifier,
	}, nil
}

// AuthCodeExchangeOptions configures the parameters for exchanging an
// OAuth 2.0 authorization code for an access token.
type AuthCodeExchangeOptions struct {
	// TokenURL is the authorization server's issuer base URL; the token
	// endpoint is taken from RFC 8414 metadata at this issuer.
	TokenURL     string
	ClientID     string
	ClientSecret string
	Code         string
	RedirectURI  string
	CodeVerifier string
}

// ExchangeAuthCodeForToken exchanges an OAuth 2.0 authorization code for an
// access token using the Authorization Code + PKCE flow. It returns just
// the access token string.
func ExchangeAuthCodeForToken(ctx context.Context, opts AuthCodeExchangeOptions) (string, error) {
	if opts.TokenURL == "" {
		return "", errors.New("token URL (issuer) is required")
	}
	if opts.Code == "" {
		return "", errors.New("authorization code is required")
	}

	metadata, err := discoverOAuthASMetadata(ctx, opts.TokenURL)
	if err != nil {
		return "", errors.Wrap(err, "discovering token endpoint")
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {opts.Code},
		"redirect_uri":  {opts.RedirectURI},
		"client_id":     {opts.ClientID},
		"client_secret": {opts.ClientSecret},
		"code_verifier": {opts.CodeVerifier},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, metadata.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", errors.Wrap(err, "building auth code exchange request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "sending auth code exchange request")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "reading auth code exchange response")
	}

	if resp.StatusCode != http.StatusOK {
		return "", parseTokenExchangeError(resp.StatusCode, body)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", errors.Wrap(err, "unmarshalling auth code exchange response")
	}
	if tokenResp.Error != "" {
		return "", errors.Errorf("auth code exchange error: %s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}
	if tokenResp.AccessToken == "" {
		return "", errors.New("auth code exchange returned empty access token")
	}

	return tokenResp.AccessToken, nil
}

// createCodeVerifier creates a new code verifier for a PKCE flow.
// The code verifier should be at least 43 characters long and no more than 128 characters long.
func createCodeVerifier() ([]byte, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}
