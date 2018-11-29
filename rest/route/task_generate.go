package route

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func makeGenerateTasksHandler(sc data.Connector) gimlet.RouteHandler {
	return &generateHandler{
		sc: sc,
	}
}

type generateHandler struct {
	files  []json.RawMessage
	taskID string
	sc     data.Connector
}

func (h *generateHandler) Factory() gimlet.RouteHandler {
	return &generateHandler{
		sc: h.sc,
	}
}

func (h *generateHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error
	if h.files, err = parseJson(r); err != nil {
		failedJson := []byte{}
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("error reading JSON from body (%s):\n%s", err, string(failedJson)),
		}
	}
	h.taskID = gimlet.GetVars(r)["task_id"]
	if _, code, err := dbModel.ValidateTask(h.taskID, true, r); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: code,
			Message:    "task is invalid",
		}
	}
	if _, code, err := dbModel.ValidateHost("", r); err != nil {
		return gimlet.ErrorResponse{
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

func (h *generateHandler) Run(ctx context.Context) gimlet.Responder {
	if err := h.sc.GenerateTasks(ctx, h.taskID, h.files); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error generating tasks",
			"task_id": h.taskID,
		}))
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
}
