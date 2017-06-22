package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// types and functions for Version Cost Route
type costByVersionHandler struct {
	versionId string
}

func getCostByVersionIdRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &costByVersionHandler{},
				MethodType:     evergreen.MethodGet,
			},
		},
		Version: version,
	}
}

func (cbvh *costByVersionHandler) Handler() RequestHandler {
	return &costByVersionHandler{}
}

func (cbvh *costByVersionHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	cbvh.versionId = mux.Vars(r)["version_id"]

	if cbvh.versionId == "" {
		return errors.New("request data incomplete")
	}

	return nil
}

func (cbvh *costByVersionHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	foundVersionCost, err := sc.FindCostByVersionId(cbvh.versionId)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	versionCostModel := &model.APIVersionCost{}
	err = versionCostModel.BuildFromService(foundVersionCost)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}
	return ResponseData{
		Result: []model.Model{versionCostModel},
	}, nil
}

// types and functions for Distro Cost Route
type costByDistroHandler struct {
	distroId string
}

func getCostByDistroIdRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &costByDistroHandler{},
				MethodType:     evergreen.MethodGet,
			},
		},
		Version: version,
	}
}

func (cbvh *costByDistroHandler) Handler() RequestHandler {
	return &costByDistroHandler{}
}

func (cbvh *costByDistroHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	cbvh.distroId = mux.Vars(r)["distro_id"]

	if cbvh.distroId == "" {
		return errors.New("request data incomplete")
	}

	return nil
}

func (cbvh *costByDistroHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	foundDistroCost, err := sc.FindCostByDistroId(cbvh.distroId)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	distroCostModel := &model.APIDistroCost{}
	err = distroCostModel.BuildFromService(foundDistroCost)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}
	return ResponseData{
		Result: []model.Model{distroCostModel},
	}, nil
}
