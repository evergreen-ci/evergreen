package graphql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	werrors "github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// BaseTaskStatuses is the resolver for the baseTaskStatuses field.
func (r *versionResolver) BaseTaskStatuses(ctx context.Context, obj *restModel.APIVersion) ([]string, error) {
	versionID := utility.FromStringPtr(obj.Id)
	baseVersion, err := model.FindBaseVersionForVersion(ctx, versionID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding base version for version '%s': %s", versionID, err.Error()))
	}
	if baseVersion == nil {
		return nil, nil
	}
	statuses, err := task.GetBaseStatusesForActivatedTasks(ctx, versionID, baseVersion.Id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting base statuses for version '%s': %s", versionID, err.Error()))
	}
	return statuses, nil
}

// BaseVersion is the resolver for the baseVersion field.
func (r *versionResolver) BaseVersion(ctx context.Context, obj *restModel.APIVersion) (*restModel.APIVersion, error) {
	baseVersion, err := model.VersionFindOne(ctx, model.BaseVersionByProjectIdAndRevision(utility.FromStringPtr(obj.Project), utility.FromStringPtr(obj.Revision)))
	if baseVersion == nil || err != nil {
		return nil, nil
	}

	apiVersion := restModel.APIVersion{}
	apiVersion.BuildFromService(ctx, *baseVersion)
	return &apiVersion, nil
}

// BuildVariants is the resolver for the buildVariants field.
func (r *versionResolver) BuildVariants(ctx context.Context, obj *restModel.APIVersion, options BuildVariantOptions) ([]*GroupedBuildVariant, error) {
	versionID := utility.FromStringPtr(obj.Id)
	// If activated is nil in the db we should resolve it and cache it for subsequent queries. There is a very low likely hood of this field being hit
	if obj.Activated == nil {
		version, err := model.VersionFindOneIdWithBuildVariants(ctx, versionID)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding version '%s': %s", versionID, err.Error()))
		}
		if err = setVersionActivationStatus(ctx, version); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("setting version activation status for version '%s': %s", versionID, err.Error()))
		}
		obj.Activated = version.Activated
	}

	if obj.IsPatchRequester() && !utility.FromBoolPtr(obj.Activated) {
		return nil, nil
	}
	groupedBuildVariants, err := generateBuildVariants(ctx, versionID, options, utility.FromStringPtr(obj.Requester), r.sc.GetURL())
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("generating build variants for version '%s': %s", versionID, err.Error()))
	}
	return groupedBuildVariants, nil
}

// BuildVariantStats is the resolver for the buildVariantStats field.
func (r *versionResolver) BuildVariantStats(ctx context.Context, obj *restModel.APIVersion, options BuildVariantOptions) ([]*task.GroupedTaskStatusCount, error) {
	includeNeverActivatedTasks := options.IncludeNeverActivatedTasks
	if includeNeverActivatedTasks == nil {
		includeNeverActivatedTasks = utility.ToBoolPtr(false)
	}
	opts := task.GetTasksByVersionOptions{
		TaskNames:                  options.Tasks,
		Variants:                   options.Variants,
		Statuses:                   options.Statuses,
		IncludeNeverActivatedTasks: *includeNeverActivatedTasks || !obj.IsPatchRequester(),
	}
	versionID := utility.FromStringPtr(obj.Id)
	stats, err := task.GetGroupedTaskStatsByVersion(ctx, versionID, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting task stats for version '%s': %s", versionID, err.Error()))
	}
	return stats, nil
}

