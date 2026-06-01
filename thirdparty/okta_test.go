package thirdparty

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTokenPath = "/v1/token"

func validTokenExchangeOpts(issuer string) OktaTokenExchangeOptions {
	return OktaTokenExchangeOptions{
		Issuer:           issuer,
		ClientID:         "client-id",
		ClientSecret:     "client-secret",
		SubjectToken:     "existing-token",
		SubjectTokenType: TokenTypeAccessToken,
	}
}

// newTokenExchangeServer serves RFC 8414 metadata at the issuer base URL and
// POSTs to testTokenPath for token exchange. Call resetCachedTokenURL before
// use when discovery must run (global token URL cache).
func newTokenExchangeServer(t *testing.T, tokenHandler http.HandlerFunc) *httptest.Server {
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case oauthASMetadataPath:
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(oauthASMetadata{
				Issuer:        srvURL,
				TokenEndpoint: srvURL + testTokenPath,
			}))
		case testTokenPath:
			tokenHandler(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	srvURL = srv.URL
	t.Cleanup(srv.Close)
	return srv
}

func resetCachedTokenURL(t *testing.T) {
	cachedTokenURL.Store("")
	t.Cleanup(func() { cachedTokenURL.Store("") })
}

func TestValidateTokenExchangeOptions(t *testing.T) {
	t.Run("MissingClientID", func(t *testing.T) {
		opts := validTokenExchangeOpts("https://issuer.example")
		opts.ClientID = ""
		assert.ErrorContains(t, opts.validate(), "client ID is required")
	})

	t.Run("MissingClientSecret", func(t *testing.T) {
		opts := validTokenExchangeOpts("https://issuer.example")
		opts.ClientSecret = ""
		assert.ErrorContains(t, opts.validate(), "client secret is required")
	})

	t.Run("MissingSubjectToken", func(t *testing.T) {
		opts := validTokenExchangeOpts("https://issuer.example")
		opts.SubjectToken = ""
		assert.ErrorContains(t, opts.validate(), "subject token is required")
	})

	t.Run("MissingSubjectTokenType", func(t *testing.T) {
		opts := validTokenExchangeOpts("https://issuer.example")
		opts.SubjectTokenType = ""
		assert.ErrorContains(t, opts.validate(), "subject token type is required")
	})

	t.Run("Valid", func(t *testing.T) {
		opts := validTokenExchangeOpts("https://issuer.example")
		assert.NoError(t, opts.validate())
	})
}

func TestResolveTokenURL(t *testing.T) {
	t.Run("UsesCachedTokenURL", func(t *testing.T) {
		resetCachedTokenURL(t)
		cachedTokenURL.Store("https://cached.example.com/token")

		opts := OktaTokenExchangeOptions{Issuer: "https://not-a-real-issuer.invalid"}
		resolved, err := opts.resolveTokenURL(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "https://cached.example.com/token", resolved)
	})

	t.Run("FailsDiscoveryWithBadIssuer", func(t *testing.T) {
		resetCachedTokenURL(t)
		opts := OktaTokenExchangeOptions{
			Issuer: "https://not-a-real-issuer.invalid",
		}
		_, err := opts.resolveTokenURL(t.Context())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "fetching OAuth AS metadata")
	})

	t.Run("DiscoverFromMetadata", func(t *testing.T) {
		resetCachedTokenURL(t)

		var srvURL string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, oauthASMetadataPath, r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)

			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(oauthASMetadata{
				Issuer:        srvURL,
				TokenEndpoint: srvURL + testTokenPath,
			}))
		}))
		defer srv.Close()
		srvURL = srv.URL

		opts := OktaTokenExchangeOptions{Issuer: srv.URL}
		resolved, err := opts.resolveTokenURL(t.Context())
		require.NoError(t, err)
		assert.Equal(t, srv.URL+testTokenPath, resolved)
	})

	t.Run("DiscoveryCachesResult", func(t *testing.T) {
		resetCachedTokenURL(t)

		callCount := 0
		var srvURL string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(oauthASMetadata{
				TokenEndpoint: srvURL + testTokenPath,
			}))
		}))
		defer srv.Close()
		srvURL = srv.URL

		opts := OktaTokenExchangeOptions{Issuer: srv.URL}

		resolved, err := opts.resolveTokenURL(t.Context())
		require.NoError(t, err)
		assert.Equal(t, srv.URL+testTokenPath, resolved)
		assert.Equal(t, 1, callCount)

		resolved, err = opts.resolveTokenURL(t.Context())
		require.NoError(t, err)
		assert.Equal(t, srv.URL+testTokenPath, resolved)
		assert.Equal(t, 1, callCount, "should not have fetched metadata again")
	})

	t.Run("DiscoveryFailsOnEmptyTokenEndpoint", func(t *testing.T) {
		resetCachedTokenURL(t)

		var srvURL string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(oauthASMetadata{
				Issuer: srvURL,
			}))
		}))
		defer srv.Close()
		srvURL = srv.URL

		opts := OktaTokenExchangeOptions{Issuer: srv.URL}
		_, err := opts.resolveTokenURL(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no token endpoint")
	})
}

