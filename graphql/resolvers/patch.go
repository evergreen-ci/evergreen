package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	gqlError "github.com/evergreen-ci/evergreen/graphql/errors"
	"github.com/evergreen-ci/evergreen/graphql/generated"
	gqlModel "github.com/evergreen-ci/evergreen/graphql/model"
	"github.com/evergreen-ci/evergreen/graphql/resolvers/util"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	adb "github.com/mongodb/anser/db"
	werrors "github.com/pkg/errors"
)

func (r *mutationResolver) EnqueuePatch(ctx context.Context, patchID string, commitMessage *string) (*restModel.APIPatch, error) {
	user := util.MustHaveUser(ctx)
	existingPatch, err := data.FindPatchById(patchID)
	if err != nil {
		gimletErr, ok := err.(gimlet.ErrorResponse)
		if ok {
			return nil, util.MapHTTPStatusToGqlError(ctx, gimletErr.StatusCode, err)
		}
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error getting patch '%s'", patchID))
	}

	if !util.HasEnqueuePatchPermission(user, existingPatch) {
		return nil, gqlError.Forbidden.Send(ctx, "can't enqueue another user's patch")
	}

	if commitMessage == nil {
		commitMessage = existingPatch.Description
	}

	newPatch, err := data.CreatePatchForMerge(ctx, patchID, utility.FromStringPtr(commitMessage))
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error creating new patch: %s", err.Error()))
	}
	item := restModel.APICommitQueueItem{
		Issue:   newPatch.Id,
		PatchId: newPatch.Id,
		Source:  utility.ToStringPtr(commitqueue.SourceDiff)}
	_, err = data.EnqueueItem(utility.FromStringPtr(newPatch.ProjectId), item, false)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error enqueuing new patch: %s", err.Error()))
	}
	return newPatch, nil
}

func (r *mutationResolver) SchedulePatch(ctx context.Context, patchID string, configure gqlModel.PatchConfigure) (*restModel.APIPatch, error) {
	patchUpdateReq := util.BuildFromGqlInput(configure)
	version, err := model.VersionFindOneId(patchID)
	if err != nil && !adb.ResultsNotFound(err) {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error occurred fetching patch `%s`: %s", patchID, err.Error()))
	}
	statusCode, err := units.SchedulePatch(ctx, patchID, version, patchUpdateReq)
	if err != nil {
		return nil, util.MapHTTPStatusToGqlError(ctx, statusCode, werrors.Errorf("Error scheduling patch `%s`: %s", patchID, err.Error()))
	}
	scheduledPatch, err := data.FindPatchById(patchID)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting scheduled patch `%s`: %s", patchID, err))
	}
	return scheduledPatch, nil
}

func (r *mutationResolver) SchedulePatchTasks(ctx context.Context, patchID string) (*string, error) {
	modifications := model.VersionModification{
		Action: evergreen.SetActiveAction,
		Active: true,
		Abort:  false,
	}
	err := util.ModifyVersionHandler(ctx, patchID, modifications)
	if err != nil {
		return nil, err
	}
	return &patchID, nil
}