// ChildVersions is the resolver for the childVersions field.
func (r *versionResolver) ChildVersions(ctx context.Context, obj *restModel.APIVersion) ([]*restModel.APIVersion, error) {
	if !evergreen.IsPatchRequester(utility.FromStringPtr(obj.Requester)) {
		return nil, nil
	}
	patchID := utility.FromStringPtr(obj.Id)
	if err := data.ValidatePatchID(patchID); err != nil {
		return nil, werrors.WithStack(err)
	}
	foundPatch, err := patch.FindOneId(ctx, patchID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching patch '%s': %s", patchID, err.Error()))
	}
	if foundPatch == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("patch '%s' not found", patchID))
	}
	childPatchIds := foundPatch.Triggers.ChildPatches
	if len(childPatchIds) > 0 {
		childVersions := []*restModel.APIVersion{}
		for _, cp := range childPatchIds {
			// this calls the graphql Version query resolver
			cv, err := r.Query().Version(ctx, cp)
			if err != nil {
				// before erroring due to the version being nil or not found,
				// fetch the child patch to see if it's activated
				p, err := patch.FindOneId(ctx, cp)
				if err != nil {
					return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching child patch '%s': %s", cp, err.Error()))
				}
				if p == nil {
					return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("child patch '%s' not found", cp))
				}
				if p.Version != "" {
					// only return the error if the version is activated (and we therefore expect it to be there)
					return nil, InternalServerError.Send(ctx, fmt.Sprintf("child patch '%s' has empty version", cp))
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
	projectID := utility.FromStringPtr(obj.Project)
	pRef, err := data.FindProjectById(ctx, projectID, false, false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", projectID, err.Error()))
	}
	if pRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", projectID))
	}
	var externalLinks []*ExternalLinkForMetadata

	for _, link := range pRef.ExternalLinks {
		if utility.StringSliceContains(link.Requesters, utility.FromStringPtr(obj.Requester)) {
			// replace {version_id} with the actual version id
			formattedURL := strings.Replace(link.URLTemplate, "{version_id}", utility.FromStringPtr(obj.Id), -1)
			externalLinks = append(externalLinks, &ExternalLinkForMetadata{
				URL:         formattedURL,
				DisplayName: link.DisplayName,
			})
		}
	}
	return externalLinks, nil
}

// GeneratedTaskCounts is the resolver for the generatedTaskCounts field.
func (r *versionResolver) GeneratedTaskCounts(ctx context.Context, obj *restModel.APIVersion) ([]*GeneratedTaskCountResults, error) {
	versionID := utility.FromStringPtr(obj.Id)
	v, err := model.VersionFindOneId(ctx, versionID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching version '%s': %s", versionID, err.Error()))
	}
	if v == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("version '%s' not found", versionID))
	}

	var res []*GeneratedTaskCountResults
	versionGeneratorTasks, err := task.Find(ctx, bson.M{
		task.VersionKey:      versionID,
		task.GenerateTaskKey: true,
	})
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding generator tasks from version '%s': %s", versionID, err.Error()))
	}
	for _, generatorTask := range versionGeneratorTasks {
		res = append(res, &GeneratedTaskCountResults{
			TaskID:         utility.ToStringPtr(generatorTask.Id),
			EstimatedTasks: utility.FromIntPtr(generatorTask.EstimatedNumActivatedGeneratedTasks),
		})
	}
	return res, nil
}

// IsPatch is the resolver for the isPatch field.
func (r *versionResolver) IsPatch(ctx context.Context, obj *restModel.APIVersion) (bool, error) {
	return evergreen.IsPatchRequester(utility.FromStringPtr(obj.Requester)), nil
}

// Manifest is the resolver for the manifest field.
func (r *versionResolver) Manifest(ctx context.Context, obj *restModel.APIVersion) (*Manifest, error) {
	versionID := utility.FromStringPtr(obj.Id)
	m, err := manifest.FindFromVersion(ctx, versionID, utility.FromStringPtr(obj.Project), utility.FromStringPtr(obj.Revision), utility.FromStringPtr(obj.Requester))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting manifest for version '%s': %s", versionID, err.Error()))
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
	modules := map[string]any{}
	for key, module := range m.Modules {
		modules[key] = module
	}
	versionManifest.Modules = modules

	return &versionManifest, nil
}

// Patch is the resolver for the patch field.
func (r *versionResolver) Patch(ctx context.Context, obj *restModel.APIVersion) (*restModel.APIPatch, error) {
	if !evergreen.IsPatchRequester(utility.FromStringPtr(obj.Requester)) {
		return nil, nil
	}
	patchID := utility.FromStringPtr(obj.Id)
	apiPatch, err := data.FindPatchById(ctx, patchID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding patch '%s': %s", patchID, err.Error()))
	}
	return apiPatch, nil
}

// PreviousVersion is the resolver for the previousVersion field.
func (r *versionResolver) PreviousVersion(ctx context.Context, obj *restModel.APIVersion) (*restModel.APIVersion, error) {
	if !obj.IsPatchRequester() {
		previousVersion, err := model.VersionFindOne(ctx, model.VersionByProjectIdAndOrder(utility.FromStringPtr(obj.Project), obj.Order-1))
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding previous version for version '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
		}
		if previousVersion == nil {
			return nil, nil
		}
		apiVersion := restModel.APIVersion{}
		apiVersion.BuildFromService(ctx, *previousVersion)
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
	versionID := utility.FromStringPtr(obj.Id)
	v, err := model.VersionFindOneId(ctx, versionID)
	if err != nil {
		return "", InternalServerError.Send(ctx, fmt.Sprintf("fetching version '%s': %s", versionID, err.Error()))
	}
	if v == nil {
		return "", ResourceNotFound.Send(ctx, fmt.Sprintf("version '%s' not found", versionID))
	}
	return getDisplayStatus(ctx, v)
}

