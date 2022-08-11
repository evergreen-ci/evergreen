package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/gimlet"
)

// GET /rest/v2/agent/cedar_config

type agentCedarConfig struct {
	config evergreen.CedarConfig
}

func makeAgentCedarConfig(config evergreen.CedarConfig) *agentCedarConfig {
	return &agentCedarConfig{
		config: config,
	}
}

func (h *agentCedarConfig) Factory() gimlet.RouteHandler {
	return &agentCedarConfig{
		config: h.config,
	}
}

func (*agentCedarConfig) Parse(_ context.Context, _ *http.Request) error { return nil }

func (h *agentCedarConfig) Run(ctx context.Context) gimlet.Responder {
	return gimlet.NewJSONResponse(apimodels.CedarConfig{
		BaseURL:  h.config.BaseURL,
		RPCPort:  h.config.RPCPort,
		Username: h.config.User,
		APIKey:   h.config.APIKey,
		Insecure: h.config.Insecure,
	})
}

// GET /rest/v2/agent/data_pipes_config

type agentDataPipesConfig struct {
	config evergreen.DataPipesConfig
}

func makeAgentDataPipesConfig(config evergreen.DataPipesConfig) *agentDataPipesConfig {
	return &agentDataPipesConfig{
		config: config,
	}
}

func (h *agentDataPipesConfig) Factory() gimlet.RouteHandler {
	return &agentDataPipesConfig{
		config: h.config,
	}
}

func (*agentDataPipesConfig) Parse(_ context.Context, _ *http.Request) error { return nil }

func (h *agentDataPipesConfig) Run(ctx context.Context) gimlet.Responder {
	return gimlet.NewJSONResponse(apimodels.DataPipesConfig{
		Host:         h.config.Host,
		Region:       h.config.Region,
		AWSAccessKey: h.config.AWSAccessKey,
		AWSSecretKey: h.config.AWSSecretKey,
		AWSToken:     h.config.AWSToken,
	})
}
