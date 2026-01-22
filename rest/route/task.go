package route

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	evergreenutil "github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	anserdb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// Constants related to S3 URL normalization.
const (
	s3Amazonaws = "amazonaws"
	s3          = "s3"
	com         = "com"
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
		foundTask, err = task.FindOneId(ctx, tgh.taskID)
	} else {
		foundTask, err = task.FindOneIdAndExecution(ctx, tgh.taskID, tgh.execution)
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
		tasks, err = task.FindOldWithDisplayTasks(ctx, task.ByOldTaskID(tgh.taskID))
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

// //////////////////////////////////////////////////////////////////////
//
// Handler for updating a single artifact file's URL for a specific task execution.
//
//	PATCH /tasks/{task_id}/artifacts/url
type updateArtifactURLHandler struct {
	taskID    string
	execution *int
	body      updateArtifactURLRequest
	task      *task.Task
	user      gimlet.User

	// fields for signed URL handling
	isSignedURL    bool
	currentFileKey string
	newFileKey     string
}

// updateArtifactURLRequest represents the request body for updating a single
// artifact file's URL for a specific task execution.
type updateArtifactURLRequest struct {
	// ArtifactName is the name of the artifact file whose URL will be updated.
	ArtifactName string `json:"artifact_name"`
	// CurrentURL is the existing URL for the artifact file.
	CurrentURL string `json:"current_url"`
	// NewURL is the new URL that will replace the current URL.
	NewURL string `json:"new_url"`
}

func makeUpdateArtifactURLRoute() gimlet.RouteHandler { return &updateArtifactURLHandler{} }

// Factory creates an instance of the artifact URL update handler.
//
//	@Summary		Update an artifact file URL
//	@Description	Updates the URL for an artifact file. For signed S3 URLs, both current_url and new_url must be valid S3 URLs in the same bucket, and the S3 file key is rotated. For non-signed URLs, the link is simply updated to the new URL.
//	@Tags			tasks
//	@Router			/tasks/{task_id}/artifacts/url [patch]
//	@Security		Api-User || Api-Key
//	@Param			task_id		path		string						true	"Task ID"
//	@Param			execution	query		int							false	"0-based execution number; if omitted updates latest execution"
//	@Param			{object}	body		updateArtifactURLRequest	true	"parameters"
//	@Success		200			{object}	model.APITask
func (h *updateArtifactURLHandler) Factory() gimlet.RouteHandler { return &updateArtifactURLHandler{} }

func (h *updateArtifactURLHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	if err := utility.ReadJSON(r.Body, &h.body); err != nil {
		return errors.Wrap(err, "reading artifact url update request body")
	}
	if h.body.ArtifactName == "" || h.body.CurrentURL == "" || h.body.NewURL == "" {
		return gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: "artifact_name, current_url, and new_url are all required"}
	}
	if err := evergreenutil.CheckURL(h.body.NewURL); err != nil {
		return gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: fmt.Sprintf("new_url invalid: %s", err.Error())}
	}

	h.isSignedURL = strings.Contains(h.body.CurrentURL, "Token=")
	if h.isSignedURL {
		currentBucket, currentFileKey := parseS3URL(h.body.CurrentURL)
		newBucket, newFileKey := parseS3URL(h.body.NewURL)

		if currentBucket == "" {
			return gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: "current_url must be a valid S3 URL"}
		}
		if newBucket == "" {
			return gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: "new_url must be a valid S3 URL"}
		}
		if currentBucket != newBucket {
			return gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: "current_url and new_url must be in the same S3 bucket"}
		}

		h.currentFileKey = currentFileKey
		h.newFileKey = newFileKey
	}

	if execStr := r.URL.Query().Get("execution"); execStr != "" {
		val, err := strconv.Atoi(execStr)
		if err != nil || val < 0 {
			return gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: "execution must be a non-negative integer"}
		}
		h.execution = utility.ToIntPtr(val)
	}
	projCtx := MustHaveProjectContext(ctx)
	if projCtx.Task == nil {
		return gimlet.ErrorResponse{StatusCode: http.StatusNotFound, Message: "task not found"}
	}
	h.task = projCtx.Task
	h.user = MustHaveUser(ctx)
	return nil
}

