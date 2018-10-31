package route

import (
	"context"
	"errors"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
)

func makeTriggerMetadataHandler(sc data.Connector) gimlet.RouteHandler {
	return &triggerMetadataHandler{
		sc: sc,
	}
}

type triggerMetadataHandler struct {
	taskID string
	sc     data.Connector
}

func (h *triggerMetadataHandler) Factory() gimlet.RouteHandler {
	return &triggerMetadataHandler{
		sc: h.sc,
	}
}

func (h *triggerMetadataHandler) Parse(ctx context.Context, r *http.Request) error {
	h.taskID = gimlet.GetVars(r)["task_id"]

	if h.taskID == "" {
		return errors.New("task_id is required")
	}

	return nil
}

func (h *triggerMetadataHandler) Run(ctx context.Context) gimlet.Responder {
	metadata, err := h.sc.TriggerDataFromTask(h.taskID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	return gimlet.NewJSONResponse(metadata)
}
