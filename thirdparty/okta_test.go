package thirdparty

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setOktaTokenURL(tokenURL string) {
	cachedTokenURL.Store(tokenURL)
}

func validOpts(tokenURL string) OktaTokenExchangeOptions {
	return OktaTokenExchangeOptions{
		TokenURL:         tokenURL,
		ClientID:         "client-id",
		ClientSecret:     "client-secret",
		SubjectToken:     "existing-token",
		SubjectTokenType: TokenTypeAccessToken,
	}
}

func TestValidateTokenExchangeOptions(t *testing.T) {
	t.Run("MissingIssuerAndTokenURL", func(t *testing.T) {
		opts := validOpts("")
		opts.TokenURL = ""
		opts.Issuer = ""
		assert.ErrorContains(t, opts.validate(), "either issuer or token URL is required")
	})

	t.Run("MissingClientID", func(t *testing.T) {
		opts := validOpts("https://example.com/token")
		opts.ClientID = ""
		assert.ErrorContains(t, opts.validate(), "client ID is required")
	})

	t.Run("MissingClientSecret", func(t *testing.T) {
		opts := validOpts("https://example.com/token")
		opts.ClientSecret = ""
		assert.ErrorContains(t, opts.validate(), "client secret is required")
	})

	t.Run("MissingSubjectToken", func(t *testing.T) {
		opts := validOpts("https://example.com/token")
		opts.SubjectToken = ""
		assert.ErrorContains(t, opts.validate(), "subject token is required")
	})

	t.Run("MissingSubjectTokenType", func(t *testing.T) {
		opts := validOpts("https://example.com/token")
		opts.SubjectTokenType = ""
		assert.ErrorContains(t, opts.validate(), "subject token type is required")
	})

	t.Run("ValidWithTokenURL", func(t *testing.T) {
		opts := validOpts("https://example.com/token")
		assert.NoError(t, opts.validate())
	})

	t.Run("ValidWithIssuer", func(t *testing.T) {
		opts := validOpts("")
		opts.TokenURL = ""
		opts.Issuer = "https://issuer.example.com"
		assert.NoError(t, opts.validate())
	})
}

func resetCachedTokenURL(t *testing.T) {
	t.Helper()
	setOktaTokenURL("")
	t.Cleanup(func() { setOktaTokenURL("") })
}

func TestResolveTokenURL(t *testing.T) {
	t.Run("UsesExplicitTokenURL", func(t *testing.T) {
		resetCachedTokenURL(t)
		opts := validOpts("https://explicit.example.com/token")
		resolved, err := opts.resolveTokenURL(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "https://explicit.example.com/token", resolved)
	})

	t.Run("ExplicitTokenURLTakesPriorityOverCache", func(t *testing.T) {
		resetCachedTokenURL(t)
		setOktaTokenURL("https://cached.example.com/token")

		opts := validOpts("https://explicit.example.com/token")
		resolved, err := opts.resolveTokenURL(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "https://explicit.example.com/token", resolved)
	})

	t.Run("UsesCachedTokenURL", func(t *testing.T) {
		resetCachedTokenURL(t)
		setOktaTokenURL("https://cached.example.com/token")

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
			json.NewEncoder(w).Encode(oauthASMetadata{
				Issuer:        srvURL,
				TokenEndpoint: srvURL + "/v1/token",
			})
		}))
		defer srv.Close()
		srvURL = srv.URL

		opts := OktaTokenExchangeOptions{Issuer: srv.URL}
		resolved, err := opts.resolveTokenURL(t.Context())
		require.NoError(t, err)
		assert.Equal(t, srv.URL+"/v1/token", resolved)
	})

	t.Run("DiscoveryCachesResult", func(t *testing.T) {
		resetCachedTokenURL(t)

		callCount := 0
		var srvURL string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(oauthASMetadata{
				TokenEndpoint: srvURL + "/v1/token",
			})
		}))
		defer srv.Close()
		srvURL = srv.URL

		opts := OktaTokenExchangeOptions{Issuer: srv.URL}

		resolved, err := opts.resolveTokenURL(t.Context())
		require.NoError(t, err)
		assert.Equal(t, srv.URL+"/v1/token", resolved)
		assert.Equal(t, 1, callCount)

		resolved, err = opts.resolveTokenURL(t.Context())
		require.NoError(t, err)
		assert.Equal(t, srv.URL+"/v1/token", resolved)
		assert.Equal(t, 1, callCount, "should not have fetched metadata again")
	})

	t.Run("DiscoveryFailsOnEmptyTokenEndpoint", func(t *testing.T) {
		resetCachedTokenURL(t)

		var srvURL string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(oauthASMetadata{
				Issuer: srvURL,
			})
		}))
		defer srv.Close()
		srvURL = srv.URL

		opts := OktaTokenExchangeOptions{Issuer: srv.URL}
		_, err := opts.resolveTokenURL(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no token endpoint")
	})
}

