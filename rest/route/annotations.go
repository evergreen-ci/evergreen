package route

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/builds/{build_id}/annotations

type annotationsByBuildHandler struct {
	buildId            string
	fetchAllExecutions bool
}

func makeFetchAnnotationsByBuild() gimlet.RouteHandler {
	return &annotationsByBuildHandler{}
}

func (h *annotationsByBuildHandler) Factory() gimlet.RouteHandler {
	return &annotationsByBuildHandler{}
}

func (h *annotationsByBuildHandler) Parse(ctx context.Context, r *http.Request) error {
	h.buildId = gimlet.GetVars(r)["build_id"]
	if h.buildId == "" {
		return gimlet.ErrorResponse{
			Message:    "build ID cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}

	h.fetchAllExecutions = r.URL.Query().Get("fetch_all_executions") == "true"
	return nil
}

func (h *annotationsByBuildHandler) Run(ctx context.Context) gimlet.Responder {
	taskIds, err := task.FindAllTaskIDsFromBuild(h.buildId)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "error finding task IDs for build '%s'", h.buildId))
	}

	return getAPIAnnotationsForTaskIds(taskIds, h.fetchAllExecutions)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/versions/{version_id}/annotations

type annotationsByVersionHandler struct {
	versionId          string
	fetchAllExecutions bool
}

func makeFetchAnnotationsByVersion() gimlet.RouteHandler {
	return &annotationsByVersionHandler{}
}

func (h *annotationsByVersionHandler) Factory() gimlet.RouteHandler {
	return &annotationsByVersionHandler{}
}

func (h *annotationsByVersionHandler) Parse(ctx context.Context, r *http.Request) error {
	h.versionId = gimlet.GetVars(r)["version_id"]
	if h.versionId == "" {
		return gimlet.ErrorResponse{
			Message:    "version ID cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}

	h.fetchAllExecutions = r.URL.Query().Get("fetch_all_executions") == "true"
	return nil
}

func (h *annotationsByVersionHandler) Run(ctx context.Context) gimlet.Responder {
	taskIds, err := task.FindAllTaskIDsFromVersion(h.versionId)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "error finding task IDs for version '%s'", h.versionId))
	}
	return getAPIAnnotationsForTaskIds(taskIds, h.fetchAllExecutions)
}

func getAPIAnnotationsForTaskIds(taskIds []string, allExecutions bool) gimlet.Responder {
	allAnnotations, err := annotations.FindByTaskIds(taskIds)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "error finding task annotations"))
	}
	annotationsToReturn := allAnnotations
	if !allExecutions {
		annotationsToReturn = annotations.GetLatestExecutions(allAnnotations)
	}
	var res []restModel.APITaskAnnotation
	for _, a := range annotationsToReturn {
		apiAnnotation := restModel.APITaskAnnotationBuildFromService(a)
		res = append(res, *apiAnnotation)
	}

	return gimlet.NewJSONResponse(res)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/tasks/{task_id}/annotations

type annotationByTaskGetHandler struct {
	taskId             string
	fetchAllExecutions bool
	execution          int
}

func makeFetchAnnotationsByTask() gimlet.RouteHandler {
	return &annotationByTaskGetHandler{}
}

func (h *annotationByTaskGetHandler) Factory() gimlet.RouteHandler {
	return &annotationByTaskGetHandler{}
}

func (h *annotationByTaskGetHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error

	h.taskId = gimlet.GetVars(r)["task_id"]
	if h.taskId == "" {
		return gimlet.ErrorResponse{
			Message:    "task ID cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}

	vals := r.URL.Query()
	h.fetchAllExecutions = vals.Get("fetch_all_executions") == "true"
	execution := vals.Get("execution")

	if execution != "" && h.fetchAllExecutions == true {
		return gimlet.ErrorResponse{
			Message:    "fetchAllExecutions=true cannot be combined with execution={execution}",
			StatusCode: http.StatusBadRequest,
		}
	}

	if execution != "" {
		h.execution, err = strconv.Atoi(execution)
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
		h.execution = -1
	}
	return nil
}

func (h *annotationByTaskGetHandler) Run(ctx context.Context) gimlet.Responder {
	// get a specific execution
	if h.execution != -1 {
		a, err := annotations.FindOneByTaskIdAndExecution(h.taskId, h.execution)
		if err != nil {
			return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "error finding task annotation"))
		}
		if a == nil {
			return gimlet.NewJSONResponse([]restModel.APITaskAnnotation{})
		}
		taskAnnotation := restModel.APITaskAnnotationBuildFromService(*a)
		return gimlet.NewJSONResponse([]restModel.APITaskAnnotation{*taskAnnotation})
	}

	allAnnotations, err := annotations.FindByTaskId(h.taskId)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "error finding task annotations"))
	}
	// get the latest execution
	annotationsToReturn := allAnnotations
	if !h.fetchAllExecutions {
		annotationsToReturn = annotations.GetLatestExecutions(allAnnotations)
	}

	var res []restModel.APITaskAnnotation
	for _, a := range annotationsToReturn {
		apiAnnotation := restModel.APITaskAnnotationBuildFromService(a)
		res = append(res, *apiAnnotation)
	}

	return gimlet.NewJSONResponse(res)
}

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/tasks/annotations

