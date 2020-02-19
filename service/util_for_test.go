package service

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/gimlet"
)

// newTestUIRouter is the same as makeAuthTestUIRouter but without any user
// authentication middleware configuration.
// kim: TODO: see if we can get rid of config and just replace with
// env.Settings()
func newTestUIRouter(ctx context.Context, env evergreen.Environment) (http.Handler, error) {
	return makeAuthTestUIRouter(ctx, env, gimlet.UserMiddlewareConfiguration{})
}

// newAuthTestUIRouter is the same as makeAuthTestUIRouter but allows
// cookie-based and API key-based user authentication.
func newAuthTestUIRouter(ctx context.Context, env evergreen.Environment) (http.Handler, error) {
	return makeAuthTestUIRouter(ctx, env, gimlet.UserMiddlewareConfiguration{
		CookieName:     evergreen.AuthTokenCookie,
		HeaderKeyName:  evergreen.APIKeyHeader,
		HeaderUserName: evergreen.APIUserHeader,
	})
}

// makeAuthTestUIRouter creates a router for testing the UI server with a user
// middleware configuration.
func makeAuthTestUIRouter(ctx context.Context, env evergreen.Environment, umconf gimlet.UserMiddlewareConfiguration) (http.Handler, error) {
	settings := env.Settings()
	uis := &UIServer{
		RootURL:  settings.Ui.Url,
		Settings: *settings,
		env:      env,
	}
	uis.render = gimlet.NewHTMLRenderer(gimlet.RendererOptions{
		Directory:    filepath.Join(evergreen.FindEvergreenHome(), WebRootPath, Templates),
		DisableCache: true,
	})
	app := GetRESTv1App(uis)
	app.AddMiddleware(gimlet.UserMiddleware(env.UserManager(), umconf))
	return app.Handler()
}
