package route

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
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
	cbvh.versionId = mux.Vars(r)["version_id"]

	if cbvh.versionId == "" {
		return errors.New("request data incomplete")
	}

	return nil
}

func (cbvh *costByVersionHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	foundVersionCost, err := sc.FindCostByVersionId(cbvh.versionId)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	versionCostModel := &model.APIVersionCost{}
	err = versionCostModel.BuildFromService(foundVersionCost)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
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
		return time.Time{}, 0, rest.APIError{
			Message:    "both starttime and duration must be given as form values",
			StatusCode: http.StatusBadRequest,
		}
	}

	// Parse time information.
	st, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		return time.Time{}, 0, rest.APIError{
			Message: fmt.Sprintf("problem parsing time from '%s' (%s). Time must be given in the following format: %s",
				startTime, err.Error(), time.RFC3339),
			StatusCode: http.StatusBadRequest,
		}
	}
	d, err := time.ParseDuration(duration)
	if err != nil {
		return time.Time{}, 0, rest.APIError{
			Message:    fmt.Sprintf("problem parsing duration from '%s' (%s). Duration must be given in the following format: 4h, 2h45m, etc.", duration, err.Error()),
			StatusCode: http.StatusBadRequest,
		}
	}
	return st, d, nil
}

func (cbvh *costByDistroHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	cbvh.distroId = mux.Vars(r)["distro_id"]
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
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	distroCostModel := &model.APIDistroCost{}
	err = distroCostModel.BuildFromService(foundDistroCost)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
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
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    c.Handler(),
				MethodType:        http.MethodGet,
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
	args := costTasksByProjectArgs{User: GetUser(ctx)}
	args.projectID = mux.Vars(r)["project_id"]
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
		if _, ok := err.(rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}

	// Make the previous page
	prevTasks, err := sc.FindCostTaskByProject(project, key, starttime, endttime, limit, -1)
	if err != nil {
		if _, ok := err.(rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
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
			return []model.Model{}, nil, rest.APIError{
				Message:    "problem converting task to APITaskCost",
				StatusCode: http.StatusInternalServerError,
			}
		}
		models = append(models, taskCostModel)
	}
	return models, pages, nil
}
