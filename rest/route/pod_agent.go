package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
)

/////////////////////////////////////////
//
// GET /rest/v2/pods/{pod_id}/agent/setup

type podAgentSetup struct {
	settings *evergreen.Settings
}

func makePodAgentSetup(settings *evergreen.Settings) gimlet.RouteHandler {
	return &podAgentSetup{
		settings: settings,
	}
}

func (h *podAgentSetup) Factory() gimlet.RouteHandler {
	return &podAgentSetup{
		settings: h.settings,
	}
}

func (h *podAgentSetup) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *podAgentSetup) Run(ctx context.Context) gimlet.Responder {
	data := apimodels.AgentSetupData{
		SplunkServerURL:   h.settings.Splunk.ServerURL,
		SplunkClientToken: h.settings.Splunk.Token,
		SplunkChannel:     h.settings.Splunk.Channel,
		S3Bucket:          h.settings.Providers.AWS.S3.Bucket,
		S3Key:             h.settings.Providers.AWS.S3.Key,
		S3Secret:          h.settings.Providers.AWS.S3.Secret,
		TaskSync:          h.settings.Providers.AWS.TaskSync,
		LogkeeperURL:      h.settings.LoggerConfig.LogkeeperURL,
	}
	return gimlet.NewJSONResponse(data)
}

////////////////////////////////////////////////
//
// GET /rest/v2/pods/{pod_id}/agent/cedar_config

type podAgentCedarConfig struct {
	settings *evergreen.Settings
}

func makePodAgentCedarConfig(settings *evergreen.Settings) gimlet.RouteHandler {
	return &podAgentCedarConfig{
		settings: settings,
	}
}

func (h *podAgentCedarConfig) Factory() gimlet.RouteHandler {
	return &podAgentCedarConfig{
		settings: h.settings,
	}
}

func (h *podAgentCedarConfig) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *podAgentCedarConfig) Run(ctx context.Context) gimlet.Responder {
	data := apimodels.CedarConfig{
		BaseURL:  h.settings.Cedar.BaseURL,
		RPCPort:  h.settings.Cedar.RPCPort,
		Username: h.settings.Cedar.User,
		APIKey:   h.settings.Cedar.APIKey,
	}
	return gimlet.NewJSONResponse(data)
}

////////////////////////////////////////////////
//
// GET /rest/v2/pods/{pod_id}/agent/next_task

type podAgentNextTask struct {
	env   evergreen.Environment
	sc    data.Connector
	podID string
}

func makePodAgentNextTask(env evergreen.Environment, sc data.Connector) gimlet.RouteHandler {
	return &podAgentNextTask{
		env: env,
		sc:  sc,
	}
}

func (h *podAgentNextTask) Factory() gimlet.RouteHandler {
	return &podAgentNextTask{
		env: h.env,
		sc:  h.sc,
	}
}

func (h *podAgentNextTask) Parse(ctx context.Context, r *http.Request) error {
	h.podID = gimlet.GetVars(r)["pod_id"]
	if h.podID == "" {
		return errors.New("missing pod ID")
	}
	return nil
}

func (h *podAgentNextTask) Run(ctx context.Context) gimlet.Responder {
	j := units.NewPodTerminationJob(h.podID, "reached end of pod lifecycle", utility.RoundPartOfMinute(0))
	if err := amboy.EnqueueUniqueJob(ctx, h.env.RemoteQueue(), j); err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "enqueueing job to terminate pod").Error(),
		})
	}

	return gimlet.NewJSONResponse(apimodels.NextTaskResponse{})
}
