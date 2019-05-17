package route

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/evergreen-ci/evergreen/util"

	"github.com/pkg/errors"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
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
	projectVars, err := h.sc.FindProjectVarsById(h.projectID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "problem getting variables for project '%s'", h.projectID))
	}
	projectVars.RedactPrivateVars()
	varsModel := model.DbProjectVarsToRestModel(*projectVars)
	return gimlet.NewJSONResponse(varsModel)
}

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/projects/{project_id}/variables

type patchProjectVarsHandler struct {
	projectID string
	body      []byte
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
	b, err := ioutil.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "Argument read error")
	}
	h.body = b
	return nil
}

func (h *patchProjectVarsHandler) Run(ctx context.Context) gimlet.Responder {
	varsModel := model.APIProjectVars{}
	if err := json.Unmarshal(h.body, &varsModel); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error while unmarshalling JSON"))
	}

	newVars := model.DbProjectVarsFromRestModel(varsModel)
	newVars.Id = h.projectID
	varsToDelete := varsModel.VarsToDelete
	if err := h.sc.UpdateProjectVars(&newVars, varsToDelete); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "problem updating variables for project '%s'", h.projectID))
	}
	newVars.RedactPrivateVars()
	return gimlet.NewJSONResponse(model.DbProjectVarsToRestModel(newVars))
}