type bulkCreateAnnotationsOpts struct {
	TaskUpdates []TaskUpdate `bson:"task_updates" json:"task_updates"`
}
type TaskUpdate struct {
	Tasks      []TaskData                  `bson:"tasks" json:"tasks"`
	Annotation restModel.APITaskAnnotation `bson:"annotation" json:"annotation"`
}
type TaskData struct {
	TaskId    string `bson:"task_id" json:"task_id"`
	Execution int    `bson:"execution" json:"execution"`
}

type bulkCreateAnnotationHandler struct {
	user gimlet.User
	opts bulkCreateAnnotationsOpts
}

func makeBulkCreateAnnotations() gimlet.RouteHandler {
	return &bulkCreateAnnotationHandler{}
}

func (h *bulkCreateAnnotationHandler) Factory() gimlet.RouteHandler {
	return &bulkCreateAnnotationHandler{}
}

func (h *bulkCreateAnnotationHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error
	body := utility.NewRequestReader(r)
	defer body.Close()
	err = json.NewDecoder(body).Decode(&h.opts)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("API error while unmarshalling JSON: '%s'", err.Error()),
			StatusCode: http.StatusBadRequest,
		}
	}
	for _, update := range h.opts.TaskUpdates {
		for _, t := range update.Tasks {
			// check if the task exists
			foundTask, err := task.FindOneIdAndExecution(t.TaskId, t.Execution)
			if err != nil {
				return errors.Wrap(err, "error finding task")
			}
			if foundTask == nil {
				return gimlet.ErrorResponse{
					Message:    fmt.Sprintf("the task '%s' does not exist", t.TaskId),
					StatusCode: http.StatusBadRequest,
				}
			}
			if !evergreen.IsFailedTaskStatus(foundTask.Status) {
				return gimlet.ErrorResponse{
					Message:    fmt.Sprintf("cannot create annotation when task status is '%s'", foundTask.Status),
					StatusCode: http.StatusBadRequest,
				}
			}
		}

		catcher := grip.NewBasicCatcher()
		catcher.Add(restModel.ValidateIssues(update.Annotation.Issues))
		catcher.Add(restModel.ValidateIssues(update.Annotation.SuspectedIssues))
		if catcher.HasErrors() {
			return gimlet.ErrorResponse{
				Message:    catcher.Resolve().Error(),
				StatusCode: http.StatusBadRequest,
			}
		}
	}
	u := MustHaveUser(ctx)
	h.user = u
	return nil
}

func (h *bulkCreateAnnotationHandler) Run(ctx context.Context) gimlet.Responder {
	taskAnnotations := []*annotations.TaskAnnotation{}
	for _, update := range h.opts.TaskUpdates {
		for _, t := range update.Tasks {
			ann := update.Annotation
			ann.TaskId = &t.TaskId
			ann.TaskExecution = &t.Execution
			taskAnnotations = append(taskAnnotations, restModel.APITaskAnnotationToService(ann))
		}
	}
	err := annotations.InsertManyAnnotations(taskAnnotations, h.user.DisplayName())
	if err != nil {
		gimlet.NewJSONInternalErrorResponse(err)
	}

	responder := gimlet.NewJSONResponse(struct{}{})
	if err = responder.SetStatus(http.StatusOK); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusOK))
	}
	return responder
}

////////////////////////////////////////////////////////////////////////
//
// PUT /rest/v2/tasks/{task_id}/annotation

type annotationByTaskPutHandler struct {
	taskId string

	user       gimlet.User
	annotation *restModel.APITaskAnnotation
}

func makePutAnnotationsByTask() gimlet.RouteHandler {
	return &annotationByTaskPutHandler{}
}

func (h *annotationByTaskPutHandler) Factory() gimlet.RouteHandler {
	return &annotationByTaskPutHandler{}
}

