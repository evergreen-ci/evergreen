package route

import (
	"context"
	"net/http"

	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/repos/{repo_id}

type repoIDGetHandler struct {
	repoName string
	sc       data.Connector
}

func makeGetRepoByID(sc data.Connector) gimlet.RouteHandler {
	return &repoIDGetHandler{
		sc: sc,
	}
}

func (h *repoIDGetHandler) Factory() gimlet.RouteHandler {
	return &repoIDGetHandler{
		sc: h.sc,
	}
}

func (h *repoIDGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.repoName = gimlet.GetVars(r)["repo_id"]
	return nil
}

func (h *repoIDGetHandler) Run(ctx context.Context) gimlet.Responder {
	repo, err := dbModel.FindOneRepoRef(h.repoName)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	if repo == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("repo '%s' doesn't exist", h.repoName))
	}

	repoModel := &model.APIProjectRef{}
	if err = repoModel.BuildFromService(repo.ProjectRef); err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "problem converting repo document",
			StatusCode: http.StatusInternalServerError,
		})
	}

	repoVars, err := h.sc.FindProjectVarsById(repo.Id, true)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	repoModel.Variables = *repoVars

	if repoModel.Aliases, err = h.sc.FindProjectAliases(repo.Id); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(repoModel)
}
