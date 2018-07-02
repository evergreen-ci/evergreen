package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
)

type listCreateHostHandler struct {
	taskID string
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
	results := make([]model.Model, len(hosts))
	for i := range hosts {
		results[i] = model.Model(&hosts[i])
	}
	return ResponseData{Result: results}, nil
}
