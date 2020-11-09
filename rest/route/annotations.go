package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

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
	// Fetch all of the tasks to be returned in this page plus the tasks used for
	// calculating information about the next page. Here the limit is multiplied
	// by two to fetch the next page.

	taskIds, err := task.FindAllTaskIDsFromBuild(h.buildId)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "error finding task IDs for build '%s'", h.buildId))
	}
	allAnnotations, err := annotations.FindAPIAnnotationsByTaskIds(taskIds)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "error finding task annotations"))
	}
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
