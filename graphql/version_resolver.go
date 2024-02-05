package graphql

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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

// BaseTaskStatuses is the resolver for the baseTaskStatuses field.
func (r *versionResolver) BaseTaskStatuses(ctx context.Context, obj *restModel.APIVersion) ([]string, error) {
	baseVersion, err := model.FindBaseVersionForVersion(*obj.Id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding base version for version '%s': %s", *obj.Id, err.Error()))
	}
	if baseVersion == nil {
		return nil, nil
	}
	statuses, err := task.GetBaseStatusesForActivatedTasks(ctx, *obj.Id, baseVersion.Id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting base statuses for version '%s': %s", *obj.Id, err.Error()))
	}
	return statuses, nil
}

// BaseVersion is the resolver for the baseVersion field.
func (r *versionResolver) BaseVersion(ctx context.Context, obj *restModel.APIVersion) (*restModel.APIVersion, error) {
	baseVersion, err := model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(*obj.Project, *obj.Revision))
	if baseVersion == nil || err != nil {
		return nil, nil
	}

	apiVersion := restModel.APIVersion{}
	apiVersion.BuildFromService(*baseVersion)
	return &apiVersion, nil
}

// BuildVariants is the resolver for the buildVariants field.
func (r *versionResolver) BuildVariants(ctx context.Context, obj *restModel.APIVersion, options BuildVariantOptions) ([]*GroupedBuildVariant, error) {
	// If activated is nil in the db we should resolve it and cache it for subsequent queries. There is a very low likely hood of this field being hit
	if obj.Activated == nil {
		version, err := model.VersionFindOne(model.VersionById(*obj.Id))
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error fetching version: %s : %s", *obj.Id, err.Error()))
		}
		if err = setVersionActivationStatus(ctx, version); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error setting version activation status: %s", err.Error()))
		}
		obj.Activated = version.Activated
	}

	if obj.IsPatchRequester() && !utility.FromBoolPtr(obj.Activated) {
		return nil, nil
	}
	groupedBuildVariants, err := generateBuildVariants(ctx, utility.FromStringPtr(obj.Id), options, utility.FromStringPtr(obj.Requester), r.sc.GetURL())
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error generating build variants for version %s : %s", *obj.Id, err.Error()))
	}
	return groupedBuildVariants, nil
}

// BuildVariantStats is the resolver for the buildVariantStats field.
func (r *versionResolver) BuildVariantStats(ctx context.Context, obj *restModel.APIVersion, options BuildVariantOptions) ([]*task.GroupedTaskStatusCount, error) {
	opts := task.GetTasksByVersionOptions{
		TaskNames:                  options.Tasks,
		Variants:                   options.Variants,
		Statuses:                   options.Statuses,
		IncludeNeverActivatedTasks: !obj.IsPatchRequester(),
	}
	stats, err := task.GetGroupedTaskStatsByVersion(ctx, utility.FromStringPtr(obj.Id), opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting version task stats: %s", err.Error()))
	}
	return stats, nil
}

// ChildVersions is the resolver for the childVersions field.
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
				// before erroring due to the version being nil or not found,
				// fetch the child patch to see if it's activated
				p, err := patch.FindOneId(cp)
				if err != nil {
					return nil, InternalServerError.Send(ctx, fmt.Sprintf("Encountered an error while fetching a child patch: %s", err.Error()))
				}
				if p == nil {
					return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find child patch %s", cp))
				}
				if p.Version != "" {
					// only return the error if the version is activated (and we therefore expect it to be there)
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

// ExternalLinksForMetadata is the resolver for the externalLinksForMetadata field.
func (r *versionResolver) ExternalLinksForMetadata(ctx context.Context, obj *restModel.APIVersion) ([]*ExternalLinkForMetadata, error) {
	pRef, err := data.FindProjectById(*obj.Project, false, false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding project `%s`: %s", *obj.Project, err.Error()))
	}
	if pRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Project `%s` not found", *obj.Project))
	}
	var externalLinks []*ExternalLinkForMetadata

	for _, link := range pRef.ExternalLinks {
		if utility.StringSliceContains(link.Requesters, utility.FromStringPtr(obj.Requester)) {
			// replace {version_id} with the actual version id
			formattedURL := strings.Replace(link.URLTemplate, "{version_id}", *obj.Id, -1)
			externalLinks = append(externalLinks, &ExternalLinkForMetadata{
				URL:         formattedURL,
				DisplayName: link.DisplayName,
			})
		}
	}
	return externalLinks, nil
}

