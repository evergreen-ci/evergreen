package graphql

import (
	"context"
	"fmt"
	"strings"

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
	author := utility.FromStringPtr(obj.Author)
	usr, err := user.FindOneByIdContext(ctx, author)
	if err != nil {
		return "", InternalServerError.Send(ctx, fmt.Sprintf("getting user corresponding to author '%s': %s", author, err.Error()))
	}
	if usr == nil {
		return "", ResourceNotFound.Send(ctx, fmt.Sprintf("user corresponding to author '%s' not found", author))
	}
	return usr.DisplayName(), nil
}

// BaseTaskStatuses is the resolver for the baseTaskStatuses field.
func (r *patchResolver) BaseTaskStatuses(ctx context.Context, obj *restModel.APIPatch) ([]string, error) {
	versionID := utility.FromStringPtr(obj.Id)
	baseVersion, err := model.FindBaseVersionForVersion(ctx, versionID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching base version for version '%s': %s", versionID, err.Error()))
	}
	if baseVersion == nil {
		return nil, nil
	}
	statuses, err := task.GetBaseStatusesForActivatedTasks(ctx, versionID, baseVersion.Id)
	if err != nil {
		return nil, nil
	}
	return statuses, nil
}

// Builds is the resolver for the builds field.
func (r *patchResolver) Builds(ctx context.Context, obj *restModel.APIPatch) ([]*restModel.APIBuild, error) {
	versionID := utility.FromStringPtr(obj.Version)
	builds, err := build.FindBuildsByVersions(ctx, []string{versionID})
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching builds for version '%s': %s", versionID, err.Error()))
	}
	var apiBuilds []*restModel.APIBuild
	for _, build := range builds {
		apiBuild := restModel.APIBuild{}
		apiBuild.BuildFromService(ctx, build, nil)
		apiBuilds = append(apiBuilds, &apiBuild)
	}
	return apiBuilds, nil
}

// Duration is the resolver for the duration field.
func (r *patchResolver) Duration(ctx context.Context, obj *restModel.APIPatch) (*PatchDuration, error) {
	patchID := utility.FromStringPtr(obj.Id)
	query := db.Query(task.ByVersion(patchID)).WithFields(task.TimeTakenKey, task.StartTimeKey, task.FinishTimeKey, task.DisplayOnlyKey, task.ExecutionKey)
	tasks, err := task.FindAllFirstExecution(ctx, query)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	if tasks == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("no tasks for patch '%s' found", patchID))
	}
	timeTaken, makespan := task.GetFormattedTimeSpent(tasks)

	return makePatchDuration(timeTaken, makespan), nil
}

// GeneratedTaskCounts is the resolver for the generatedTaskCounts field.
func (r *patchResolver) GeneratedTaskCounts(ctx context.Context, obj *restModel.APIPatch) ([]*GeneratedTaskCountResults, error) {
	patchID := utility.FromStringPtr(obj.Id)
	p, err := patch.FindOneId(ctx, patchID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching patch '%s': %s", patchID, err.Error()))
	}
	if p == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("patch '%s' not found", patchID))
	}
	proj, _, err := model.FindAndTranslateProjectForPatch(ctx, evergreen.GetEnvironment().Settings(), p)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project config for patch '%s': %s", patchID, err.Error()))
	}

	generatorTasks := proj.TasksThatCallCommand(evergreen.GenerateTasksCommandName)

	patchProjectVariantsAndTasks, err := model.GetVariantsAndTasksFromPatchProject(ctx, evergreen.GetEnvironment().Settings(), p)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project variants and tasks for patch '%s': %s", p.Id.Hex(), err.Error()))
	}
	var res []*GeneratedTaskCountResults
	type taskQueryKey struct {
		Project      string
		BuildVariant string
		DisplayName  string
	}
	var queryKeys []taskQueryKey
	for _, buildVariant := range patchProjectVariantsAndTasks.Variants {
		for _, taskUnit := range buildVariant.Tasks {
			if _, ok := generatorTasks[taskUnit.Name]; ok {
				queryKeys = append(queryKeys, taskQueryKey{
					Project:      p.Project,
					BuildVariant: buildVariant.Name,
					DisplayName:  taskUnit.Name,
				})
			}
		}
	}
	if len(queryKeys) == 0 {
		return res, nil
	}

	// Batch fetch all relevant tasks in one query
	var orQueries []bson.M
	for _, k := range queryKeys {
		orQueries = append(orQueries, bson.M{
			task.ProjectKey:      k.Project,
			task.BuildVariantKey: k.BuildVariant,
			task.DisplayNameKey:  k.DisplayName,
			task.StatusKey:       evergreen.TaskSucceeded,
		})
	}
	query := db.Query(bson.M{"$or": orQueries}).Sort([]string{"-" + task.FinishTimeKey})
	dbTasks, err := task.FindAll(ctx, query)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting generated tasks: %s", err.Error()))
	}

	// Use map for quick lookup of tasks by composite key, favoring most recent (sorted query)
	taskMap := make(map[taskQueryKey]*task.Task)
	for _, t := range dbTasks {
		k := taskQueryKey{
			Project:      t.Project,
			BuildVariant: t.BuildVariant,
			DisplayName:  t.DisplayName,
		}
		if _, exists := taskMap[k]; !exists {
			taskMap[k] = &t
		}
	}

	for _, k := range queryKeys {
		if dbTask, ok := taskMap[k]; ok {
			res = append(res, &GeneratedTaskCountResults{
				BuildVariantName: utility.ToStringPtr(k.BuildVariant),
				TaskName:         utility.ToStringPtr(k.DisplayName),
				EstimatedTasks:   utility.FromIntPtr(dbTask.EstimatedNumActivatedGeneratedTasks),
			})
		}
	}
	return res, nil
}

