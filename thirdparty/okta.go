package thirdparty

import (
	"context"
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

// RFC 8693 token type identifiers.
const (
	TokenTypeAccessToken  = "urn:ietf:params:oauth:token-type:access_token"
	TokenTypeIDToken      = "urn:ietf:params:oauth:token-type:id_token"
	TokenTypeRefreshToken = "urn:ietf:params:oauth:token-type:refresh_token"

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
	Issuer              string   `json:"issuer"`
	TokenEndpoint       string   `json:"token_endpoint"`
	GrantTypesSupported []string `json:"grant_types_supported"`
}

func getOktaCachedTokenURL() string {
	v, _ := cachedTokenURL.Load().(string)
	return v
}

// OktaTokenExchangeOptions configures the parameters for an OAuth 2.0
// Token Exchange (RFC 8693) against an Okta authorization server.
type OktaTokenExchangeOptions struct {
	// Issuer is the OAuth 2.0 Authorization Server issuer URL. Used for
	// discovering the token endpoint via RFC 8414 metadata when TokenURL
	// is not set.
	Issuer string
	// TokenURL overrides discovery and uses this URL directly as the
	// token endpoint. When set, Issuer is not used for discovery.
	TokenURL string
	// ClientID is the OAuth2 client identifier.
	ClientID string
	// ClientSecret is the OAuth2 client secret.
	ClientSecret string
	// SubjectToken is the existing token to exchange.
	SubjectToken string
	// SubjectTokenType identifies the type of SubjectToken
	// (e.g. TokenTypeAccessToken, TokenTypeIDToken).
	SubjectTokenType string
	// ActorToken is a token representing the identity of the acting party.
	// Optional. Only sent when explicitly set.
	ActorToken string
	// ActorTokenType identifies the type of ActorToken.
	// Required when ActorToken is set.
	ActorTokenType string
	// Audience is the target audience for the requested token. Optional.
	Audience string
	// Scopes is the set of scopes to request on the new token. Optional.
	Scopes []string
	// RequestedTokenType is the desired token type for the response. Optional.
	RequestedTokenType string
}

// tokenExchangeResponse represents the JSON response from an RFC 8693 token
// exchange as defined in Section 2.2 of the spec.
type tokenExchangeResponse struct {
	AccessToken     string `json:"access_token"`
	IssuedTokenType string `json:"issued_token_type"`
	TokenType       string `json:"token_type"`
	ExpiresIn       int    `json:"expires_in"`
	Scope           string `json:"scope"`
	RefreshToken    string `json:"refresh_token,omitempty"`
}

type tokenExchangeErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// ExchangeOktaToken performs an OAuth 2.0 Token Exchange (RFC 8693) against
// Okta. It resolves the token endpoint via OAuth Authorization Server Metadata
// discovery (RFC 8414) or uses an explicit TokenURL, then exchanges the
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
	if o.TokenURL == "" && o.Issuer == "" {
		return errors.New("either issuer or token URL is required")
	}
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
//  1. The explicit TokenURL on the options
//  2. The process-wide cached value (from a prior discovery)
//  3. OAuth 2.0 AS Metadata discovery on the Issuer (result is cached for subsequent calls)
func (o *OktaTokenExchangeOptions) resolveTokenURL(ctx context.Context) (string, error) {
	if o.TokenURL != "" {
		return o.TokenURL, nil
	}

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
	if opts.ActorToken != "" {
		form.Set("actor_token", opts.ActorToken)
		form.Set("actor_token_type", opts.ActorTokenType)
	}
	if opts.Audience != "" {
		form.Set("audience", opts.Audience)
	}
	if len(opts.Scopes) > 0 {
		form.Set("scope", strings.Join(opts.Scopes, " "))
	}
	if opts.RequestedTokenType != "" {
		form.Set("requested_token_type", opts.RequestedTokenType)
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
