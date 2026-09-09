package graphql

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/graphql/loaders"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// ID is the resolver for the id field.
func (r *patchResolver) ID(ctx context.Context, obj *patch.Patch) (string, error) {
	return obj.Id.Hex(), nil
}

// ChildPatchAliases is the resolver for the childPatchAliases field.
func (r *patchResolver) ChildPatchAliases(ctx context.Context, obj *patch.Patch) ([]*ChildPatchAlias, error) {
	result := make([]*ChildPatchAlias, 0, len(obj.Triggers.ChildPatches))
	for i, childPatchID := range obj.Triggers.ChildPatches {
		if i < len(obj.Triggers.Aliases) {
			result = append(result, &ChildPatchAlias{
				Alias:   obj.Triggers.Aliases[i],
				PatchID: childPatchID,
			})
		}
	}
	return result, nil
}

// ChildPatches is the resolver for the childPatches field.
func (r *patchResolver) ChildPatches(ctx context.Context, obj *patch.Patch) ([]*patch.Patch, error) {
	if len(obj.Triggers.ChildPatches) == 0 {
		return []*patch.Patch{}, nil
	}
	loaders.PreloadPatches(ctx, obj.Triggers.ChildPatches)

	result := make([]*patch.Patch, 0, len(obj.Triggers.ChildPatches))
	for _, pId := range obj.Triggers.ChildPatches {
		p, err := loaders.GetPatch(ctx, pId)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting child patch '%s': %s", pId, err.Error()), err)
		}
		if p == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("child patch '%s' not found", pId))
		}
		result = append(result, p)
	}
	return result, nil
}

// GeneratedTaskCounts is the resolver for the generatedTaskCounts field.
func (r *patchResolver) GeneratedTaskCounts(ctx context.Context, obj *patch.Patch) ([]*GeneratedTaskCountResults, error) {
	patchID := obj.Id.Hex()
	p, err := loaders.GetPatch(ctx, patchID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching patch '%s': %s", patchID, err.Error()), err)
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
	for _, buildVariant := range patchProjectVariantsAndTasks.Variants {
		var generatorDisplayNames []string
		for _, taskUnit := range buildVariant.Tasks {
			if _, ok := generatorTasks[taskUnit.Name]; ok {
				generatorDisplayNames = append(generatorDisplayNames, taskUnit.Name)
			}
		}

		estimations, err := task.GetBatchedGenerateTasksEstimations(ctx, p.Project, buildVariant.Name, generatorDisplayNames)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting generated task estimations for build variant '%s': %s", buildVariant.Name, err.Error()))
		}
		for _, displayName := range generatorDisplayNames {
			if estimation, ok := estimations[displayName]; ok {
				res = append(res, &GeneratedTaskCountResults{
					BuildVariantName: utility.ToStringPtr(buildVariant.Name),
					TaskName:         utility.ToStringPtr(displayName),
					EstimatedTasks:   estimation.EstimatedNumActivatedGeneratedTasks,
				})
			}
		}
	}
	return res, nil
}

// InvalidatedByUpstream is the resolver for the invalidatedByUpstream field.
func (r *patchResolver) InvalidatedByUpstream(ctx context.Context, obj *patch.Patch) (bool, error) {
	return obj.GithubMergeData.InvalidatedByUpstream, nil
}

// ModuleCodeChanges is the resolver for the moduleCodeChanges field.
func (r *patchResolver) ModuleCodeChanges(ctx context.Context, obj *patch.Patch) ([]*restModel.APIModulePatch, error) {
	identifier, err := model.GetIdentifierForProjectSecondary(ctx, obj.Project)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting project identifier for project '%s': %s", obj.Project, err.Error()), err)
	}
	apiURL := evergreen.GetEnvironment().Settings().Api.URL
	codeChanges := restModel.BuildModuleCodeChanges(*obj, identifier, apiURL)
	result := make([]*restModel.APIModulePatch, 0, len(codeChanges))
	for i := range codeChanges {
		result = append(result, &codeChanges[i])
	}
	return result, nil
}

