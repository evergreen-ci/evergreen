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
	projectID            string
	includeProjectConfig bool
}

func makeFetchAliases() gimlet.RouteHandler {
	return &aliasGetHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get a project's aliases
//	@Description	Returns the project's aliases. This endpoint serves the data returned by the "evergreen list --patch-aliases" command.
//	@Tags			projects
//	@Router			/alias/{project_id} [get]
//	@Security		Api-User || Api-Key
//	@Param			project_id				path		string	true	"the project ID"
//	@Param			includeProjectConfig	query		bool	false	"Setting to true will return the merged result of the project and the config properties set in the project YAML. Defaults to false"
//	@Success		200						{object}	[]model.APIProjectAlias
func (a *aliasGetHandler) Factory() gimlet.RouteHandler {
	return &aliasGetHandler{}
}

func (a *aliasGetHandler) Parse(ctx context.Context, r *http.Request) error {
	a.projectID = gimlet.GetVars(r)["project_id"]
	a.includeProjectConfig = r.URL.Query().Get("includeProjectConfig") == "true"
	return nil
}

func (a *aliasGetHandler) Run(ctx context.Context) gimlet.Responder {
	pRef, err := model.FindBranchProjectRef(ctx, a.projectID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "finding project '%s'", a.projectID))
	}
	if pRef == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("project '%s' not found", a.projectID))
	}
	aliasModels, err := data.FindMergedProjectAliases(ctx, pRef.Id, pRef.RepoRefId, nil, a.includeProjectConfig)
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
