package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/projects/{project_id}/copy

type projectCopyHandler struct {
	oldProjectId string
	newProjectId string
	sc           data.Connector
}

func makeCopyProject(sc data.Connector) gimlet.RouteHandler {
	return &projectCopyHandler{
		sc: sc,
	}
}

func (p *projectCopyHandler) Factory() gimlet.RouteHandler {
	return &projectCopyHandler{
		sc: p.sc,
	}
}

func (p *projectCopyHandler) Parse(ctx context.Context, r *http.Request) error {
	p.oldProjectId = gimlet.GetVars(r)["project_id"]
	p.newProjectId = r.FormValue("new_project")
	return nil
}

func (p *projectCopyHandler) Run(ctx context.Context) gimlet.Responder {
	projectToCopy, err := p.sc.FindProjectById(p.oldProjectId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error finding project '%s'", p.oldProjectId))
	}

	// verify project with new ID doesn't exist
	_, err = p.sc.FindProjectById(p.newProjectId)
	if err == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("Project '%s' already exists", p.newProjectId))
	}
	if err != nil {
		apiErr := err.(gimlet.ErrorResponse)
		if apiErr.StatusCode != http.StatusNotFound {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error finding project '%s'", p.newProjectId))
		}
	}

	// copy project
	projectToCopy.Identifier = p.newProjectId
	projectToCopy.Enabled = false
	if err = p.sc.CreateProject(projectToCopy); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error creating project for id '%s'", p.newProjectId))
	}
	apiProjectRef := &model.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(*projectToCopy); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "error building API project from service"))
	}

	// copy variables and aliases
	if err = p.sc.CopyProjectVars(p.oldProjectId, p.newProjectId); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "error copying project vars from project '%s'", p.oldProjectId))
	}
	if err = p.sc.CopyProjectAliases(p.oldProjectId, p.newProjectId); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "error copying aliases from project '%s'", p.oldProjectId))
	}

	return gimlet.NewJSONResponse(apiProjectRef)
}