// Parameters is the resolver for the parameters field.
func (r *patchResolver) Parameters(ctx context.Context, obj *patch.Patch) ([]*restModel.APIParameter, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}

	projectId := obj.Project
	projVars, err := model.FindMergedProjectVars(ctx, projectId)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting project vars for project '%s': %s", projectId, err.Error()))
	}

	redactKeys := config.LoggerConfig.RedactKeys
	var res []*restModel.APIParameter
	for _, param := range obj.Parameters {
		redactedParam := &restModel.APIParameter{
			Key:   utility.ToStringPtr(param.Key),
			Value: utility.ToStringPtr(param.Value),
		}
		for _, pattern := range redactKeys {
			if strings.Contains(strings.ToLower(param.Key), pattern) {
				redactedParam.Value = utility.ToStringPtr(evergreen.RedactedValue)
				break
			}
		}
		if projVars != nil {
			for varKey, varValue := range projVars.Vars {
				if strings.Contains(param.Value, varValue) && projVars.PrivateVars[varKey] {
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
func (r *patchResolver) PatchTriggerAliases(ctx context.Context, obj *patch.Patch) ([]*restModel.APIPatchTriggerDefinition, error) {
	projectID := obj.Project
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

		matchingTasks, err := project.VariantTasksForSelectors(ctx, []patch.PatchTriggerDefinition{alias}, obj.GetRequester())
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

		identifier, err := model.GetIdentifierForProjectSecondary(ctx, alias.ChildProject)
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
func (r *patchResolver) Project(ctx context.Context, obj *patch.Patch) (*PatchProject, error) {
	patchProjectVariantsAndTasks, err := model.GetVariantsAndTasksFromPatchProject(ctx, evergreen.GetEnvironment().Settings(), obj)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting project variants and tasks for patch '%s': %s", obj.Id, err.Error()))
	}

	// convert variants to UI data structure
	variants := []*ProjectBuildVariant{}
	for _, buildVariant := range patchProjectVariantsAndTasks.Variants {
		projBuildVariant := ProjectBuildVariant{
			Name:        buildVariant.Name,
			DisplayName: buildVariant.DisplayName,
		}
		projTasks := []string{}
		executionTasks := map[string]bool{}
		for _, displayTask := range buildVariant.DisplayTasks {
			projTasks = append(projTasks, displayTask.Name)
			for _, execTask := range displayTask.ExecTasks {
				executionTasks[execTask] = true
			}
		}
		for _, taskUnit := range buildVariant.Tasks {
			// Only add task if it is not an execution task.
			if !executionTasks[taskUnit.Name] {
				projTasks = append(projTasks, taskUnit.Name)
			}
		}
		// Sort tasks alphanumerically by display name.
		sort.SliceStable(projTasks, func(i, j int) bool {
			return projTasks[i] < projTasks[j]
		})
		projBuildVariant.Tasks = projTasks
		variants = append(variants, &projBuildVariant)
	}
	sort.SliceStable(variants, func(i, j int) bool {
		return variants[i].DisplayName < variants[j].DisplayName
	})

	patchProject := PatchProject{
		Variants: variants,
	}
	return &patchProject, nil
}

// ProjectMetadata is the resolver for the projectMetadata field.
func (r *patchResolver) ProjectMetadata(ctx context.Context, obj *patch.Patch) (*restModel.APIProjectRef, error) {
	apiProjectRef, err := getAPIProjectRef(ctx, utility.ToStringPtr(obj.Project))
	return apiProjectRef, err
}

// User is the resolver for the user field.
func (r *patchResolver) User(ctx context.Context, obj *patch.Patch) (*user.DBUser, error) {
	// If only id is requested, we can return it without a database call.
	requestedFields := graphql.CollectAllFields(ctx)
	if len(requestedFields) == 1 && requestedFields[0] == "id" {
		return &user.DBUser{Id: obj.Author}, nil
	}

	authorId := obj.Author
	currentUser := mustHaveUser(ctx)
	if currentUser.Id == authorId {
		return currentUser, nil
	}

	dbUser, err := loaders.GetUser(ctx, authorId)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting user '%s': %s", authorId, err.Error()), err)
	}
	// This is most likely a service user, so just return their ID.
	if dbUser == nil {
		return &user.DBUser{Id: authorId}, nil
	}
	return dbUser, nil
}

// VariantsTasks is the resolver for the variantsTasks field.
func (r *patchResolver) VariantsTasks(ctx context.Context, obj *patch.Patch) ([]*restModel.VariantTask, error) {
	result := make([]*restModel.VariantTask, 0, len(obj.VariantsTasks))
	for _, vt := range obj.VariantsTasks {
		result = append(result, &restModel.VariantTask{
			Name:  utility.ToStringPtr(vt.Variant),
			Tasks: utility.ToStringPtrSlice(vt.Tasks),
		})
	}
	return result, nil
}

// Version is the resolver for the version field.
func (r *patchResolver) Version(ctx context.Context, obj *patch.Patch) (*model.Version, error) {
	versionID := obj.Version
	if versionID == "" {
		return nil, nil
	}
	v, err := loaders.GetVersion(ctx, versionID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching version '%s': %s", versionID, err.Error()))
	}
	if v == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("version '%s' not found", versionID))
	}
	return v, nil
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
func (r *patchesResolver) Patches(ctx context.Context, obj *Patches) ([]*patch.Patch, error) {
	fc := graphql.GetFieldContext(ctx)
	opts, err := buildOptionsFromParentArgs(ctx, fc)
	if err != nil {
		return nil, err
	}

	patches, err := patch.ProjectOrUserPatchesPage(ctx, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching patches: %s", err.Error()))
	}

	patchList := make([]*patch.Patch, 0, len(patches))
	projectIDs := make([]string, 0, len(patches))
	patchIDs := make([]string, 0, len(patches))
	for _, p := range patches {
		patchList = append(patchList, &p)
		patchIDs = append(patchIDs, p.Id.Hex())
		projectIDs = append(projectIDs, p.Project)
	}

	if len(patchIDs) > 0 {
		loaders.PreloadPatches(ctx, patchIDs)
	}
	if len(projectIDs) > 0 {
		loaders.PreloadProjects(ctx, projectIDs)
	}

	return patchList, nil
}

// Patch returns PatchResolver implementation.
func (r *Resolver) Patch() PatchResolver { return &patchResolver{r} }

// Patches returns PatchesResolver implementation.
func (r *Resolver) Patches() PatchesResolver { return &patchesResolver{r} }

type patchResolver struct{ *Resolver }
type patchesResolver struct{ *Resolver }
