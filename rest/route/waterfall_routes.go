package route

import (
	"context"
	"net/http"
	"strconv"

	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

type waterfallDataGetHandler struct {
	project string
	limit   int
	skip    int
	variant string
}

func (h *waterfallDataGetHandler) Handler() RequestHandler {
	return &waterfallDataGetHandler{}
}

func getWaterfallDataManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &waterfallDataGetHandler{},
				MethodType:     http.MethodGet,
			},
		},
		Version: version,
	}
}

func (h *waterfallDataGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	var err error
	vars := mux.Vars(r)
	h.project = vars["project_id"]
	query := r.URL.Query()

	limit := query.Get(dbModel.WaterfallLimitParam)
	if limit != "" {
		h.limit, err = strconv.Atoi(limit)
		if err != nil {
			return rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "Invalid limit",
			}
		}
	} else {
		h.limit = dbModel.WaterfallPerPageLimit
	}

	skip := query.Get(dbModel.WaterfallSkipParam)
	if skip != "" {
		h.skip, err = strconv.Atoi(skip)
		if err != nil {
			return rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "Invalid skip value",
			}
		}
	} else {
		h.skip = 0
	}

	h.variant = query.Get(dbModel.WaterfallBVFilterParam)

	return nil
}

func (h *waterfallDataGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	projRef, err := dbModel.FindOneProjectRef(h.project)
	if err != nil {
		return ResponseData{}, rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Project not found",
		}
	}

	project, err := dbModel.FindProject("", projRef)
	if err != nil {
		return ResponseData{}, rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Project not found",
		}
	}

	vvData, err := dbModel.GetWaterfallVersionsAndVariants(
		h.skip, h.limit, project, h.variant,
	)

	if err != nil {
		return ResponseData{}, rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "Can't get versions and variants data").Error(),
		}
	}

	finalData, err := dbModel.WaterfallDataAdaptor(vvData, project, h.skip, h.limit, h.variant)

	if err != nil {
		return ResponseData{}, rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "Can't process waterfal data").Error(),
		}
	}

	wrapper := &model.APIWaterfallData{}
	err = wrapper.BuildFromService(finalData)
	if err != nil {
		return ResponseData{}, rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "Can't cast waterfal data").Error(),
		}
	}

	return ResponseData{
		Result: []model.Model{wrapper},
	}, nil
}
