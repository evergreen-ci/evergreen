package route

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// types and functions for Version Cost Route
type costByVersionHandler struct {
	versionId string
}

func getCostByVersionIdRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &costByVersionHandler{},
				MethodType:     http.MethodGet,
			},
		},
		Version: version,
	}
}

func (cbvh *costByVersionHandler) Handler() RequestHandler {
	return &costByVersionHandler{}
}

func (cbvh *costByVersionHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	cbvh.versionId = gimlet.GetVars(r)["version_id"]

	if cbvh.versionId == "" {
		return errors.New("request data incomplete")
	}

	return nil
}

func (cbvh *costByVersionHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	foundVersionCost, err := sc.FindCostByVersionId(cbvh.versionId)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error")
	}

	versionCostModel := &model.APIVersionCost{}
	err = versionCostModel.BuildFromService(foundVersionCost)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "API model error")
	}
	return ResponseData{
		Result: []model.Model{versionCostModel},
	}, nil
}

// types and functions for Distro Cost Route
type costByDistroHandler struct {
	distroId  string
	startTime time.Time
	duration  time.Duration
}

func getCostByDistroIdRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &costByDistroHandler{},
				MethodType:     http.MethodGet,
			},
		},
		Version: version,
	}
}

func (cbvh *costByDistroHandler) Handler() RequestHandler {
	return &costByDistroHandler{}
}

func parseTime(r *http.Request) (time.Time, time.Duration, error) {
	// Parse start time and duration
	startTime := r.FormValue("starttime")
	duration := r.FormValue("duration")

	// Invalid if not both starttime and duration are given.
	if startTime == "" || duration == "" {
		return time.Time{}, 0, gimlet.ErrorResponse{
			Message:    "both starttime and duration must be given as form values",
			StatusCode: http.StatusBadRequest,
		}
	}

	// Parse time information.
	st, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		return time.Time{}, 0, gimlet.ErrorResponse{
			Message: fmt.Sprintf("problem parsing time from '%s' (%s). Time must be given in the following format: %s",
				startTime, err.Error(), time.RFC3339),
			StatusCode: http.StatusBadRequest,
		}
	}
	d, err := time.ParseDuration(duration)
	if err != nil {
		return time.Time{}, 0, gimlet.ErrorResponse{
			Message:    fmt.Sprintf("problem parsing duration from '%s' (%s). Duration must be given in the following format: 4h, 2h45m, etc.", duration, err.Error()),
			StatusCode: http.StatusBadRequest,
		}
	}
	return st, d, nil
}

func (cbvh *costByDistroHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	cbvh.distroId = gimlet.GetVars(r)["distro_id"]
	if cbvh.distroId == "" {
		return errors.New("request data incomplete")
	}

	st, d, err := parseTime(r)
	if err != nil {
		return err
	}
	cbvh.startTime = st
	cbvh.duration = d

	return nil
}

func (cbvh *costByDistroHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	foundDistroCost, err := sc.FindCostByDistroId(cbvh.distroId, cbvh.startTime, cbvh.duration)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error")
	}

	distroCostModel := &model.APIDistroCost{}
	err = distroCostModel.BuildFromService(foundDistroCost)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "API model error")
	}
	return ResponseData{
		Result: []model.Model{distroCostModel},
	}, nil
}

type costTasksByProjectHandler struct {
	*PaginationExecutor
}

type costTasksByProjectArgs struct {
	projectID string
	starttime time.Time
	duration  time.Duration
	User      *user.DBUser
}

func getCostTaskByProjectRouteManager(route string, version int) *RouteManager {
	c := &costTasksByProjectHandler{}
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &RequireUserAuthenticator{},
				RequestHandler: c.Handler(),
				MethodType:     http.MethodGet,
			},
		},
		Version: version,
	}
}

func (h *costTasksByProjectHandler) Handler() RequestHandler {
	return &costTasksByProjectHandler{&PaginationExecutor{
		KeyQueryParam:   "start_at",
		LimitQueryParam: "limit",
		Paginator:       costTasksByProjectPaginator,
	}}
}

func (h *costTasksByProjectHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	usr := MustHaveUser(ctx)
	args := costTasksByProjectArgs{User: usr}
	args.projectID = gimlet.GetVars(r)["project_id"]
	if args.projectID == "" {
		return errors.New("request data incomplete")
	}

	st, d, err := parseTime(r)
	if err != nil {
		return err
	}
	args.starttime = st
	args.duration = d

	h.Args = args
	return h.PaginationExecutor.ParseAndValidate(ctx, r)
}

func costTasksByProjectPaginator(key string, limit int, args interface{},
	sc data.Connector) ([]model.Model, *PageResult, error) {
	grip.Debugln("fetching all tasks for project given time range")

	project := args.(costTasksByProjectArgs).projectID
	starttime := args.(costTasksByProjectArgs).starttime
	endttime := starttime.Add(args.(costTasksByProjectArgs).duration)

	tasks, err := sc.FindCostTaskByProject(project, key, starttime, endttime, limit*2, 1)
	if err != nil {
		return []model.Model{}, nil, errors.Wrap(err, "Database error")
	}

	// Make the previous page
	prevTasks, err := sc.FindCostTaskByProject(project, key, starttime, endttime, limit, -1)
	if err != nil {
		return []model.Model{}, nil, errors.Wrap(err, "Database error")
	}

	// Populate page info
	pages := &PageResult{}
	if len(tasks) > limit {
		pages.Next = &Page{
			Relation: "next",
			Key:      tasks[limit].Id,
			Limit:    len(tasks) - limit,
		}
	}
	if len(prevTasks) >= 1 {
		pages.Prev = &Page{
			Relation: "prev",
			Key:      prevTasks[len(prevTasks)-1].Id,
			Limit:    len(prevTasks),
		}
	}
	// Truncate results data if there's a next page
	if pages.Next != nil {
		tasks = tasks[:limit]
	}
	models := []model.Model{}
	for _, t := range tasks {
		taskCostModel := &model.APITaskCost{}
		if err = taskCostModel.BuildFromService(t); err != nil {
			return []model.Model{}, nil, gimlet.ErrorResponse{
				Message:    "problem converting task to APITaskCost",
				StatusCode: http.StatusInternalServerError,
			}
		}
		models = append(models, taskCostModel)
	}
	return models, pages, nil
}
