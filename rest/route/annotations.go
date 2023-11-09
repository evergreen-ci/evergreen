package route

import (
	"context"
	"encoding/json"
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

// Factory creates an instance of the handler.
//
//	@Summary		List task annotations by build
//	@Description	Fetches the annotations for all the tasks in a build.
//	@Tags			annotations
//	@Router			/tasks/{build_id}/annotations [get]
//	@Security		Api-User || Api-Key
//	@Param			build_id				path	string	true	"build_id"
//	@Param			fetch_all_executions	query	string	false	"Fetches previous executions of the task if they are available"
//	@Success		200						{array}	model.APITaskAnnotation
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
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "finding task IDs for build '%s'", h.buildId))
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

// Factory creates an instance of the handler.
//
//	@Summary		List task annotations by version
//	@Description	Fetches the annotations for all the tasks in a version.
//	@Tags			annotations
//	@Router			/tasks/{version_id}/annotations [get]
//	@Security		Api-User || Api-Key
//	@Param			version_id				path	string	true	"version_id"
//	@Param			fetch_all_executions	query	string	false	"Fetches previous executions of the task if they are available"
//	@Success		200						{array}	model.APITaskAnnotation
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
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "finding task IDs for version '%s'", h.versionId))
	}
	return getAPIAnnotationsForTaskIds(taskIds, h.fetchAllExecutions)
}

func getAPIAnnotationsForTaskIds(taskIds []string, allExecutions bool) gimlet.Responder {
	allAnnotations, err := annotations.FindByTaskIds(taskIds)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "finding task annotations"))
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

// Factory creates an instance of the handler.
//
//	@Summary		Fetch task annotations
//	@Description	Returns a list containing the latest annotation for the given task, or null if there are no annotations.
//	@Tags			annotations
//	@Router			/tasks/{task_id}/annotations [get]
//	@Security		Api-User || Api-Key
//	@Param			task_id					path	string	true	"task ID"
//	@Param			execution				query	int		false	"The 0-based number corresponding to the execution of the task ID. Defaults to the latest execution"
//	@Param			fetch_all_executions	query	string	false	"Fetches previous executions of the task if they are available"
//	@Success		200						{array}	model.APITaskAnnotation
func (h *annotationByTaskGetHandler) Factory() gimlet.RouteHandler {
	return &annotationByTaskGetHandler{}
}

func (h *annotationByTaskGetHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error

	h.taskId = gimlet.GetVars(r)["task_id"]
	if h.taskId == "" {
		return errors.New("task ID cannot be empty")
	}

	vals := r.URL.Query()
	h.fetchAllExecutions = vals.Get("fetch_all_executions") == "true"
	execution := vals.Get("execution")

	if execution != "" && h.fetchAllExecutions {
		return errors.New("cannot both fetch all executions and request a specific execution")
	}

	if execution != "" {
		h.execution, err = strconv.Atoi(execution)
		if err != nil {
			return errors.Wrap(err, "parsing task execution number")
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
			return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "finding task annotation for execution %d of task '%s'", h.execution, h.taskId))
		}
		if a == nil {
			return gimlet.NewJSONResponse([]restModel.APITaskAnnotation{})
		}
		taskAnnotation := restModel.APITaskAnnotationBuildFromService(*a)
		return gimlet.NewJSONResponse([]restModel.APITaskAnnotation{*taskAnnotation})
	}

	allAnnotations, err := annotations.FindByTaskId(h.taskId)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "finding task annotation for task '%s'", h.taskId))
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
// PUT /rest/v2/tasks/{task_id}/annotation

