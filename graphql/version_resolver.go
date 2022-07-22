package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	werrors "github.com/pkg/errors"
)

func (r *versionResolver) BaseTaskStatuses(ctx context.Context, obj *restModel.APIVersion) ([]string, error) {
	var baseVersion *model.Version
	var err error

	if evergreen.IsPatchRequester(utility.FromStringPtr(obj.Requester)) || utility.FromStringPtr(obj.Requester) == evergreen.AdHocRequester {
		// Get base commit if patch or periodic build.
		baseVersion, err = model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(utility.FromStringPtr(obj.Project), utility.FromStringPtr(obj.Revision)))
	} else {
		// Get previous commit if mainline commit.
		baseVersion, err = model.VersionFindOne(model.VersionByProjectIdAndOrder(utility.FromStringPtr(obj.Project), obj.Order-1))
	}
	if baseVersion == nil || err != nil {
		return nil, nil
	}
	statuses, err := task.GetBaseStatusesForActivatedTasks(*obj.Id, baseVersion.Id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting base version tasks: '%s'", err.Error()))
	}
	return statuses, nil
}

func (r *versionResolver) BaseVersion(ctx context.Context, obj *restModel.APIVersion) (*restModel.APIVersion, error) {
	baseVersion, err := model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(*obj.Project, *obj.Revision))
	if baseVersion == nil || err != nil {
		return nil, nil
	}
	apiVersion := restModel.APIVersion{}
	apiVersion.BuildFromService(*baseVersion)
	return &apiVersion, nil
}

func (r *versionResolver) BuildVariants(ctx context.Context, obj *restModel.APIVersion, options *BuildVariantOptions) ([]*GroupedBuildVariant, error) {
	// If activated is nil in the db we should resolve it and cache it for subsequent queries. There is a very low likely hood of this field being hit
	if obj.Activated == nil {
		version, err := model.VersionFindOne(model.VersionById(*obj.Id))
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error fetching version: %s : %s", *obj.Id, err.Error()))
		}
		if err = setVersionActivationStatus(version); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error setting version activation status: %s", err.Error()))
		}
		obj.Activated = version.Activated
	}

	if !utility.FromBoolPtr(obj.Activated) {
		return nil, nil
	}
	groupedBuildVariants, err := generateBuildVariants(utility.FromStringPtr(obj.Id), *options)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error generating build variants for version %s : %s", *obj.Id, err.Error()))
	}
	return groupedBuildVariants, nil
}

func (r *versionResolver) BuildVariantStats(ctx context.Context, obj *restModel.APIVersion, options *BuildVariantOptions) ([]*task.GroupedTaskStatusCount, error) {
	opts := task.GetTasksByVersionOptions{
		TaskNames:                      options.Tasks,
		Variants:                       options.Variants,
		Statuses:                       options.Statuses,
		IncludeBuildVariantDisplayName: true,
	}
	stats, err := task.GetGroupedTaskStatsByVersion(utility.FromStringPtr(obj.Id), opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting version task stats: %s", err.Error()))
	}
	return stats, nil
}

func (r *versionResolver) ChildVersions(ctx context.Context, obj *restModel.APIVersion) ([]*restModel.APIVersion, error) {
	if !evergreen.IsPatchRequester(*obj.Requester) {
		return nil, nil
	}
	if err := data.ValidatePatchID(*obj.Id); err != nil {
		return nil, werrors.WithStack(err)
	}
	foundPatch, err := patch.FindOneId(*obj.Id)
	if err != nil {
		return nil, err
	}
	if foundPatch == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("patch with id %s not found", *obj.Id),
		}
	}
	childPatchIds := foundPatch.Triggers.ChildPatches
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Couldn't find a patch with id: `%s` %s", *obj.Id, err.Error()))
	}
	if len(childPatchIds) > 0 {
		childVersions := []*restModel.APIVersion{}
		for _, cp := range childPatchIds {
			// this calls the graphql Version query resolver
			cv, err := r.Query().Version(ctx, cp)
			if err != nil {
				//before erroring due to the version being nil or not found,
				// fetch the child patch to see if it's activated
				p, err := patch.FindOneId(cp)
				if err != nil {
					return nil, InternalServerError.Send(ctx, fmt.Sprintf("Encountered an error while fetching a child patch: %s", err.Error()))
				}
				if p == nil {
					return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to child patch %s", cp))
				}
				if p.Version != "" {
					//only return the error if the version is activated (and we therefore expect it to be there)
					return nil, InternalServerError.Send(ctx, "An unexpected error occurred. Could not find a child version and expected one.")
				}
			}
			if cv != nil {
				childVersions = append(childVersions, cv)
			}
		}
		return childVersions, nil
	}
	return nil, nil
}

