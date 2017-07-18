package route

import (
	"fmt"
	"net/http"
	"time"

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
	distroId  string
	startTime time.Time
	duration  time.Duration
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

	// Parse start time and duration
	startTime := r.FormValue("starttime")
	duration := r.FormValue("duration")

	// Invalid if not both starttime and duration are given.
	if startTime == "" || duration == "" {
		return rest.APIError{
			Message:    "both starttime and duration must be given as form values",
			StatusCode: http.StatusBadRequest,
		}
	}

	// Parse time information.
	st, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		return rest.APIError{
			Message: fmt.Sprintf("problem parsing time from '%s' (%s). Time must be given in the following format: %s",
				startTime, err.Error(), time.RFC3339),
			StatusCode: http.StatusBadRequest,
		}
	}
	d, err := time.ParseDuration(duration)
	if err != nil {
		return rest.APIError{
			Message:    fmt.Sprintf("problem parsing duration from '%s' (%s). Duration must be given in the following format: 4h, 2h45m, etc.", duration, err.Error()),
			StatusCode: http.StatusBadRequest,
		}
	}

	cbvh.startTime = st
	cbvh.duration = d

	return nil
}

func (cbvh *costByDistroHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	foundDistroCost, err := sc.FindCostByDistroId(cbvh.distroId, cbvh.startTime, cbvh.duration)
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
