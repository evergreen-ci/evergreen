package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// taskGetHandler implements the route GET /tasks/{task_id}. It fetches the associated
// task and returns it to the user.
type taskGetHandler struct {
	taskID             string
	fetchAllExecutions bool
	execution          int
	url                string
}

func makeGetTaskRoute(url string) gimlet.RouteHandler {
	return &taskGetHandler{url: url}
}

func (tgh *taskGetHandler) Factory() gimlet.RouteHandler {
	return &taskGetHandler{url: tgh.url}
}

// ParseAndValidate fetches the taskId from the http request.
func (tgh *taskGetHandler) Parse(ctx context.Context, r *http.Request) error {
	tgh.taskID = gimlet.GetVars(r)["task_id"]
	_, tgh.fetchAllExecutions = r.URL.Query()["fetch_all_executions"]
	execution := r.URL.Query().Get("execution")

	if execution != "" && tgh.fetchAllExecutions {
		return errors.New("cannot both fetch all executions and also fetch a specific execution")
	}

	if execution != "" {
		var err error
		tgh.execution, err = strconv.Atoi(execution)
		if err != nil {
			return errors.Wrap(err, "invalid execution")
		}
	} else {
		// since an int in go defaults to 0, we won't know if the user
		// specifically wanted execution 0, or if they want the latest.
		// we use -1 to indicate "not specified"
		tgh.execution = -1
	}
	return nil
}

// Execute calls the data task.FindOneId function and returns the task
// from the provider.
func (tgh *taskGetHandler) Run(ctx context.Context) gimlet.Responder {
	var foundTask *task.Task
	var err error
	if tgh.execution == -1 {
		foundTask, err = task.FindOneId(tgh.taskID)
	} else {
		foundTask, err = task.FindOneIdAndExecution(tgh.taskID, tgh.execution)
	}
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "finding task"))
	}
	if foundTask == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", tgh.taskID),
		})
	}

	taskModel := &model.APITask{}
	err = taskModel.BuildFromArgs(foundTask, &model.APITaskArgs{
		IncludeProjectIdentifier: true,
		IncludeAMI:               true,
		IncludeArtifacts:         true,
		LogURL:                   tgh.url,
	})
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "converting task to API model"))
	}

	if tgh.fetchAllExecutions {
		var tasks []task.Task
		tasks, err = task.FindOldWithDisplayTasks(task.ByOldTaskID(tgh.taskID))
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "finding archived tasks"))
		}

		if err = taskModel.BuildPreviousExecutions(tasks, tgh.url); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "adding previous task executions to API model"))
		}
	}

	start, err := dbModel.GetEstimatedStartTime(*foundTask)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "getting estimated start time"))
	}
	taskModel.EstimatedStart = model.NewAPIDuration(start)

	return gimlet.NewJSONResponse(taskModel)
}

////////////////////////////////////////////////////////////////////////
//
// Handler for the tasks for a project
//
//    /projects/{project_id}/versions/tasks
type projectTaskGetHandler struct {
	startedAfter   time.Time
	finishedBefore time.Time
	projectId      string
	statuses       []string
}

func makeFetchProjectTasks() gimlet.RouteHandler {
	return &projectTaskGetHandler{}
}

func (h *projectTaskGetHandler) Factory() gimlet.RouteHandler {
	return &projectTaskGetHandler{}
}

func (h *projectTaskGetHandler) Parse(ctx context.Context, r *http.Request) error {
	// Parse project_id
	h.projectId = gimlet.GetVars(r)["project_id"]

	var err error
	vals := r.URL.Query()
	startedAfter := vals.Get("started_after")
	finishedBefore := vals.Get("finished_before")
	statuses := vals["status"]

	// Parse started-after
	if startedAfter != "" {
		h.startedAfter, err = time.ParseInLocation(time.RFC3339, startedAfter, time.UTC)
		if err != nil {
			return errors.Wrapf(err, "parsing start time %s in RFC3339 format", startedAfter)
		}
	} else {
		// Default is 7 days before now
		h.startedAfter = time.Now().AddDate(0, 0, -7)
	}

	// Parse finished-before
	if finishedBefore != "" {
		h.finishedBefore, err = time.ParseInLocation(time.RFC3339, finishedBefore, time.UTC)
		if err != nil {
			return errors.Wrapf(err, "parsing end time %s in RFC3339 format", finishedBefore)
		}
	}

	// Parse status
	if len(statuses) > 0 {
		h.statuses = statuses
	}

	return nil
}

