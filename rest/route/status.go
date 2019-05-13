package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

const (
	defaultDurationStatusQuery    = 30
	maxDurationStatusQueryMinutes = 24 * 60
)

type recentTasksGetHandler struct {
	minutes   int
	verbose   bool
	byDistro  bool
	byProject bool
	taskType  string
	sc        data.Connector
}

func makeRecentTaskStatusHandler(sc data.Connector) gimlet.RouteHandler {
	return &recentTasksGetHandler{
		sc: sc,
	}
}

func (h *recentTasksGetHandler) Factory() gimlet.RouteHandler {
	return &recentTasksGetHandler{
		sc: h.sc,
	}
}

func (h *recentTasksGetHandler) Parse(ctx context.Context, r *http.Request) error {
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

	byDistroStr := r.URL.Query().Get("by_distro")
	if byDistroStr == "true" || byDistroStr == "1" {
		h.byDistro = true
	}

	byProjectStr := r.URL.Query().Get("by_project")
	if byProjectStr == "true" || byProjectStr == "1" {
		h.byProject = true
	}

	if h.byDistro && h.byProject {
		return errors.New("by_distro and by_project can't both be true")
	}

	h.taskType = r.URL.Query().Get("status")

	return nil
}

func (h *recentTasksGetHandler) Run(ctx context.Context) gimlet.Responder {
	tasks, stats, err := h.sc.FindRecentTasks(h.minutes)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	if h.taskType != "" {
		tasks = task.FilterTasksOnStatus(tasks, h.taskType)
		h.verbose = true
	}

	if h.verbose {
		response := make([]model.Model, len(tasks))
		for i, t := range tasks {
			taskModel := model.APITask{}
			err = taskModel.BuildFromService(&t)
			if err != nil {
				return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
			}

			response[i] = &taskModel

		}
		return gimlet.NewJSONResponse(response)
	}

	if h.byDistro || h.byProject {
		var stats *task.ResultCountList
		if h.byDistro {
			stats, err = h.sc.FindRecentTaskListDistro(h.minutes)
		} else {
			stats, err = h.sc.FindRecentTaskListProject(h.minutes)
		}
		if err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}

		statsModel := &model.APIRecentTaskStatsList{}
		if err = statsModel.BuildFromService(stats); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
		return gimlet.NewJSONResponse(statsModel)
	}

	statsModel := &model.APIRecentTaskStats{}
	if err := statsModel.BuildFromService(stats); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	return gimlet.NewJSONResponse(statsModel)
}

// this is the route manager for /status/hosts/distros, which returns a count of up hosts grouped by distro
type hostStatsByDistroHandler struct {
	sc data.Connector
}

func makeHostStatusByDistroRoute(sc data.Connector) gimlet.RouteHandler {
	return &hostStatsByDistroHandler{
		sc: sc,
	}
}

func (h *hostStatsByDistroHandler) Factory() gimlet.RouteHandler {
	return &hostStatsByDistroHandler{sc: h.sc}
}

func (h *hostStatsByDistroHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *hostStatsByDistroHandler) Run(ctx context.Context) gimlet.Responder {
	stats, err := h.sc.GetHostStatsByDistro()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	statsModel := &model.APIHostStatsByDistro{}
	if err := statsModel.BuildFromService(stats); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	return gimlet.NewJSONResponse(statsModel)
}
