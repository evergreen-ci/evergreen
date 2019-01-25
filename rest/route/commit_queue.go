package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
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
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "can't get commit queue from database"))
	}
	apiCommitQueue := model.APICommitQueue{}
	if err = apiCommitQueue.BuildFromService(*commitQueue); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "can't read commit queue into API model"))
	}

	return gimlet.NewJSONResponse(apiCommitQueue)
}
