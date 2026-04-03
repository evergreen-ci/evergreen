package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/auth

type authPermissionGetHandler struct {
	resource      string
	resourceType  string
	permission    string
	requiredLevel int
}

func (h *authPermissionGetHandler) Factory() gimlet.RouteHandler { return h }
func (h *authPermissionGetHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error

	vals := r.URL.Query()
	h.resource = vals.Get("resource")
	h.resourceType = vals.Get("resource_type")
	h.permission = vals.Get("permission")
	h.requiredLevel, err = strconv.Atoi(vals.Get("required_level"))

	return err
}

func (h *authPermissionGetHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)

	opts := gimlet.PermissionOpts{
		Resource:      h.resource,
		ResourceType:  h.resourceType,
		Permission:    h.permission,
		RequiredLevel: h.requiredLevel,
	}
	return gimlet.NewTextResponse(u.HasPermission(ctx, opts))
}

// tokenExchangeAuthorizeHandler is an HTTP handler that redirects the user to
// the Okta authorization server to get an authorization code that can be used
// to exchange for a token that can be used on behalf of the user, saving it on
// the user's document.
// This token is used in the spawn host setup commands to authenticate with the
// Evergreen API on behalf of the user.
func tokenExchangeAuthorizeHandler(env evergreen.Environment) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		settings := env.Settings()
		if settings.AuthConfig.Okta == nil {
			http.Error(w, "Okta auth config is not set", http.StatusInternalServerError)
			return
		}
		if err := settings.OktaServiceConfig.Validate(); err != nil {
			http.Error(w, errors.Wrap(err, "validating Okta service config").Error(), http.StatusInternalServerError)
			return
		}
		user := MustHaveUser(r.Context())

		// The initial authorization flow uses the Okta web app config.
		result, err := thirdparty.BuildAuthorizeURL(r.Context(), thirdparty.OktaAuthorizeOptions{
			Issuer:      settings.AuthConfig.Okta.Issuer,
			ClientID:    settings.AuthConfig.Okta.ClientID,
			Scopes:      settings.AuthConfig.Okta.Scopes,
			RedirectURI: fmt.Sprintf("%s/rest/v2/auth/token_exchange/callback", strings.TrimRight(settings.Ui.Url, "/")),
		})
		if err != nil {
			http.Error(w, errors.Wrap(err, "building authorize URL").Error(), http.StatusInternalServerError)
			return
		}

		if err := user.UpdateTokenExchangeState(r.Context(), result.State, result.CodeVerifier); err != nil {
			http.Error(w, errors.Wrap(err, "updating token exchange values").Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, result.AuthorizeURL, http.StatusTemporaryRedirect)
	})
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/auth/token_exchange/callback
//
// Receives the authorization code from Okta, exchanges it for the user's
// access token, then exchanges that access token at the target authorization
// server for a token that can be used on behalf of the user.

func makeTokenExchangeCallback(env evergreen.Environment) gimlet.RouteHandler {
	return &tokenExchangeCallbackHandler{env: env}
}

type tokenExchangeCallbackHandler struct {
	env   evergreen.Environment
	code  string
	state string
}

func (h *tokenExchangeCallbackHandler) Factory() gimlet.RouteHandler {
	return &tokenExchangeCallbackHandler{env: h.env}
}

func (h *tokenExchangeCallbackHandler) Parse(_ context.Context, r *http.Request) error {
	h.code = r.URL.Query().Get("code")
	if h.code == "" {
		return errors.New("missing 'code' query parameter")
	}
	h.state = r.URL.Query().Get("state")
	if h.state == "" {
		return errors.New("missing 'state' query parameter")
	}
	return nil
}

func (h *tokenExchangeCallbackHandler) Run(ctx context.Context) gimlet.Responder {
	settings := h.env.Settings()
	if settings.AuthConfig.Okta == nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.New("Okta auth config is not set"))
	}
	if err := settings.OktaServiceConfig.Validate(); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "validating Okta service config"))
	}

	u := MustHaveUser(ctx)

	// Check if the state matches and remove the token exchange state if it does.
	// Checking if the state is the same prevents CSRF attacks.
	codeVerifier, removed, err := u.RemoveTokenExchangeStateIfMatches(ctx, h.state)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "removing token exchange state"))
	}
	if !removed {
		return gimlet.MakeJSONErrorResponder(errors.New("OAuth 'state' does not match the authorization request"))
	}

	// The Okta web app config is used to exchange the authorization code for a user access token.
	callbackURL := fmt.Sprintf("%s/rest/v2/auth/token_exchange/callback", strings.TrimRight(settings.Ui.Url, "/"))
	userAccessToken, err := thirdparty.ExchangeAuthCodeForToken(ctx, thirdparty.AuthCodeExchangeOptions{
		TokenURL:     settings.AuthConfig.Okta.Issuer,
		ClientID:     settings.AuthConfig.Okta.ClientID,
		ClientSecret: settings.AuthConfig.Okta.ClientSecret,
		Code:         h.code,
		RedirectURI:  callbackURL,
		CodeVerifier: codeVerifier,
	})
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "exchanging authorization code for user access token"))
	}

	// The Okta service config is used to perform the token exchange.
	exchangedToken, err := thirdparty.ExchangeOktaToken(ctx, thirdparty.OktaTokenExchangeOptions{
		Issuer:           settings.OktaServiceConfig.Issuer,
		Audience:         settings.OktaServiceConfig.Audience,
		ClientID:         settings.OktaServiceConfig.ClientID,
		ClientSecret:     settings.OktaServiceConfig.ClientSecret,
		SubjectToken:     userAccessToken,
		SubjectTokenType: thirdparty.TokenTypeAccessToken,
		Scopes:           settings.OktaServiceConfig.Scopes,
	})
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "performing token exchange"))
	}

	if err := u.UpdateTokenExchangeToken(ctx, exchangedToken); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "updating token exchange token"))
	}

	return gimlet.NewTextResponse("You can now close this window and return to the application.")
}