func (r *mutationResolver) ScheduleUndispatchedBaseTasks(ctx context.Context, patchID string) ([]*restModel.APITask, error) {
	opts := task.GetTasksByVersionOptions{
		Statuses:                       evergreen.TaskFailureStatuses,
		IncludeExecutionTasks:          true,
		IncludeBaseTasks:               false,
		IncludeBuildVariantDisplayName: false,
	}
	tasks, _, err := task.GetTasksByVersion(patchID, opts)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Could not fetch tasks for patch: %s ", err.Error()))
	}

	scheduledTasks := []*restModel.APITask{}
	tasksToSchedule := make(map[string]bool)

	for _, t := range tasks {
		// If a task is a generated task don't schedule it until we get all of the generated tasks we want to generate
		if t.GeneratedBy == "" {
			// We can ignore an error while fetching tasks because this could just mean the task didn't exist on the base commit.
			baseTask, _ := t.FindTaskOnBaseCommit()
			if baseTask != nil && baseTask.Status == evergreen.TaskUndispatched {
				tasksToSchedule[baseTask.Id] = true
			}
			// If a task is generated lets find its base task if it exists otherwise we need to generate it
		} else if t.GeneratedBy != "" {
			baseTask, _ := t.FindTaskOnBaseCommit()
			// If the task is undispatched or doesn't exist on the base commit then we want to schedule
			if baseTask == nil {
				generatorTask, err := task.FindByIdExecution(t.GeneratedBy, nil)
				if err != nil {
					return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Experienced an error trying to find the generator task: %s", err.Error()))
				}
				if generatorTask != nil {
					baseGeneratorTask, _ := generatorTask.FindTaskOnBaseCommit()
					// If baseGeneratorTask is nil then it didn't exist on the base task and we can't do anything
					if baseGeneratorTask != nil && baseGeneratorTask.Status == evergreen.TaskUndispatched {
						err = baseGeneratorTask.SetGeneratedTasksToActivate(t.BuildVariant, t.DisplayName)
						if err != nil {
							return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Could not activate generated task: %s", err.Error()))
						}
						tasksToSchedule[baseGeneratorTask.Id] = true

					}
				}
			} else if baseTask.Status == evergreen.TaskUndispatched {
				tasksToSchedule[baseTask.Id] = true
			}

		}
	}

	taskIDs := []string{}
	for taskId := range tasksToSchedule {
		taskIDs = append(taskIDs, taskId)
	}
	scheduled, err := util.SetManyTasksScheduled(ctx, r.sc.GetURL(), true, taskIDs...)
	if err != nil {
		return nil, err
	}
	scheduledTasks = append(scheduledTasks, scheduled...)
	// sort scheduledTasks by display name to guarantee the order of the tasks
	sort.Slice(scheduledTasks, func(i, j int) bool {
		return utility.FromStringPtr(scheduledTasks[i].DisplayName) < utility.FromStringPtr(scheduledTasks[j].DisplayName)
	})

	return scheduledTasks, nil
}

func (r *mutationResolver) SetPatchPriority(ctx context.Context, patchID string, priority int) (*string, error) {
	modifications := model.VersionModification{
		Action:   evergreen.SetPriorityAction,
		Priority: int64(priority),
	}
	err := util.ModifyVersionHandler(ctx, patchID, modifications)
	if err != nil {
		return nil, err
	}
	return &patchID, nil
}

func (r *mutationResolver) UnschedulePatchTasks(ctx context.Context, patchID string, abort bool) (*string, error) {
	modifications := model.VersionModification{
		Action: evergreen.SetActiveAction,
		Active: false,
		Abort:  abort,
	}
	err := util.ModifyVersionHandler(ctx, patchID, modifications)
	if err != nil {
		return nil, err
	}
	return &patchID, nil
}

func (r *patchResolver) AuthorDisplayName(ctx context.Context, obj *restModel.APIPatch) (string, error) {
	usr, err := user.FindOneById(*obj.Author)
	if err != nil {
		return "", gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Error getting user from user ID: %s", err.Error()))
	}
	if usr == nil {
		return "", gqlError.ResourceNotFound.Send(ctx, "Could not find user from user ID")
	}
	return usr.DisplayName(), nil
}

func (r *patchResolver) BaseTaskStatuses(ctx context.Context, obj *restModel.APIPatch) ([]string, error) {
	baseTasks, err := util.GetVersionBaseTasks(*obj.Id)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting version base tasks: %s", err.Error()))
	}
	return util.GetAllTaskStatuses(baseTasks), nil
}

func (r *patchResolver) BaseVersionID(ctx context.Context, obj *restModel.APIPatch) (*string, error) {
	baseVersion, err := model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(*obj.ProjectId, *obj.Githash).Project(bson.M{model.VersionIdentifierKey: 1}))
	if baseVersion == nil || err != nil {
		return nil, nil
	}
	return &baseVersion.Id, nil
}

func (r *patchResolver) Builds(ctx context.Context, obj *restModel.APIPatch) ([]*restModel.APIBuild, error) {
	builds, err := build.FindBuildsByVersions([]string{*obj.Version})
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding build by version %s: %s", *obj.Version, err.Error()))
	}
	var apiBuilds []*restModel.APIBuild
	for _, build := range builds {
		apiBuild := restModel.APIBuild{}
		err = apiBuild.BuildFromService(build)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building APIBuild from service: %s", err.Error()))
		}
		apiBuilds = append(apiBuilds, &apiBuild)
	}
	return apiBuilds, nil
}

func (r *patchResolver) CommitQueuePosition(ctx context.Context, obj *restModel.APIPatch) (*int, error) {
	var commitQueuePosition *int
	if *obj.Alias == evergreen.CommitQueueAlias {
		cq, err := commitqueue.FindOneId(*obj.ProjectId)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting commit queue position for patch %s: %s", *obj.Id, err.Error()))
		}
		if cq != nil {
			position := cq.FindItem(*obj.Id)
			commitQueuePosition = &position
		}
	}
	return commitQueuePosition, nil
}