// TaskCount is the resolver for the taskCount field.
func (r *versionResolver) TaskCount(ctx context.Context, obj *restModel.APIVersion, options *TaskCountOptions) (*int, error) {
	versionID := utility.FromStringPtr(obj.Id)
	// if includeNeverActivatedTasks is nil, we default to using the value of the requester
	includeNeverActivatedTasks := utility.ToBoolPtr(!obj.IsPatchRequester())
	if options != nil && options.IncludeNeverActivatedTasks != nil {
		includeNeverActivatedTasks = options.IncludeNeverActivatedTasks
	}
	taskCount, err := task.Count(ctx, db.Query(task.DisplayTasksByVersion(versionID, utility.FromBoolPtr(includeNeverActivatedTasks))))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting task count for version '%s': %s", versionID, err.Error()))
	}
	return &taskCount, nil
}

// Tasks is the resolver for the tasks field.
func (r *versionResolver) Tasks(ctx context.Context, obj *restModel.APIVersion, options TaskFilterOptions) (*VersionTasks, error) {
	versionID := utility.FromStringPtr(obj.Id)
	includeNeverActivatedTasks := options.IncludeNeverActivatedTasks
	if includeNeverActivatedTasks == nil {
		includeNeverActivatedTasks = utility.ToBoolPtr(false)
	}
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
				return nil, InputValidationError.Send(ctx, fmt.Sprintf("invalid sort key '%s'", singleSort.Key))
			}
			order := 1
			if singleSort.Direction == SortDirectionDesc {
				order = -1
			}
			taskSorts = append(taskSorts, task.TasksSortOrder{Key: key, Order: order})
		}
	}
	baseVersionID := ""
	baseVersion, err := model.FindBaseVersionForVersion(ctx, utility.FromStringPtr(obj.Id))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding base version for version '%s': %s", versionID, err.Error()))
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
		IncludeNeverActivatedTasks: *includeNeverActivatedTasks || !evergreen.IsPatchRequester(utility.FromStringPtr(obj.Requester)),
		BaseVersionID:              baseVersionID,
	}
	tasks, count, err := task.GetTasksByVersion(ctx, versionID, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting tasks for version '%s': %s", versionID, err.Error()))
	}

	var apiTasks []*restModel.APITask
	for _, t := range tasks {
		apiTask := restModel.APITask{}
		err := apiTask.BuildFromService(ctx, &t, nil)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to APITask: %s", t.Id, err.Error()))
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
	versionID := utility.FromStringPtr(obj.Id)
	statuses, err := task.GetTaskStatusesByVersion(ctx, versionID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting task statuses for version '%s': %s", versionID, err.Error()))
	}
	return statuses, nil
}

// TaskStatusStats is the resolver for the taskStatusStats field.
func (r *versionResolver) TaskStatusStats(ctx context.Context, obj *restModel.APIVersion, options BuildVariantOptions) (*task.TaskStats, error) {
	includeNeverActivatedTasks := options.IncludeNeverActivatedTasks
	if includeNeverActivatedTasks == nil {
		includeNeverActivatedTasks = utility.ToBoolPtr(false)
	}
	opts := task.GetTasksByVersionOptions{
		IncludeExecutionTasks: false,
		TaskNames:             options.Tasks,
		Variants:              options.Variants,
		Statuses:              getValidTaskStatusesFilter(options.Statuses),
		// If the version is a patch, we don't want to include its never activated tasks.
		IncludeNeverActivatedTasks: *includeNeverActivatedTasks || !obj.IsPatchRequester(),
	}

	versionID := utility.FromStringPtr(obj.Id)
	stats, err := task.GetTaskStatsByVersion(ctx, versionID, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting task status stats for version '%s': %s", versionID, err.Error()))
	}
	return stats, nil
}