// Parsing logic for task annotation put and patch routes.
func annotationByTaskPutOrPatchParser(ctx context.Context, r *http.Request) (string, *restModel.APITaskAnnotation, error) {
	var taskId string
	var annotation *restModel.APITaskAnnotation
	var err error
	taskId = gimlet.GetVars(r)["task_id"]
	taskExecutionsAsString := r.URL.Query().Get("execution")
	if taskId == "" {
		return "", nil, gimlet.ErrorResponse{
			Message:    "task ID cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}

	body := utility.NewRequestReader(r)
	defer body.Close()
	err = json.NewDecoder(body).Decode(&annotation)
	if err != nil {
		return "", nil, errors.Wrap(err, "reading annotation from JSON request body")
	}

	if taskExecutionsAsString != "" {
		taskExecution, err := strconv.Atoi(taskExecutionsAsString)
		if err != nil {
			return "", nil, errors.Wrap(err, "converting execution to integer value")
		}

		if annotation.TaskExecution == nil {
			annotation.TaskExecution = &taskExecution
		} else if *annotation.TaskExecution != taskExecution {
			return "", nil, errors.Errorf("task execution number from query parameter (%d) must equal the task execution number specified in the annotation (%d)", taskExecution, *annotation.TaskExecution)
		}
	} else if annotation.TaskExecution == nil {
		return "", nil, errors.New("task execution must be specified in the request query parameter or the request body's annotation")
	}

	// check if the task exists
	t, err := task.FindByIdExecution(taskId, annotation.TaskExecution)
	if err != nil {
		return "", nil, errors.Wrap(err, "finding task")
	}
	if t == nil {
		return "", nil, errors.Errorf("task '%s' not found", taskId)
	}
	if !evergreen.IsFailedTaskStatus(t.Status) {
		return "", nil, errors.Errorf("cannot create annotation when task status is '%s'", t.Status)
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(restModel.ValidateIssues(annotation.Issues))
	catcher.Add(restModel.ValidateIssues(annotation.SuspectedIssues))
	if catcher.HasErrors() {
		return "", nil, errors.Wrap(catcher.Resolve(), "invalid issue")
	}
	apiMetadataLinks := []*restModel.APIMetadataLink{}
	for _, link := range annotation.MetadataLinks {
		apiMetadataLinks = append(apiMetadataLinks, &link)
	}
	metadataLinks := restModel.APIMetadataLinksToService(apiMetadataLinks)
	if err = annotations.ValidateMetadataLinks(metadataLinks...); err != nil {
		return "", nil, errors.Wrap(err, "invalid task link")
	}

	if annotation.TaskId == nil {
		annotation.TaskId = &taskId
	} else if *annotation.TaskId != taskId {
		return "", nil, errors.Errorf("task ID parameter '%s' must equal the task ID specified in the annotation '%s'", taskId, *annotation.TaskId)
	}
	return taskId, annotation, nil
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

// Factory creates an instance of the handler.
//
//	@Summary		Create or update a new task annotation
//	@Description	Creates a task annotation, or updates an existing task annotation, overwriting any existing fields that are included in the update. The annotation is created based on the annotation specified in the request body. Task execution must be provided for this endpoint, either in the request body or set as a url parameter. If no task_execution is specified in the request body or in the url, a bad status error will be returned. Note that usage of this endpoint requires that the requesting user have security to modify task annotations. The user does not need to specify the source, it will be added automatically.
//	@Tags			annotations
//	@Router			/tasks/{task_id}/annotations [put]
//	@Security		Api-User || Api-Key
//	@Param			task_id		path	string					true	"task ID"
//	@Param			execution	query	int						false	"Can be set in lieu of specifying task_execution in the request body."
//	@Param			{object}	body	model.APITaskAnnotation	true	"parameters"
//	@Success		200
func (h *annotationByTaskPutHandler) Factory() gimlet.RouteHandler {
	return &annotationByTaskPutHandler{}
}

func (h *annotationByTaskPutHandler) Parse(ctx context.Context, r *http.Request) error {
	taskId, annotation, err := annotationByTaskPutOrPatchParser(ctx, r)
	if err != nil {
		return err
	}
	h.taskId = taskId
	h.annotation = annotation

	u := MustHaveUser(ctx)
	h.user = u
	return nil
}

func (h *annotationByTaskPutHandler) Run(ctx context.Context) gimlet.Responder {
	err := annotations.UpdateAnnotation(restModel.APITaskAnnotationToService(*h.annotation), h.user.DisplayName())
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "updating annotation"))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/tasks/{task_id}/annotation

type annotationByTaskPatchHandler struct {
	taskId string
	upsert bool

	user       gimlet.User
	annotation *restModel.APITaskAnnotation
}

func makePatchAnnotationsByTask() gimlet.RouteHandler {
	return &annotationByTaskPatchHandler{}
}

func (h *annotationByTaskPatchHandler) Factory() gimlet.RouteHandler {
	return &annotationByTaskPatchHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Create or update a new task annotation by appending
//	@Description	Creates a task annotation, or updates an existing task annotation, appending issues and suspected issues that are included in the update. A new annotation is created based if the annotation exists and if upsert is true. Task execution must be provided for this endpoint, either in the request body or set as a url parameter. If no task_execution is specified in the request body or in the url, a bad status error will be returned. Note that usage of this endpoint requires that the requesting user have security to modify task annotations. The user does not need to specify the source, it will be added automatically.
//	@Tags			annotations
//	@Router			/tasks/{task_id}/annotations [patch]
//	@Security		Api-User || Api-Key
//	@Param			task_id		path	string					true	"task ID"
//	@Param			execution	query	int						false	"Can be set in lieu of specifying task_execution in the request body."
//	@Param			upsert		query	boolean					false	"Will create a new annotation if task annotation isn't found and upsert is true."
//	@Param			{object}	body	model.APITaskAnnotation	true	"parameters"
//	@Success		200
func (h *annotationByTaskPatchHandler) Parse(ctx context.Context, r *http.Request) error {
	taskId, annotation, err := annotationByTaskPutOrPatchParser(ctx, r)
	if err != nil {
		return err
	}
	h.taskId = taskId
	h.annotation = annotation
	h.upsert = r.URL.Query().Get("upsert") == "true"

	u := MustHaveUser(ctx)
	h.user = u
	return nil
}

func (h *annotationByTaskPatchHandler) Run(ctx context.Context) gimlet.Responder {
	err := annotations.PatchAnnotation(restModel.APITaskAnnotationToService(*h.annotation), h.user.DisplayName(), h.upsert)
	if err != nil {
		gimlet.NewJSONInternalErrorResponse(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
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

// Factory creates an instance of the handler.
//
//	@Summary		Send a newly created ticket for a task.
//	@Description	If a file ticket webhook is configured for a project, this endpoint should be used to let evergreen know when a ticket was filed for a task so that it can be stored and displayed to the user. The request body should include the ticket url and issue_key. Note that usage of this endpoint requires that the requesting user have security to modify task annotations. The user does not need to specify the source of the ticket, it will be added automatically.
//	@Tags			annotations
//	@Router			/tasks/{task_id}/created_ticket [put]
//	@Security		Api-User || Api-Key
//	@Param			task_id		path	string				true	"task ID"
//	@Param			{object}	body	model.APIIssueLink	true	"parameters"
//	@Success		200
func (h *createdTicketByTaskPutHandler) Factory() gimlet.RouteHandler {
	return &createdTicketByTaskPutHandler{}
}

func (h *createdTicketByTaskPutHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error
	h.taskId = gimlet.GetVars(r)["task_id"]
	if h.taskId == "" {
		return errors.New("task ID cannot be empty")
	}
	// for now, tickets will be created for a specific task execution only
	// and will not be visible on other executions
	executionString := r.URL.Query().Get("execution")
	if executionString == "" {
		return errors.New("task execution must be specified")
	}
	execution, err := strconv.Atoi(executionString)
	if err != nil {
		return errors.Wrap(err, "parsing task execution")
	}
	h.execution = execution

	t, err := task.FindOneId(h.taskId)
	if err != nil {
		return errors.Wrapf(err, "finding task '%s'", h.taskId)
	}
	if t == nil {
		return errors.Errorf("task '%s' not found", h.taskId)
	}

	body := utility.NewRequestReader(r)
	defer body.Close()
	err = json.NewDecoder(body).Decode(&h.ticket)
	if err != nil {
		return errors.Wrap(err, "unmarshalling ticket from JSON request body")
	}

	if err = util.CheckURL(utility.FromStringPtr(h.ticket.URL)); err != nil {
		return errors.Wrap(err, "invalid ticket URL")
	}

	u := MustHaveUser(ctx)
	h.user = u
	return nil
}

func (h *createdTicketByTaskPutHandler) Run(ctx context.Context) gimlet.Responder {
	err := annotations.AddCreatedTicket(h.taskId, h.execution, *restModel.APIIssueLinkToService(*h.ticket), h.user.DisplayName())
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
}