func (h *updateArtifactURLHandler) Run(ctx context.Context) gimlet.Responder {
	exec := h.task.Execution
	if h.execution != nil {
		exec = *h.execution
	}

	var err error
	if h.isSignedURL {
		err = artifact.UpdateFileKey(ctx, h.task.Id, exec, h.body.ArtifactName, h.currentFileKey, h.newFileKey)
	} else {
		err = artifact.UpdateFileLink(ctx, h.task.Id, exec, h.body.ArtifactName, h.body.CurrentURL, h.body.NewURL)
	}

	if err != nil {
		if err == anserdb.ErrNotFound {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{StatusCode: http.StatusNotFound, Message: "artifact file not found for task"})
		}
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "updating artifact URL"))
	}

	// FindByIdExecution will find the latest execution if h.execution is nil
	taskForResponse, err := task.FindByIdExecution(ctx, h.task.Id, h.execution)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s' execution after artifact update", h.task.Id))
	}
	if taskForResponse == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{StatusCode: http.StatusNotFound, Message: "task not found after update"})
	}

	apiTask := &model.APITask{}
	if err := apiTask.BuildFromService(ctx, taskForResponse, &model.APITaskArgs{IncludeProjectIdentifier: true, IncludeAMI: true, IncludeArtifacts: true}); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "building API task model after artifact update"))
	}
	return gimlet.NewJSONResponse(apiTask)
}

// parseS3URL extracts the bucket name and file key from an S3 URL.
// Supports both virtual-hosted-style and path-style S3 URLs.
// Returns empty strings if the URL is not a valid S3 URL.
//
// Examples:
//   - https://bucket.s3.amazonaws.com/path/to/file.log -> ("bucket", "path/to/file.log")
//   - https://bucket.s3.us-east-1.amazonaws.com/path/file.log?X-Amz-... -> ("bucket", "path/file.log")
//   - https://s3.amazonaws.com/bucket/path/to/file.log -> ("bucket", "path/to/file.log")
func parseS3URL(raw string) (bucket, fileKey string) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}

	parts := strings.Split(u.Host, ".")
	path := strings.TrimPrefix(u.Path, "/")

	// Virtual-hosted-style: bucket.s3[.region].amazonaws.com/path/to/file
	if len(parts) >= 4 && parts[1] == s3 && parts[len(parts)-2] == s3Amazonaws && parts[len(parts)-1] == com {
		return parts[0], path
	}

	// Path-style: s3[.region].amazonaws.com/bucket/path/to/file
	if (len(parts) == 3 || len(parts) == 4) && parts[0] == s3 && parts[len(parts)-2] == s3Amazonaws && parts[len(parts)-1] == com {
		bucket, fileKey, _ = strings.Cut(path, "/")
		return bucket, fileKey
	}

	return "", ""
}

// Factory creates an instance of the handler.
//
//	@Summary		Change a task's execution status
//	@Description	Change the current execution status of a task. Accepts a JSON body with the new task status to be set.
//	@Tags			tasks
//	@Router			/tasks/{task_id} [patch]
//	@Security		Api-User || Api-Key
//	@Param			task_id		path		string						true	"task ID"
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
	refreshedTask, err := task.FindOneId(ctx, tep.task.Id)
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
	t, err := task.FindOneId(ctx, rh.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", rh.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", rh.taskID),
		})
	}

	dt, err := t.GetDisplayTask(ctx)
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
	taskInfos, err := data.FindGeneratedTasksFromID(ctx, rh.taskID)
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

// DELETE /rest/v2/task/{task_id}/github_dynamic_access_tokens
// This route is used to revoke GitHub access tokens used for fetching task data.
type deleteGitHubDynamicAccessTokens struct {
	taskID string
	Tokens []string `json:"tokens"`
}

func makeDeleteGitHubDynamicAccessTokens() gimlet.RouteHandler {
	return &deleteGitHubDynamicAccessTokens{}
}

func (h *deleteGitHubDynamicAccessTokens) Factory() gimlet.RouteHandler {
	return &deleteGitHubDynamicAccessTokens{}
}

func (h *deleteGitHubDynamicAccessTokens) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task_id")
	}

	if err := utility.ReadJSON(r.Body, &h.Tokens); err != nil {
		return errors.Wrapf(err, "reading token JSON request body for task '%s'", h.taskID)
	}

	if len(h.Tokens) == 0 {
		return errors.New("no token to redact")
	}
	return nil
}

func (h *deleteGitHubDynamicAccessTokens) Run(ctx context.Context) gimlet.Responder {
	catcher := grip.NewBasicCatcher()

	for i, token := range h.Tokens {
		if err := thirdparty.RevokeInstallationToken(ctx, token); err != nil {
			catcher.Wrapf(err, "revoking token %d for task '%s'", i, h.taskID)
		}
	}

	if catcher.HasErrors() {
		return gimlet.MakeJSONInternalErrorResponder(catcher.Resolve())
	}

	return gimlet.NewJSONResponse(struct{}{})
}
