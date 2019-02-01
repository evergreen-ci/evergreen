package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type commitQueueGetHandler struct {
	project string

	sc data.Connector
}

func makeGetCommitQueueItems(sc data.Connector) gimlet.RouteHandler {
	return commitQueueGetHandler{
		sc: sc,
	}
}

func (cq commitQueueGetHandler) Factory() gimlet.RouteHandler {
	return &commitQueueGetHandler{
		sc: cq.sc,
	}
}

func (cq commitQueueGetHandler) Parse(ctx context.Context, r *http.Request) error {
	cq.project = gimlet.GetVars(r)["project_id"]
	return nil
}

func (cq commitQueueGetHandler) Run(ctx context.Context) gimlet.Responder {
	commitQueue, err := cq.sc.FindCommitQueueByID(cq.project)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "can't get commit queue"))
	}
	if commitQueue == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("no matching project for ID %s", cq.project),
		})
	}

	return gimlet.NewJSONResponse(commitQueue)
}

type commitQueueDeleteItemHandler struct {
	project string
	item    string

	sc data.Connector
}

func makeDeleteCommitQueueItems(sc data.Connector) gimlet.RouteHandler {
	return commitQueueDeleteItemHandler{
		sc: sc,
	}
}

func (cq commitQueueDeleteItemHandler) Factory() gimlet.RouteHandler {
	return &commitQueueDeleteItemHandler{
		sc: cq.sc,
	}
}

func (cq commitQueueDeleteItemHandler) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	cq.project = vars["project_id"]
	cq.item = vars["item"]

	return nil
}

func (cq commitQueueDeleteItemHandler) Run(ctx context.Context) gimlet.Responder {
	found, err := cq.sc.CommitQueueRemoveItem(cq.project, cq.item)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "can't delete item"))
	}
	if !found {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("no matching item for project ID '%s', item '%s'", cq.project, cq.item),
		})
	}

	response := gimlet.NewJSONResponse(struct{}{})
	if err := response.SetStatus(http.StatusNoContent); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusNoContent))
	}
	return response
}