func TestSetOktaTokenURL(t *testing.T) {
	t.Run("SetAndGet", func(t *testing.T) {
		resetCachedTokenURL(t)
		assert.Empty(t, getOktaCachedTokenURL())

		setOktaTokenURL("https://override.example.com/token")
		assert.Equal(t, "https://override.example.com/token", getOktaCachedTokenURL())
	})

	t.Run("ClearWithEmptyString", func(t *testing.T) {
		resetCachedTokenURL(t)
		setOktaTokenURL("https://override.example.com/token")
		assert.NotEmpty(t, getOktaCachedTokenURL())

		setOktaTokenURL("")
		assert.Empty(t, getOktaCachedTokenURL())
	})

	t.Run("OverrideUsedByExchange", func(t *testing.T) {
		resetCachedTokenURL(t)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tokenExchangeResponse{
				AccessToken: "from-cached-url",
				TokenType:   "Bearer",
			})
		}))
		defer srv.Close()

		setOktaTokenURL(srv.URL)

		opts := OktaTokenExchangeOptions{
			Issuer:           "https://not-a-real-issuer.invalid",
			ClientID:         "client-id",
			ClientSecret:     "client-secret",
			SubjectToken:     "existing-token",
			SubjectTokenType: TokenTypeAccessToken,
		}
		token, err := ExchangeOktaToken(t.Context(), opts)
		require.NoError(t, err)
		assert.Equal(t, "from-cached-url", token.AccessToken)
	})
}

