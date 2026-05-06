package client

import (
	"context"
	"sync"
	"time"

	"github.com/kanopy-platform/kanopy-oidc-lib/pkg/dex"
	"golang.org/x/oauth2"
)

// TokenSource provides access tokens, refreshing them as needed.
type TokenSource interface {
	// Token returns a valid access token string, refreshing proactively if near expiry.
	Token(ctx context.Context) (string, error)
	// ForceRefresh discards the cached token and obtains a new one.
	ForceRefresh(ctx context.Context) (string, error)
}

// OIDCTokenSource manages OIDC tokens with proactive refresh.
type OIDCTokenSource struct {
	mu sync.Mutex

	accessToken string
	expiry      time.Time

	doNotUseBrowser bool
	dexOpts         []dex.ClientOption
	refreshBuffer   time.Duration
}

// NewOIDCTokenSource creates a token source initialized with an existing token.
func NewOIDCTokenSource(token *oauth2.Token, doNotUseBrowser bool, refreshBuffer time.Duration, dexOpts ...dex.ClientOption) *OIDCTokenSource {
	return &OIDCTokenSource{
		accessToken:     token.AccessToken,
		expiry:          token.Expiry,
		doNotUseBrowser: doNotUseBrowser,
		dexOpts:         dexOpts,
		refreshBuffer:   refreshBuffer,
	}
}

// Token returns a valid access token, refreshing proactively if within the refresh buffer of expiry.
func (ts *OIDCTokenSource) Token(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.accessToken != "" && time.Now().Before(ts.expiry.Add(-ts.refreshBuffer)) {
		return ts.accessToken, nil
	}

	return ts.refresh(ctx)
}

// ForceRefresh unconditionally obtains a new token, used after receiving a 401.
func (ts *OIDCTokenSource) ForceRefresh(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	return ts.refresh(ctx)
}

func (ts *OIDCTokenSource) refresh(ctx context.Context) (string, error) {
	token, _, err := GetOAuthToken(ctx, ts.doNotUseBrowser, ts.dexOpts...)
	if err != nil {
		return "", err
	}

	ts.accessToken = token.AccessToken
	ts.expiry = token.Expiry
	return ts.accessToken, nil
}
