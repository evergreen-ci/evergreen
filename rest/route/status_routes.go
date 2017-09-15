package route

import (
	"net/http"
	"regexp"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

const (
	defaultDurationMinutes = 30
	maxDurationMinutes     = 24 * 60
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
				MethodType:     evergreen.MethodGet,
			},
		},
		Version: version,
	}
}

func (h *recentTasksGetHandler) Handler() RequestHandler {
	return &recentTasksGetHandler{}
}

func (h *recentTasksGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	minutes := r.URL.Query().Get("minutes")
	minutesParsed, err := h.parseMinutes(minutes)
	if err != nil {
		return err
	}
	if minutesParsed > maxDurationMinutes {
		return errors.Errorf("Cannot query for more than %d minutes", maxDurationMinutes)
	}
	h.minutes = minutesParsed

	tasksStr := r.URL.Query().Get("verbose")
	if tasksStr == "true" {
		h.verbose = true
	}

	return nil
}

func (h *recentTasksGetHandler) parseMinutes(minutes string) (minutesInt int, err error) {
	re := regexp.MustCompile("[0-9]+")
	minutesParsed := re.FindString(minutes)
	if minutesParsed == "" {
		minutesInt = defaultDurationMinutes
	} else {
		minutesInt, err = strconv.Atoi(minutesParsed)
		if err != nil {
			return 0, err
		}
	}
	return minutesInt, nil
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
