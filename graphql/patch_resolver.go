package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"go.mongodb.org/mongo-driver/bson"
)

// AuthorDisplayName is the resolver for the authorDisplayName field.
func (r *patchResolver) AuthorDisplayName(ctx context.Context, obj *restModel.APIPatch) (string, error) {
	usr, err := user.FindOneById(*obj.Author)
	if err != nil {
		return "", InternalServerError.Send(ctx, fmt.Sprintf("getting user from user ID: %s", err.Error()))
	}
	if usr == nil {
		return "", ResourceNotFound.Send(ctx, "Could not find user from user ID")
	}
	return usr.DisplayName(), nil
}

// BaseTaskStatuses is the resolver for the baseTaskStatuses field.
func (r *patchResolver) BaseTaskStatuses(ctx context.Context, obj *restModel.APIPatch) ([]string, error) {
	baseVersion, err := model.FindBaseVersionForVersion(*obj.Id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding base version for version '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
	}
	if baseVersion == nil {
		return nil, nil
	}
	statuses, err := task.GetBaseStatusesForActivatedTasks(ctx, *obj.Id, baseVersion.Id)
	if err != nil {
		return nil, nil
	}
	return statuses, nil
}

// Builds is the resolver for the builds field.
func (r *patchResolver) Builds(ctx context.Context, obj *restModel.APIPatch) ([]*restModel.APIBuild, error) {
	builds, err := build.FindBuildsByVersions([]string{*obj.Version})
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding build by version '%s': %s", utility.FromStringPtr(obj.Version), err.Error()))
	}
	var apiBuilds []*restModel.APIBuild
	for _, build := range builds {
		apiBuild := restModel.APIBuild{}
		apiBuild.BuildFromService(build, nil)
		apiBuilds = append(apiBuilds, &apiBuild)
	}
	return apiBuilds, nil
}

// Duration is the resolver for the duration field.
func (r *patchResolver) Duration(ctx context.Context, obj *restModel.APIPatch) (*PatchDuration, error) {
	query := db.Query(task.ByVersion(*obj.Id)).WithFields(task.TimeTakenKey, task.StartTimeKey, task.FinishTimeKey, task.DisplayOnlyKey, task.ExecutionKey)
	tasks, err := task.FindAllFirstExecution(query)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	if tasks == nil {
		return nil, ResourceNotFound.Send(ctx, "Could not find any tasks for patch")
	}
	timeTaken, makespan := task.GetFormattedTimeSpent(tasks)

	return makePatchDuration(timeTaken, makespan), nil
}

// GeneratedTaskCounts is the resolver for the generatedTaskCounts field.
func (r *patchResolver) GeneratedTaskCounts(ctx context.Context, obj *restModel.APIPatch) ([]*GeneratedTaskCountResults, error) {
	patchID := utility.FromStringPtr(obj.Id)
	p, err := patch.FindOneId(patchID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding patch with id '%s': %s", patchID, err.Error()))
	}
	if p == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("patch '%s' does not exist", patchID))
	}
	proj, _, err := model.FindAndTranslateProjectForPatch(ctx, evergreen.GetEnvironment().Settings(), p)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding project config for patch '%s': %s", patchID, err.Error()))
	}

	generatorTasks := proj.TasksThatCallCommand(evergreen.GenerateTasksCommandName)

	patchProjectVariantsAndTasks, err := model.GetVariantsAndTasksFromPatchProject(ctx, evergreen.GetEnvironment().Settings(), p)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting project variants and tasks for patch '%s': %s", p.Id.Hex(), err.Error()))
	}
	var res []*GeneratedTaskCountResults
	for _, buildVariant := range patchProjectVariantsAndTasks.Variants {
		for _, taskUnit := range buildVariant.Tasks {
			if _, ok := generatorTasks[taskUnit.Name]; ok {
				dbTask, err := task.FindOne(db.Query(bson.M{
					task.ProjectKey:      proj.DisplayName,
					task.BuildVariantKey: buildVariant.Name,
					task.DisplayNameKey:  taskUnit.Name,
					task.GenerateTaskKey: true,
				}).Sort([]string{"-" + task.FinishTimeKey}))
				if err != nil {
					return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting task with variant '%s' and name '%s': %s", buildVariant.Name, taskUnit.Name, err.Error()))
				}
				if dbTask != nil {
					res = append(res, &GeneratedTaskCountResults{
						BuildVariantName: utility.ToStringPtr(buildVariant.Name),
						TaskName:         utility.ToStringPtr(taskUnit.Name),
						EstimatedTasks:   utility.FromIntPtr(dbTask.EstimatedNumActivatedGeneratedTasks),
					})
				}
			}
		}
	}
	return res, nil
}

