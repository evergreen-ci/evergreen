package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
)

// GetHandlerPprof returns a handler for pprof endpoints.
func GetHandlerPprof(settings *evergreen.Settings) (http.Handler, error) {
	app := util.GetPprofApp()
	app.AddMiddleware(gimlet.MakeRecoveryLogger())

	return app.Handler()
}
