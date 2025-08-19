package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const firingStatus = "firing"

func makeSetDegradedMode() gimlet.RouteHandler {
	return &degradedModeHandler{}
}

type degradedModeHandler struct {
	Status string `json:"status"`
}

func (h *degradedModeHandler) Factory() gimlet.RouteHandler {
	return &degradedModeHandler{}
}

func (h *degradedModeHandler) Parse(ctx context.Context, r *http.Request) error {
	if err := gimlet.GetJSON(r.Body, h); err != nil {
		return errors.Wrap(err, "parsing request")
	}
	if h.Status != firingStatus {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "alert is in incorrect state to trigger degraded mode",
		}
	}
	return nil
}

func (h *degradedModeHandler) Run(ctx context.Context) gimlet.Responder {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "retrieving config"))
	}
	if config.ServiceFlags.CPUDegradedModeDisabled {
		config.ServiceFlags.CPUDegradedModeDisabled = false
		if err = config.ServiceFlags.Set(ctx); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting service flags"))
		}
		grip.Info(message.Fields{
			"message": "CPU degraded mode has been triggered",
			"route":   "/degraded_mode",
		})
		if config.Banner == "" {
			msg := fmt.Sprintf("Evergreen is under high load, max config YAML size has been reduced from %dMB to %dMB. "+
				"Existing tasks with large (>16MB) config YAMLs may also experience slower scheduling.", config.TaskLimits.MaxParserProjectSize, config.TaskLimits.MaxDegradedModeParserProjectSize)
			if err = evergreen.SetBanner(ctx, msg); err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting banner text"))
			}
		}
		if err = data.SetBannerTheme(ctx, string(evergreen.Information)); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting banner theme"))
		}
	}
	return gimlet.NewJSONResponse(struct{}{})
}