// PatchTriggerAliases is the resolver for the patchTriggerAliases field.
func (r *patchResolver) PatchTriggerAliases(ctx context.Context, obj *restModel.APIPatch) ([]*restModel.APIPatchTriggerDefinition, error) {
	projectRef, err := data.FindProjectById(*obj.ProjectId, true, true)
	if err != nil || projectRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find project: %s : %s", *obj.ProjectId, err.Error()))
	}

	if len(projectRef.PatchTriggerAliases) == 0 {
		return nil, nil
	}

	projectCache := map[string]*model.Project{}
	aliases := []*restModel.APIPatchTriggerDefinition{}
	for _, alias := range projectRef.PatchTriggerAliases {
		project, projectCached := projectCache[alias.ChildProject]
		if !projectCached {
			_, project, _, err = model.FindLatestVersionWithValidProject(alias.ChildProject, false)
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting last known project for '%s': %s", alias.ChildProject, err.Error()))
			}
			projectCache[alias.ChildProject] = project
		}

		matchingTasks, err := project.VariantTasksForSelectors([]patch.PatchTriggerDefinition{alias}, *obj.Requester)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Problem matching tasks to alias definitions: %v", err.Error()))
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
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Problem getting child project identifier: %v", err.Error()))
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

// Project is the resolver for the project field.
func (r *patchResolver) Project(ctx context.Context, obj *restModel.APIPatch) (*PatchProject, error) {
	patchProject, err := getPatchProjectVariantsAndTasksForUI(ctx, obj)
	if err != nil {
		return nil, err
	}
	return patchProject, nil
}

// ProjectIdentifier is the resolver for the projectIdentifier field.
func (r *patchResolver) ProjectIdentifier(ctx context.Context, obj *restModel.APIPatch) (string, error) {
	obj.GetIdentifier()
	return utility.FromStringPtr(obj.ProjectIdentifier), nil
}

// ProjectMetadata is the resolver for the projectMetadata field.
func (r *patchResolver) ProjectMetadata(ctx context.Context, obj *restModel.APIPatch) (*restModel.APIProjectRef, error) {
	apiProjectRef, err := getProjectMetadata(ctx, obj.ProjectId, obj.Id)
	return apiProjectRef, err
}

// TaskCount is the resolver for the taskCount field.
func (r *patchResolver) TaskCount(ctx context.Context, obj *restModel.APIPatch) (*int, error) {
	taskCount, err := task.Count(db.Query(task.DisplayTasksByVersion(*obj.Id, false)))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting task count for patch '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
	}
	return &taskCount, nil
}

// TaskStatuses is the resolver for the taskStatuses field.
func (r *patchResolver) TaskStatuses(ctx context.Context, obj *restModel.APIPatch) ([]string, error) {
	statuses, err := task.GetTaskStatusesByVersion(ctx, utility.FromStringPtr(obj.Id))
	if err != nil {
		return nil, nil
	}
	return statuses, nil
}

// Time is the resolver for the time field.
func (r *patchResolver) Time(ctx context.Context, obj *restModel.APIPatch) (*PatchTime, error) {
	usr := mustHaveUser(ctx)

	started, err := getFormattedDate(obj.StartTime, usr.Settings.Timezone)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	finished, err := getFormattedDate(obj.FinishTime, usr.Settings.Timezone)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	submittedAt, err := getFormattedDate(obj.CreateTime, usr.Settings.Timezone)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}

	return &PatchTime{
		Started:     started,
		Finished:    finished,
		SubmittedAt: *submittedAt,
	}, nil
}

// VersionFull is the resolver for the versionFull field.
func (r *patchResolver) VersionFull(ctx context.Context, obj *restModel.APIPatch) (*restModel.APIVersion, error) {
	if utility.FromStringPtr(obj.Version) == "" {
		return nil, nil
	}
	v, err := model.VersionFindOneId(*obj.Version)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("while finding version with id '%s': %s", utility.FromStringPtr(obj.Version), err.Error()))
	}
	if v == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find version with id: `%s`", *obj.Version))
	}
	apiVersion := restModel.APIVersion{}
	apiVersion.BuildFromService(*v)
	return &apiVersion, nil
}

// Patch returns PatchResolver implementation.
func (r *Resolver) Patch() PatchResolver { return &patchResolver{r} }

type patchResolver struct{ *Resolver }