func (r *patchResolver) Duration(ctx context.Context, obj *restModel.APIPatch) (*gqlModel.PatchDuration, error) {
	query := db.Query(task.ByVersion(*obj.Id)).WithFields(task.TimeTakenKey, task.StartTimeKey, task.FinishTimeKey, task.DisplayOnlyKey, task.ExecutionKey)
	tasks, err := task.FindAllFirstExecution(query)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, err.Error())
	}
	if tasks == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, "Could not find any tasks for patch")
	}
	timeTaken, makespan := task.GetTimeSpent(tasks)

	// return nil if rounded timeTaken/makespan == 0s
	t := timeTaken.Round(time.Second).String()
	var tPointer *string
	if t != "0s" {
		tFormated := util.FormatDuration(t)
		tPointer = &tFormated
	}
	m := makespan.Round(time.Second).String()
	var mPointer *string
	if m != "0s" {
		mFormated := util.FormatDuration(m)
		mPointer = &mFormated
	}

	return &gqlModel.PatchDuration{
		Makespan:  mPointer,
		TimeTaken: tPointer,
	}, nil
}

func (r *patchResolver) PatchTriggerAliases(ctx context.Context, obj *restModel.APIPatch) ([]*restModel.APIPatchTriggerDefinition, error) {
	projectRef, err := data.FindProjectById(*obj.ProjectId, true, true)
	if err != nil || projectRef == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find project: %s : %s", *obj.ProjectId, err))
	}

	if len(projectRef.PatchTriggerAliases) == 0 {
		return nil, nil
	}

	projectCache := map[string]*model.Project{}
	aliases := []*restModel.APIPatchTriggerDefinition{}
	for _, alias := range projectRef.PatchTriggerAliases {
		project, projectCached := projectCache[alias.ChildProject]
		if !projectCached {
			_, project, err = model.FindLatestVersionWithValidProject(alias.ChildProject)
			if err != nil {
				return nil, gqlError.InternalServerError.Send(ctx, werrors.Wrapf(err, "Problem getting last known project for '%s'", alias.ChildProject).Error())
			}
			projectCache[alias.ChildProject] = project
		}

		matchingTasks, err := project.VariantTasksForSelectors([]patch.PatchTriggerDefinition{alias}, *obj.Requester)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Problem matching tasks to alias definitions: %v", err.Error()))
		}

		variantsTasks := []restModel.VariantTask{}
		for _, vt := range matchingTasks {
			variantsTasks = append(variantsTasks, restModel.VariantTask{
				Name:  utility.ToStringPtr(vt.Variant),
				Tasks: utility.ToStringPtrSlice(vt.Tasks),
			})
		}

		identifier, err := model.GetIdentifierForProject(alias.ChildProject)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Problem getting child project identifier: %v", err.Error()))
		}

		aliases = append(aliases, &restModel.APIPatchTriggerDefinition{
			Alias:                  utility.ToStringPtr(alias.Alias),
			ChildProjectId:         utility.ToStringPtr(alias.ChildProject),
			ChildProjectIdentifier: utility.ToStringPtr(identifier),
			VariantsTasks:          variantsTasks,
		})
	}

	return aliases, nil
}

func (r *patchResolver) Project(ctx context.Context, obj *restModel.APIPatch) (*gqlModel.PatchProject, error) {
	patchProject, err := util.GetPatchProjectVariantsAndTasksForUI(ctx, obj)
	if err != nil {
		return nil, err
	}
	return patchProject, nil
}

func (r *patchResolver) TaskCount(ctx context.Context, obj *restModel.APIPatch) (*int, error) {
	taskCount, err := task.Count(db.Query(task.DisplayTasksByVersion(*obj.Id)))
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting task count for patch %s: %s", *obj.Id, err.Error()))
	}
	return &taskCount, nil
}

func (r *patchResolver) TaskStatuses(ctx context.Context, obj *restModel.APIPatch) ([]string, error) {
	defaultSort := []task.TasksSortOrder{
		{Key: task.DisplayNameKey, Order: 1},
	}
	opts := task.GetTasksByVersionOptions{
		Sorts:                          defaultSort,
		IncludeBaseTasks:               false,
		IncludeBuildVariantDisplayName: false,
	}
	tasks, _, err := task.GetTasksByVersion(*obj.Id, opts)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting version tasks: %s", err.Error()))
	}
	return util.GetAllTaskStatuses(tasks), nil
}