func (r *versionResolver) IsPatch(ctx context.Context, obj *restModel.APIVersion) (bool, error) {
	return evergreen.IsPatchRequester(*obj.Requester), nil
}

func (r *versionResolver) Manifest(ctx context.Context, obj *restModel.APIVersion) (*Manifest, error) {
	m, err := manifest.FindFromVersion(*obj.Id, *obj.Project, *obj.Revision, *obj.Requester)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error fetching manifest for version %s : %s", *obj.Id, err.Error()))
	}
	if m == nil {
		return nil, nil
	}
	versionManifest := Manifest{
		ID:              m.Id,
		Revision:        m.Revision,
		Project:         m.ProjectName,
		Branch:          m.Branch,
		IsBase:          m.IsBase,
		ModuleOverrides: m.ModuleOverrides,
	}
	modules := map[string]interface{}{}
	for key, module := range m.Modules {
		modules[key] = module
	}
	versionManifest.Modules = modules

	return &versionManifest, nil
}

func (r *versionResolver) Patch(ctx context.Context, obj *restModel.APIVersion) (*restModel.APIPatch, error) {
	if !evergreen.IsPatchRequester(*obj.Requester) {
		return nil, nil
	}
	apiPatch, err := data.FindPatchById(*obj.Id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Couldn't find a patch with id: `%s` %s", *obj.Id, err.Error()))
	}
	return apiPatch, nil
}

func (r *versionResolver) PreviousVersion(ctx context.Context, obj *restModel.APIVersion) (*restModel.APIVersion, error) {
	if !evergreen.IsPatchRequester(utility.FromStringPtr(obj.Requester)) {
		previousVersion, err := model.VersionFindOne(model.VersionByProjectIdAndOrder(utility.FromStringPtr(obj.Project), obj.Order-1))
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding previous version for `%s`: %s", *obj.Id, err.Error()))
		}
		if previousVersion == nil {
			return nil, nil
		}
		apiVersion := restModel.APIVersion{}
		apiVersion.BuildFromService(*previousVersion)
		return &apiVersion, nil
	} else {
		return nil, nil
	}
}

func (r *versionResolver) ProjectMetadata(ctx context.Context, obj *restModel.APIVersion) (*restModel.APIProjectRef, error) {
	projectRef, err := model.FindMergedProjectRef(*obj.Project, *obj.Id, false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding project ref for project `%s`: %s", *obj.Project, err.Error()))
	}
	if projectRef == nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding project ref for project `%s`: %s", *obj.Project, "Project not found"))
	}
	apiProjectRef := restModel.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(*projectRef); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("building APIProjectRef from service for `%s`: %s", projectRef.Id, err.Error()))
	}
	return &apiProjectRef, nil
}

func (r *versionResolver) Status(ctx context.Context, obj *restModel.APIVersion) (string, error) {
	collectiveStatusArray, err := getCollectiveStatusArray(*obj)
	if err != nil {
		return "", InternalServerError.Send(ctx, fmt.Sprintf("getting collective status array: %s", err.Error()))
	}
	status := patch.GetCollectiveStatus(collectiveStatusArray)
	return status, nil
}

func (r *versionResolver) TaskCount(ctx context.Context, obj *restModel.APIVersion) (*int, error) {
	taskCount, err := task.Count(db.Query(task.DisplayTasksByVersion(*obj.Id)))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting task count for version `%s`: %s", *obj.Id, err.Error()))
	}
	return &taskCount, nil
}

func (r *versionResolver) TaskStatuses(ctx context.Context, obj *restModel.APIVersion) ([]string, error) {
	defaultSort := []task.TasksSortOrder{
		{Key: task.DisplayNameKey, Order: 1},
	}
	opts := task.GetTasksByVersionOptions{
		Sorts:                          defaultSort,
		IncludeBaseTasks:               false,
		FieldsToProject:                []string{task.DisplayStatusKey},
		IncludeBuildVariantDisplayName: false,
	}
	tasks, _, err := task.GetTasksByVersion(*obj.Id, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting version tasks: %s", err.Error()))
	}
	return getAllTaskStatuses(tasks), nil
}

