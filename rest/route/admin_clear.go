package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/gimlet"
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

	if tq, err := model.LoadTaskQueue(h.distro); err != nil || tq == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("unable to find task queue for distro '%s'", h.distro),
		}
	}

	return nil
}

func (h *clearTaskQueueHandler) Run(ctx context.Context) gimlet.Responder {
	if err := model.ClearTaskQueue(h.distro); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
}
