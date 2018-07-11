package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

// this manages the /admin/service_flags route, which allows setting the service flags
func getServiceFlagsRouteManager(route string, version int) *RouteManager {
	fph := &flagsPostHandler{}
	flagsPost := MethodHandler{
		Authenticator:  &SuperUserAuthenticator{},
		RequestHandler: fph.Handler(),
		MethodType:     http.MethodPost,
	}

	flagsRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{flagsPost},
		Version: version,
	}
	return &flagsRoute
}

type flagsPostHandler struct {
	Flags model.APIServiceFlags `json:"service_flags"`
}

func (h *flagsPostHandler) Handler() RequestHandler {
	return &flagsPostHandler{}
}

func (h *flagsPostHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	return errors.WithStack(util.ReadJSONInto(r.Body, h))
}

func (h *flagsPostHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)
	flags, err := h.Flags.ToService()
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "API model error")
	}

	err = sc.SetServiceFlags(flags.(evergreen.ServiceFlags), u)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error")
	}
	return ResponseData{
		Result: []model.Model{&h.Flags},
	}, nil
}
