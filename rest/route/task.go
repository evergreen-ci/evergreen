package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// taskGetHandler implements the route GET /tasks/{task_id}. It fetches the associated
// task and returns it to the user.
type taskGetHandler struct {
	taskID             string
	fetchAllExecutions bool
	execution          int
	sc                 data.Connector
}

func makeGetTaskRoute(sc data.Connector) gimlet.RouteHandler {
	return &taskGetHandler{
		sc: sc,
	}
}

func (tgh *taskGetHandler) Factory() gimlet.RouteHandler {
	return &taskGetHandler{
		sc: tgh.sc,
	}
}

// ParseAndValidate fetches the taskId from the http request.
func (tgh *taskGetHandler) Parse(ctx context.Context, r *http.Request) error {
	tgh.taskID = gimlet.GetVars(r)["task_id"]
	_, tgh.fetchAllExecutions = r.URL.Query()["fetch_all_executions"]
	execution := r.URL.Query().Get("execution")

	if execution != "" && tgh.fetchAllExecutions {
		return gimlet.ErrorResponse{
			Message:    "fetch_all_executions=true cannot be combined with execution={task_execution}",
			StatusCode: http.StatusBadRequest,
		}
	}

	if execution != "" {
		var err error
		tgh.execution, err = strconv.Atoi(execution)
		if err != nil {
			return gimlet.ErrorResponse{
				Message:    fmt.Sprintf("Invalid execution: '%s'", err.Error()),
				StatusCode: http.StatusBadRequest,
			}
		}
	} else {
		// since an int in go defaults to 0, we won't know if the user
		// specifically wanted execution 0, or if they want the latest.
		// we use -1 to indicate "not specified"
		tgh.execution = -1
	}
	return nil
}

// Execute calls the data FindTaskById function and returns the task
// from the provider.
func (tgh *taskGetHandler) Run(ctx context.Context) gimlet.Responder {
	var foundTask *task.Task
	var err error
	if tgh.execution == -1 {
		foundTask, err = tgh.sc.FindTaskById(tgh.taskID)
	} else {
		foundTask, err = tgh.sc.FindTaskByIdAndExecution(tgh.taskID, tgh.execution)
	}
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	taskModel := &model.APITask{}
	err = taskModel.BuildFromService(foundTask)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
	}

	err = taskModel.BuildFromService(tgh.sc.GetURL())
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
	}

	if tgh.fetchAllExecutions {
		var tasks []task.Task
		tasks, err = tgh.sc.FindOldTasksByIDWithDisplayTasks(tgh.taskID)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
		}

		if err = taskModel.BuildPreviousExecutions(tasks, tgh.sc.GetURL()); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
		}
	}

	start, err := dbModel.GetEstimatedStartTime(*foundTask)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error getting estimated start time"))
	}
	taskModel.EstimatedStart = model.NewAPIDuration(start)

	err = taskModel.GetArtifacts()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error retrieving artifacts"))
	}

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
	sc             data.Connector
}

func makeFetchProjectTasks(sc data.Connector) gimlet.RouteHandler {
	return &projectTaskGetHandler{
		sc: sc,
	}
}

func (h *projectTaskGetHandler) Factory() gimlet.RouteHandler {
	return &projectTaskGetHandler{
		sc: h.sc,
	}
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
			return gimlet.ErrorResponse{
				Message:    fmt.Sprintf("problem parsing time from '%s' (%s)", startedAfter, err.Error()),
				StatusCode: http.StatusBadRequest,
			}
		}
	} else {
		// Default is 7 days before now
		h.startedAfter = time.Now().AddDate(0, 0, -7)
	}

	// Parse finished-before
	if finishedBefore != "" {
		h.finishedBefore, err = time.ParseInLocation(time.RFC3339, finishedBefore, time.UTC)
		if err != nil {
			return gimlet.ErrorResponse{
				Message:    fmt.Sprintf("problem parsing time from '%s' (%s)", finishedBefore, err.Error()),
				StatusCode: http.StatusBadRequest,
			}
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
		return gimlet.MakeJSONErrorResponder(err)
	}

	tasks, err := h.sc.FindTaskWithinTimePeriod(h.startedAfter, h.finishedBefore, h.projectId, h.statuses)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	for _, task := range tasks {
		taskModel := &model.APITask{}
		err = taskModel.BuildFromService(&task)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
		}

		if err = resp.AddData(taskModel); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return resp
}

// TaskExecutionPatchHandler implements the route PATCH /task/{task_id}. It
// fetches the changes from request, changes in activation and priority, and
// calls out to functions in the data to change these values.
type taskExecutionPatchHandler struct {
	Activated *bool  `json:"activated"`
	Priority  *int64 `json:"priority"`

	user gimlet.User
	task *task.Task
	sc   data.Connector
}

func makeModifyTaskRoute(sc data.Connector) gimlet.RouteHandler {
	return &taskExecutionPatchHandler{
		sc: sc,
	}
}

func (tep *taskExecutionPatchHandler) Factory() gimlet.RouteHandler {
	return &taskExecutionPatchHandler{
		sc: tep.sc,
	}
}

