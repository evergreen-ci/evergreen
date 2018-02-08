package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

const (
	incorrectArgsTypeErrorMessage = "programmer error: incorrect type for paginator args"
)

func getTaskRestartRouteManager(route string, version int) *RouteManager {
	trh := &taskRestartHandler{}
	taskRestart := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser, PrefetchProjectContext},
		Authenticator:     &RequireUserAuthenticator{},
		RequestHandler:    trh.Handler(),
		MethodType:        http.MethodPost,
	}

	taskRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{taskRestart},
		Version: version,
	}
	return &taskRoute
}

func getTasksByBuildRouteManager(route string, version int) *RouteManager {
	tbh := &tasksByBuildHandler{}
	tasksByBuild := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &RequireUserAuthenticator{},
		RequestHandler:    tbh.Handler(),
		MethodType:        http.MethodGet,
	}

	taskRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{tasksByBuild},
		Version: version,
	}
	return &taskRoute
}

func getTaskRouteManager(route string, version int) *RouteManager {
	tep := &TaskExecutionPatchHandler{}
	taskExecutionPatch := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchProjectContext, PrefetchUser},
		Authenticator:     &NoAuthAuthenticator{},
		RequestHandler:    tep.Handler(),
		MethodType:        http.MethodPatch,
	}

	tgh := &taskGetHandler{}
	taskGet := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &RequireUserAuthenticator{},
		RequestHandler:    tgh.Handler(),
		MethodType:        http.MethodGet,
	}

	taskRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{taskExecutionPatch, taskGet},
		Version: version,
	}
	return &taskRoute
}

func getTasksByProjectAndCommitRouteManager(route string, version int) *RouteManager {
	tph := &tasksByProjectHandler{}
	tasksByProj := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &RequireUserAuthenticator{},
		RequestHandler:    tph.Handler(),
		MethodType:        http.MethodGet,
	}

	taskRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{tasksByProj},
		Version: version,
	}
	return &taskRoute
}

func getTaskAbortManager(route string, version int) *RouteManager {
	t := &taskAbortHandler{}
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				MethodType:        http.MethodPost,
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    t.Handler(),
			},
		},
	}
}

// taskByProjectHandler implements the GET /projects/{project_id}/revisions/{commit_hash}/tasks.
// It fetches the associated tasks and returns them to the user.
type tasksByProjectHandler struct {
	*PaginationExecutor
}

type tasksByProjectArgs struct {
	projectId  string
	commitHash string
	status     string
}

