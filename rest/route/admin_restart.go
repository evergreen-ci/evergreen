package route

import (
	"context"
	"net/http"
	"time"

	dataModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
)

func makeRestartRoute(queue amboy.Queue) gimlet.RouteHandler {
	return &restartHandler{
		queue: queue,
	}
}

type restartHandler struct {
	StartTime          time.Time `json:"start_time"`
	EndTime            time.Time `json:"end_time"`
	DryRun             bool      `json:"dry_run"`
	IncludeTestFailed  bool      `json:"include_test_failed"`
	IncludeSysFailed   bool      `json:"include_sys_failed"`
	IncludeSetupFailed bool      `json:"include_setup_failed"`
	queue              amboy.Queue
}

func (h *restartHandler) Factory() gimlet.RouteHandler {
	return &restartHandler{
		queue: h.queue,
	}
}

func (h *restartHandler) Parse(ctx context.Context, r *http.Request) error {
	if err := gimlet.GetJSON(r.Body, h); err != nil {
		return errors.Wrap(err, "parsing request body")
	}

	if h.EndTime.Before(h.StartTime) {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "end time cannot be before start time",
		}
	}

	return nil
}

func (h *restartHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	opts := dataModel.RestartOptions{
		DryRun:             h.DryRun,
		IncludeTestFailed:  h.IncludeTestFailed,
		IncludeSysFailed:   h.IncludeSysFailed,
		IncludeSetupFailed: h.IncludeSetupFailed,
		StartTime:          h.StartTime,
		EndTime:            h.EndTime,
		User:               u.Username(),
	}
	resp, err := data.RestartFailedTasks(ctx, h.queue, opts)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "restarting failed tasks"))
	}
	return gimlet.NewJSONResponse(resp)
}
