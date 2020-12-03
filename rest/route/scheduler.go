package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
)

type compareTasksRoute struct {
	taskIds []string
	sc      data.Connector
}

func makeCompareTasksRoute(sc data.Connector) gimlet.RouteHandler {
	return &compareTasksRoute{
		sc: sc,
	}
}

func (p *compareTasksRoute) Factory() gimlet.RouteHandler {
	return &compareTasksRoute{
		sc: p.sc,
	}
}

func (p *compareTasksRoute) Parse(ctx context.Context, r *http.Request) error {
	request := model.CompareTasksRequest{}
	err := utility.ReadJSON(r.Body, &request)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}
	p.taskIds = request.Tasks
	return nil
}

func (p *compareTasksRoute) Run(ctx context.Context) gimlet.Responder {
	order, logic, err := p.sc.CompareTasks(p.taskIds)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	resp := model.CompareTasksResponse{
		Order: order,
		Logic: logic,
	}
	return gimlet.NewJSONResponse(resp)
}