// ParseAndValidate fetches the project context and task status from the request
// and loads them into the arguments to be used by the execution.
func (tph *tasksByProjectHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	args := tasksByProjectArgs{
		projectId:  mux.Vars(r)["project_id"],
		commitHash: mux.Vars(r)["commit_hash"],
		status:     r.URL.Query().Get("status"),
	}
	if args.projectId == "" {
		return rest.APIError{
			Message:    "ProjectId cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}

	if args.commitHash == "" {
		return rest.APIError{
			Message:    "Revision cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}
	tph.Args = args
	return tph.PaginationExecutor.ParseAndValidate(ctx, r)
}

func tasksByProjectPaginator(key string, limit int, args interface{}, sc data.Connector) ([]model.Model,
	*PageResult, error) {
	ptArgs, ok := args.(tasksByProjectArgs)
	if !ok {
		panic("ARGS HAD WRONG TYPE!")
	}
	tasks, err := sc.FindTasksByProjectAndCommit(ptArgs.projectId, ptArgs.commitHash, key, ptArgs.status, limit*2, 1)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}

	// Make the previous page
	prevTasks, err := sc.FindTasksByProjectAndCommit(ptArgs.projectId, ptArgs.commitHash, key, ptArgs.status, limit, -1)
	if err != nil {
		if apiErr, ok := err.(*rest.APIError); !ok || apiErr.StatusCode != http.StatusNotFound {
			return []model.Model{}, nil, errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}

	nextPage := makeNextTasksPage(tasks, limit)

	pageResults := &PageResult{
		Next: nextPage,
		Prev: makePrevTasksPage(prevTasks),
	}

	lastIndex := len(tasks)
	if nextPage != nil {
		lastIndex = limit
	}

	// Truncate the tasks to just those that will be returned.
	tasks = tasks[:lastIndex]

	models := make([]model.Model, len(tasks))
	for ix, st := range tasks {
		taskModel := &model.APITask{}
		err = taskModel.BuildFromService(&st)
		if err != nil {
			return []model.Model{}, nil, err
		}
		err = taskModel.BuildFromService(sc.GetURL())
		if err != nil {
			return []model.Model{}, nil, err
		}
		models[ix] = taskModel
	}
	return models, pageResults, nil
}

func makeNextTasksPage(tasks []task.Task, limit int) *Page {
	var nextPage *Page
	if len(tasks) > limit {
		nextLimit := len(tasks) - limit
		nextPage = &Page{
			Relation: "next",
			Key:      tasks[limit].Id,
			Limit:    nextLimit,
		}
	}
	return nextPage
}

func makePrevTasksPage(tasks []task.Task) *Page {
	var prevPage *Page
	if len(tasks) > 1 {
		prevPage = &Page{
			Relation: "prev",
			Key:      tasks[0].Id,
			Limit:    len(tasks),
		}
	}
	return prevPage
}

func (tph *tasksByProjectHandler) Handler() RequestHandler {
	taskPaginationExecutor := &PaginationExecutor{
		KeyQueryParam:   "start_at",
		LimitQueryParam: "limit",
		Paginator:       tasksByProjectPaginator,
		Args:            tasksByProjectArgs{},
	}

	return &tasksByProjectHandler{taskPaginationExecutor}
}

// taskGetHandler implements the route GET /task/{task_id}. It fetches the associated
// task and returns it to the user.
type taskGetHandler struct {
	taskID             string
	fetchAllExecutions bool
}

// ParseAndValidate fetches the taskId from the http request.
func (tgh *taskGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	vars := mux.Vars(r)
	tgh.taskID = vars["task_id"]
	_, tgh.fetchAllExecutions = r.URL.Query()["fetch_all_executions"]
	return nil
}

// Execute calls the data FindTaskById function and returns the task
// from the provider.
func (tgh *taskGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	foundTask, err := sc.FindTaskById(tgh.taskID)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	taskModel := &model.APITask{}
	err = taskModel.BuildFromService(foundTask)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}

	err = taskModel.BuildFromService(sc.GetURL())
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}

	if tgh.fetchAllExecutions {
		tasks, err := sc.FindOldTasksByID(tgh.taskID)
		if err != nil {
			return ResponseData{}, errors.Wrap(err, "API model error")
		}

		if err = taskModel.BuildPreviousExecutions(tasks); err != nil {
			return ResponseData{}, errors.Wrap(err, "API model error")
		}
	}

	return ResponseData{
		Result: []model.Model{taskModel},
	}, nil
}

func (trh *taskGetHandler) Handler() RequestHandler {
	return &taskGetHandler{}
}

type tasksByBuildHandler struct {
	*PaginationExecutor
}

type tasksByBuildArgs struct {
	buildId            string
	status             string
	fetchAllExecutions bool
}

func (tbh *tasksByBuildHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	args := tasksByBuildArgs{
		buildId: mux.Vars(r)["build_id"],
		status:  r.URL.Query().Get("status"),
	}
	if args.buildId == "" {
		return rest.APIError{
			Message:    "buildId cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}
	_, args.fetchAllExecutions = r.URL.Query()["fetch_all_executions"]
	tbh.Args = args
	return tbh.PaginationExecutor.ParseAndValidate(ctx, r)
}

func tasksByBuildPaginator(key string, limit int, args interface{}, sc data.Connector) ([]model.Model,
	*PageResult, error) {
	btArgs, ok := args.(tasksByBuildArgs)
	if !ok {
		panic(incorrectArgsTypeErrorMessage)
	}
	// Fetch all of the tasks to be returned in this page plus the tasks used for
	// calculating information about the next page. Here the limit is multiplied
	// by two to fetch the next page.
	tasks, err := sc.FindTasksByBuildId(btArgs.buildId, key, btArgs.status, limit*2, 1)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}

	// Fetch tasks to get information about the previous page.
	prevTasks, err := sc.FindTasksByBuildId(btArgs.buildId, key, btArgs.status, limit, -1)
	if err != nil {
		if apiErr, ok := err.(*rest.APIError); !ok || apiErr.StatusCode != http.StatusNotFound {
			return []model.Model{}, nil, errors.Wrap(err, "Database error")
		}
	}

	nextPage := makeNextTasksPage(tasks, limit)
	pageResults := &PageResult{
		Next: nextPage,
		Prev: makePrevTasksPage(prevTasks),
	}

	lastIndex := len(tasks)
	if nextPage != nil {
		lastIndex = limit
	}

	// Truncate the tasks to just those that will be returned, removing the
	// tasks that would be used to create the next page.
	tasks = tasks[:lastIndex]

	// Create an array of models which will be returned.
	models := make([]model.Model, len(tasks))
	for ix, st := range tasks {
		taskModel := &model.APITask{}
		// Build an APIModel from the task and place it into the array.
		err = taskModel.BuildFromService(&st)
		if err != nil {
			return []model.Model{}, nil, errors.Wrap(err, "API model error")
		}
		err = taskModel.BuildFromService(sc.GetURL())
		if err != nil {
			return []model.Model{}, nil, errors.Wrap(err, "API model error")
		}

		if btArgs.fetchAllExecutions {
			oldTasks, err := sc.FindOldTasksByID(st.Id)
			if err != nil {
				return []model.Model{}, nil, errors.Wrap(err, "error fetching old tasks")
			}
			if err = taskModel.BuildPreviousExecutions(oldTasks); err != nil {
				return []model.Model{}, nil, errors.Wrap(err, "API model error")
			}
		}
		models[ix] = taskModel
	}
	return models, pageResults, nil
}

func (hgh *tasksByBuildHandler) Handler() RequestHandler {
	taskPaginationExecutor := &PaginationExecutor{
		KeyQueryParam:   "start_at",
		LimitQueryParam: "limit",
		Paginator:       tasksByBuildPaginator,

		Args: tasksByBuildArgs{},
	}

	return &tasksByBuildHandler{taskPaginationExecutor}
}

// taskRestartHandler implements the route POST /task/{task_id}/restart. It
// fetches the needed task and project and calls the service function to
// set the proper fields when reseting the task.
type taskRestartHandler struct {
	taskId  string
	project *serviceModel.Project

	username string
}

// ParseAndValidate fetches the taskId and Project from the request context and
// sets them on the taskRestartHandler to be used by Execute.
func (trh *taskRestartHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	projCtx := MustHaveProjectContext(ctx)
	if projCtx.Task == nil {
		return rest.APIError{
			Message:    "Task not found",
			StatusCode: http.StatusNotFound,
		}
	}
	if projCtx.ProjectRef == nil {
		return rest.APIError{
			Message:    "Project not found",
			StatusCode: http.StatusNotFound,
		}
	}
	trh.taskId = projCtx.Task.Id
	project, err := projCtx.GetProject()
	if err != nil || project == nil {
		return errors.Wrap(err, "Unable to fetch associated project")
	}
	trh.project = project
	u := MustHaveUser(ctx)
	trh.username = u.DisplayName()
	return nil
}

// Execute calls the data ResetTask function and returns the refreshed
// task from the service.
func (trh *taskRestartHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	err := sc.ResetTask(trh.taskId, trh.username, trh.project)
	if err != nil {
		return ResponseData{},
			rest.APIError{
				Message:    err.Error(),
				StatusCode: http.StatusBadRequest,
			}
	}

	refreshedTask, err := sc.FindTaskById(trh.taskId)
	if err != nil {
		return ResponseData{}, err
	}

	taskModel := &model.APITask{}
	err = taskModel.BuildFromService(refreshedTask)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	return ResponseData{
		Result: []model.Model{taskModel},
	}, nil
}

func (trh *taskRestartHandler) Handler() RequestHandler {
	return &taskRestartHandler{}
}

// TaskExecutionPatchHandler implements the route PATCH /task/{task_id}. It
// fetches the changes from request, changes in activation and priority, and
// calls out to functions in the data to change these values.
type TaskExecutionPatchHandler struct {
	Activated *bool  `json:"activated"`
	Priority  *int64 `json:"priority"`

	user auth.User
	task *task.Task
}

// ParseAndValidate fetches the needed data from the request and errors otherwise.
// It fetches the task and user from the request context and fetches the changes
// in activation and priority from the request body.
func (tep *TaskExecutionPatchHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	body := util.NewRequestReader(r)
	defer body.Close()

	decoder := json.NewDecoder(body)
	if err := decoder.Decode(tep); err != nil {
		if err == io.EOF {
			return rest.APIError{
				Message:    "No request body sent",
				StatusCode: http.StatusBadRequest,
			}
		}
		if e, ok := err.(*json.UnmarshalTypeError); ok {
			return rest.APIError{
				Message: fmt.Sprintf("Incorrect type given, expecting '%s' "+
					"but receieved '%s'",
					e.Type, e.Value),
				StatusCode: http.StatusBadRequest,
			}
		}
		return errors.Wrap(err, "JSON unmarshal error")
	}

	if tep.Activated == nil && tep.Priority == nil {
		return rest.APIError{
			Message:    "Must set 'activated' or 'priority'",
			StatusCode: http.StatusBadRequest,
		}
	}
	projCtx := MustHaveProjectContext(ctx)
	if projCtx.Task == nil {
		return rest.APIError{
			Message:    "Task not found",
			StatusCode: http.StatusNotFound,
		}
	}

	tep.task = projCtx.Task
	u := MustHaveUser(ctx)
	tep.user = u
	return nil
}

// Execute sets the Activated and Priority field of the given task and returns
// an updated version of the task.
func (tep *TaskExecutionPatchHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	if tep.Priority != nil {
		priority := *tep.Priority
		if priority > evergreen.MaxTaskPriority &&
			!auth.IsSuperUser(sc.GetSuperUsers(), tep.user) {
			return ResponseData{}, rest.APIError{
				Message: fmt.Sprintf("Insufficient privilege to set priority to %d, "+
					"non-superusers can only set priority at or below %d", priority, evergreen.MaxTaskPriority),
				StatusCode: http.StatusForbidden,
			}
		}
		if err := sc.SetTaskPriority(tep.task, tep.user.Username(), priority); err != nil {
			return ResponseData{}, errors.Wrap(err, "Database error")
		}
	}
	if tep.Activated != nil {
		activated := *tep.Activated
		if err := sc.SetTaskActivated(tep.task.Id, tep.user.Username(), activated); err != nil {
			return ResponseData{}, errors.Wrap(err, "Database error")
		}
	}
	refreshedTask, err := sc.FindTaskById(tep.task.Id)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error")
	}

	taskModel := &model.APITask{}
	err = taskModel.BuildFromService(refreshedTask)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	return ResponseData{
		Result: []model.Model{taskModel},
	}, nil
}

func (tep *TaskExecutionPatchHandler) Handler() RequestHandler {
	return &TaskExecutionPatchHandler{}
}

type taskAbortHandler struct {
	taskId string
}

func (t *taskAbortHandler) Handler() RequestHandler {
	return &taskAbortHandler{}
}

func (t *taskAbortHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	vars := mux.Vars(r)
	t.taskId = vars["task_id"]
	return nil
}

func (t *taskAbortHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	err := sc.AbortTask(t.taskId, GetUser(ctx).Id)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Abort error")
		}
		return ResponseData{}, err
	}

	foundTask, err := sc.FindTaskById(t.taskId)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	taskModel := &model.APITask{}
	err = taskModel.BuildFromService(foundTask)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}

	return ResponseData{
		Result: []model.Model{taskModel},
	}, nil
}
