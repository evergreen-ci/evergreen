package route

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	dbModel "github.com/evergreen-ci/evergreen/model"
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
	var err error
	if h.files, err = parseJson(r); err != nil {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("error reading JSON from body (%s)", err),
		}
	}
	vars := mux.Vars(r)
	h.taskID = vars["task_id"]
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
	return nil
}

func parseJson(r *http.Request) ([]json.RawMessage, error) {
	var files []json.RawMessage
	err := util.ReadJSONInto(r.Body, &files)
	return files, err
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
