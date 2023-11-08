package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// taskGetHandler implements the route GET /tasks/{task_id}. It fetches the associated
// task and returns it to the user.
type taskGetHandler struct {
	taskID             string
	fetchAllExecutions bool
	execution          int

	url        string
	parsleyURL string
}

func makeGetTaskRoute(parsleyURL, url string) gimlet.RouteHandler {
	return &taskGetHandler{
		parsleyURL: parsleyURL,
		url:        url}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get a single task
//	@Description	Fetch a single task using its ID
//	@Tags			tasks
//	@Router			/tasks/{task_id} [get]
//	@Security		Api-User || Api-Key
//	@Param			task_id					path		string	true	"task ID"
//	@Param			execution				query		int		false	"The 0-based number corresponding to the execution of the task ID. Defaults to the latest execution"
//	@Param			fetch_all_executions	query		string	false	"Fetches previous executions of the task if they are available"
//	@Success		200						{object}	model.APITask
func (tgh *taskGetHandler) Factory() gimlet.RouteHandler {
	return &taskGetHandler{parsleyURL: tgh.parsleyURL, url: tgh.url}
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
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", tgh.taskID))
	}
	if foundTask == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", tgh.taskID),
		})
	}

	taskModel := &model.APITask{}
	err = taskModel.BuildFromService(ctx, foundTask, &model.APITaskArgs{
		IncludeProjectIdentifier: true,
		IncludeAMI:               true,
		IncludeArtifacts:         true,
		LogURL:                   tgh.url,
		ParsleyLogURL:            tgh.parsleyURL,
	})
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "converting task '%s' to API model", tgh.taskID))
	}

	if tgh.fetchAllExecutions {
		var tasks []task.Task
		tasks, err = task.FindOldWithDisplayTasks(task.ByOldTaskID(tgh.taskID))
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding archived executions for task '%s'", tgh.taskID))
		}

		if err = taskModel.BuildPreviousExecutions(ctx, tasks, tgh.url, tgh.parsleyURL); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "adding previous task executions to API model for task '%s'", tgh.taskID))
		}
	}

	start, err := dbModel.GetEstimatedStartTime(ctx, *foundTask)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting estimated start time for task '%s'", tgh.taskID))
	}
	taskModel.EstimatedStart = model.NewAPIDuration(start)

	return gimlet.NewJSONResponse(taskModel)
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

// Factory creates an instance of the handler.
//
//	@Summary		Change a task's execution status
//	@Description	Change the current execution status of a task. Accepts a JSON body with the new task status to be set.
//	@Tags			tasks
//	@Router			/tasks/{task_id} [patch]
//	@Security		Api-User || Api-Key
//	@Param			{object}	body		taskExecutionPatchHandler	true	"parameters"
//	@Success		200			{object}	model.APITask
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
		if err := dbModel.SetTaskPriority(ctx, *tep.task, priority, tep.user.Username()); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "setting priority for task '%s'", tep.task.Id))
		}
	}
	if tep.Activated != nil {
		activated := *tep.Activated
		if err := dbModel.SetActiveStateById(ctx, tep.task.Id, tep.user.Username(), activated); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "setting activation state for task '%s'", tep.task.Id))
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
	err = taskModel.BuildFromService(ctx, refreshedTask, &model.APITaskArgs{
		IncludeProjectIdentifier: true,
		IncludeAMI:               true,
	})
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "converting task '%s' to API model", tep.task.Id))
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
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", rh.taskID))
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
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	return gimlet.NewJSONResponse(settings.Providers.AWS.TaskSyncRead)
}

type generatedTasksGetHandler struct {
	taskID string
}

func makeGetGeneratedTasks() gimlet.RouteHandler {
	return &generatedTasksGetHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get info about generated tasks
//	@Description	Fetch basic info about all tasks generated by a task that ran generate.tasks.
//	@Tags			tasks
//	@Router			/tasks/{task_id}/generated_tasks [get]
//	@Security		Api-User || Api-Key
//	@Param			task_id	path	string	true	"task ID"
//	@Success		200		{array}	model.APIGeneratedTaskInfo
func (rh *generatedTasksGetHandler) Factory() gimlet.RouteHandler {
	return rh
}

func (rh *generatedTasksGetHandler) Parse(ctx context.Context, r *http.Request) error {
	if rh.taskID = gimlet.GetVars(r)["task_id"]; rh.taskID == "" {
		return errors.New("missing task ID")
	}
	return nil
}

func (rh *generatedTasksGetHandler) Run(ctx context.Context) gimlet.Responder {
	taskInfos, err := data.FindGeneratedTasksFromID(rh.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding tasks generated from ID '%s'", rh.taskID))
	}

	apiTaskInfos := make([]model.APIGeneratedTaskInfo, 0, len(taskInfos))
	for _, info := range taskInfos {
		var apiTaskInfo model.APIGeneratedTaskInfo
		apiTaskInfo.BuildFromService(info)
		apiTaskInfos = append(apiTaskInfos, apiTaskInfo)
	}

	return gimlet.NewJSONResponse(apiTaskInfos)
}
