package route

import (
	"context"
	"net/http"

	dbModel "github.com/evergreen-ci/evergreen/model"

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
	oldProjectName string
	newProjectName string
	sc             data.Connector
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
	p.oldProjectName = gimlet.GetVars(r)["project_id"]
	p.newProjectName = r.FormValue("new_project")
	if p.newProjectName == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "must provide new project ID",
		}
	}
	return nil
}

func (p *projectCopyHandler) Run(ctx context.Context) gimlet.Responder {
	projectToCopy, err := p.sc.FindProjectById(p.oldProjectName)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error finding project '%s'", p.oldProjectName))
	}
	if projectToCopy == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("project '%s' doesn't exist", p.oldProjectName))
	}

	// verify project with new name doesn't exist
	_, err = dbModel.FindIdentifierForProject(p.newProjectName)
	if err == nil { // indicates the project already exists
		return gimlet.MakeJSONErrorResponder(errors.Errorf("provide different ID for new project"))
	}
	apiErr, ok := err.(gimlet.ErrorResponse)
	if !ok {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("Type assertion failed: type %T does not hold an error", err))
	}
	if apiErr.StatusCode != http.StatusNotFound {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error finding project '%s'", p.newProjectName))
	}

	// copy project, disable necessary settings
	oldIdentifier := projectToCopy.Identifier
	projectToCopy.Identifier = p.newProjectName // TODO: this will be internally generated in future work
	projectToCopy.Name = p.newProjectName
	projectToCopy.Enabled = false
	projectToCopy.PRTestingEnabled = false
	projectToCopy.CommitQueue.Enabled = false
	u := gimlet.GetUser(ctx).(*user.DBUser)
	if err = p.sc.CreateProject(projectToCopy, u); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error creating project for id '%s'", p.newProjectName))
	}
	apiProjectRef := &model.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(*projectToCopy); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "error building API project from service"))
	}

	// copy variables, aliases, and subscriptions
	if err = p.sc.CopyProjectVars(oldIdentifier, projectToCopy.Identifier); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "error copying project vars from project '%s'", p.oldProjectName))
	}
	if err = p.sc.CopyProjectAliases(oldIdentifier, projectToCopy.Identifier); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "error copying aliases from project '%s'", p.oldProjectName))
	}
	if err = p.sc.CopyProjectSubscriptions(oldIdentifier, projectToCopy.Identifier); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "error copying subscriptions from project '%s'", p.oldProjectName))
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
	copyToIdentifier, err := dbModel.FindIdentifierForProject(p.opts.CopyTo)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error finding project '%s'", p.opts.CopyTo))
	}
	copyFromIdentifier, err := dbModel.FindIdentifierForProject(p.copyFrom)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error finding project '%s'", p.opts.CopyTo))
	}

	varsToCopy, err := p.sc.FindProjectVarsById(copyFromIdentifier, p.opts.DryRun) //dont redact private variables unless it's a dry run
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

	if err := p.sc.UpdateProjectVars(copyToIdentifier, varsToCopy, p.opts.Overwrite); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "error copying project vars from project '%s'", p.copyFrom))
	}

	return gimlet.NewJSONResponse(struct{}{})
}
