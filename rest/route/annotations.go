package route

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
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
// GET /rest/v2/task/{task_id}/annotation

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
			Message:    "fetchAllExecutions=true cannot be combined with execution={task_execution}",
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
// PUT /rest/v2/task/{task_id}/annotation

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
	if h.taskId == "" {
		return gimlet.ErrorResponse{
			Message:    "task ID cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}

	// check if the task exists
	t, err := task.FindOne(task.ById(h.taskId))
	if err != nil {
		return errors.Wrap(err, "error finding task")
	}
	if t == nil {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("the task %s does not exist", h.taskId),
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
	if h.annotation.TaskExecution == nil {
		h.annotation.TaskExecution = &t.Execution
	}
	if h.annotation.TaskId == nil {
		taskId := h.taskId
		h.annotation.TaskId = &taskId
	}

	if *h.annotation.TaskId != h.taskId {
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
	a := h.annotation
	source := &annotations.Source{
		Author:    h.user.DisplayName(),
		Time:      time.Now(),
		Requester: annotations.APIRequester,
	}

	err := annotations.UpdateAnnotation(model.APITaskAnnotationToService(*a), source)
	if err != nil {
		gimlet.NewJSONInternalErrorResponse(err)
	}

	responder := gimlet.NewJSONResponse(struct{}{})
	if err = responder.SetStatus(http.StatusOK); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusOK))
	}
	return responder
}
