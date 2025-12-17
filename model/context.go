package model

import (
	"context"

	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
)

// Context is the set of all the related entities in a
// task/build/version/project hierarchy. Using the LoadContext
// function, all the other applicable fields in the Context can
// inferred and populated from the id of any one of the fields.
type Context struct {
	Task       *task.Task
	Build      *build.Build
	Version    *Version
	Patch      *patch.Patch
	ProjectRef *ProjectRef
	RepoRef    *RepoRef

	project *Project
}

// LoadContext builds a Context from the set of given resource ID's
// by inferring all the relationships between them - for example, e.g. loading a project based on
// the the task, or the version based on the patch, etc.
func LoadContext(ctx context.Context, taskId, buildId, versionId, patchId, projectId string) (Context, error) {
	c := Context{}

	pID, err := c.populateTaskBuildVersion(ctx, taskId, buildId, versionId)
	if err != nil {
		return c, err
	}

	if len(projectId) == 0 || (len(pID) > 0 && pID != projectId) {
		projectId = pID
	}

	err = c.populatePatch(ctx, patchId)
	if err != nil {
		return c, err
	}
	if c.Patch != nil && len(projectId) == 0 {
		projectId = c.Patch.Project
	}

	// Try to load project for the ID we found, and set cookie with it for subsequent requests
	if len(projectId) > 0 {
		// Also lookup the ProjectRef itself and add it to context.
		c.ProjectRef, err = FindMergedProjectRef(ctx, projectId, versionId, true)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return c, err
		}
		c.RepoRef, err = FindOneRepoRef(ctx, projectId)
		if err != nil {
			return c, err
		}
	}

	return c, nil
}

func (ctx *Context) GetProjectRef(c context.Context) (*ProjectRef, error) {
	// if no project, use the default
	if ctx.ProjectRef == nil {
		var err error
		ctx.ProjectRef, err = FindAnyRestrictedProjectRef(c)
		if err != nil {
			return nil, errors.Wrap(err, "finding project ref")
		}
	}

	return ctx.ProjectRef, nil
}

// GetProject returns the project associated with the Context.
func (ctx *Context) GetProject(c context.Context) (*Project, error) {
	if ctx.project != nil {
		return ctx.project, nil
	}

	pref, err := ctx.GetProjectRef(c)
	if err != nil {
		return nil, errors.Wrap(err, "finding project")
	}

	_, ctx.project, _, err = FindLatestVersionWithValidProject(c, pref.Id, false)
	if err != nil {
		return nil, errors.Wrapf(err, "finding project from last good version for project ref '%s'", pref.Id)
	}

	return ctx.project, nil
}

func (ctx *Context) HasProjectOrRepoRef() bool {
	return ctx.ProjectRef != nil || ctx.RepoRef != nil
}

func (ctx *Context) GetProjectOrRepoRefId() string {
	if ctx.ProjectRef != nil {
		return ctx.ProjectRef.Id
	}
	return ctx.RepoRef.Id
}

// populateTaskBuildVersion takes a task, build, and version ID and populates a Context
// with as many of the task, build, and version documents as possible.
// If any of the provided IDs is blank, they will be inferred from the more selective ones.
// Returns the project ID of the data found, which may be blank if the IDs are empty.
func (c *Context) populateTaskBuildVersion(ctx context.Context, taskId, buildId, versionId string) (string, error) {
	projectId := ""
	var err error
	// Fetch task if there's a task ID present; if we find one, populate build/version IDs from it
	if len(taskId) > 0 {
		c.Task, err = task.FindOneId(ctx, taskId)
		if err != nil || c.Task == nil {
			// if no task found, see if this is an old task
			task, err := task.FindOneOldId(ctx, taskId)
			if err != nil {
				return "", err
			}
			c.Task = task
		}

		if c.Task != nil {
			// override build and version ID with the ones this task belongs to
			buildId = c.Task.BuildId
			versionId = c.Task.Version
			projectId = c.Task.Project
		}
	}

	// Fetch build if there's a build ID present; if we find one, populate version ID from it
	if len(buildId) > 0 {
		c.Build, err = build.FindOne(ctx, build.ById(buildId))
		if err != nil {
			return "", err
		}
		if c.Build != nil {
			versionId = c.Build.Version
			projectId = c.Build.Project
		}
	}
	if len(versionId) > 0 {
		c.Version, err = VersionFindOneIdWithBuildVariants(ctx, versionId)
		if err != nil {
			return "", err
		}
		if c.Version != nil {
			projectId = c.Version.Identifier
		}
	}
	return projectId, nil
}

// populatePatch loads a patch into the project context, using patchId if provided.
// If patchId is blank, will try to infer the patch ID from the version already loaded
// into context, if available.
func (ctx *Context) populatePatch(c context.Context, patchId string) error {
	var err error
	if len(patchId) > 0 {
		// The patch is explicitly identified in the URL, so fetch it
		if !patch.IsValidId(patchId) {
			return errors.Errorf("patch id '%s' is not an object id", patchId)
		}
		ctx.Patch, err = patch.FindOne(c, patch.ByStringId(patchId).Project(patch.ExcludePatchDiff))
	} else if ctx.Version != nil {
		// patch isn't in URL but the version in context has one, get it
		ctx.Patch, err = patch.FindOne(c, patch.ByVersion(ctx.Version.Id).Project(patch.ExcludePatchDiff))
	}
	if err != nil {
		return err
	}

	// If there's a finalized patch loaded into context but not a version, load the version
	// associated with the patch as the context's version.
	if ctx.Version == nil && ctx.Patch != nil && ctx.Patch.Version != "" {
		ctx.Version, err = VersionFindOneIdWithBuildVariants(c, ctx.Patch.Version)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
