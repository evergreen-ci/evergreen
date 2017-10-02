package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

const (
	defaultDurationStatusQuery    = 30
	maxDurationStatusQueryMinutes = 24 * 60
)

type recentTasksGetHandler struct {
	minutes int
	verbose bool
}

func getRecentTasksRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &recentTasksGetHandler{},
				MethodType:     http.MethodGet,
			},
		},
		Version: version,
	}
}

func (h *recentTasksGetHandler) Handler() RequestHandler {
	return &recentTasksGetHandler{}
}

func (h *recentTasksGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	minutesInt, err := util.GetIntValue(r, "minutes", defaultDurationStatusQuery)
	if err != nil {
		return err
	}
	if minutesInt > maxDurationStatusQueryMinutes {
		return errors.Errorf("Cannot query for more than %d minutes", maxDurationStatusQueryMinutes)
	}
	if minutesInt <= 0 {
		return errors.Errorf("Minutes must be positive")
	}
	h.minutes = minutesInt

	tasksStr := r.URL.Query().Get("verbose")
	if tasksStr == "true" {
		h.verbose = true
	}

	return nil
}

func (h *recentTasksGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	tasks, stats, err := sc.FindRecentTasks(h.minutes)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	if h.verbose {
		response := make([]model.Model, len(tasks))
		for i, t := range tasks {
			taskModel := model.APITask{}
			err = taskModel.BuildFromService(&t)
			if err != nil {
				if _, ok := err.(*rest.APIError); !ok {
					err = errors.Wrap(err, "API model error")
				}
				return ResponseData{}, err
			}

			response[i] = &taskModel

		}
		return ResponseData{
			Result: response,
		}, nil
	}

	models := make([]model.Model, 1)
	statsModel := &model.APITaskStats{}
	if err := statsModel.BuildFromService(stats); err != nil {
		return ResponseData{}, err
	}
	models[0] = statsModel
	return ResponseData{
		Result: models,
	}, nil
}

// this is the route manager for /status/hosts/distros, which returns a count of up hosts grouped by distro
type hostStatsByDistroHandler struct{}

func getHostStatsByDistroManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &hostStatsByDistroHandler{},
				MethodType:     http.MethodGet,
			},
		},
		Version: version,
	}
}

func (h *hostStatsByDistroHandler) Handler() RequestHandler {
	return &hostStatsByDistroHandler{}
}

func (h *hostStatsByDistroHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *hostStatsByDistroHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	stats, err := sc.GetHostStatsByDistro()
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	statsModel := &model.APIHostStatsByDistro{}
	if err := statsModel.BuildFromService(stats); err != nil {
		return ResponseData{}, err
	}
	return ResponseData{
		Result: []model.Model{statsModel},
	}, nil
}
