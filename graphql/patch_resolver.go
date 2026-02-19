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
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AuthorDisplayName is the resolver for the authorDisplayName field.
func (r *patchResolver) AuthorDisplayName(ctx context.Context, obj *restModel.APIPatch) (string, error) {
	author := utility.FromStringPtr(obj.Author)
	usr, err := user.FindOneById(ctx, author)
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
	p, err := patch.FindOneId(ctx, patchID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching patch '%s': %s", patchID, err.Error()))
	}
	if p == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("patch '%s' not found", patchID))
	}
	versionIDs := []string{patchID}
	if p.IsParent() {
		versionIDs = append(versionIDs, p.Triggers.ChildPatches...)
	}
	query := db.Query(task.ByVersions(versionIDs)).WithFields(task.TimeTakenKey, task.StartTimeKey, task.FinishTimeKey, task.DisplayOnlyKey, task.ExecutionKey)
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

	// Build $or conditions for the aggregation pipeline
	var orQueries []bson.M
	for _, k := range queryKeys {
		orQueries = append(orQueries, bson.M{
			task.ProjectKey:      k.Project,
			task.BuildVariantKey: k.BuildVariant,
			task.DisplayNameKey:  k.DisplayName,
			task.GenerateTaskKey: true,                   // Only include actual generator tasks
			task.StatusKey:       evergreen.TaskSucceeded, // Only successful tasks have reliable estimation data
		})
	}

	// Time filter: limit to recent tasks for performance. 30 days provides
	// sufficient history for accurate estimates while keeping result sets manageable.
	const generatedTaskLookbackDays = 30
	lookbackTime := time.Now().Add(-generatedTaskLookbackDays * 24 * time.Hour)

	// Use aggregation to get only the most recent task per (project, build_variant, display_name)
	// This avoids loading all matching tasks into memory
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{
			"$and": []bson.M{
				{"$or": orQueries},
				{task.FinishTimeKey: bson.M{"$gte": lookbackTime}},
			},
		}}},
		bson.D{{Key: "$sort", Value: bson.M{task.FinishTimeKey: -1}}},
		bson.D{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				task.ProjectKey:      "$" + task.ProjectKey,
				task.BuildVariantKey: "$" + task.BuildVariantKey,
				task.DisplayNameKey:  "$" + task.DisplayNameKey,
			},
			"task": bson.M{"$first": "$$ROOT"},
		}}},
		bson.D{{Key: "$replaceRoot", Value: bson.M{"newRoot": "$task"}}},
	}

	// Hint to use TaskHistoricalDataIndex (project, build_variant, display_name, status, finish_time)
	opts := options.Aggregate().SetHint(task.TaskHistoricalDataIndex)

	cursor, err := evergreen.GetEnvironment().DB().Collection(task.Collection).Aggregate(ctx, pipeline, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting generated tasks: %s", err.Error()))
	}
	var dbTasks []task.Task
	if err = cursor.All(ctx, &dbTasks); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("decoding generated tasks: %s", err.Error()))
	}

	// Build map for quick lookup by composite key.
	// Use index-based iteration to get stable pointers to slice elements,
	// avoiding the subtle bug of storing pointers to a loop variable.
	taskMap := make(map[taskQueryKey]*task.Task)
	for i := range dbTasks {
		t := &dbTasks[i]
		k := taskQueryKey{
			Project:      t.Project,
			BuildVariant: t.BuildVariant,
			DisplayName:  t.DisplayName,
		}
		taskMap[k] = t
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

// IncludedLocalModules is the resolver for the includedLocalModules field.
func (r *patchResolver) IncludedLocalModules(ctx context.Context, obj *restModel.APIPatch) ([]*restModel.APILocalModuleInclude, error) {
	// Convert []APILocalModuleInclude to []*APILocalModuleInclude
	result := make([]*restModel.APILocalModuleInclude, len(obj.LocalModuleIncludes))
	for i, module := range obj.LocalModuleIncludes {
		result[i] = &module
	}
	return result, nil
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
			_, project, _, err = model.FindLatestVersionWithValidProject(ctx, alias.ChildProject, false)
			if err != nil {
				// Skip this alias if the child project has no valid versions.
				// E.g., all versions expired due to TTL or project has no mainline commits.
				continue
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

// User is the resolver for the user field.
func (r *patchResolver) User(ctx context.Context, obj *restModel.APIPatch) (*restModel.APIDBUser, error) {
	// If only userId is requested, we can return it without a database call.
	requestedFields := graphql.CollectAllFields(ctx)
	if len(requestedFields) == 1 && requestedFields[0] == "userId" {
		return &restModel.APIDBUser{
			UserID: obj.Author,
		}, nil
	}

	authorId := utility.FromStringPtr(obj.Author)
	currentUser := mustHaveUser(ctx)
	if currentUser.Id == authorId {
		apiUser := &restModel.APIDBUser{}
		apiUser.BuildFromService(*currentUser)
		return apiUser, nil
	}

	author, err := user.FindOneById(ctx, authorId)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting user '%s': %s", authorId, err.Error()))
	}
	// This is most likely a reaped user, so just return their ID
	if author == nil {
		return &restModel.APIDBUser{
			UserID: obj.Author,
		}, nil
	}

	apiUser := &restModel.APIDBUser{}
	apiUser.BuildFromService(*author)
	return apiUser, nil
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

// FilteredPatchCount is the resolver for the filteredPatchCount field.
func (r *patchesResolver) FilteredPatchCount(ctx context.Context, obj *Patches) (int, error) {
	fc := graphql.GetFieldContext(ctx)
	opts, err := buildOptionsFromParentArgs(ctx, fc)
	if err != nil {
		return 0, err
	}

	count, err := patch.ProjectOrUserPatchesCount(ctx, opts)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("fetching patch count: %s", err.Error()))
	}
	return count, nil
}

// Patches is the resolver for the patches field.
func (r *patchesResolver) Patches(ctx context.Context, obj *Patches) ([]*restModel.APIPatch, error) {
	fc := graphql.GetFieldContext(ctx)
	opts, err := buildOptionsFromParentArgs(ctx, fc)
	if err != nil {
		return nil, err
	}

	patches, err := patch.ProjectOrUserPatchesPage(ctx, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching patches: %s", err.Error()))
	}

	apiPatches := []*restModel.APIPatch{}
	for _, p := range patches {
		apiPatch := restModel.APIPatch{}
		if err := apiPatch.BuildFromService(ctx, p, nil); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting patch '%s' to APIPatch: %s", p.Id.Hex(), err.Error()))
		}
		apiPatches = append(apiPatches, &apiPatch)
	}

	return apiPatches, nil
}

// Patch returns PatchResolver implementation.
func (r *Resolver) Patch() PatchResolver { return &patchResolver{r} }

// Patches returns PatchesResolver implementation.
func (r *Resolver) Patches() PatchesResolver { return &patchesResolver{r} }

type patchResolver struct{ *Resolver }
type patchesResolver struct{ *Resolver }
