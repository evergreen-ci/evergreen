package route

import (
	"context"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
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
	allAnnotations, err := annotations.FindAnnotationsByTaskIds(taskIds)
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
// GET /rest/v2/task/{task_id}/{task_execution}/annotation

type annotationByTaskHandler struct {
	taskId             string
	fetchAllExecutions bool
	execution          int
	sc                 data.Connector
}

func makeFetchAnnotationByTask(sc data.Connector) gimlet.RouteHandler {
	return &annotationByTaskHandler{
		sc: sc,
	}
}

func (h *annotationByTaskHandler) Factory() gimlet.RouteHandler {
	return &annotationByTaskHandler{
		sc: h.sc,
	}
}

func (h *annotationByTaskHandler) Parse(ctx context.Context, r *http.Request) error {
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
				Message:    "Invalid execution",
				StatusCode: http.StatusBadRequest,
			}
		}
	} else {
		// since an int in go defaults to 0, we won't know if the user
		// specifically wanted execution 0, or if they want the latest.
		// we use -1 to indicate "not specified"
		h.execution = -1
	}

	//todo: don't allow giving both fetchall and an execution

	return nil
}

func (h *annotationByTaskHandler) Run(ctx context.Context) gimlet.Responder {
	// get a specific execution
	if !h.fetchAllExecutions && h.execution != -1 {
		a, err := annotations.FindAnnotationByTaskIdAndExecution(h.taskId, h.execution)
		if err != nil {
			return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "error finding task annotation"))
		}
		if a == nil {
			return gimlet.NewJSONResponse(model.APITaskAnnotation{})
		}
		taskAnnotation := model.APITaskAnnotationBuildFromService(*a)
		return gimlet.NewJSONResponse([]model.APITaskAnnotation{*taskAnnotation})
	}

	allAnnotations, err := annotations.FindAnnotationsByTaskId(h.taskId)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "error finding task annotations"))
	}
	// get the latest execution
	annotationsToReturn := allAnnotations
	if !h.fetchAllExecutions && h.execution == -1 {
		annotationsToReturn = annotations.GetLatestExecutions(allAnnotations)
	}

	var res []model.APITaskAnnotation
	for _, a := range annotationsToReturn {
		apiAnnotation := model.APITaskAnnotationBuildFromService(a)
		res = append(res, *apiAnnotation)
	}

	return gimlet.NewJSONResponse(res)
}