func TestExchangeOktaTokenUsesCachedTokenURL(t *testing.T) {
	resetCachedTokenURL(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(tokenExchangeResponse{
			AccessToken: "from-cached-url",
			TokenType:   "Bearer",
		}))
	}))
	defer srv.Close()

	cachedTokenURL.Store(srv.URL)

	opts := validTokenExchangeOpts("https://not-a-real-issuer.invalid")
	token, err := ExchangeOktaToken(t.Context(), opts)
	require.NoError(t, err)
	assert.Equal(t, "from-cached-url", token.AccessToken)
}

func TestExchangeOktaToken(t *testing.T) {
	t.Run("SuccessfulExchange", func(t *testing.T) {
		resetCachedTokenURL(t)
		srv := newTokenExchangeServer(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
			assert.Equal(t, "application/json", r.Header.Get("Accept"))

			user, pass, ok := r.BasicAuth()
			assert.True(t, ok)
			assert.Equal(t, "client-id", user)
			assert.Equal(t, "client-secret", pass)

			require.NoError(t, r.ParseForm())
			assert.Equal(t, tokenExchangeGrantType, r.FormValue("grant_type"))
			assert.Equal(t, "existing-token", r.FormValue("subject_token"))
			assert.Equal(t, TokenTypeAccessToken, r.FormValue("subject_token_type"))

			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(tokenExchangeResponse{
				AccessToken: "new-access-token",
				TokenType:   "Bearer",
				ExpiresIn:   3600,
			}))
		})

		token, err := ExchangeOktaToken(t.Context(), validTokenExchangeOpts(srv.URL))
		require.NoError(t, err)
		require.NotNil(t, token)
		assert.Equal(t, "new-access-token", token.AccessToken)
		assert.Equal(t, "Bearer", token.TokenType)
		assert.Empty(t, token.RefreshToken)
		assert.WithinDuration(t, time.Now().Add(3600*time.Second), token.Expiry, 5*time.Second)
	})

	t.Run("IncludesOptionalAudienceAndScopes", func(t *testing.T) {
		resetCachedTokenURL(t)
		srv := newTokenExchangeServer(t, func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "api://target", r.FormValue("audience"))
			assert.Equal(t, "openid email", r.FormValue("scope"))

			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(tokenExchangeResponse{
				AccessToken: "token-with-opts",
				TokenType:   "Bearer",
			}))
		})

		opts := validTokenExchangeOpts(srv.URL)
		opts.Audience = "api://target"
		opts.Scopes = []string{"openid", "email"}

		token, err := ExchangeOktaToken(t.Context(), opts)
		require.NoError(t, err)
		assert.Equal(t, "token-with-opts", token.AccessToken)
	})

	t.Run("OmitsEmptyOptionalFields", func(t *testing.T) {
		resetCachedTokenURL(t)
		srv := newTokenExchangeServer(t, func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			assert.Empty(t, r.FormValue("audience"))
			assert.Empty(t, r.FormValue("scope"))

			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(tokenExchangeResponse{
				AccessToken: "tok",
				TokenType:   "Bearer",
			}))
		})

		_, err := ExchangeOktaToken(t.Context(), validTokenExchangeOpts(srv.URL))
		require.NoError(t, err)
	})

	t.Run("WithRefreshToken", func(t *testing.T) {
		resetCachedTokenURL(t)
		srv := newTokenExchangeServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(tokenExchangeResponse{
				AccessToken:  "access",
				TokenType:    "Bearer",
				RefreshToken: "refresh",
				ExpiresIn:    7200,
			}))
		})

		token, err := ExchangeOktaToken(t.Context(), validTokenExchangeOpts(srv.URL))
		require.NoError(t, err)
		assert.Equal(t, "refresh", token.RefreshToken)
	})

	t.Run("NoExpiry", func(t *testing.T) {
		resetCachedTokenURL(t)
		srv := newTokenExchangeServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(tokenExchangeResponse{
				AccessToken: "access",
				TokenType:   "Bearer",
				ExpiresIn:   0,
			}))
		})

		token, err := ExchangeOktaToken(t.Context(), validTokenExchangeOpts(srv.URL))
		require.NoError(t, err)
		assert.True(t, token.Expiry.IsZero())
	})

	t.Run("OAuthErrorResponse", func(t *testing.T) {
		resetCachedTokenURL(t)
		srv := newTokenExchangeServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			require.NoError(t, json.NewEncoder(w).Encode(tokenExchangeErrorResponse{
				Error:            "invalid_grant",
				ErrorDescription: "The subject token is expired",
			}))
		})

		_, err := ExchangeOktaToken(t.Context(), validTokenExchangeOpts(srv.URL))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid_grant")
		assert.Contains(t, err.Error(), "The subject token is expired")
		assert.Contains(t, err.Error(), "400")
	})

	t.Run("NonJSONErrorResponse", func(t *testing.T) {
		resetCachedTokenURL(t)
		srv := newTokenExchangeServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, err := w.Write([]byte("internal server error"))
			require.NoError(t, err)
		})

		_, err := ExchangeOktaToken(t.Context(), validTokenExchangeOpts(srv.URL))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
		assert.Contains(t, err.Error(), "internal server error")
	})

	t.Run("MalformedJSONResponse", func(t *testing.T) {
		resetCachedTokenURL(t)
		srv := newTokenExchangeServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte("{not valid json"))
			require.NoError(t, err)
		})

		_, err := ExchangeOktaToken(t.Context(), validTokenExchangeOpts(srv.URL))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshalling token exchange response")
	})
}

