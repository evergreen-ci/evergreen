package route

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func getGenerateManager(route string, version int) *RouteManager {
	h := &generateHandler{}
	generate := MethodHandler{
		RequestHandler: h.Handler(),
		MethodType:     http.MethodPost,
	}

	return &RouteManager{
		Route:   route,
		Methods: []MethodHandler{generate},
		Version: version,
	}
}

type generateHandler struct {
	files  []json.RawMessage
	taskID string
}

func (h *generateHandler) Handler() RequestHandler {
	return &generateHandler{}
}

func (h *generateHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	var files []json.RawMessage
	if err := util.ReadJSONInto(r.Body, &files); err != nil {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "error reading JSON from body",
		}
	}
	h.files = files
	vars := mux.Vars(r)
	if vars["task_id"] != "" {
		h.taskID = vars["task_id"]
	} else {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "task_id must not be empty",
		}
	}
	return nil
}

func (h *generateHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	if err := sc.GenerateTasks(h.taskID, h.files); err != nil {
		grip.Error(message.Fields{
			"message": "error generating tasks",
			"error":   err,
			"task_id": h.taskID,
		})
		return ResponseData{}, errors.Wrap(err, "error generating tasks")
	}
	return ResponseData{}, nil
}
