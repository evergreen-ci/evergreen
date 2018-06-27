package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
)

func makePlaceHolderManger(sc data.Connector) gimlet.RouteHandler {
	return &placeHolderHandler{
		sc: sc,
	}
}

type placeHolderHandler struct {
	sc data.Connector
}

func (p *placeHolderHandler) Factory() gimlet.RouteHandler {
	return &placeHolderHandler{
		sc: p.sc,
	}
}

func (p *placeHolderHandler) Parse(ctx context.Context, r *http.Request) (context.Context, error) {
	return ctx, nil
}

func (p *placeHolderHandler) Run(ctx context.Context) gimlet.Responder {
	return gimlet.NewTextResponse("this is a placeholder for now")
}