// IsPatch is the resolver for the isPatch field.
func (r *versionResolver) IsPatch(ctx context.Context, obj *restModel.APIVersion) (bool, error) {
	return evergreen.IsPatchRequester(*obj.Requester), nil
}

// Manifest is the resolver for the manifest field.
func (r *versionResolver) Manifest(ctx context.Context, obj *restModel.APIVersion) (*Manifest, error) {
	m, err := manifest.FindFromVersion(*obj.Id, *obj.Project, *obj.Revision, *obj.Requester)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error fetching manifest for version '%s': %s", *obj.Id, err.Error()))
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

// Patch is the resolver for the patch field.
func (r *versionResolver) Patch(ctx context.Context, obj *restModel.APIVersion) (*restModel.APIPatch, error) {
	if !evergreen.IsPatchRequester(*obj.Requester) {
		return nil, nil
	}
	apiPatch, err := data.FindPatchById(*obj.Id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Couldn't find a patch with id '%s': %s", *obj.Id, err.Error()))
	}
	return apiPatch, nil
}

// PreviousVersion is the resolver for the previousVersion field.
func (r *versionResolver) PreviousVersion(ctx context.Context, obj *restModel.APIVersion) (*restModel.APIVersion, error) {
	if !obj.IsPatchRequester() {
		previousVersion, err := model.VersionFindOne(model.VersionByProjectIdAndOrder(utility.FromStringPtr(obj.Project), obj.Order-1))
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding previous version for '%s': %s", *obj.Id, err.Error()))
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

// ProjectMetadata is the resolver for the projectMetadata field.
func (r *versionResolver) ProjectMetadata(ctx context.Context, obj *restModel.APIVersion) (*restModel.APIProjectRef, error) {
	apiProjectRef, err := getProjectMetadata(ctx, obj.Project, obj.Id)
	return apiProjectRef, err
}

// Status is the resolver for the status field.
func (r *versionResolver) Status(ctx context.Context, obj *restModel.APIVersion) (string, error) {
	versionId := utility.FromStringPtr(obj.Id)
	v, err := model.VersionFindOneId(versionId)
	if err != nil {
		return "", InternalServerError.Send(ctx, fmt.Sprintf("Error finding version '%s': %s", versionId, err.Error()))
	}
	if v == nil {
		return "", ResourceNotFound.Send(ctx, fmt.Sprintf("Version '%s' not found", versionId))
	}
	return getDisplayStatus(v)
}

// TaskCount is the resolver for the taskCount field.
func (r *versionResolver) TaskCount(ctx context.Context, obj *restModel.APIVersion) (*int, error) {
	taskCount, err := task.Count(db.Query(task.DisplayTasksByVersion(*obj.Id, !obj.IsPatchRequester())))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting task count for version `%s`: %s", *obj.Id, err.Error()))
	}
	return &taskCount, nil
}

// Tasks is the resolver for the tasks field.
func (r *versionResolver) Tasks(ctx context.Context, obj *restModel.APIVersion, options TaskFilterOptions) (*VersionTasks, error) {
	versionId := utility.FromStringPtr(obj.Id)
	pageParam := 0
	if options.Page != nil {
		pageParam = *options.Page
	}
	limitParam := 0
	if options.Limit != nil {
		limitParam = *options.Limit
	}
	variantParam := ""
	if options.Variant != nil {
		variantParam = *options.Variant
	}
	taskNameParam := ""
	if options.TaskName != nil {
		taskNameParam = *options.TaskName
	}
	var taskSorts []task.TasksSortOrder
	if len(options.Sorts) > 0 {
		taskSorts = []task.TasksSortOrder{}
		for _, singleSort := range options.Sorts {
			key := ""
			switch singleSort.Key {
			// the keys here should be the keys for the column headers of the tasks table
			case TaskSortCategoryName:
				key = task.DisplayNameKey
			case TaskSortCategoryStatus:
				key = task.DisplayStatusKey
			case TaskSortCategoryBaseStatus:
				key = task.BaseTaskStatusKey
			case TaskSortCategoryVariant:
				key = task.BuildVariantKey
			case TaskSortCategoryDuration:
				key = task.TimeTakenKey
			default:
				return nil, InputValidationError.Send(ctx, fmt.Sprintf("invalid sort key: '%s'", singleSort.Key))
			}
			order := 1
			if singleSort.Direction == SortDirectionDesc {
				order = -1
			}
			taskSorts = append(taskSorts, task.TasksSortOrder{Key: key, Order: order})
		}
	}
	baseVersionID := ""
	baseVersion, err := model.FindBaseVersionForVersion(utility.FromStringPtr(obj.Id))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding base version id for version '%s': %s", versionId, err.Error()))
	}
	if baseVersion != nil {
		baseVersionID = baseVersion.Id
	}
	opts := task.GetTasksByVersionOptions{
		Statuses:     getValidTaskStatusesFilter(options.Statuses),
		BaseStatuses: getValidTaskStatusesFilter(options.BaseStatuses),
		Variants:     []string{variantParam},
		TaskNames:    []string{taskNameParam},
		Page:         pageParam,
		Limit:        limitParam,
		Sorts:        taskSorts,
		// If the version is a patch, we want to exclude inactive tasks by default.
		IncludeNeverActivatedTasks: !evergreen.IsPatchRequester(utility.FromStringPtr(obj.Requester)) || utility.FromBoolPtr(options.IncludeEmptyActivation) || utility.FromBoolPtr(options.IncludeNeverActivatedTasks),
		BaseVersionID:              baseVersionID,
	}
	tasks, count, err := task.GetTasksByVersion(ctx, versionId, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting tasks for version with id '%s': %s", versionId, err.Error()))
	}

	var apiTasks []*restModel.APITask
	for _, t := range tasks {
		apiTask := restModel.APITask{}
		err := apiTask.BuildFromService(ctx, &t, nil)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting task item db model to api model: %s", err.Error()))
		}
		apiTasks = append(apiTasks, &apiTask)
	}
	versionTasks := VersionTasks{
		Count: count,
		Data:  apiTasks,
	}
	return &versionTasks, nil
}

