package route

import (
	"context"
	"net/http"
	"time"

	dataModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
)

// this manages the /admin/restart route, which restarts failed tasks
func getRestartRouteManager(queue amboy.Queue) routeManagerFactory {
	return func(route string, version int) *RouteManager {
		rh := &restartHandler{
			queue: queue,
		}

		restartHandler := MethodHandler{
			Authenticator:  &SuperUserAuthenticator{},
			RequestHandler: rh.Handler(),
			MethodType:     http.MethodPost,
		}

		restartRoute := RouteManager{
			Route:   route,
			Methods: []MethodHandler{restartHandler},
			Version: version,
		}
		return &restartRoute
	}
}

type restartHandler struct {
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	DryRun     bool      `json:"dry_run"`
	OnlyRed    bool      `json:"only_red"`
	OnlyPurple bool      `json:"only_purple"`

	queue amboy.Queue
}

func (h *restartHandler) Handler() RequestHandler {
	return &restartHandler{
		queue: h.queue,
	}
}

func (h *restartHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	if err := util.ReadJSONInto(r.Body, h); err != nil {
		return err
	}
	if h.EndTime.Before(h.StartTime) {
		return rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "End time cannot be before start time",
		}
	}
	return nil
}

func (h *restartHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)
	opts := dataModel.RestartTaskOptions{
		DryRun:     h.DryRun,
		OnlyRed:    h.OnlyRed,
		OnlyPurple: h.OnlyPurple,
		StartTime:  h.StartTime,
		EndTime:    h.EndTime,
		User:       u.Username(),
	}
	resp, err := sc.RestartFailedTasks(h.queue, opts)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Error restarting tasks")
		}
		return ResponseData{}, err
	}
	restartModel := &model.RestartTasksResponse{}
	if err = restartModel.BuildFromService(resp); err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}
	return ResponseData{
		Result: []model.Model{restartModel},
	}, nil
}

func getClearTaskQueueRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			MethodHandler{
				Authenticator:  &SuperUserAuthenticator{},
				RequestHandler: &clearTaskQueueHandler{},
				MethodType:     http.MethodDelete,
			},
		},
		Version: version,
	}
}

func (h *clearTaskQueueHandler) Handler() RequestHandler {
	return &clearTaskQueueHandler{}
}

func (h *clearTaskQueueHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	h.distro = vars["distro"]
	_, err := distro.FindOne(distro.ById(h.distro))
	if err != nil {
		return &rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    "unable to find distro",
		}
	}

	return nil
}

func (h *clearTaskQueueHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	return ResponseData{}, sc.ClearTaskQueue(h.distro)
}
