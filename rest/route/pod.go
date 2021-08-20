package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////
//
// POST /rest/v2/pods

type podPostHandler struct {
	env evergreen.Environment
	sc  data.Connector
	p   model.APICreatePod
}

func makePostPod(env evergreen.Environment, sc data.Connector) gimlet.RouteHandler {
	return &podPostHandler{
		env: env,
		sc:  sc,
	}
}

func (h *podPostHandler) Factory() gimlet.RouteHandler {
	return &podPostHandler{
		env: h.env,
		sc:  h.sc,
	}
}

// Parse fetches the podID and JSON payload from the HTTP request.
func (h *podPostHandler) Parse(ctx context.Context, r *http.Request) error {
	body := util.NewRequestReader(r)
	defer body.Close()

	if err := utility.ReadJSON(r.Body, &h.p); err != nil {
		return errors.Wrap(err, "reading payload body")
	}
	if err := h.validatePayload(); err != nil {
		return errors.Wrap(err, "invalid API input")
	}

	return nil
}

// validatePayload validates that the input request payload is valid.
func (h *podPostHandler) validatePayload() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(utility.FromStringPtr(h.p.Image) == "", "missing image")
	catcher.NewWhen(utility.FromStringPtr(h.p.OS) == "", "missing OS")
	catcher.NewWhen(utility.FromStringPtr(h.p.Arch) == "", "missing architecture")
	catcher.NewWhen(utility.FromIntPtr(h.p.CPU) <= 0, "CPU must be a positive non-zero value")
	catcher.NewWhen(utility.FromIntPtr(h.p.Memory) <= 0, "memory must be a positive non-zero value")
	for i, envVar := range h.p.EnvVars {
		catcher.ErrorfWhen(utility.FromStringPtr(envVar.Name) == "", "missing environment variable name for variable at index %d", i)
	}
	return catcher.Resolve()
}

// Run creates a new resource based on the Request-URI and JSON payload.
func (h *podPostHandler) Run(ctx context.Context) gimlet.Responder {
	res, err := h.sc.CreatePod(h.p)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "creating new pod"))
	}

	responder, err := gimlet.NewBasicResponder(http.StatusCreated, gimlet.JSON, res)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "constructing response"))
	}

	j := units.NewPodCreationJob(res.ID, utility.RoundPartOfMinute(0).Format(units.TSFormat))
	if err := amboy.EnqueueUniqueJob(ctx, h.env.RemoteQueue(), j); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not enqueue Amboy job to create pod",
			"pod_id":  res.ID,
			"job_id":  j.ID(),
			"route":   "/rest/v2/pods",
		}))
	}

	return responder
}
