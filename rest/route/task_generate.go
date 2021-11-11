package route

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func makeGenerateTasksHandler(sc data.Connector, q amboy.QueueGroup) gimlet.RouteHandler {
	return &generateHandler{
		sc:    sc,
		queue: q,
	}
}

type generateHandler struct {
	files  []json.RawMessage
	taskID string
	sc     data.Connector
	queue  amboy.QueueGroup
}

func (h *generateHandler) Factory() gimlet.RouteHandler {
	return &generateHandler{
		sc:    h.sc,
		queue: h.queue,
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

	return nil
}

func parseJson(r *http.Request) ([]json.RawMessage, error) {
	var files []json.RawMessage
	err := utility.ReadJSON(r.Body, &files)
	return files, err
}

func (h *generateHandler) Run(ctx context.Context) gimlet.Responder {
	if err := h.sc.GenerateTasks(ctx, h.taskID, h.files, h.queue); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error generating tasks",
			"task_id": h.taskID,
		}))
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
}

func makeGenerateTasksPollHandler(sc data.Connector, q amboy.QueueGroup) gimlet.RouteHandler {
	return &generatePollHandler{
		sc:    sc,
		queue: q,
	}
}

type generatePollHandler struct {
	taskID string
	sc     data.Connector
	queue  amboy.QueueGroup
}

func (h *generatePollHandler) Factory() gimlet.RouteHandler {
	return &generatePollHandler{
		sc:    h.sc,
		queue: h.queue,
	}
}

func (h *generatePollHandler) Parse(ctx context.Context, r *http.Request) error {
	h.taskID = gimlet.GetVars(r)["task_id"]

	return nil
}

func (h *generatePollHandler) Run(ctx context.Context) gimlet.Responder {
	finished, jobErrs, err := h.sc.GeneratePoll(ctx, h.taskID, h.queue)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error polling for generated tasks",
			"task_id": h.taskID,
		}))
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	shouldExit := false
	if len(jobErrs) > 0 { // exit early if we know the error will keep recurring
		jobErr := errors.New(strings.Join(jobErrs, ", "))
		shouldExit = db.IsDocumentLimit(jobErr)
	}
	return gimlet.NewJSONResponse(&apimodels.GeneratePollResponse{
		Finished:   finished,
		ShouldExit: shouldExit,
		Errors:     jobErrs,
	})
}
