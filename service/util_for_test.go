package service

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// newTestUIRouter is the same as makeAuthTestUIRouter but without any user
// authentication middleware configuration.
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
	if env.UserManager() == nil {
		um, info, err := auth.LoadUserManager(settings)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		env.SetUserManager(um)
		env.SetUserManagerInfo(info)
	}
	uis := &UIServer{
		RootURL:  settings.Ui.Url,
		Settings: *settings,
		env:      env,
	}
	app := GetRESTv1App(uis)
	app.AddMiddleware(gimlet.UserMiddleware(ctx, env.UserManager(), umconf))
	return app.Handler()
}
