package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type aliasGetHandler struct {
	name string
}

func makeFetchAliases() gimlet.RouteHandler {
	return &aliasGetHandler{}
}

func (a *aliasGetHandler) Factory() gimlet.RouteHandler {
	return &aliasGetHandler{}
}

func (a *aliasGetHandler) Parse(ctx context.Context, r *http.Request) error {
	a.name = gimlet.GetVars(r)["name"]
	return nil
}

func (a *aliasGetHandler) Run(ctx context.Context) gimlet.Responder {
	pRef, err := model.FindBranchProjectRef(a.name)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "finding project '%s'", a.name))
	}
	if pRef == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("project '%s' not found", a.name))
	}
	aliasModels, err := data.FindMergedProjectAliases(pRef.Id, pRef.RepoRefId, nil, false)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project aliases for project '%s'", pRef.Id))
	}

	resp := gimlet.NewResponseBuilder()

	for _, alias := range aliasModels {
		if err := resp.AddData(alias); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	return resp
}
