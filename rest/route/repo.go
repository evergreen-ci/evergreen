package route

import (
	"context"
	"net/http"

	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/repos/{repo_id}

type repoIDGetHandler struct {
	repoID string
}

func makeGetRepoByID() gimlet.RouteHandler {
	return &repoIDGetHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get a repo
//	@Description	Returns the repo settings (restricted to users with project settings view permissions to at least one branch project).
//	@Tags			repos
//	@Router			/repos/{repo_id} [get]
//	@Security		Api-User || Api-Key
//	@Param			repo_id	path		string	true	"the repo ID"
//	@Success		200		{object}	model.APIProjectRef
func (h *repoIDGetHandler) Factory() gimlet.RouteHandler {
	return &repoIDGetHandler{}
}

func (h *repoIDGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.repoID = gimlet.GetVars(r)["repo_id"]
	return nil
}

func (h *repoIDGetHandler) Run(ctx context.Context) gimlet.Responder {
	repoRef, err := dbModel.FindOneRepoRef(ctx, h.repoID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding repo '%s'", h.repoID))
	}
	if repoRef == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("repo '%s' not found", h.repoID))
	}

	repoModel := &model.APIProjectRef{}
	if err = repoModel.BuildFromService(ctx, repoRef.ProjectRef); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "converting repo '%s' to API model", h.repoID))
	}

	return gimlet.NewJSONResponse(repoModel)
}