func (h *projectTaskGetHandler) Run(ctx context.Context) gimlet.Responder {
	resp := gimlet.NewResponseBuilder()
	if err := resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting JSON response format"))
	}
	tasks, err := findTaskWithinTimePeriod(h.startedAfter, h.finishedBefore, h.projectId, h.statuses)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "finding tasks within range"))
	}

	for _, task := range tasks {
		taskModel := &model.APITask{}
		err = taskModel.BuildFromArgs(&task, &model.APITaskArgs{IncludeProjectIdentifier: true, IncludeAMI: true})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "converting task to API model"))
		}
		if err = resp.AddData(taskModel); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "adding response data for task '%s'", task.Id))
		}
	}

	return resp
}

func findTaskWithinTimePeriod(startedAfter, finishedBefore time.Time,
	project string, statuses []string) ([]task.Task, error) {
	id, err := dbModel.GetIdForProject(project)
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"func":    "FindTaskWithinTimePeriod",
			"message": "error getting id for project",
			"project": project,
		}))
		// don't return an error here to preserve existing behavior
		return nil, nil
	}

	tasks, err := task.Find(task.WithinTimePeriod(startedAfter, finishedBefore, id, statuses))

	if err != nil {
		return nil, err
	}

	return tasks, nil
}

// TaskExecutionPatchHandler implements the route PATCH /task/{task_id}. It
// fetches the changes from request, changes in activation and priority, and
// calls out to functions in the data to change these values.
type taskExecutionPatchHandler struct {
	Activated *bool  `json:"activated"`
	Priority  *int64 `json:"priority"`

	user gimlet.User
	task *task.Task
}

func makeModifyTaskRoute() gimlet.RouteHandler {
	return &taskExecutionPatchHandler{}
}

func (tep *taskExecutionPatchHandler) Factory() gimlet.RouteHandler {
	return &taskExecutionPatchHandler{}
}

// ParseAndValidate fetches the needed data from the request and errors otherwise.
// It fetches the task and user from the request context and fetches the changes
// in activation and priority from the request body.
func (tep *taskExecutionPatchHandler) Parse(ctx context.Context, r *http.Request) error {
	if err := utility.ReadJSON(r.Body, tep); err != nil {
		return errors.Wrap(err, "reading task modification options from JSON request body")
	}

	if tep.Activated == nil && tep.Priority == nil {
		return errors.New("must set activated or priority")
	}
	projCtx := MustHaveProjectContext(ctx)
	if projCtx.Task == nil {
		return gimlet.ErrorResponse{
			Message:    "task not found",
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
func (tep *taskExecutionPatchHandler) Run(ctx context.Context) gimlet.Responder {
	if tep.Priority != nil {
		priority := *tep.Priority
		if priority > evergreen.MaxTaskPriority {
			requiredPermission := gimlet.PermissionOpts{
				Resource:      tep.task.Project,
				ResourceType:  "project",
				Permission:    evergreen.PermissionTasks,
				RequiredLevel: evergreen.TasksAdmin.Value,
			}
			if !tep.user.HasPermission(requiredPermission) {
				return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					Message: fmt.Sprintf("insufficient privilege to set priority to %d, "+
						"non-superusers can only set priority at or below %d", priority, evergreen.MaxTaskPriority),
					StatusCode: http.StatusUnauthorized,
				})
			}
		}
		if err := dbModel.SetTaskPriority(*tep.task, priority, tep.user.Username()); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting task priority"))
		}
	}
	if tep.Activated != nil {
		activated := *tep.Activated
		if err := dbModel.SetActiveStateById(tep.task.Id, tep.user.Username(), activated); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting task activation state"))
		}
	}
	refreshedTask, err := task.FindOneId(tep.task.Id)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", tep.task.Id))
	}
	if refreshedTask == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", tep.task.Id),
		})
	}

	taskModel := &model.APITask{}
	err = taskModel.BuildFromArgs(refreshedTask, &model.APITaskArgs{
		IncludeProjectIdentifier: true,
		IncludeAMI:               true,
	})
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "converting task to API model"))
	}

	return gimlet.NewJSONResponse(taskModel)
}

