package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////
//
// POST /rest/v2/pods

type podPostHandler struct {
	sc data.Connector
	p  model.APICreatePod
}

func makePostPod(sc data.Connector) gimlet.RouteHandler {
	return &podPostHandler{
		sc: sc,
		p:  model.APICreatePod{},
	}
}

func (h *podPostHandler) Factory() gimlet.RouteHandler {
	return &podPostHandler{
		sc: h.sc,
		p:  model.APICreatePod{},
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

	if err := json.Unmarshal(b, &h.p); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("API error while unmarshalling JSON"),
		}
	}

	if utility.FromStringPtr(h.p.Image) == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintln("Invalid API input: missing or empty image input"),
		}
	}
	if utility.FromStringPtr(h.p.OS) == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintln("Invalid API input: missing or empty OS"),
		}
	}
	if utility.FromStringPtr(h.p.Arch) == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintln("Invalid API input: missing or empty architecture"),
		}
	}
	if utility.FromIntPtr(h.p.CPU) <= 0 {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintln("Invalid API input: missing or invalid CPU"),
		}
	}
	if utility.FromIntPtr(h.p.Memory) <= 0 {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintln("Invalid API input: missing or invalid memory"),
		}
	}

	for _, envVar := range h.p.EnvVars {
		if utility.FromStringPtr(envVar.Name) == "" {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("Invalid API input: missing or empty environment variable name"),
			}
		}
	}

	return nil
}

// Run creates a new resource based on the Request-URI and JSON payload.
func (h *podPostHandler) Run(ctx context.Context) gimlet.Responder {
	res, err := h.sc.CreatePod(h.p)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Creating new pod"))
	}

	responder := gimlet.NewJSONResponse(res)

	if err := responder.SetStatus(http.StatusCreated); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusCreated))
	}

	return responder
}
