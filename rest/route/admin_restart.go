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

func makeRestartRoute(restartType string, queue amboy.Queue) gimlet.RouteHandler {
	return &restartHandler{
		queue:       queue,
		restartType: restartType,
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
	queue       amboy.Queue
}

func (h *restartHandler) Factory() gimlet.RouteHandler {
	return &restartHandler{
		queue:       h.queue,
		restartType: h.restartType,
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
	if h.restartType != evergreen.RestartTasks && h.restartType != evergreen.RestartVersions {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "restart type must be tasks or versions",
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
		resp, err := data.RestartFailedCommitQueueVersions(opts)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "restarting failed commit queue versions"))
		}
		return gimlet.NewJSONResponse(resp)
	}

	resp, err := data.RestartFailedTasks(ctx, h.queue, opts)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "restarting failed tasks"))
	}
	return gimlet.NewJSONResponse(resp)
}
