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

type buildGetHandler struct {
	buildId string
}

func getBuildGetRouteManager(route string, version int) *RouteManager {
	b := &buildGetHandler{}
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: b.Handler(),
				MethodType:     evergreen.MethodGet,
			},
		},
	}
}

func (b *buildGetHandler) Handler() RequestHandler {
	return &buildGetHandler{}
}

func (b *buildGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	vars := mux.Vars(r)
	b.buildId = vars["build_id"]
	return nil
}

func (b *buildGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	foundBuild, err := sc.FindBuildById(b.buildId)
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

type buildAbortHandler struct {
	buildId string
}

func getBuildAbortRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				Authenticator:  &RequireUserAuthenticator{},
				RequestHandler: (&buildAbortHandler{}).Handler(),
				MethodType:     evergreen.MethodPost,
			},
		},
	}
}

func (b *buildAbortHandler) Handler() RequestHandler {
	return &buildAbortHandler{}
}

func (b *buildAbortHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	vars := mux.Vars(r)
	b.buildId = vars["build_id"]
	return nil
}

func (b *buildAbortHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
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
