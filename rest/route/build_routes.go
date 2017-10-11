package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// Handler for fetching build by id
//
//    /builds/{build_id}

type buildGetHandler struct {
	buildId string
}

func getBuildByIdRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &buildGetHandler{},
				MethodType:     http.MethodGet,
			},
			{
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    &buildChangeStatusHandler{},
				MethodType:        http.MethodPatch,
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

type buildChangeStatusHandler struct {
	Activated *bool  `json:"activated"`
	Priority  *int64 `json:"priority"`

	buildId string
}

func (b *buildChangeStatusHandler) Handler() RequestHandler {
	return &buildChangeStatusHandler{}
}

func (b *buildChangeStatusHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	b.buildId = mux.Vars(r)["build_id"]
	body := util.NewRequestReader(r)
	defer body.Close()

	if err := util.ReadJSONInto(body, b); err != nil {
		return errors.Wrap(err, "Argument read error")
	}

	if b.Activated == nil && b.Priority == nil {
		return &rest.APIError{
			Message:    "Must set 'activated' or 'priority'",
			StatusCode: http.StatusBadRequest,
		}
	}
	return nil
}

func (b *buildChangeStatusHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	user := GetUser(ctx)
	if b.Priority != nil {
		priority := *b.Priority
		if ok := validPriority(priority, user, sc); !ok {
			return ResponseData{}, &rest.APIError{
				Message: fmt.Sprintf("Insufficient privilege to set priority to %d, "+
					"non-superusers can only set priority at or below %d", priority, evergreen.MaxTaskPriority),
				StatusCode: http.StatusForbidden,
			}
		}
		if err := sc.SetBuildPriority(b.buildId, priority); err != nil {
			return ResponseData{}, errors.Wrap(err, "Database error")
		}
	}
	if b.Activated != nil {
		if err := sc.SetBuildActivated(b.buildId, user.Username(), *b.Activated); err != nil {
			return ResponseData{}, errors.Wrap(err, "Database error")
		}
	}
	foundBuild, err := sc.FindBuildById(b.buildId)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error")
	}

	buildModel := &model.APIBuild{}
	err = buildModel.BuildFromService(*foundBuild)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	return ResponseData{
		Result: []model.Model{buildModel},
	}, nil
}

////////////////////////////////////////////////////////////////////////
//
// Handler for aborting build by id
//
//    /builds/{build_id}/abort

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
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    (&buildAbortHandler{}).Handler(),
				MethodType:        http.MethodPost,
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

////////////////////////////////////////////////////////////////////////
//
// Handler for restarting build by id
//
//    /builds/{build_id}/restart

type buildRestartHandler struct {
	buildId string
}

func getBuildRestartManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    (&buildRestartHandler{}).Handler(),
				MethodType:        http.MethodPost,
			},
		},
	}
}

func (b *buildRestartHandler) Handler() RequestHandler {
	return &buildRestartHandler{}
}

func (b *buildRestartHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	vars := mux.Vars(r)
	b.buildId = vars["build_id"]
	return nil
}

func (b *buildRestartHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	err := sc.RestartBuild(b.buildId, GetUser(ctx).Id)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Restart error")
		}
		return ResponseData{}, err
	}

	foundBuild, err := sc.FindBuildById(b.buildId)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
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
