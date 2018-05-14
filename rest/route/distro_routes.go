package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

type distroGetHandler struct{}

func getDistroRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &distroGetHandler{},
				MethodType:     http.MethodGet,
			},
		},
		Version: version,
	}
}

func (dgh *distroGetHandler) Handler() RequestHandler {
	return &distroGetHandler{}
}

func (dgh *distroGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	return nil
}

func (dgh *distroGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	distros, err := sc.FindAllDistros()
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	models := make([]model.Model, len(distros))
	for i, d := range distros {
		distroModel := &model.APIDistro{}
		if err := distroModel.BuildFromService(d); err != nil {
			return ResponseData{}, err
		}
		models[i] = distroModel
	}

	return ResponseData{
		Result: models,
	}, nil
}

type clearTaskQueueHandler struct {
	distro string
}

func getClearTaskQueueRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			MethodHandler{
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				Authenticator:     &SuperUserAuthenticator{},
				RequestHandler:    &clearTaskQueueHandler{},
				MethodType:        http.MethodDelete,
			},
		},
		Version: version,
	}
}

func (h *clearTaskQueueHandler) Handler() RequestHandler {
	return &clearTaskQueueHandler{}
}

func (h *clearTaskQueueHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	vars := mux.Vars(r)
	h.distro = vars["distro"]
	_, err := distro.FindOne(distro.ById(h.distro))
	if err != nil {
		return &rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    "unable to find distro",
		}
	}

	return nil
}

func (h *clearTaskQueueHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	return ResponseData{}, sc.ClearTaskQueue(h.distro)
}
