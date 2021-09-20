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
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
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
	sc                 data.Connector
}

func makeFetchAnnotationsByBuild(sc data.Connector) gimlet.RouteHandler {
	return &annotationsByBuildHandler{
		sc: sc,
	}
}

func (h *annotationsByBuildHandler) Factory() gimlet.RouteHandler {
	return &annotationsByBuildHandler{
		sc: h.sc,
	}
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
	sc                 data.Connector
}

func makeFetchAnnotationsByVersion(sc data.Connector) gimlet.RouteHandler {
	return &annotationsByVersionHandler{
		sc: sc,
	}
}

func (h *annotationsByVersionHandler) Factory() gimlet.RouteHandler {
	return &annotationsByVersionHandler{
		sc: h.sc,
	}
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
	var res []model.APITaskAnnotation
	for _, a := range annotationsToReturn {
		apiAnnotation := model.APITaskAnnotationBuildFromService(a)
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
	sc                 data.Connector
}

func makeFetchAnnotationsByTask(sc data.Connector) gimlet.RouteHandler {
	return &annotationByTaskGetHandler{
		sc: sc,
	}
}

func (h *annotationByTaskGetHandler) Factory() gimlet.RouteHandler {
	return &annotationByTaskGetHandler{
		sc: h.sc,
	}
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
			return gimlet.NewJSONResponse([]model.APITaskAnnotation{})
		}
		taskAnnotation := model.APITaskAnnotationBuildFromService(*a)
		return gimlet.NewJSONResponse([]model.APITaskAnnotation{*taskAnnotation})
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

	var res []model.APITaskAnnotation
	for _, a := range annotationsToReturn {
		apiAnnotation := model.APITaskAnnotationBuildFromService(a)
		res = append(res, *apiAnnotation)
	}

	return gimlet.NewJSONResponse(res)
}

////////////////////////////////////////////////////////////////////////
//
// PUT /rest/v2/tasks/{task_id}/annotation

type annotationByTaskPutHandler struct {
	taskId string

	user       gimlet.User
	annotation *model.APITaskAnnotation
	sc         data.Connector
}

func makePutAnnotationsByTask(sc data.Connector) gimlet.RouteHandler {
	return &annotationByTaskPutHandler{
		sc: sc,
	}
}

func (h *annotationByTaskPutHandler) Factory() gimlet.RouteHandler {
	return &annotationByTaskPutHandler{
		sc: h.sc,
	}
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

	body := util.NewRequestReader(r)
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
	for _, issue := range h.annotation.Issues {
		catcher.Add(util.CheckURL(utility.FromStringPtr(issue.URL)))
	}
	for _, issue := range h.annotation.SuspectedIssues {
		catcher.Add(util.CheckURL(utility.FromStringPtr(issue.URL)))
	}
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
	err := annotations.UpdateAnnotation(model.APITaskAnnotationToService(*h.annotation), h.user.DisplayName())
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
	ticket    *model.APIIssueLink
	sc        data.Connector
}

func makeCreatedTicketByTask(sc data.Connector) gimlet.RouteHandler {
	return &createdTicketByTaskPutHandler{
		sc: sc,
	}
}

func (h *createdTicketByTaskPutHandler) Factory() gimlet.RouteHandler {
	return &createdTicketByTaskPutHandler{
		sc: h.sc,
	}
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
	// if there is no custom webhook configured, return an error because the
	// purpose of this endpoint is to store the ticket created by the web-hook
	_, ok, err := plugin.IsWebhookConfigured(t.Project, t.Version)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("Error while retrieving webhook config: '%s'", err.Error()),
			StatusCode: http.StatusBadRequest,
		}
	}
	if !ok {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("there is no webhook configured for '%s'", t.Project),
			StatusCode: http.StatusBadRequest,
		}
	}

	body := util.NewRequestReader(r)
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
	err := annotations.AddCreatedTicket(h.taskId, h.execution, *model.APIIssueLinkToService(*h.ticket), h.user.DisplayName())
	if err != nil {
		gimlet.NewJSONInternalErrorResponse(err)
	}

	responder := gimlet.NewJSONResponse(struct{}{})
	if err = responder.SetStatus(http.StatusOK); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusOK))
	}
	return responder
}
