package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/gimlet"
)

func makePlaceHolder() gimlet.RouteHandler {
	return &placeHolderHandler{}
}

type placeHolderHandler struct{}

func (p *placeHolderHandler) Factory() gimlet.RouteHandler {
	return &placeHolderHandler{}
}

func (p *placeHolderHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (p *placeHolderHandler) Run(ctx context.Context) gimlet.Responder {
	return gimlet.NewTextResponse("this is a placeholder for now")
}