func (r *patchResolver) Time(ctx context.Context, obj *restModel.APIPatch) (*gqlModel.PatchTime, error) {
	usr := util.MustHaveUser(ctx)

	started, err := util.GetFormattedDate(obj.StartTime, usr.Settings.Timezone)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, err.Error())
	}
	finished, err := util.GetFormattedDate(obj.FinishTime, usr.Settings.Timezone)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, err.Error())
	}
	submittedAt, err := util.GetFormattedDate(obj.CreateTime, usr.Settings.Timezone)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, err.Error())
	}

	return &gqlModel.PatchTime{
		Started:     started,
		Finished:    finished,
		SubmittedAt: *submittedAt,
	}, nil
}

func (r *patchResolver) VersionFull(ctx context.Context, obj *restModel.APIPatch) (*restModel.APIVersion, error) {
	if utility.FromStringPtr(obj.Version) == "" {
		return nil, nil
	}
	v, err := model.VersionFindOneId(*obj.Version)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error while finding version with id: `%s`: %s", *obj.Version, err.Error()))
	}
	if v == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find version with id: `%s`", *obj.Version))
	}
	apiVersion := restModel.APIVersion{}
	if err = apiVersion.BuildFromService(v); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building APIVersion from service for `%s`: %s", *obj.Version, err.Error()))
	}
	return &apiVersion, nil
}

func (r *projectResolver) Patches(ctx context.Context, obj *restModel.APIProjectRef, patchesInput gqlModel.PatchesInput) (*gqlModel.Patches, error) {
	opts := patch.ByPatchNameStatusesCommitQueuePaginatedOptions{
		Project:         obj.Id,
		PatchName:       patchesInput.PatchName,
		Statuses:        patchesInput.Statuses,
		Page:            patchesInput.Page,
		Limit:           patchesInput.Limit,
		OnlyCommitQueue: patchesInput.OnlyCommitQueue,
	}

	patches, count, err := patch.ByPatchNameStatusesCommitQueuePaginated(opts)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error while fetching patches for this project : %s", err.Error()))
	}
	apiPatches := []*restModel.APIPatch{}
	for _, p := range patches {
		apiPatch := restModel.APIPatch{}
		err = apiPatch.BuildFromService(p)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("problem building APIPatch from service for patch: %s : %s", p.Id.Hex(), err.Error()))
		}
		apiPatches = append(apiPatches, &apiPatch)
	}
	return &gqlModel.Patches{Patches: apiPatches, FilteredPatchCount: count}, nil
}

func (r *queryResolver) Patch(ctx context.Context, id string) (*restModel.APIPatch, error) {
	patch, err := data.FindPatchById(id)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, err.Error())
	}

	if evergreen.IsFinishedPatchStatus(*patch.Status) {
		failedAndAbortedStatuses := append(evergreen.TaskFailureStatuses, evergreen.TaskAborted)
		opts := task.GetTasksByVersionOptions{
			Statuses:                       failedAndAbortedStatuses,
			FieldsToProject:                []string{task.DisplayStatusKey},
			IncludeBaseTasks:               false,
			IncludeBuildVariantDisplayName: false,
		}
		tasks, _, err := task.GetTasksByVersion(id, opts)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Could not fetch tasks for patch: %s ", err.Error()))
		}

		if len(patch.ChildPatches) > 0 {
			for _, cp := range patch.ChildPatches {
				// add the child patch tasks to tasks so that we can consider their status
				childPatchTasks, _, err := task.GetTasksByVersion(*cp.Id, opts)
				if err != nil {
					return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Could not fetch tasks for child patch: %s ", err.Error()))
				}
				tasks = append(tasks, childPatchTasks...)
			}
		}
		statuses := util.GetAllTaskStatuses(tasks)

		// If theres an aborted task we should set the patch status to aborted if there are no other failures
		if utility.StringSliceContains(statuses, evergreen.TaskAborted) {
			if len(utility.StringSliceIntersection(statuses, evergreen.TaskFailureStatuses)) == 0 {
				patch.Status = utility.ToStringPtr(evergreen.PatchAborted)
			}
		}
	}
	return patch, nil
}