func TestBuildAuthorizeURL(t *testing.T) {
	t.Run("ValidationErrors", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			opts OktaAuthorizeOptions
			want string
		}{
			{
				name: "MissingIssuer",
				opts: OktaAuthorizeOptions{ClientID: "c", RedirectURI: "https://x/cb", Scopes: []string{"openid"}},
				want: "issuer is required",
			},
			{
				name: "MissingClientID",
				opts: OktaAuthorizeOptions{Issuer: "https://x", RedirectURI: "https://x/cb", Scopes: []string{"openid"}},
				want: "client ID is required",
			},
			{
				name: "MissingRedirectURI",
				opts: OktaAuthorizeOptions{Issuer: "https://x", ClientID: "c", Scopes: []string{"openid"}},
				want: "redirect URI is required",
			},
			{
				name: "MissingScopes",
				opts: OktaAuthorizeOptions{Issuer: "https://x", ClientID: "c", RedirectURI: "https://x/cb"},
				want: "scopes are required",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := BuildAuthorizeURL(t.Context(), tc.opts)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.want)
			})
		}
	})

	t.Run("FailsWhenNoAuthorizationEndpoint", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(oauthASMetadata{
				TokenEndpoint: "https://example.com/token",
			}))
		}))
		defer srv.Close()

		_, err := BuildAuthorizeURL(t.Context(), OktaAuthorizeOptions{
			Issuer:      srv.URL,
			ClientID:    "client-id",
			RedirectURI: "https://example.com/callback",
			Scopes:      []string{"openid"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no authorization endpoint")
	})

	t.Run("URLContainsExpectedParamsAndPKCE", func(t *testing.T) {
		var srvURL string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(oauthASMetadata{
				AuthorizationEndpoint: srvURL + "/v1/authorize",
			}))
		}))
		defer srv.Close()
		srvURL = srv.URL

		result, err := BuildAuthorizeURL(t.Context(), OktaAuthorizeOptions{
			Issuer:      srv.URL,
			ClientID:    "my-client-id",
			RedirectURI: "https://app.example.com/callback",
			Scopes:      []string{"openid", "email"},
		})
		require.NoError(t, err)
		assert.NotEmpty(t, result.State)
		assert.NotEmpty(t, result.CodeVerifier)
		assert.NotEmpty(t, result.AuthorizeURL)

		parsed, err := url.Parse(result.AuthorizeURL)
		require.NoError(t, err)
		assert.Equal(t, srvURL+"/v1/authorize", parsed.Scheme+"://"+parsed.Host+parsed.Path)
		assert.Equal(t, "my-client-id", parsed.Query().Get("client_id"))
		assert.Equal(t, "code", parsed.Query().Get("response_type"))
		assert.Equal(t, "openid email", parsed.Query().Get("scope"))
		assert.Equal(t, "https://app.example.com/callback", parsed.Query().Get("redirect_uri"))
		assert.Equal(t, "S256", parsed.Query().Get("code_challenge_method"))
		assert.Equal(t, result.State, parsed.Query().Get("state"))
		assert.NotEmpty(t, parsed.Query().Get("code_challenge"))

		h := sha256.Sum256([]byte(result.CodeVerifier))
		expectedChallenge := base64.RawURLEncoding.EncodeToString(h[:])
		assert.Equal(t, expectedChallenge, parsed.Query().Get("code_challenge"))
	})

	t.Run("UniqueStateAndVerifierPerCall", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(oauthASMetadata{
				AuthorizationEndpoint: "https://example.com/authorize",
			}))
		}))
		defer srv.Close()

		opts := OktaAuthorizeOptions{
			Issuer:      srv.URL,
			ClientID:    "test-client",
			RedirectURI: "https://example.com/callback",
			Scopes:      []string{"openid"},
		}
		result1, err := BuildAuthorizeURL(t.Context(), opts)
		require.NoError(t, err)
		result2, err := BuildAuthorizeURL(t.Context(), opts)
		require.NoError(t, err)

		assert.NotEqual(t, result1.State, result2.State)
		assert.NotEqual(t, result1.CodeVerifier, result2.CodeVerifier)
	})
}

