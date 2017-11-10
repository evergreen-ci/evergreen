package model

import (
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/pkg/errors"
)

// Context is the set of all the related entities in a
// task/build/version/project hierarchy. Using the LoadContext
// function, all the other applicable fields in the Context can
// inferred and populated from the id of any one of the fields.
type Context struct {
	projectRef *ProjectRef
	project    *Project
	patch      *patch.Patch
	version    *version.Version
	build      *build.Build
	task       *task.Task

	taskID    string
	buildID   string
	versionID string
	patchID   string
	projectID string
}

// LoadContext builds a Context from the set of given resource ID's
// by inferring all the relationships between them - for example, e.g. loading a project based on
// the the task, or the version based on the patch, etc.
func LoadContext(taskId, buildId, versionId, patchId, projectId string) *Context {
	return &Context{
		taskID:    taskId,
		buildID:   buildId,
		versionID: versionId,
		patchID:   patchId,
		projectID: projectId,
	}
}

// CreateContext builds a context from its constituent parts, and is
// mostly useful for testing purposes.
func CreateContext(pref *ProjectRef, proj *Project, patchDoc *patch.Patch, ver *version.Version, b *build.Build, t *task.Task) *Context {
	return &Context{
		projectRef: pref,
		project:    proj,
		patch:      patchDoc,
		version:    ver,
		build:      b,
		task:       t,
	}
}

func (ctx *Context) GetTask() (*task.Task, error) {
	if ctx.task != nil {
		return ctx.task, nil
	}

	if ctx.taskID == "" {
		return nil, errors.New("cannot resolve task from request context")
	}

	var err error

	ctx.task, err = task.FindOne(task.ById(ctx.taskID))
	if err != nil {
		ctx.taskID = ""
		return nil, err
	}

	if ctx.task == nil {
		return nil, errors.New("could not resolve task from request")
	}

	if ctx.buildID != ctx.task.BuildId {
		ctx.build = nil
		ctx.buildID = ctx.task.BuildId
	}

	if ctx.versionID != ctx.task.Version {
		ctx.version = nil
		ctx.versionID = ctx.task.Version
	}

	if ctx.projectID != ctx.task.Project {
		ctx.project = nil
		ctx.projectRef = nil
		ctx.projectID = ctx.task.Project
	}

	return ctx.task, nil
}

func (ctx *Context) GetBuild() (*build.Build, error) {
	if ctx.build != nil {
		return ctx.build, nil
	}

	if ctx.buildID == "" {
		return nil, errors.New("cannot resolve build from request context")
	}

	var err error

	ctx.build, err = build.FindOne(build.ById(ctx.buildID))
	if err != nil {
		ctx.buildID = ""
		return nil, errors.Wrapf(err, "problem resolving build from id '%s'", ctx.buildID)
	}

	if ctx.build == nil {
		return nil, errors.New("could not resolve build")
	}

	if ctx.versionID != ctx.build.Version {
		ctx.versionID = ctx.build.Version
		ctx.version = nil
	}

	if ctx.projectID != ctx.build.Project {
		ctx.projectID = ctx.build.Project
		ctx.project = nil
		ctx.projectRef = nil
	}

	return ctx.build, nil

}

func (ctx *Context) GetVersion() (*version.Version, error) {
	if ctx.version != nil {
		return ctx.version, nil
	}

	if ctx.versionID == "" {
		return nil, errors.New("no version specified, cannot populate version")
	}

	var err error

	ctx.version, err = version.FindOne(version.ById(ctx.versionID))
	if err != nil {
		ctx.versionID = ""
		return nil, err
	}

	if ctx.version == nil {
		return nil, errors.New("could not resolve version from request context")
	}

	if ctx.projectID != ctx.version.Identifier {
		ctx.projectID = ctx.version.Identifier
		ctx.projectRef = nil
		ctx.project = nil
	}

	return ctx.version, nil
}

func (ctx *Context) GetPatch() (*patch.Patch, error) {
	if ctx.patch != nil {
		return ctx.patch, nil
	}

	if ctx.patchID == "" {
		if ctx.taskID != "" {
			t, _ := ctx.GetTask()
			if t != nil {
				ctx.buildID = t.BuildId
				ctx.versionID = t.Version
				ctx.GetVersion()
			} else {
				return nil, errors.New("could not resolve patch from request context")
			}
		}
	}

	if err := ctx.populatePatch(ctx.patchID); err != nil {
		ctx.patchID = ""
		return nil, err
	}

	if ctx.patch == nil {
		return nil, errors.New("could not resolve patch from request context")
	}

	if ctx.projectID != ctx.patch.Project {
		ctx.projectID = ctx.patch.Project
		ctx.projectRef = nil
		ctx.project = nil
	}

	return ctx.patch, nil
}

// GetProject returns the project associated with the Context.
func (ctx *Context) GetProject() (*Project, error) {
	if ctx.project != nil {
		return ctx.project, nil
	}

	projRef, err := ctx.GetProjectRef()
	if err != nil {
		return nil, errors.Wrap(err, "problem resolving project ref")
	}

	ctx.project, err = FindProject("", projRef)
	if err != nil {
		return nil, errors.Wrap(err, "error finding project")
	}

	return ctx.project, nil
}

func (ctx *Context) GetProjectRef() (*ProjectRef, error) {
	if ctx.projectRef != nil {
		return ctx.projectRef, nil
	}

	var err error

	if ctx.projectID == "" {
		ctx.projectRef, err = FindFirstProjectRef()
		if err != nil {
			return nil, errors.Wrap(err, "error finding project ref")
		}

		ctx.projectID = ctx.projectRef.Identifier

		return ctx.projectRef, nil
	}

	ctx.projectRef, err = FindOneProjectRef(ctx.projectID)
	if err != nil {
		ctx.projectID = ""
		return nil, errors.Wrapf(err, "problem finding project record for '%s'", ctx.projectID)
	}

	return ctx.projectRef, nil
}

// populatePatch loads a patch into the project context, using patchId if provided.
// If patchId is blank, will try to infer the patch ID from the version already loaded
// into context, if available.
func (ctx *Context) populatePatch(patchId string) error {
	var err error
	if len(patchId) > 0 {
		// The patch is explicitly identified in the URL, so fetch it
		if !patch.IsValidId(patchId) {
			return errors.Errorf("patch id '%s' is not an object id", patchId)
		}
		ctx.patch, err = patch.FindOne(patch.ById(patch.NewId(patchId)).Project(patch.ExcludePatchDiff))
	} else if ctx.version != nil {
		// patch isn't in URL but the version in context has one, get it
		ctx.patch, err = patch.FindOne(patch.ByVersion(ctx.version.Id).Project(patch.ExcludePatchDiff))
	}
	if err != nil {
		return err
	}

	// If there's a finalized patch loaded into context but not a version, load the version
	// associated with the patch as the context's version.
	if ctx.version == nil && ctx.patch != nil && ctx.patch.Version != "" {
		ctx.version, err = version.FindOne(version.ById(ctx.patch.Version))
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
