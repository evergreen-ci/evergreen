package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/projects/{project_id}/copy

type projectCopyHandler struct {
	oldProject string
	newProject string
	sc         data.Connector
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
	p.oldProject = gimlet.GetVars(r)["project_id"]
	p.newProject = r.FormValue("new_project")
	if p.newProject == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "must provide new project ID",
		}
	}
	return nil
}

func (p *projectCopyHandler) Run(ctx context.Context) gimlet.Responder {
	projectToCopy, err := p.sc.FindProjectById(p.oldProject)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error finding project '%s'", p.oldProject))
	}
	if projectToCopy == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("project '%s' doesn't exist", p.oldProject))
	}

	// verify project with new name doesn't exist
	_, err = p.sc.FindProjectById(p.newProject)
	if err == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("provide different ID for new project"))
	}
	apiErr, ok := err.(gimlet.ErrorResponse)
	if !ok {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("Type assertion failed: type %T does not hold an error", err))
	}
	if apiErr.StatusCode != http.StatusNotFound {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error finding project '%s'", p.newProject))
	}

	// copy project, disable necessary settings
	oldId := projectToCopy.Id
	projectToCopy.Id = p.newProject // TODO: this will be internally generated in future work
	projectToCopy.Identifier = p.newProject
	projectToCopy.Enabled = false
	projectToCopy.PRTestingEnabled = false
	projectToCopy.CommitQueue.Enabled = false
	u := gimlet.GetUser(ctx).(*user.DBUser)
	if err = p.sc.CreateProject(projectToCopy, u); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error creating project for id '%s'", p.newProject))
	}
	apiProjectRef := &model.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(*projectToCopy); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "error building API project from service"))
	}

	// copy variables, aliases, and subscriptions
	if err = p.sc.CopyProjectVars(oldId, projectToCopy.Id); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "error copying project vars from project '%s'", p.oldProject))
	}
	if err = p.sc.CopyProjectAliases(oldId, projectToCopy.Id); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "error copying aliases from project '%s'", p.oldProject))
	}
	if err = p.sc.CopyProjectSubscriptions(oldId, projectToCopy.Id); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "error copying subscriptions from project '%s'", p.oldProject))
	}

	return gimlet.NewJSONResponse(apiProjectRef)
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/projects/{project_id}/copy/variables

type copyVariablesHandler struct {
	copyFrom string
	opts     copyVariablesOptions
	sc       data.Connector
}

type copyVariablesOptions struct {
	CopyTo         string `json:"copy_to"`
	DryRun         bool   `json:"dry_run"`
	IncludePrivate bool   `json:"include_private"`
	Overwrite      bool   `json:"overwrite"`
}

func makeCopyVariables(sc data.Connector) gimlet.RouteHandler {
	return &copyVariablesHandler{
		sc: sc,
	}
}

func (p *copyVariablesHandler) Factory() gimlet.RouteHandler {
	return &copyVariablesHandler{
		sc: p.sc,
	}
}

func (p *copyVariablesHandler) Parse(ctx context.Context, r *http.Request) error {
	p.copyFrom = gimlet.GetVars(r)["project_id"]
	if err := utility.ReadJSON(r.Body, &p.opts); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}
	if p.opts.CopyTo == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "must provide new project ID",
		}
	}
	return nil
}

func (p *copyVariablesHandler) Run(ctx context.Context) gimlet.Responder {
	copyToProject, err := p.sc.FindProjectById(p.opts.CopyTo) // ensure project is existing
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error finding project '%s'", p.opts.CopyTo))
	}

	copyFromProject, err := p.sc.FindProjectById(p.copyFrom) // ensure project is existing
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error finding project '%s'", p.copyFrom))
	}

	varsToCopy, err := p.sc.FindProjectVarsById(copyFromProject.Id, p.opts.DryRun) //dont redact private variables unless it's a dry run
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error finding variables for '%s'", p.copyFrom))
	}
	if !p.opts.IncludePrivate {
		for key, isPrivate := range varsToCopy.PrivateVars {
			if isPrivate {
				delete(varsToCopy.Vars, key)
				delete(varsToCopy.RestrictedVars, key)
			}
		}
		varsToCopy.PrivateVars = map[string]bool{}
	}

	// return the variables that would be copied
	if p.opts.DryRun {
		return gimlet.NewJSONResponse(varsToCopy)
	}

	if err := p.sc.UpdateProjectVars(copyToProject.Id, varsToCopy, p.opts.Overwrite); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "error copying project vars from project '%s'", p.copyFrom))
	}

	return gimlet.NewJSONResponse(struct{}{})
}
