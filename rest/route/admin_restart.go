package route

import (
	"context"
	"net/http"
	"time"

	dataModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
)

func makeRestartRoute(sc data.Connector, queue amboy.Queue) gimlet.RouteHandler {
	return &restartHandler{
		queue: queue,
		sc:    sc,
	}
}

type restartHandler struct {
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	DryRun     bool      `json:"dry_run"`
	OnlyRed    bool      `json:"only_red"`
	OnlyPurple bool      `json:"only_purple"`

	sc    data.Connector
	queue amboy.Queue
}

func (h *restartHandler) Factory() gimlet.RouteHandler {
	return &restartHandler{
		queue: h.queue,
		sc:    h.sc,
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

	return nil
}

func (h *restartHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	opts := dataModel.RestartTaskOptions{
		DryRun:     h.DryRun,
		OnlyRed:    h.OnlyRed,
		OnlyPurple: h.OnlyPurple,
		StartTime:  h.StartTime,
		EndTime:    h.EndTime,
		User:       u.Username(),
	}
	resp, err := h.sc.RestartFailedTasks(h.queue, opts)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error restarting tasks"))
	}

	restartModel := &model.RestartTasksResponse{}

	if err = restartModel.BuildFromService(resp); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
	}

	return gimlet.NewJSONResponse(restartModel)
}
