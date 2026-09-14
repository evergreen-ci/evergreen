package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type githubIntentProcessingErrorHandler struct {
	errorID mgobson.ObjectId
}

type githubIntentProcessingErrorResponse struct {
	Message string `json:"message"`
}

func makeGitHubIntentProcessingError() gimlet.RouteHandler {
	return &githubIntentProcessingErrorHandler{}
}

func (h *githubIntentProcessingErrorHandler) Factory() gimlet.RouteHandler {
	return &githubIntentProcessingErrorHandler{}
}

func (h *githubIntentProcessingErrorHandler) Parse(ctx context.Context, r *http.Request) error {
	errorID := gimlet.GetVars(r)["error_id"]
	if !mgobson.IsObjectIdHex(errorID) {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid processing error ID",
		}
	}
	h.errorID = mgobson.ObjectIdHex(errorID)
	return nil
}

func (h *githubIntentProcessingErrorHandler) Run(ctx context.Context) gimlet.Responder {
	processingError, err := patch.FindGitHubIntentProcessingError(ctx, h.errorID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding GitHub intent processing error '%s'", h.errorID.Hex()))
	}
	if processingError == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("GitHub intent processing error '%s' not found", h.errorID.Hex()),
		})
	}

	usr := MustHaveUser(ctx)
	hasPermission, err := usr.HasPermissionErr(ctx, gimlet.PermissionOpts{
		Resource:      processingError.ProjectID,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionTasks,
		RequiredLevel: evergreen.TasksView.Value,
	})
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "checking project permission"))
	}
	if !hasPermission {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "not authorized",
		})
	}

	return gimlet.NewJSONResponse(githubIntentProcessingErrorResponse{Message: processingError.Message})
}
