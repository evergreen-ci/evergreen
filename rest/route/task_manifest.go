package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// getManifestHandler implements the route GET /tasks/{task_id}/manifest.
// It fetches the associated manifest and returns it to the user.
type getManifestHandler struct {
	taskID string
}

func makeGetManifestHandler() gimlet.RouteHandler {
	return &getManifestHandler{}
}

func (h *getManifestHandler) Factory() gimlet.RouteHandler {
	return &getManifestHandler{}
}

// ParseAndValidate fetches the taskId from the http request.
func (h *getManifestHandler) Parse(ctx context.Context, r *http.Request) error {
	h.taskID = gimlet.GetVars(r)["task_id"]
	return nil
}

// Execute returns the manifest for the given task.
func (h *getManifestHandler) Run(ctx context.Context) gimlet.Responder {
	manifest, err := data.GetManifestByTask(h.taskID)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "getting manifest using task '%s'", h.taskID))
	}
	return gimlet.NewJSONResponse(manifest)
}
