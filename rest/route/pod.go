package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
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
// PUT /rest/v2/pods/{pod_id}

type podIDPutHandler struct {
	podID string
	body  []byte
	sc    data.Connector
}

func makePutPod(sc data.Connector) gimlet.RouteHandler {
	return &podIDPutHandler{
		sc: sc,
	}
}

func (h *podIDPutHandler) Factory() gimlet.RouteHandler {
	return &podIDPutHandler{
		sc: h.sc,
	}
}

// Parse fetches the podID and JSON payload from the HTTP request.
func (h *podIDPutHandler) Parse(ctx context.Context, r *http.Request) error {
	h.podID = gimlet.GetVars(r)["pod_id"]

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
func (h *podIDPutHandler) Run(ctx context.Context) gimlet.Responder {
	apiPod := &model.APICreatePod{
		Name: utility.ToStringPtr(h.podID),
	}
	if err := json.Unmarshal(h.body, apiPod); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error while unmarshalling JSON"))
	}

	newPod, respErr := validateCreatePod(ctx, apiPod, h.podID, true)
	if respErr != nil {
		return respErr
	}

	responder := gimlet.NewJSONResponse(struct{}{})
	if err := responder.SetStatus(http.StatusCreated); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusCreated))
	}
	if err := h.sc.CreatePod(newPod); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for insert() pod with pod id '%s", h.podID))
	}

	return responder
}

////////////////////////////////////////////////////////////////////////

func validateCreatePod(ctx context.Context, apiPod *model.APICreatePod, resourceID string, isNewPod bool) (*pod.Pod, gimlet.Responder) {
	i, err := apiPod.ToService()
	if err != nil {
		return nil, gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error converting from model.APICreatePod to pod.Pod"))
	}

	p, ok := i.(*pod.Pod)
	if !ok {
		return nil, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("Unexpected type %T for pod.Pod", i),
		})
	}

	id := utility.FromStringPtr(apiPod.Name)
	if resourceID != id {
		return nil, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusForbidden,
			Message:    fmt.Sprintf("A pod's name is immutable; cannot rename pod '%s'", resourceID),
		})
	}

	return p, nil
}
