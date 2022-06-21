package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	gqlError "github.com/evergreen-ci/evergreen/graphql/errors"
	"github.com/evergreen-ci/evergreen/graphql/generated"
	gqlModel "github.com/evergreen-ci/evergreen/graphql/model"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	werrors "github.com/pkg/errors"
)

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
	baseTasks, err := getVersionBaseTasks(*obj.Id)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting version base tasks: %s", err.Error()))
	}
	return getAllTaskStatuses(baseTasks), nil
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
		tFormated := formatDuration(t)
		tPointer = &tFormated
	}
	m := makespan.Round(time.Second).String()
	var mPointer *string
	if m != "0s" {
		mFormated := formatDuration(m)
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
	patchProject, err := getPatchProjectVariantsAndTasksForUI(ctx, obj)
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
	return getAllTaskStatuses(tasks), nil
}

func (r *patchResolver) Time(ctx context.Context, obj *restModel.APIPatch) (*gqlModel.PatchTime, error) {
	usr := mustHaveUser(ctx)

	started, err := getFormattedDate(obj.StartTime, usr.Settings.Timezone)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, err.Error())
	}
	finished, err := getFormattedDate(obj.FinishTime, usr.Settings.Timezone)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, err.Error())
	}
	submittedAt, err := getFormattedDate(obj.CreateTime, usr.Settings.Timezone)
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
