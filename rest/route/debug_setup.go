package route

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/gimlet"
)

type debugSetupCompleteHandler struct {
	hostID      string
	setupSecret string
}

func makeDebugSetupComplete() gimlet.RouteHandler {
	return &debugSetupCompleteHandler{}
}

func (h *debugSetupCompleteHandler) Factory() gimlet.RouteHandler {
	return &debugSetupCompleteHandler{}
}

func (h *debugSetupCompleteHandler) Parse(ctx context.Context, r *http.Request) error {
	h.hostID = gimlet.GetVars(r)["host_id"]
	h.setupSecret = r.Header.Get(evergreen.DebugSetupSecretHeader)
	if h.hostID == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "host ID is required",
		}
	}
	if h.setupSecret == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "setup secret is required",
		}
	}
	return nil
}

func (h *debugSetupCompleteHandler) Run(ctx context.Context) gimlet.Responder {
	foundHost, err := host.FindOneId(ctx, h.hostID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	if foundHost == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "host not found",
		})
	}
	if !foundHost.IsDebug || foundHost.ProvisionOptions == nil || foundHost.ProvisionOptions.SetupSecret == "" {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "host does not have a setup secret",
		})
	}
	if subtle.ConstantTimeCompare([]byte(h.setupSecret), []byte(foundHost.ProvisionOptions.SetupSecret)) != 1 {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "invalid setup secret",
		})
	}

	if err := foundHost.ClearSetupSecret(ctx); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
}
