package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type clearTaskQueueHandler struct {
	distro string
}

func makeClearTaskQueueHandler() gimlet.RouteHandler {
	return &clearTaskQueueHandler{}
}

func (h *clearTaskQueueHandler) Factory() gimlet.RouteHandler {
	return &clearTaskQueueHandler{}
}

func (h *clearTaskQueueHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distro = r.FormValue("distro")

	return nil
}

func (h *clearTaskQueueHandler) Run(ctx context.Context) gimlet.Responder {
	tq, err := model.LoadTaskQueue(h.distro)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task queue for distro '%s'", h.distro))
	}
	if tq == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task queue for distro '%s' not found", h.distro),
		})
	}

	if err := model.ClearTaskQueue(h.distro); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
}
