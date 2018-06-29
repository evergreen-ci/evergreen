package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/apimodels"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type createHostHandler struct {
	taskID     string
	createHost apimodels.CreateHost
}

type listCreateHostHandler struct {
	taskID string
}

func getCreateHostRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &createHostHandler{},
				MethodType:     http.MethodPost,
			},
		},
	}
}

func (h *createHostHandler) Handler() RequestHandler {
	return &createHostHandler{}
}

func (h *createHostHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	taskID := mux.Vars(r)["task_id"]
	if taskID == "" {
		return rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "must provide task ID",
		}
	}
	h.taskID = taskID
	if _, code, err := dbModel.ValidateTask(h.taskID, true, r); err != nil {
		return &rest.APIError{
			StatusCode: code,
			Message:    "task is invalid",
		}
	}
	if _, code, err := dbModel.ValidateHost("", r); err != nil {
		return &rest.APIError{
			StatusCode: code,
			Message:    "host is invalid",
		}
	}
	if err := util.ReadJSONInto(r.Body, h.createHost); err != nil {
		return rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}
	return nil
}

func (h *createHostHandler) Execute(ctx context.Context, c data.Connector) (ResponseData, error) {
	err := c.CreateHostsForTask(h.taskID, h.createHost)
	if err != nil {
		return ResponseData{}, rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "error creating hosts for task",
		}
	}
	return ResponseData{}, nil
}

func getListCreateHostRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &listCreateHostHandler{},
				MethodType:     http.MethodGet,
			},
		},
		Version: version,
	}
}

func (h *listCreateHostHandler) Handler() RequestHandler {
	return &listCreateHostHandler{}
}

func (h *listCreateHostHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	taskID := gimlet.GetVars(r)["task_id"]
	if taskID == "" {
		return rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "must provide task ID",
		}
	}
	h.taskID = taskID
	return nil
}

func (h *listCreateHostHandler) Execute(ctx context.Context, c data.Connector) (ResponseData, error) {
	hosts, err := c.ListHostsForTask(h.taskID)
	if err != nil {
		return ResponseData{}, rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "error listing hosts for task",
		}
	}
	catcher := grip.NewBasicCatcher()
	results := make([]model.Model, len(hosts))
	for i := range hosts {
		createHost := model.CreateHost{}
		if err := createHost.BuildFromService(&hosts[i]); err != nil {
			catcher.Add(errors.Wrap(err, "error building api host from service"))
		}
		results[i] = &createHost
	}
	if catcher.HasErrors() {
		return ResponseData{}, rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    catcher.String(),
		}
	}
	return ResponseData{Result: results}, nil
}
