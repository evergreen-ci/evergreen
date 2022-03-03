package route

import (
	"context"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"

	dataModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
)

func makeRestartRoute(sc data.Connector, restartType string, queue amboy.Queue) gimlet.RouteHandler {
	return &restartHandler{
		queue:       queue,
		restartType: restartType,
		sc:          sc,
	}
}

type restartHandler struct {
	StartTime          time.Time `json:"start_time"`
	EndTime            time.Time `json:"end_time"`
	DryRun             bool      `json:"dry_run"`
	IncludeTestFailed  bool      `json:"include_test_failed"`
	IncludeSysFailed   bool      `json:"include_sys_failed"`
	IncludeSetupFailed bool      `json:"include_setup_failed"`

	restartType string
	sc          data.Connector
	queue       amboy.Queue
}

func (h *restartHandler) Factory() gimlet.RouteHandler {
	return &restartHandler{
		queue:       h.queue,
		restartType: h.restartType,
		sc:          h.sc,
	}
}

func (h *restartHandler) Parse(ctx context.Context, r *http.Request) error {
	if err := gimlet.GetJSON(r.Body, h); err != nil {
		return errors.Wrap(err, "problem parsing request body")
	}

	if h.EndTime.Before(h.StartTime) {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "End time cannot be before start time",
		}
	}
	if h.restartType != evergreen.RestartTasks && h.restartType != evergreen.RestartVersions {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "RestartAction type must be tasks or versions",
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
	if h.restartType == evergreen.RestartVersions {
		resp, err := h.sc.RestartFailedCommitQueueVersions(opts)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "error restarting versions"))
		}
		return gimlet.NewJSONResponse(resp)
	}

	resp, err := h.sc.RestartFailedTasks(h.queue, opts)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Error restarting tasks"))
	}
	return gimlet.NewJSONResponse(resp)
}
