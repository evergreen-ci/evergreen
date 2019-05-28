package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/projects/{project_id}/variables

type getProjectVarsHandler struct {
	projectID string
	sc        data.Connector
}

func makeGetVariablesByProject(sc data.Connector) gimlet.RouteHandler {
	return &getProjectVarsHandler{
		sc: sc,
	}
}

func (h *getProjectVarsHandler) Factory() gimlet.RouteHandler {
	return &projectIDGetHandler{
		sc: h.sc,
	}
}

func (h *getProjectVarsHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectID = gimlet.GetVars(r)["project_id"]
	return nil
}

func (h *getProjectVarsHandler) Run(ctx context.Context) gimlet.Responder {
	varsModel, err := h.sc.FindProjectVarsById(h.projectID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "problem getting variables for project '%s'", h.projectID))
	}
	return gimlet.NewJSONResponse(*varsModel)
}

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/projects/{project_id}/variables

type patchProjectVarsHandler struct {
	projectID string
	varsModel model.APIProjectVars
	sc        data.Connector
}

func makePatchVariablesByProject(sc data.Connector) gimlet.RouteHandler {
	return &patchProjectVarsHandler{
		sc: sc,
	}
}

func (h *patchProjectVarsHandler) Factory() gimlet.RouteHandler {
	return &projectIDGetHandler{
		sc: h.sc,
	}
}

func (h *patchProjectVarsHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectID = gimlet.GetVars(r)["project_id"]
	body := util.NewRequestReader(r)
	defer body.Close()

	varsModel := model.APIProjectVars{}
	if err := gimlet.GetJSON(body, &varsModel); err != nil {
		return errors.Wrap(err, "Argument read error")
	}
	h.varsModel = varsModel
	return nil
}

func (h *patchProjectVarsHandler) Run(ctx context.Context) gimlet.Responder {
	if err := h.sc.UpdateProjectVars(h.projectID, &h.varsModel); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "problem updating variables for project '%s'", h.projectID))
	}

	return gimlet.NewJSONResponse(h.varsModel)
}
