package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/gimlet"
)

type githubIntentProcessingErrorHandler struct{}

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
	processingError, err := GetGitHubIntentInfo(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	return gimlet.NewJSONResponse(githubIntentProcessingErrorResponse{Message: processingError.Message})
}