// UpstreamProject is the resolver for the upstreamProject field.
func (r *versionResolver) UpstreamProject(ctx context.Context, obj *restModel.APIVersion) (*UpstreamProject, error) {
	if utility.FromStringPtr(obj.Requester) != evergreen.TriggerRequester {
		return nil, nil
	}

	versionID := utility.FromStringPtr(obj.Id)
	v, err := model.VersionFindOneId(ctx, versionID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching version '%s': %s", versionID, err.Error()))
	}
	if v == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("version '%s' not found", versionID))
	}
	if v.TriggerID == "" || v.TriggerType == "" {
		return nil, nil
	}

	var projectID string
	var upstreamProject *UpstreamProject
	if v.TriggerType == model.ProjectTriggerLevelTask {
		upstreamTask, err := task.FindOneId(ctx, v.TriggerID)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching upstream task '%s': %s", v.TriggerID, err.Error()))
		}
		if upstreamTask == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("upstream task '%s' not found", v.TriggerID))
		}

		apiTask := restModel.APITask{}
		if err = apiTask.BuildFromService(ctx, upstreamTask, nil); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to APITask: %s", upstreamTask.Id, err.Error()))
		}

		projectID = upstreamTask.Project
		upstreamProject = &UpstreamProject{
			Revision: upstreamTask.Revision,
			Task:     &apiTask,
		}
	} else if v.TriggerType == model.ProjectTriggerLevelBuild {
		upstreamBuild, err := build.FindOneId(ctx, v.TriggerID)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching upstream build '%s': %s", v.TriggerID, err.Error()))
		}
		if upstreamBuild == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("upstream build '%s' not found", v.TriggerID))
		}

		upstreamVersion, err := model.VersionFindOneIdWithBuildVariants(ctx, upstreamBuild.Version)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching upstream version '%s': %s", upstreamBuild.Version, err.Error()))
		}
		if upstreamVersion == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("upstream version '%s' not found", upstreamBuild.Version))
		}

		apiVersion := restModel.APIVersion{}
		apiVersion.BuildFromService(ctx, *upstreamVersion)

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
	upstreamProjectRef, err := model.FindBranchProjectRef(ctx, projectID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching upstream project '%s': %s", projectID, err.Error()))
	}
	if upstreamProjectRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("upstream project '%s' not found", projectID))
	}

	upstreamProject.Owner = upstreamProjectRef.Owner
	upstreamProject.Repo = upstreamProjectRef.Repo
	upstreamProject.Project = upstreamProjectRef.Identifier
	upstreamProject.TriggerID = v.TriggerID
	upstreamProject.TriggerType = v.TriggerType
	return upstreamProject, nil
}

// User is the resolver for the user field.
func (r *versionResolver) User(ctx context.Context, obj *restModel.APIVersion) (*restModel.APIDBUser, error) {
	authorId := utility.FromStringPtr(obj.Author)
	currentUser := mustHaveUser(ctx)
	if currentUser.Id == authorId {
		apiUser := &restModel.APIDBUser{}
		apiUser.BuildFromService(*currentUser)
		return apiUser, nil
	}

	author, err := user.FindOneByIdContext(ctx, authorId)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting user '%s': %s", authorId, err.Error()))
	}
	// This is most likely a reaped user, so just return their ID
	if author == nil {
		return &restModel.APIDBUser{
			UserID: utility.ToStringPtr(authorId),
		}, nil
	}

	apiUser := &restModel.APIDBUser{}
	apiUser.BuildFromService(*author)
	return apiUser, nil
}

// VersionTiming is the resolver for the versionTiming field.
func (r *versionResolver) VersionTiming(ctx context.Context, obj *restModel.APIVersion) (*VersionTiming, error) {
	versionID := utility.FromStringPtr(obj.Id)
	v, err := model.VersionFindOneId(ctx, versionID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching version '%s': %s", versionID, err.Error()))
	}
	if v == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("version '%s' not found", versionID))
	}
	timeTaken, makespan, err := v.GetTimeSpent(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting timing for version '%s': %s", versionID, err.Error()))
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
	versionID := utility.FromStringPtr(obj.Id)
	v, err := model.VersionFindOneId(ctx, versionID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching version '%s': %s", versionID, err.Error()))
	}
	if v == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("version '%s' not found", versionID))
	}
	return v.Warnings, nil
}

// WaterfallBuilds is the resolver for the waterfallBuilds field.
func (r *versionResolver) WaterfallBuilds(ctx context.Context, obj *restModel.APIVersion) ([]*model.WaterfallBuild, error) {
	versionID := utility.FromStringPtr(obj.Id)

	// No need to fetch build variants for unactivated versions
	if !utility.FromBoolPtr(obj.Activated) {
		return nil, nil
	}

	parentWaterfall, ok := graphql.GetFieldContext(ctx).Parent.Parent.Parent.Result.(*Waterfall)
	if ok {
		// If we can't find the activeVersionIds in the parent query, eagerly continue with this aggregation.
		activeVersionIds := parentWaterfall.Pagination.ActiveVersionIds
		if !utility.StringSliceContains(activeVersionIds, versionID) {
			return nil, nil
		}
	}

	builds, err := model.GetVersionBuilds(ctx, versionID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting build variants for version '%s': %s", versionID, err.Error()))
	}
	versionBuilds := []*model.WaterfallBuild{}
	for _, b := range builds {
		bCopy := b
		versionBuilds = append(versionBuilds, &bCopy)
	}
	return versionBuilds, nil
}

// Version returns VersionResolver implementation.
func (r *Resolver) Version() VersionResolver { return &versionResolver{r} }

type versionResolver struct{ *Resolver }
