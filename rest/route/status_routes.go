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
		var taskModel *model.APITask
		models := make([]model.Model, len(tasks))
		for i, t := range tasks {
			taskModel = &model.APITask{}
			if err := taskModel.BuildFromService(&t); err != nil {
				return ResponseData{}, err
			}
			models[i] = taskModel
		}
		return ResponseData{
			Result: models,
		}, nil
	}

	models := make([]model.Model, 1)
	statsModel := &model.APIStats{}
	if err := statsModel.BuildFromService(stats); err != nil {
		return ResponseData{}, err
	}
	models[0] = statsModel
	return ResponseData{
		Result: models,
	}, nil
}
