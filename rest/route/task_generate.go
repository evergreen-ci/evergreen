package route

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// POST /task/{task_id}/generate

func makeGenerateTasksHandler(env evergreen.Environment) gimlet.RouteHandler {
	return &generateHandler{env: env}
}

type generateHandler struct {
	r      *http.Request
	files  []json.RawMessage
	taskID string
	env    evergreen.Environment
}

func (h *generateHandler) Factory() gimlet.RouteHandler {
	return &generateHandler{env: h.env}
}

func (h *generateHandler) Parse(ctx context.Context, r *http.Request) error {
	h.taskID = gimlet.GetVars(r)["task_id"]
	h.r = r
	return nil
}

func parseJson(r *http.Request) ([]json.RawMessage, error) {
	var files []json.RawMessage
	err := utility.ReadJSON(r.Body, &files)
	return files, err
}

func validateFileSize(files []json.RawMessage, maxSizeInMB int) error {
	// Don't validate if the maximum size is not set.
	if maxSizeInMB == 0 {
		return nil
	}

	maxSize := maxSizeInMB * 1024 * 1024
	fileSize := 0
	for _, f := range files {
		fileSize += len(f)
	}
	if fileSize > maxSize {
		return errors.Errorf("JSON is %d bytes, which exceeds maximum of %d bytes", fileSize, maxSize)
	}
	return nil
}

// processFiles needs to be called in Run and not Parse so that we can return a 500 error rather
// than a 400 error when the context is canceled. This will allow the client to retry.
func (h *generateHandler) processFiles(ctx context.Context, r *http.Request) error {
	// If ctx is canceled, return immediately to avoid reading a potentially corrupted request body
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var err error
	if h.files, err = parseJson(r); err != nil {
		// make sure the context wasn't canceled while we were reading the request body
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.Wrap(err, "reading raw JSON from request body")
	}

	err = validateFileSize(h.files, h.env.Settings().TaskLimits.MaxGenerateTaskJSONSize)
	return errors.Wrap(err, "validating JSON size")
}

func (h *generateHandler) Run(ctx context.Context) gimlet.Responder {
	if err := h.processFiles(ctx, h.r); err != nil {
		if utility.IsContextError(errors.Cause(err)) {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "context canceled"))
		}
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "reading JSON from request body"))
	}

	if err := data.GenerateTasks(ctx, h.env.Settings(), h.taskID, h.files); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "generating tasks for task '%s'", h.taskID))
	}

	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("task '%s' not found", h.taskID))
	}
	grip.Warning(message.WrapError(units.CreateAndEnqueueGenerateTasks(ctx, h.env, []task.Task{*t}, utility.RoundPartOfMinute(1).Format(units.TSFormat)), message.Fields{
		"message": "could not enqueue generate tasks job",
		"version": t.Version,
		"task_id": t.Id,
	}))

	return gimlet.NewJSONResponse(struct{}{})
}

// GET /task/{task_id}/generate

func makeGenerateTasksPollHandler() gimlet.RouteHandler {
	return &generatePollHandler{}
}

type generatePollHandler struct {
	taskID string
}

func (h *generatePollHandler) Factory() gimlet.RouteHandler {
	return &generatePollHandler{}
}

func (h *generatePollHandler) Parse(ctx context.Context, r *http.Request) error {
	h.taskID = gimlet.GetVars(r)["task_id"]

	return nil
}

func (h *generatePollHandler) Run(ctx context.Context) gimlet.Responder {
	finished, jobErr, err := data.GeneratePoll(ctx, h.taskID)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error polling for generated tasks",
			"task_id": h.taskID,
		}))
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "polling generate tasks for task '%s'", h.taskID))
	}

	return gimlet.NewJSONResponse(&apimodels.GeneratePollResponse{
		Finished: finished,
		Error:    jobErr,
	})
}
