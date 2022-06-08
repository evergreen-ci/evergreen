package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/gimlet"
)

////////////////////////////////////////////////
//
// GET /rest/v2/agent/cedar_config

type agentCedarConfig struct {
	settings *evergreen.Settings
}

func makeAgentCedarConfig(settings *evergreen.Settings) gimlet.RouteHandler {
	return &agentCedarConfig{
		settings: settings,
	}
}

func (h *agentCedarConfig) Factory() gimlet.RouteHandler {
	return &agentCedarConfig{
		settings: h.settings,
	}
}

func (h *agentCedarConfig) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *agentCedarConfig) Run(ctx context.Context) gimlet.Responder {
	data := apimodels.CedarConfig{
		BaseURL:  h.settings.Cedar.BaseURL,
		RPCPort:  h.settings.Cedar.RPCPort,
		Username: h.settings.Cedar.User,
		APIKey:   h.settings.Cedar.APIKey,
	}
	return gimlet.NewJSONResponse(data)
}