// TaskStatuses is the resolver for the taskStatuses field.
func (r *versionResolver) TaskStatuses(ctx context.Context, obj *restModel.APIVersion) ([]string, error) {
	statuses, err := task.GetTaskStatusesByVersion(ctx, *obj.Id, false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting task statuses for version with id '%s': %s", *obj.Id, err.Error()))
	}
	return statuses, nil
}

// TaskStatusStats is the resolver for the taskStatusStats field.
func (r *versionResolver) TaskStatusStats(ctx context.Context, obj *restModel.APIVersion, options BuildVariantOptions) (*task.TaskStats, error) {
	opts := task.GetTasksByVersionOptions{
		IncludeExecutionTasks: false,
		TaskNames:             options.Tasks,
		Variants:              options.Variants,
		Statuses:              getValidTaskStatusesFilter(options.Statuses),
		// If the version is a patch, we don't want to include its never activated tasks.
		IncludeNeverActivatedTasks: !obj.IsPatchRequester(),
	}

	stats, err := task.GetTaskStatsByVersion(ctx, *obj.Id, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting version task status stats: %s", err.Error()))
	}
	return stats, nil
}

// UpstreamProject is the resolver for the upstreamProject field.
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
		if err = apiTask.BuildFromService(ctx, upstreamTask, nil); err != nil {
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
	} else if v.TriggerType == model.ProjectTriggerLevelPush {
		projectID = v.TriggerID
		upstreamProject = &UpstreamProject{
			Revision: v.TriggerSHA,
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

// VersionTiming is the resolver for the versionTiming field.
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

// Warnings is the resolver for the warnings field.
func (r *versionResolver) Warnings(ctx context.Context, obj *restModel.APIVersion) ([]string, error) {
	v, err := model.VersionFindOneId(utility.FromStringPtr(obj.Id))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding version with id `%s`: %s", *obj.Id, err.Error()))
	}
	if v == nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding version with id `%s`: %s", *obj.Id, "version not found"))
	}
	return v.Warnings, nil
}

// Version returns VersionResolver implementation.
func (r *Resolver) Version() VersionResolver { return &versionResolver{r} }

type versionResolver struct{ *Resolver }