// GET /tasks/{task_id}/display_task

type displayTaskGetHandler struct {
	taskID string
}

func makeGetDisplayTaskHandler() gimlet.RouteHandler {
	return &displayTaskGetHandler{}
}

func (rh *displayTaskGetHandler) Factory() gimlet.RouteHandler {
	return &displayTaskGetHandler{}
}

func (rh *displayTaskGetHandler) Parse(ctx context.Context, r *http.Request) error {
	if rh.taskID = gimlet.GetVars(r)["task_id"]; rh.taskID == "" {
		return errors.New("missing task ID")
	}
	return nil
}

func (rh *displayTaskGetHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(rh.taskID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "finding task '%s'", rh.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", rh.taskID),
		})
	}

	dt, err := t.GetDisplayTask()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding display task for task '%s'", rh.taskID))
	}

	info := &apimodels.DisplayTaskInfo{}
	if dt != nil {
		info.ID = dt.Id
		info.Name = dt.DisplayName
	}
	return gimlet.NewJSONResponse(info)
}

// GET /tasks/{task_id}/sync_path

type taskSyncPathGetHandler struct {
	taskID string
}

func makeTaskSyncPathGetHandler() gimlet.RouteHandler {
	return &taskSyncPathGetHandler{}
}

func (rh *taskSyncPathGetHandler) Factory() gimlet.RouteHandler {
	return &taskSyncPathGetHandler{}
}

// ParseAndValidate fetches the needed data from the request and errors otherwise.
// It fetches the task and user from the request context and fetches the changes
// in activation and priority from the request body.
func (rh *taskSyncPathGetHandler) Parse(ctx context.Context, r *http.Request) error {
	rh.taskID = gimlet.GetVars(r)["task_id"]
	return nil
}

func (rh *taskSyncPathGetHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(rh.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", rh.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", rh.taskID),
		})
	}
	return gimlet.NewTextResponse(t.S3Path(t.BuildVariant, t.DisplayName))
}

// POST /tasks/{task_id}/set_has_cedar_results

type taskSetHasCedarResultsHandler struct {
	taskID string
	info   apimodels.CedarTestResultsTaskInfo
}

func makeTaskSetHasCedarResultsHandler() gimlet.RouteHandler {
	return &taskSetHasCedarResultsHandler{}
}

func (rh *taskSetHasCedarResultsHandler) Factory() gimlet.RouteHandler {
	return &taskSetHasCedarResultsHandler{}
}

func (rh *taskSetHasCedarResultsHandler) Parse(ctx context.Context, r *http.Request) error {
	rh.taskID = gimlet.GetVars(r)["task_id"]

	if err := gimlet.GetJSON(r.Body, &rh.info); err != nil {
		return errors.Wrap(err, "reading Cedar test results info from JSON request body")
	}

	return nil
}

func (rh *taskSetHasCedarResultsHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(rh.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", rh.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", rh.taskID),
		})
	}

	if err = t.SetHasCedarResults(true, rh.info.Failed); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "setting HasCedarResults flag for task '%s'", rh.taskID))
	}
	return gimlet.NewTextResponse("HasCedarResults flag set in task")
}

// GET /task/sync_read_credentials

type taskSyncReadCredentialsGetHandler struct{}

func makeTaskSyncReadCredentialsGetHandler() gimlet.RouteHandler {
	return &taskSyncReadCredentialsGetHandler{}
}

func (rh *taskSyncReadCredentialsGetHandler) Factory() gimlet.RouteHandler {
	return &taskSyncReadCredentialsGetHandler{}
}

func (rh *taskSyncReadCredentialsGetHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (rh *taskSyncReadCredentialsGetHandler) Run(ctx context.Context) gimlet.Responder {
	settings, err := evergreen.GetConfig()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	return gimlet.NewJSONResponse(settings.Providers.AWS.TaskSyncRead)
}