func TestExchangeOktaToken(t *testing.T) {
	t.Run("SuccessfulExchange", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			json.NewEncoder(w).Encode(tokenExchangeResponse{
				AccessToken:     "new-access-token",
				IssuedTokenType: TokenTypeAccessToken,
				TokenType:       "Bearer",
				ExpiresIn:       3600,
				Scope:           "openid",
			})
		}))
		defer srv.Close()

		token, err := ExchangeOktaToken(t.Context(), validOpts(srv.URL))
		require.NoError(t, err)
		require.NotNil(t, token)
		assert.Equal(t, "new-access-token", token.AccessToken)
		assert.Equal(t, "Bearer", token.TokenType)
		assert.Empty(t, token.RefreshToken)
		assert.WithinDuration(t, time.Now().Add(3600*time.Second), token.Expiry, 5*time.Second)
	})

	t.Run("IncludesOptionalFields", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "api://target", r.FormValue("audience"))
			assert.Equal(t, "openid email", r.FormValue("scope"))
			assert.Equal(t, TokenTypeIDToken, r.FormValue("requested_token_type"))

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tokenExchangeResponse{
				AccessToken: "token-with-opts",
				TokenType:   "Bearer",
			})
		}))
		defer srv.Close()

		opts := validOpts(srv.URL)
		opts.Audience = "api://target"
		opts.Scopes = []string{"openid", "email"}
		opts.RequestedTokenType = TokenTypeIDToken

		token, err := ExchangeOktaToken(t.Context(), opts)
		require.NoError(t, err)
		assert.Equal(t, "token-with-opts", token.AccessToken)
	})

	t.Run("IncludesActorToken", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "service-token", r.FormValue("actor_token"))
			assert.Equal(t, TokenTypeAccessToken, r.FormValue("actor_token_type"))

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tokenExchangeResponse{
				AccessToken: "delegated-token",
				TokenType:   "Bearer",
			})
		}))
		defer srv.Close()

		opts := validOpts(srv.URL)
		opts.ActorToken = "service-token"
		opts.ActorTokenType = TokenTypeAccessToken

		token, err := ExchangeOktaToken(t.Context(), opts)
		require.NoError(t, err)
		assert.Equal(t, "delegated-token", token.AccessToken)
	})

	t.Run("OmitsEmptyOptionalFields", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			assert.Empty(t, r.FormValue("actor_token"))
			assert.Empty(t, r.FormValue("actor_token_type"))
			assert.Empty(t, r.FormValue("audience"))
			assert.Empty(t, r.FormValue("scope"))
			assert.Empty(t, r.FormValue("requested_token_type"))

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tokenExchangeResponse{
				AccessToken: "tok",
				TokenType:   "Bearer",
			})
		}))
		defer srv.Close()

		_, err := ExchangeOktaToken(t.Context(), validOpts(srv.URL))
		require.NoError(t, err)
	})

	t.Run("WithRefreshToken", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tokenExchangeResponse{
				AccessToken:  "access",
				TokenType:    "Bearer",
				RefreshToken: "refresh",
				ExpiresIn:    7200,
			})
		}))
		defer srv.Close()

		token, err := ExchangeOktaToken(t.Context(), validOpts(srv.URL))
		require.NoError(t, err)
		assert.Equal(t, "refresh", token.RefreshToken)
	})

	t.Run("NoExpiry", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tokenExchangeResponse{
				AccessToken: "access",
				TokenType:   "Bearer",
				ExpiresIn:   0,
			})
		}))
		defer srv.Close()

		token, err := ExchangeOktaToken(t.Context(), validOpts(srv.URL))
		require.NoError(t, err)
		assert.True(t, token.Expiry.IsZero())
	})

	t.Run("OAuthErrorResponse", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(tokenExchangeErrorResponse{
				Error:            "invalid_grant",
				ErrorDescription: "The subject token is expired",
			})
		}))
		defer srv.Close()

		_, err := ExchangeOktaToken(t.Context(), validOpts(srv.URL))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid_grant")
		assert.Contains(t, err.Error(), "The subject token is expired")
		assert.Contains(t, err.Error(), "400")
	})

	t.Run("NonJSONErrorResponse", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal server error"))
		}))
		defer srv.Close()

		_, err := ExchangeOktaToken(t.Context(), validOpts(srv.URL))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
		assert.Contains(t, err.Error(), "internal server error")
	})

	t.Run("MalformedJSONResponse", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("{not valid json"))
		}))
		defer srv.Close()

		_, err := ExchangeOktaToken(t.Context(), validOpts(srv.URL))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshalling token exchange response")
	})

	t.Run("ValidationFailure", func(t *testing.T) {
		_, err := ExchangeOktaToken(t.Context(), OktaTokenExchangeOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token exchange options")
	})
}

func TestParseTokenExchangeError(t *testing.T) {
	t.Run("StructuredOAuthError", func(t *testing.T) {
		body, _ := json.Marshal(tokenExchangeErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "missing subject_token",
		})
		err := parseTokenExchangeError(http.StatusBadRequest, body)
		assert.Contains(t, err.Error(), "invalid_request")
		assert.Contains(t, err.Error(), "missing subject_token")
		assert.Contains(t, err.Error(), "400")
	})

	t.Run("UnstructuredError", func(t *testing.T) {
		err := parseTokenExchangeError(http.StatusServiceUnavailable, []byte("service down"))
		assert.Contains(t, err.Error(), "503")
		assert.Contains(t, err.Error(), "service down")
	})
}