func TestExchangeAuthCodeForToken(t *testing.T) {
	newMetadataAndTokenServer := func(t *testing.T, tokenHandler http.HandlerFunc) *httptest.Server {
		var srvURL string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == oauthASMetadataPath {
				w.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(w).Encode(oauthASMetadata{
					Issuer:        srvURL,
					TokenEndpoint: srvURL + testTokenPath,
				}))
				return
			}
			tokenHandler(w, r)
		}))
		srvURL = srv.URL
		t.Cleanup(srv.Close)
		return srv
	}

	t.Run("MissingTokenURL", func(t *testing.T) {
		_, err := ExchangeAuthCodeForToken(t.Context(), AuthCodeExchangeOptions{
			Code: "auth-code",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "token URL (issuer) is required")
	})

	t.Run("MissingCode", func(t *testing.T) {
		_, err := ExchangeAuthCodeForToken(t.Context(), AuthCodeExchangeOptions{
			TokenURL: "https://example.com",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authorization code is required")
	})

	t.Run("SuccessfulExchange", func(t *testing.T) {
		srv := newMetadataAndTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, testTokenPath, r.URL.Path)
			assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

			require.NoError(t, r.ParseForm())
			assert.Equal(t, "authorization_code", r.FormValue("grant_type"))
			assert.Equal(t, "the-auth-code", r.FormValue("code"))
			assert.Equal(t, "https://app.example.com/callback", r.FormValue("redirect_uri"))
			assert.Equal(t, "my-client", r.FormValue("client_id"))
			assert.Equal(t, "my-secret", r.FormValue("client_secret"))
			assert.Equal(t, "the-verifier", r.FormValue("code_verifier"))

			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]string{
				"access_token": "user-access-token",
			}))
		})

		token, err := ExchangeAuthCodeForToken(t.Context(), AuthCodeExchangeOptions{
			TokenURL:     srv.URL,
			ClientID:     "my-client",
			ClientSecret: "my-secret",
			Code:         "the-auth-code",
			RedirectURI:  "https://app.example.com/callback",
			CodeVerifier: "the-verifier",
		})
		require.NoError(t, err)
		assert.Equal(t, "user-access-token", token)
	})

	t.Run("HTTPErrorResponse", func(t *testing.T) {
		srv := newMetadataAndTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			require.NoError(t, json.NewEncoder(w).Encode(tokenExchangeErrorResponse{
				Error:            "invalid_grant",
				ErrorDescription: "authorization code expired",
			}))
		})

		_, err := ExchangeAuthCodeForToken(t.Context(), AuthCodeExchangeOptions{
			TokenURL: srv.URL,
			Code:     "expired-code",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid_grant")
		assert.Contains(t, err.Error(), "authorization code expired")
	})

	t.Run("EmptyAccessToken", func(t *testing.T) {
		srv := newMetadataAndTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]string{
				"access_token": "",
			}))
		})

		_, err := ExchangeAuthCodeForToken(t.Context(), AuthCodeExchangeOptions{
			TokenURL: srv.URL,
			Code:     "some-code",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty access token")
	})

	t.Run("MalformedJSONResponse", func(t *testing.T) {
		srv := newMetadataAndTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte("{not valid json"))
			require.NoError(t, err)
		})

		_, err := ExchangeAuthCodeForToken(t.Context(), AuthCodeExchangeOptions{
			TokenURL: srv.URL,
			Code:     "some-code",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshalling auth code exchange response")
	})

	t.Run("OAuthErrorInBody", func(t *testing.T) {
		srv := newMetadataAndTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]string{
				"error":             "invalid_client",
				"error_description": "bad credentials",
			}))
		})

		_, err := ExchangeAuthCodeForToken(t.Context(), AuthCodeExchangeOptions{
			TokenURL: srv.URL,
			Code:     "some-code",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid_client")
	})
}