func (r *versionResolver) TaskStatusStats(ctx context.Context, obj *restModel.APIVersion, options *BuildVariantOptions) (*task.TaskStats, error) {
	opts := task.GetTasksByVersionOptions{
		IncludeBaseTasks:      false,
		IncludeExecutionTasks: false,
		TaskNames:             options.Tasks,
		Variants:              options.Variants,
		Statuses:              getValidTaskStatusesFilter(options.Statuses),
	}
	if len(options.Variants) != 0 {
		opts.IncludeBuildVariantDisplayName = true // we only need the buildVariantDisplayName if we plan on filtering on it.
	}
	stats, err := task.GetTaskStatsByVersion(*obj.Id, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting version task status stats: %s", err.Error()))
	}
	return stats, nil
}

func (r *versionResolver) UpstreamProject(ctx context.Context, obj *restModel.APIVersion) (*UpstreamProject, error) {
	v, err := model.VersionFindOneId(utility.FromStringPtr(obj.Id))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding version %s: '%s'", *obj.Id, err.Error()))
	}
	if v == nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Version %s not found", *obj.Id))
	}
	if v.TriggerID == "" || v.TriggerType == "" {
		return nil, nil
	}

	var projectID string
	var upstreamProject *UpstreamProject
	if v.TriggerType == model.ProjectTriggerLevelTask {
		upstreamTask, err := task.FindOneId(v.TriggerID)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding upstream task %s: '%s'", v.TriggerID, err.Error()))
		}
		if upstreamTask == nil {
			return nil, ResourceNotFound.Send(ctx, "upstream task not found")
		}

		apiTask := restModel.APITask{}
		if err = apiTask.BuildFromService(upstreamTask, nil); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building APITask from service for `%s`: %s", upstreamTask.Id, err.Error()))
		}

		projectID = upstreamTask.Project
		upstreamProject = &UpstreamProject{
			Revision: upstreamTask.Revision,
			Task:     &apiTask,
		}
	} else if v.TriggerType == model.ProjectTriggerLevelBuild {
		upstreamBuild, err := build.FindOneId(v.TriggerID)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding upstream build %s: '%s'", v.TriggerID, err.Error()))
		}
		if upstreamBuild == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Upstream build %s not found", v.TriggerID))
		}

		upstreamVersion, err := model.VersionFindOneId(utility.FromStringPtr(&upstreamBuild.Version))
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding upstream version %s: '%s'", *obj.Id, err.Error()))
		}
		if upstreamVersion == nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("upstream version %s not found", *obj.Id))
		}

		apiVersion := restModel.APIVersion{}
		apiVersion.BuildFromService(*upstreamVersion)

		projectID = upstreamVersion.Identifier
		upstreamProject = &UpstreamProject{
			Revision: upstreamBuild.Revision,
			Version:  &apiVersion,
		}
	}
	upstreamProjectRef, err := model.FindBranchProjectRef(projectID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding upstream project, project: %s, error: '%s'", projectID, err.Error()))
	}
	if upstreamProjectRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Upstream project %s not found", projectID))
	}

	upstreamProject.Owner = upstreamProjectRef.Owner
	upstreamProject.Repo = upstreamProjectRef.Repo
	upstreamProject.Project = upstreamProjectRef.Identifier
	upstreamProject.TriggerID = v.TriggerID
	upstreamProject.TriggerType = v.TriggerType
	return upstreamProject, nil
}

func (r *versionResolver) VersionTiming(ctx context.Context, obj *restModel.APIVersion) (*VersionTiming, error) {
	v, err := model.VersionFindOneId(*obj.Id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding version `%s`: %s", *obj.Id, err.Error()))
	}
	if v == nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding version `%s`: %s", *obj.Id, "Version not found"))
	}
	timeTaken, makespan, err := v.GetTimeSpent()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting timing for version `%s`: %s", *obj.Id, err.Error()))
	}
	// return nil if rounded timeTaken/makespan == 0s
	t := timeTaken.Round(time.Second)
	m := makespan.Round(time.Second)

	var apiTimeTaken restModel.APIDuration
	var apiMakespan restModel.APIDuration
	if t.Seconds() != 0 {
		apiTimeTaken = restModel.NewAPIDuration(t)
	}
	if m.Seconds() != 0 {
		apiMakespan = restModel.NewAPIDuration(m)
	}

	return &VersionTiming{
		TimeTaken: &apiTimeTaken,
		Makespan:  &apiMakespan,
	}, nil
}

// Version returns VersionResolver implementation.
func (r *Resolver) Version() VersionResolver { return &versionResolver{r} }

type versionResolver struct{ *Resolver }
