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
	if p.newProjectId == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "must provide new project ID",
		}
	}
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
		return gimlet.MakeJSONErrorResponder(errors.Errorf("provide different ID for new project"))
	}
	if err != nil {
		apiErr, ok := err.(gimlet.ErrorResponse)
		if !ok {
			return gimlet.MakeJSONErrorResponder(errors.Errorf("Type assertion failed: type %T does not hold an error", err))
		}
		if apiErr.StatusCode != http.StatusNotFound {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error finding project '%s'", p.newProjectId))
		}
	}

	// copy project, disable necessary settings
	projectToCopy.Identifier = p.newProjectId
	projectToCopy.Enabled = false
	projectToCopy.PRTestingEnabled = false
	projectToCopy.CommitQueue.Enabled = false
	if err = p.sc.CreateProject(projectToCopy); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error creating project for id '%s'", p.newProjectId))
	}
	apiProjectRef := &model.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(*projectToCopy); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "error building API project from service"))
	}

	// copy variables, aliases, and subscriptions
	if err = p.sc.CopyProjectVars(p.oldProjectId, p.newProjectId); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "error copying project vars from project '%s'", p.oldProjectId))
	}
	if err = p.sc.CopyProjectAliases(p.oldProjectId, p.newProjectId); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "error copying aliases from project '%s'", p.oldProjectId))
	}
	if err = p.sc.CopyProjectSubscriptions(p.oldProjectId, p.newProjectId); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "error copying subscriptions from project '%s'", p.oldProjectId))
	}

	return gimlet.NewJSONResponse(apiProjectRef)
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/projects/{project_id}/copy_redacted

type copyRedactedVarsHandler struct {
	copyFrom string
	copyTo   string
	dryRun   bool
	sc       data.Connector
}

func makeCopyRedactedVars(sc data.Connector) gimlet.RouteHandler {
	return &copyRedactedVarsHandler{
		sc: sc,
	}
}

func (p *copyRedactedVarsHandler) Factory() gimlet.RouteHandler {
	return &copyRedactedVarsHandler{
		sc: p.sc,
	}
}

func (p *copyRedactedVarsHandler) Parse(ctx context.Context, r *http.Request) error {
	p.copyFrom = gimlet.GetVars(r)["project_id"]
	p.copyTo = r.FormValue("dest_project")
	p.dryRun = r.FormValue("dry_run") == "true"
	if p.copyTo == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "must provide new project ID",
		}
	}
	return nil
}

func (p *copyRedactedVarsHandler) Run(ctx context.Context) gimlet.Responder {
	_, err := p.sc.FindProjectById(p.copyTo) // ensure project is existing
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error finding project '%s'", p.copyTo))
	}

	varsToCopy, err := p.sc.FindProjectVarsById(p.copyFrom, true)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error finding variables for '%s'", p.copyFrom))
	}

	// return any variables that will be overwritten in the new project
	if p.dryRun {
		if err := p.willOverwriteVars(varsToCopy); err != nil {
			return gimlet.NewJSONErrorResponse(err)
		}
		return gimlet.NewJSONResponse(struct{}{})
	}

	if err := p.sc.UpdateProjectVars(p.copyTo, varsToCopy); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "error copying project vars from project '%s'", p.copyFrom))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

func (p *copyRedactedVarsHandler) willOverwriteVars(varsToCopy *model.APIProjectVars) error {

	overwrittenVars := []string{}
	existingVars, err := p.sc.FindProjectVarsById(p.copyTo, false)
	if err != nil {
		return errors.Wrapf(err, "Database error finding variables for '%s'", p.copyTo)
	}
	for v := range varsToCopy.Vars {
		if _, ok := existingVars.Vars[v]; ok {
			overwrittenVars = append(overwrittenVars, v)
		}
	}
	if len(overwrittenVars) > 0 {
		return errors.Errorf("These variables will be overwritten: %v", overwrittenVars)
	}
	return nil
}
