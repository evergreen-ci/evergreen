package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type aliasGetHandler struct {
	name string
	sc   data.Connector
}

func makeFetchAliases(sc data.Connector) gimlet.RouteHandler {
	return &aliasGetHandler{
		sc: sc,
	}
}

func (a *aliasGetHandler) Factory() gimlet.RouteHandler {
	return &aliasGetHandler{
		sc: a.sc,
	}
}

func (a *aliasGetHandler) Parse(ctx context.Context, r *http.Request) error {
	a.name = gimlet.GetVars(r)["name"]
	return nil
}

func (a *aliasGetHandler) Run(ctx context.Context) gimlet.Responder {
	aliasModels, err := a.sc.FindProjectAliases(a.name)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	resp := gimlet.NewResponseBuilder()

	for _, alias := range aliasModels {
		if err := resp.AddData(alias); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	return resp
}
