package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
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
	opts := data.CopyProjectOpts{
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
	opts     copyVariablesOptions
}

type copyVariablesOptions struct {
	CopyTo         string `json:"copy_to"`
	DryRun         bool   `json:"dry_run"`
	IncludePrivate bool   `json:"include_private"`
	Overwrite      bool   `json:"overwrite"`
}

func makeCopyVariables() gimlet.RouteHandler {
	return &copyVariablesHandler{}
}

func (p *copyVariablesHandler) Factory() gimlet.RouteHandler {
	return &copyVariablesHandler{}
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
		return errors.New("must provide new project ID")
	}
	return nil
}

func (p *copyVariablesHandler) Run(ctx context.Context) gimlet.Responder {
	copyToProjectId, errResp := getProjectOrRepoId(p.opts.CopyTo)
	if errResp != nil {
		fmt.Println("copy to")
		return errResp
	}
	copyFromProjectId, errResp := getProjectOrRepoId(p.copyFrom)
	if errResp != nil {
		return errResp
	}

	// Don't redact private variables unless it's a dry run
	varsToCopy, err := data.FindProjectVarsById(copyFromProjectId, "", p.opts.DryRun)
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

	if err := data.UpdateProjectVars(copyToProjectId, varsToCopy, p.opts.Overwrite); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "copying project vars from source project '%s' to target project '%s'", p.copyFrom, p.opts.CopyTo))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

func getProjectOrRepoId(identifier string) (string, gimlet.Responder) {
	id, err := model.GetIdForProject(identifier) // Ensure project is existing
	if err != nil {
		// Check if this is a repo project instead
		repoRef, err := model.FindOneRepoRef(identifier)
		if err != nil {
			return "", gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project/repo '%s'", identifier))
		}
		if repoRef == nil {
			return "", gimlet.MakeJSONErrorResponder(errors.Errorf("couldn't find project/repo '%s'", identifier))
		}
		return repoRef.Id, nil
	}
	return id, nil
}
