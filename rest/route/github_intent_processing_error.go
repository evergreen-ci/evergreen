package route

import (
	"context"
	"fmt"
	"net/http"

	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type githubIntentProcessingErrorHandler struct{}

type githubIntentProcessingErrorContextMiddleware struct{}

type githubIntentProcessingErrorContextKey struct{}

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
	return nil
}

func (h *githubIntentProcessingErrorHandler) Run(ctx context.Context) gimlet.Responder {
	processingError, ok := ctx.Value(githubIntentProcessingErrorContextKey{}).(*patch.GitHubIntentInfo)
	if !ok {
		return gimlet.MakeJSONInternalErrorResponder(errors.New("GitHub intent processing error is missing from context"))
	}
	return gimlet.NewJSONResponse(githubIntentProcessingErrorResponse{Message: processingError.Message})
}

func newGitHubIntentProcessingErrorContextMiddleware() gimlet.Middleware {
	return &githubIntentProcessingErrorContextMiddleware{}
}

func (m *githubIntentProcessingErrorContextMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	errorID := gimlet.GetVars(r)["error_id"]
	if !mgobson.IsObjectIdHex(errorID) {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid processing error ID",
		}))
		return
	}

	// Get and set the project_id request variables for the project permission middleware.
	processingError, err := patch.FindGitHubIntentInfo(r.Context(), mgobson.ObjectIdHex(errorID))
	if err != nil {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding GitHub intent processing error '%s'", errorID)))
		return
	}
	if processingError == nil {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("GitHub intent processing error '%s' not found", errorID),
		}))
		return
	}

	vars := gimlet.GetVars(r)
	vars["project_id"] = processingError.ProjectID
	r = gimlet.SetURLVars(r, vars)
	r = r.WithContext(context.WithValue(r.Context(), githubIntentProcessingErrorContextKey{}, processingError))
	next(rw, r)
}