// Parameters is the resolver for the parameters field.
func (r *patchResolver) Parameters(ctx context.Context, obj *restModel.APIPatch) ([]*restModel.APIParameter, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}

	projectId := utility.FromStringPtr(obj.ProjectId)
	projVars, err := model.FindMergedProjectVars(ctx, projectId)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting project vars for project '%s': %s", projectId, err.Error()))
	}

	redactKeys := config.LoggerConfig.RedactKeys
	var res []*restModel.APIParameter
	for _, param := range obj.Parameters {
		redactedParam := &restModel.APIParameter{
			Key:   param.Key,
			Value: param.Value,
		}
		for _, pattern := range redactKeys {
			if strings.Contains(strings.ToLower(utility.FromStringPtr(param.Key)), pattern) {
				redactedParam.Value = utility.ToStringPtr(evergreen.RedactedValue)
				break
			}
		}
		if projVars != nil {
			for varKey, varValue := range projVars.Vars {
				if strings.Contains(utility.FromStringPtr(param.Value), varValue) && projVars.PrivateVars[varKey] {
					redactedParam.Value = utility.ToStringPtr(evergreen.RedactedValue)
					break
				}
			}
		}
		res = append(res, redactedParam)
	}
	return res, nil
}

// PatchTriggerAliases is the resolver for the patchTriggerAliases field.
func (r *patchResolver) PatchTriggerAliases(ctx context.Context, obj *restModel.APIPatch) ([]*restModel.APIPatchTriggerDefinition, error) {
	projectID := utility.FromStringPtr(obj.ProjectId)
	projectRef, err := data.FindProjectById(ctx, projectID, true, true)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", projectID, err.Error()))
	}
	if projectRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", projectID))
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
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting last known child project '%s' for alias '%s': %s", alias.ChildProject, alias.Alias, err.Error()))
			}
			projectCache[alias.ChildProject] = project
		}

		matchingTasks, err := project.VariantTasksForSelectors(ctx, []patch.PatchTriggerDefinition{alias}, utility.FromStringPtr(obj.Requester))
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("matching tasks to definitions for alias '%s': %s", alias.Alias, err.Error()))
		}

		variantsTasks := []restModel.VariantTask{}
		for _, vt := range matchingTasks {
			variantsTasks = append(variantsTasks, restModel.VariantTask{
				Name:  utility.ToStringPtr(vt.Variant),
				Tasks: utility.ToStringPtrSlice(vt.Tasks),
			})
		}

		identifier, err := model.GetIdentifierForProject(ctx, alias.ChildProject)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting project identifier for child project '%s' in alias '%s': %s", alias.ChildProject, alias.Alias, err.Error()))
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
	obj.GetIdentifier(ctx)
	return utility.FromStringPtr(obj.ProjectIdentifier), nil
}

// ProjectMetadata is the resolver for the projectMetadata field.
func (r *patchResolver) ProjectMetadata(ctx context.Context, obj *restModel.APIPatch) (*restModel.APIProjectRef, error) {
	apiProjectRef, err := getProjectMetadata(ctx, obj.ProjectId, obj.Id)
	return apiProjectRef, err
}

// TaskCount is the resolver for the taskCount field.
func (r *patchResolver) TaskCount(ctx context.Context, obj *restModel.APIPatch) (*int, error) {
	patchID := utility.FromStringPtr(obj.Id)
	taskCount, err := task.Count(ctx, db.Query(task.DisplayTasksByVersion(patchID, false)))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting task count for patch '%s': %s", patchID, err.Error()))
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
	versionID := utility.FromStringPtr(obj.Version)
	if versionID == "" {
		return nil, nil
	}
	v, err := model.VersionFindOneIdWithBuildVariants(ctx, versionID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching version '%s': %s", versionID, err.Error()))
	}
	if v == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("version '%s' not found", versionID))
	}
	apiVersion := restModel.APIVersion{}
	apiVersion.BuildFromService(ctx, *v)
	return &apiVersion, nil
}

// IncludedLocalModules is the resolver for the includedLocalModules field.
func (r *patchResolver) IncludedLocalModules(ctx context.Context, obj *restModel.APIPatch) ([]*restModel.APILocalModuleInclude, error) {
	// Convert []APILocalModuleInclude to []*APILocalModuleInclude
	result := make([]*restModel.APILocalModuleInclude, len(obj.LocalModuleIncludes))
	for i, module := range obj.LocalModuleIncludes {
		result[i] = &module
	}
	return result, nil
}

// Patch returns PatchResolver implementation.
func (r *Resolver) Patch() PatchResolver { return &patchResolver{r} }

type patchResolver struct{ *Resolver }
