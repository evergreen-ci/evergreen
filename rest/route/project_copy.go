package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
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
	env        evergreen.Environment
}

func makeCopyProject(env evergreen.Environment) gimlet.RouteHandler {
	return &projectCopyHandler{
		env: env,
	}
}

// Factory creates an instance of the handler.
//
//	@Summary		Copy a project
//	@Description	Restricted to admins of the original project. Create a new, initially disabled project (PR testing and CommitQueue also initially disabled). The unique identifier is passed to the query parameter new_project and is required.  Project variables, aliases, subscriptions, etc are copied but the GitHub app ID and key are not. Returns the new project (but not variables/aliases/subscriptions).
//	@Tags			projects
//	@Router			/projects/{project_id}/copy [post]
//	@Security		Api-User || Api-Key
//	@Param			project_id	path		string	true	"the project ID"
//	@Param			new_project	query		string	true	"the new project ID"
//	@Success		200			{object}	model.APIProjectRef
func (p *projectCopyHandler) Factory() gimlet.RouteHandler {
	return &projectCopyHandler{env: p.env}
}

func (p *projectCopyHandler) Parse(ctx context.Context, r *http.Request) error {
	p.oldProject = gimlet.GetVars(r)["project_id"]
	p.newProject = r.FormValue("new_project")
	if p.newProject == "" {
		return errors.New("must provide new project ID")
	}
	return nil
}

func (p *projectCopyHandler) Run(ctx context.Context) gimlet.Responder {
	opts := restModel.CopyProjectOpts{
		ProjectIdToCopy:      p.oldProject,
		NewProjectIdentifier: p.newProject,
	}
	apiProjectRef, err := data.CopyProject(ctx, p.env, opts)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "copying source project '%s' to target project '%s'", p.oldProject, p.newProject))
	}
	return gimlet.NewJSONResponse(apiProjectRef)
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/projects/{project_id}/copy/variables

type copyVariablesHandler struct {
	copyFrom string
	usr      *user.DBUser
	opts     copyVariablesOptions
}

type copyVariablesOptions struct {
	// Required. ProjectID to copy source_project variables to.
	CopyTo string `json:"copy_to"`
	// If set to true, route returns the variables from source_project that will
	// be copied. (If private, the values will be redacted.) If dry_run is set,
	// then the route does not complete the copy, but returns OK if no project
	// variables in the source project will be overwritten (this concerns
	// all variables in the destination project, but only redacted
	// variables in the source project). Otherwise, an error is given which
	// includes the project variable keys that overlap. If dry_run is not set,
	// the copy is completed, and variables could be overwritten.
	DryRun bool `json:"dry_run"`
	// 	If set to true, private variables will also be copied.
	IncludePrivate bool `json:"include_private"`
	// If set to true, will remove variables from the copy_to project that are not in source_project.
	Overwrite bool `json:"overwrite"`
}

func makeCopyVariables() gimlet.RouteHandler {
	return &copyVariablesHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Copy variables to an existing project
//	@Description	Restricted to admins of the source project/repo and the destination project/repo. Copies variables from projectA to projectB.
//	@Tags			projects
//	@Router			/projects/{project_id}/copy/variables [post]
//	@Security		Api-User || Api-Key
//	@Param			project_id	path	string					true	"the project ID"
//	@Param			{object}	body	copyVariablesOptions	false	"parameters"
//	@Success		200
func (p *copyVariablesHandler) Factory() gimlet.RouteHandler {
	return &copyVariablesHandler{}
}

func (p *copyVariablesHandler) Parse(ctx context.Context, r *http.Request) error {
	p.copyFrom = gimlet.GetVars(r)["project_id"]
	p.usr = MustHaveUser(ctx)
	if err := utility.ReadJSON(r.Body, &p.opts); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}
	if p.opts.CopyTo == "" {
		return errors.New("must provide new project ID")
	}
	return nil
}

func (p *copyVariablesHandler) Run(ctx context.Context) gimlet.Responder {
	copyToProjectId, copyIdIsProject, err := getProjectOrRepoId(ctx, p.opts.CopyTo)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	copyFromProjectId, _, err := getProjectOrRepoId(ctx, p.copyFrom)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	// Don't redact private variables unless it's a dry run
	varsToCopy, err := data.FindProjectVarsById(ctx, copyFromProjectId, "", p.opts.DryRun)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding vars for source project '%s'", p.copyFrom))
	}
	if !p.opts.IncludePrivate {
		for key, isPrivate := range varsToCopy.PrivateVars {
			if isPrivate {
				delete(varsToCopy.Vars, key)
				delete(varsToCopy.AdminOnlyVars, key)
			}
		}
		varsToCopy.PrivateVars = map[string]bool{}
	}

	// Return the variables that would be copied
	if p.opts.DryRun {
		return gimlet.NewJSONResponse(varsToCopy)
	}

	projectBefore, err := model.GetProjectSettingsById(ctx, copyToProjectId, !copyIdIsProject)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting settings for project '%s' before copying variables", copyToProjectId))
	}

	if err := data.UpdateProjectVars(ctx, copyToProjectId, varsToCopy, p.opts.Overwrite); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "copying project vars from source project '%s' to target project '%s'", p.copyFrom, p.opts.CopyTo))
	}

	if err = model.GetAndLogProjectModified(ctx, copyToProjectId, p.usr.Id, !copyIdIsProject, projectBefore); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "logging project '%s' variables copied", copyToProjectId))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

// getProjectOrRepoId returns the ID, and returns true if this is a project ref, and false otherwise.
func getProjectOrRepoId(ctx context.Context, identifier string) (string, bool, error) {
	id, err := model.GetIdForProject(ctx, identifier) // Ensure project is existing
	if err != nil {
		// Check if this is a repo project instead
		repoRef, err := model.FindOneRepoRef(ctx, identifier)
		if err != nil {
			return "", false, gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    fmt.Sprintf("finding project/repo '%s'", identifier),
			}
		}
		if repoRef == nil {
			return "", false, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("project/repo '%s' not found", identifier),
			}
		}
		return repoRef.Id, false, nil
	}
	return id, true, nil
}