func (r *queryResolver) PatchTasks(ctx context.Context, patchID string, sorts []*gqlModel.SortOrder, page *int, limit *int, statuses []string, baseStatuses []string, variant *string, taskName *string, includeEmptyActivation *bool) (*gqlModel.PatchTasks, error) {
	pageParam := 0
	if page != nil {
		pageParam = *page
	}
	limitParam := 0
	if limit != nil {
		limitParam = *limit
	}
	variantParam := ""
	if variant != nil {
		variantParam = *variant
	}
	taskNameParam := ""
	if taskName != nil {
		taskNameParam = *taskName
	}
	var taskSorts []task.TasksSortOrder
	if len(sorts) > 0 {
		taskSorts = []task.TasksSortOrder{}
		for _, singleSort := range sorts {
			key := ""
			switch singleSort.Key {
			// the keys here should be the keys for the column headers of the tasks table
			case gqlModel.TaskSortCategoryName:
				key = task.DisplayNameKey
			case gqlModel.TaskSortCategoryStatus:
				key = task.DisplayStatusKey
			case gqlModel.TaskSortCategoryBaseStatus:
				key = task.BaseTaskStatusKey
			case gqlModel.TaskSortCategoryVariant:
				key = task.BuildVariantKey
			case gqlModel.TaskSortCategoryDuration:
				key = task.TimeTakenKey
			default:
				return nil, gqlError.InputValidationError.Send(ctx, fmt.Sprintf("invalid sort key: %s", singleSort.Key))
			}
			order := 1
			if singleSort.Direction == gqlModel.SortDirectionDesc {
				order = -1
			}
			taskSorts = append(taskSorts, task.TasksSortOrder{Key: key, Order: order})
		}
	}
	v, err := model.VersionFindOne(model.VersionById(patchID).WithFields(model.VersionRequesterKey))
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error while finding version with id: `%s`: %s", patchID, err.Error()))
	}
	if v == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find version with id: `%s`", patchID))
	}

	opts := task.GetTasksByVersionOptions{
		Statuses:                       util.GetValidTaskStatusesFilter(statuses),
		BaseStatuses:                   util.GetValidTaskStatusesFilter(baseStatuses),
		Variants:                       []string{variantParam},
		TaskNames:                      []string{taskNameParam},
		Page:                           pageParam,
		Limit:                          limitParam,
		Sorts:                          taskSorts,
		IncludeBaseTasks:               true,
		IncludeEmptyActivation:         utility.FromBoolPtr(includeEmptyActivation),
		IncludeBuildVariantDisplayName: true,
		IsMainlineCommit:               !evergreen.IsPatchRequester(v.Requester),
	}
	tasks, count, err := task.GetTasksByVersion(patchID, opts)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting patch tasks for %s: %s", patchID, err.Error()))
	}

	var apiTasks []*restModel.APITask
	for _, t := range tasks {
		apiTask := restModel.APITask{}
		err := apiTask.BuildFromArgs(&t, nil)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error converting task item db model to api model: %v", err.Error()))
		}
		apiTasks = append(apiTasks, &apiTask)
	}
	patchTasks := gqlModel.PatchTasks{
		Count: count,
		Tasks: apiTasks,
	}
	return &patchTasks, nil
}

func (r *userResolver) Patches(ctx context.Context, obj *restModel.APIDBUser, patchesInput gqlModel.PatchesInput) (*gqlModel.Patches, error) {
	opts := patch.ByPatchNameStatusesCommitQueuePaginatedOptions{
		Author:             obj.UserID,
		PatchName:          patchesInput.PatchName,
		Statuses:           patchesInput.Statuses,
		Page:               patchesInput.Page,
		Limit:              patchesInput.Limit,
		IncludeCommitQueue: patchesInput.IncludeCommitQueue,
	}
	patches, count, err := patch.ByPatchNameStatusesCommitQueuePaginated(opts)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting patches for user %s: %s", utility.FromStringPtr(obj.UserID), err.Error()))
	}

	apiPatches := []*restModel.APIPatch{}
	for _, p := range patches {
		apiPatch := restModel.APIPatch{}
		if err = apiPatch.BuildFromService(p); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error converting patch to APIPatch for patch %s : %s", p.Id, err.Error()))
		}
		apiPatches = append(apiPatches, &apiPatch)
	}
	return &gqlModel.Patches{Patches: apiPatches, FilteredPatchCount: count}, nil
}

// Patch returns generated.PatchResolver implementation.
func (r *Resolver) Patch() generated.PatchResolver { return &patchResolver{r} }

type patchResolver struct{ *Resolver }
