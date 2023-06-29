package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/host"
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
	minutes        int
	verbose        bool
	byDistro       bool
	byProject      bool
	byAgentVersion bool
	taskType       string
}

func makeRecentTaskStatusHandler() gimlet.RouteHandler {
	return &recentTasksGetHandler{}
}

func (h *recentTasksGetHandler) Factory() gimlet.RouteHandler {
	return &recentTasksGetHandler{}
}

func (h *recentTasksGetHandler) Parse(ctx context.Context, r *http.Request) error {
	minutesInt, err := util.GetIntValue(r, "minutes", defaultDurationStatusQuery)
	if err != nil {
		return err
	}
	if minutesInt > maxDurationStatusQueryMinutes {
		return errors.Errorf("cannot query for more than %d minutes", maxDurationStatusQueryMinutes)
	}
	if minutesInt <= 0 {
		return errors.Errorf("minutes must be positive")
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

	byAgentVersionStr := r.URL.Query().Get("by_agent_version")
	if byAgentVersionStr == "true" || byAgentVersionStr == "1" {
		h.byAgentVersion = true
	}

	if h.byDistro && h.byProject || h.byProject && h.byAgentVersion || h.byDistro && h.byAgentVersion {
		return errors.New("only one of the following can be true: by_distro, by_project, by_agent_version")
	}

	h.taskType = r.URL.Query().Get("status")

	return nil
}

func (h *recentTasksGetHandler) Run(ctx context.Context) gimlet.Responder {
	tasks, stats, err := data.FindRecentTasks(h.minutes)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "finding recent tasks"))
	}

	if h.taskType != "" {
		tasks = task.FilterTasksOnStatus(tasks, h.taskType)
		h.verbose = true
	}

	if h.verbose {
		response := make([]model.APITask, len(tasks))
		for i, t := range tasks {
			taskModel := model.APITask{}
			err = taskModel.BuildFromService(ctx, &t, &model.APITaskArgs{
				IncludeProjectIdentifier: true,
				IncludeAMI:               true,
			})
			if err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "converting task '%s' to API model", t.Id))
			}
			response[i] = taskModel
		}
		return gimlet.NewJSONResponse(response)
	}

	if h.byDistro || h.byProject || h.byAgentVersion {
		var stats *model.APIRecentTaskStatsList
		switch {
		case h.byDistro:
			stats, err = data.FindRecentTaskListDistro(h.minutes)
		case h.byProject:
			stats, err = data.FindRecentTaskListProject(h.minutes)
		case h.byAgentVersion:
			stats, err = data.FindRecentTaskListAgentVersion(h.minutes)
		}
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "finding recent task list by filter"))
		}

		return gimlet.NewJSONResponse(stats)
	}

	statsModel := &model.APIRecentTaskStats{}
	if stats != nil {
		statsModel.BuildFromService(*stats)
	}
	return gimlet.NewJSONResponse(statsModel)
}

// this is the route manager for /status/hosts/distros, which returns a count of up hosts grouped by distro
type hostStatsByDistroHandler struct{}

func makeHostStatusByDistroRoute() gimlet.RouteHandler {
	return &hostStatsByDistroHandler{}
}

func (h *hostStatsByDistroHandler) Factory() gimlet.RouteHandler {
	return &hostStatsByDistroHandler{}
}

func (h *hostStatsByDistroHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *hostStatsByDistroHandler) Run(ctx context.Context) gimlet.Responder {
	stats, err := host.GetStatsByDistro()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting distro host stats"))
	}

	statsModel := &model.APIHostStatsByDistro{}
	statsModel.BuildFromService(stats)
	return gimlet.NewJSONResponse(statsModel)
}
