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

// I'm not sure if this refactoring actually makes the code easier to understand
func getBuildIdFromRequest(r *http.Request) string {
	vars := mux.Vars(r)
	return vars["build_id"]
}

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
	bigh.buildId = getBuildIdFromRequest(r)
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

type buildIdAbortGetHandler struct {
	buildId string
}

func getBuildIdAbortRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				Authenticator:  &RequireUserAuthenticator{},
				RequestHandler: (&buildIdAbortGetHandler{}).Handler(),
				MethodType:     evergreen.MethodPost,
			},
		},
	}
}

func (b *buildIdAbortGetHandler) Handler() RequestHandler {
	return &buildIdAbortGetHandler{}
}

func (b *buildIdAbortGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	b.buildId = getBuildIdFromRequest(r)
	return nil
}

func (b *buildIdAbortGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	err := sc.AbortBuild(b.buildId, GetUser(ctx).Id)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Abort error")
		}
		return ResponseData{}, err
	}

	foundBuild, err := sc.FindBuildById(b.buildId)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	buildModel := &model.APIBuild{}
	err = buildModel.BuildFromService(*foundBuild)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}

	return ResponseData{
		Result: []model.Model{buildModel},
	}, err
}
