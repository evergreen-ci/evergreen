package route

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
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
// POST /rest/v2/pods

type podPostHandler struct {
	body []byte
	sc   data.Connector
}

func makePostPod(sc data.Connector) gimlet.RouteHandler {
	return &podPostHandler{
		sc: sc,
	}
}

func (h *podPostHandler) Factory() gimlet.RouteHandler {
	return &podPostHandler{
		sc: h.sc,
	}
}

// Parse fetches the podID and JSON payload from the HTTP request.
func (h *podPostHandler) Parse(ctx context.Context, r *http.Request) error {
	body := util.NewRequestReader(r)
	defer body.Close()
	b, err := ioutil.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "Argument read error")
	}
	h.body = b

	return nil
}

// Run creates a new resource based on the Request-URI and JSON payload.
func (h *podPostHandler) Run(ctx context.Context) gimlet.Responder {
	apiPod := model.APICreatePod{}

	if err := json.Unmarshal(h.body, &apiPod); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error while unmarshalling JSON"))
	}

	id, err := h.sc.CreatePod(apiPod)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Creating new pod"))
	}

	responder := gimlet.NewJSONResponse(model.APICreatePodResponse{ID: id})

	if err := responder.SetStatus(http.StatusCreated); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusCreated))
	}

	return responder
}
