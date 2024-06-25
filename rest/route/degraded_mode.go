package route

import (
	"context"
	"github.com/evergreen-ci/evergreen"
	"github.com/pkg/errors"
	"net/http"

	"github.com/evergreen-ci/gimlet"
)

const firingStatus = "firing"
const evergreenWebhook = "webhook-devprod-evergreen"

func makeSetDegradedMode() gimlet.RouteHandler {
	return &degradedModeHandler{}
}

type degradedModeHandler struct {
	Receiver string `json:"receiver"`
	Status   string `json:"status"`
}

func (h *degradedModeHandler) Factory() gimlet.RouteHandler {
	return &degradedModeHandler{}
}

func (h *degradedModeHandler) Parse(ctx context.Context, r *http.Request) error {
	if err := gimlet.GetJSON(r.Body, h); err != nil {
		return errors.Wrap(err, "problem parsing request")
	}
	if h.Status != firingStatus || h.Receiver != evergreenWebhook {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "alert is in incorrect state to trigger degraded mode",
		}
	}
	return nil
}

func (h *degradedModeHandler) Run(ctx context.Context) gimlet.Responder {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "retrieving service flags"))
	}
	flags.DegradedModeDisabled = false
	if err = flags.Set(ctx); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting service flags"))
	}
	return gimlet.NewJSONResponse(struct{}{})
}