func (h *annotationByTaskPutHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error
	h.taskId = gimlet.GetVars(r)["task_id"]
	taskExecutionsAsString := r.URL.Query().Get("execution")
	if h.taskId == "" {
		return gimlet.ErrorResponse{
			Message:    "task ID cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}

	body := utility.NewRequestReader(r)
	defer body.Close()
	err = json.NewDecoder(body).Decode(&h.annotation)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("API error while unmarshalling JSON: '%s'", err.Error()),
			StatusCode: http.StatusBadRequest,
		}
	}

	if taskExecutionsAsString != "" {
		taskExecution, err := strconv.Atoi(taskExecutionsAsString)
		if err != nil {
			return gimlet.ErrorResponse{
				Message:    "cannot convert execution to integer value",
				StatusCode: http.StatusBadRequest,
			}
		}

		if h.annotation.TaskExecution == nil {
			h.annotation.TaskExecution = &taskExecution
		} else if *h.annotation.TaskExecution != taskExecution {
			return gimlet.ErrorResponse{
				Message:    "Task execution must equal the task execution specified in the annotation",
				StatusCode: http.StatusBadRequest,
			}
		}
	} else if h.annotation.TaskExecution == nil {
		return gimlet.ErrorResponse{
			Message:    "task execution must be specified in the url or request body",
			StatusCode: http.StatusBadRequest,
		}
	}

	// check if the task exists
	t, err := task.FindByIdExecution(h.taskId, h.annotation.TaskExecution)
	if err != nil {
		return errors.Wrap(err, "error finding task")
	}
	if t == nil {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("the task '%s' does not exist", h.taskId),
			StatusCode: http.StatusBadRequest,
		}
	}
	if !evergreen.IsFailedTaskStatus(t.Status) {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("cannot create annotation when task status is '%s'", t.Status),
			StatusCode: http.StatusBadRequest,
		}
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(restModel.ValidateIssues(h.annotation.Issues))
	catcher.Add(restModel.ValidateIssues(h.annotation.SuspectedIssues))
	if catcher.HasErrors() {
		return gimlet.ErrorResponse{
			Message:    catcher.Resolve().Error(),
			StatusCode: http.StatusBadRequest,
		}
	}

	if h.annotation.TaskId == nil {
		taskId := h.taskId
		h.annotation.TaskId = &taskId
	} else if *h.annotation.TaskId != h.taskId {
		return gimlet.ErrorResponse{
			Message:    "TaskID must equal the taskId specified in the annotation",
			StatusCode: http.StatusBadRequest,
		}
	}

	u := MustHaveUser(ctx)
	h.user = u
	return nil
}

func (h *annotationByTaskPutHandler) Run(ctx context.Context) gimlet.Responder {
	err := annotations.UpdateAnnotation(restModel.APITaskAnnotationToService(*h.annotation), h.user.DisplayName())
	if err != nil {
		gimlet.NewJSONInternalErrorResponse(err)
	}

	responder := gimlet.NewJSONResponse(struct{}{})
	if err = responder.SetStatus(http.StatusOK); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusOK))
	}
	return responder
}

////////////////////////////////////////////////////////////////////////
//
// PUT /rest/v2/tasks/{task_id}/created_ticket

// this api will be used by teams who set up their own web hook to file a ticket, to send
// us the information for the ticket that was created so that we can store and display it.

type createdTicketByTaskPutHandler struct {
	taskId    string
	execution int
	user      gimlet.User
	ticket    *restModel.APIIssueLink
}

func makeCreatedTicketByTask() gimlet.RouteHandler {
	return &createdTicketByTaskPutHandler{}
}

func (h *createdTicketByTaskPutHandler) Factory() gimlet.RouteHandler {
	return &createdTicketByTaskPutHandler{}
}

func (h *createdTicketByTaskPutHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error
	h.taskId = gimlet.GetVars(r)["task_id"]
	if h.taskId == "" {
		return gimlet.ErrorResponse{
			Message:    "task ID cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}
	// for now, tickets will be created for a specific task execution only
	// and will not be visible on other executions
	executionString := r.URL.Query().Get("execution")
	if executionString == "" {
		return gimlet.ErrorResponse{
			Message:    "the task execution must be specified",
			StatusCode: http.StatusBadRequest,
		}
	}
	execution, err := strconv.Atoi(executionString)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("cannot convert '%s' to int: '%s'", executionString, err.Error()),
			StatusCode: http.StatusBadRequest,
		}
	}
	h.execution = execution

	// check if the task exists
	t, err := task.FindOneId(h.taskId)
	if err != nil {
		return errors.Wrap(err, "error finding task")
	}
	if t == nil {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("the task '%s' does not exist", h.taskId),
			StatusCode: http.StatusBadRequest,
		}
	}

	body := utility.NewRequestReader(r)
	defer body.Close()
	err = json.NewDecoder(body).Decode(&h.ticket)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("API error while unmarshalling JSON: '%s'", err.Error()),
			StatusCode: http.StatusBadRequest,
		}
	}

	//validate the url
	if err = util.CheckURL(utility.FromStringPtr(h.ticket.URL)); err != nil {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("the url is not valid: '%s' ", err.Error()),
			StatusCode: http.StatusBadRequest,
		}
	}

	u := MustHaveUser(ctx)
	h.user = u
	return nil
}

func (h *createdTicketByTaskPutHandler) Run(ctx context.Context) gimlet.Responder {
	err := annotations.AddCreatedTicket(h.taskId, h.execution, *restModel.APIIssueLinkToService(*h.ticket), h.user.DisplayName())
	if err != nil {
		gimlet.NewJSONInternalErrorResponse(err)
	}

	responder := gimlet.NewJSONResponse(struct{}{})
	if err = responder.SetStatus(http.StatusOK); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusOK))
	}
	return responder
}
