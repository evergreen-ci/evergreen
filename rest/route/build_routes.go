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

type buildIdGetHandler struct {
	buildId string
}

func getBuildIdRouteManager(route string, version int) *RouteManager {
	bigh := &buildIdGetHandler{}
	buildGet := MethodHandler{
		Authenticator:  &NoAuthAuthenticator{},
		RequestHandler: bigh.Handler(),
		MethodType:     evergreen.MethodGet,
	}

	buildRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{buildGet},
		Version: version,
	}
	return &buildRoute
}

func (bigh *buildIdGetHandler) Handler() RequestHandler {
	return &buildIdGetHandler{}
}

func (bigh *buildIdGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	vars := mux.Vars(r)
	bigh.buildId = vars["build_id"]
	return nil
}

func (bigh *buildIdGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	foundBuild, err := sc.FindBuildById(bigh.buildId)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	buildModel := &model.APIBuild{}
	ref, err := sc.FindProjectByBranch(foundBuild.Project)
	if err != nil {
		return ResponseData{}, err
	}
	var proj string
	if ref != nil {
		proj = ref.Repo
	}
	buildModel.ProjectId = model.APIString(proj)
	err = buildModel.BuildFromService(*foundBuild)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}

	return ResponseData{
		Result: []model.Model{buildModel},
	}, nil
}
