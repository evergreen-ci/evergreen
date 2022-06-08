package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
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
	p   model.APICreatePod
}

func makePostPod(env evergreen.Environment) gimlet.RouteHandler {
	return &podPostHandler{
		env: env,
	}
}

func (h *podPostHandler) Factory() gimlet.RouteHandler {
	return &podPostHandler{
		env: h.env,
	}
}

// Parse fetches the pod ID and JSON payload from the HTTP request.
func (h *podPostHandler) Parse(ctx context.Context, r *http.Request) error {
	body := utility.NewRequestReader(r)
	defer body.Close()

	if err := utility.ReadJSON(r.Body, &h.p); err != nil {
		return errors.Wrap(err, "reading pod creation options from JSON request body")
	}

	return nil
}

// Run creates a new pod based on the request payload.
func (h *podPostHandler) Run(ctx context.Context) gimlet.Responder {
	res, err := data.CreatePod(h.p)
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

////////////////////////////////////////////////
//
// GET /rest/v2/pods/{pod_id}

type podGetHandler struct {
	env   evergreen.Environment
	podID string
}

func makeGetPod(env evergreen.Environment) gimlet.RouteHandler {
	return &podGetHandler{
		env: env,
	}
}

func (h *podGetHandler) Factory() gimlet.RouteHandler {
	return &podGetHandler{
		env: h.env,
	}
}

// Parse fetches the pod ID from the HTTP request.
func (h *podGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.podID = gimlet.GetVars(r)["pod_id"]
	return nil
}

// Run finds and returns the REST pod.
func (h *podGetHandler) Run(ctx context.Context) gimlet.Responder {
	p, err := data.FindPodByID(h.podID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding pod '%s'", h.podID))
	}
	if p == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("pod '%s' not found", h.podID),
		})
	}

	return gimlet.NewJSONResponse(p)
}