// ParseAndValidate fetches the needed data from the request and errors otherwise.
// It fetches the task and user from the request context and fetches the changes
// in activation and priority from the request body.
func (tep *taskExecutionPatchHandler) Parse(ctx context.Context, r *http.Request) error {
	body := util.NewRequestReader(r)
	defer body.Close()

	decoder := json.NewDecoder(body)
	if err := decoder.Decode(tep); err != nil {
		if err == io.EOF {
			return gimlet.ErrorResponse{
				Message:    "No request body sent",
				StatusCode: http.StatusBadRequest,
			}
		}
		if e, ok := err.(*json.UnmarshalTypeError); ok {
			return gimlet.ErrorResponse{
				Message: fmt.Sprintf("Incorrect type given, expecting '%s' "+
					"but receieved '%s'",
					e.Type, e.Value),
				StatusCode: http.StatusBadRequest,
			}
		}
		return errors.Wrap(err, "JSON unmarshal error")
	}

	if tep.Activated == nil && tep.Priority == nil {
		return gimlet.ErrorResponse{
			Message:    "Must set 'activated' or 'priority'",
			StatusCode: http.StatusBadRequest,
		}
	}
	projCtx := MustHaveProjectContext(ctx)
	if projCtx.Task == nil {
		return gimlet.ErrorResponse{
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
					Message: fmt.Sprintf("Insufficient privilege to set priority to %d, "+
						"non-superusers can only set priority at or below %d", priority, evergreen.MaxTaskPriority),
					StatusCode: http.StatusUnauthorized,
				})
			}
		}
		if err := tep.sc.SetTaskPriority(tep.task, tep.user.Username(), priority); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
		}
	}
	if tep.Activated != nil {
		activated := *tep.Activated
		if err := tep.sc.SetTaskActivated(tep.task.Id, tep.user.Username(), activated); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
		}
	}
	refreshedTask, err := tep.sc.FindTaskById(tep.task.Id)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	taskModel := &model.APITask{}
	err = taskModel.BuildFromService(refreshedTask)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	return gimlet.NewJSONResponse(taskModel)
}

// GET /tasks/{task_id}/display_task

type displayTaskGetHandler struct {
	taskID string
	sc     data.Connector
}

func makeGetDisplayTaskHandler(sc data.Connector) gimlet.RouteHandler {
	return &displayTaskGetHandler{
		sc: sc,
	}
}

func (rh *displayTaskGetHandler) Factory() gimlet.RouteHandler {
	return &displayTaskGetHandler{
		sc: rh.sc,
	}
}

func (rh *displayTaskGetHandler) Parse(ctx context.Context, r *http.Request) error {
	if rh.taskID = gimlet.GetVars(r)["task_id"]; rh.taskID == "" {
		return errors.New("missing task ID")
	}
	return nil
}

func (rh *displayTaskGetHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := rh.sc.FindTaskById(rh.taskID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "finding task with ID %s", rh.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("task with ID %s not found", rh.taskID))
	}

	dt, err := t.GetDisplayTask()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding display task for task %s", rh.taskID))
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
	sc     data.Connector
}

func makeTaskSyncPathGetHandler(sc data.Connector) gimlet.RouteHandler {
	return &taskSyncPathGetHandler{
		sc: sc,
	}
}

func (rh *taskSyncPathGetHandler) Factory() gimlet.RouteHandler {
	return &taskSyncPathGetHandler{
		sc: rh.sc,
	}
}

// ParseAndValidate fetches the needed data from the request and errors otherwise.
// It fetches the task and user from the request context and fetches the changes
// in activation and priority from the request body.
func (rh *taskSyncPathGetHandler) Parse(ctx context.Context, r *http.Request) error {
	rh.taskID = gimlet.GetVars(r)["task_id"]
	return nil
}

func (rh *taskSyncPathGetHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := rh.sc.FindTaskById(rh.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "could not find task with ID '%s'", rh.taskID))
	}
	return gimlet.NewTextResponse(t.S3Path(t.BuildVariant, t.DisplayName))
}

// POST /tasks/{task_id}/set_has_cedar_results

type taskSetHasCedarResultsHandler struct {
	taskID string
	info   apimodels.CedarTestResultsTaskInfo
	sc     data.Connector
}

func makeTaskSetHasCedarResultsHandler(sc data.Connector) gimlet.RouteHandler {
	return &taskSetHasCedarResultsHandler{
		sc: sc,
	}
}

func (rh *taskSetHasCedarResultsHandler) Factory() gimlet.RouteHandler {
	return &taskSetHasCedarResultsHandler{
		sc: rh.sc,
	}
}

func (rh *taskSetHasCedarResultsHandler) Parse(ctx context.Context, r *http.Request) error {
	rh.taskID = gimlet.GetVars(r)["task_id"]

	if err := gimlet.GetJSON(r.Body, &rh.info); err != nil {
		return errors.Wrap(err, "unmarshaling the request body")
	}

	return nil
}

func (rh *taskSetHasCedarResultsHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := rh.sc.FindTaskById(rh.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "could not find task with ID '%s'", rh.taskID))
	}

	if err = t.SetHasCedarResults(true, rh.info.Failed); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "failed to set HasCedarResults flag for task with ID '%s'", rh.taskID))
	}
	return gimlet.NewTextResponse("HasCedarResults flag set in task")
}

// GET /task/sync_read_credentials

type taskSyncReadCredentialsGetHandler struct {
	taskID string
	sc     data.Connector
}

func makeTaskSyncReadCredentialsGetHandler(sc data.Connector) gimlet.RouteHandler {
	return &taskSyncReadCredentialsGetHandler{
		sc: sc,
	}
}

func (rh *taskSyncReadCredentialsGetHandler) Factory() gimlet.RouteHandler {
	return &taskSyncReadCredentialsGetHandler{
		sc: rh.sc,
	}
}

func (rh *taskSyncReadCredentialsGetHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (rh *taskSyncReadCredentialsGetHandler) Run(ctx context.Context) gimlet.Responder {
	settings, err := rh.sc.GetEvergreenSettings()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	return gimlet.NewJSONResponse(settings.Providers.AWS.TaskSyncRead)
}
