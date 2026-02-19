package validator

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type projectValidator func(*model.Project) ValidationErrors

type projectConfigValidator func(ctx context.Context, config *model.ProjectConfig) ValidationErrors

type projectSettingsValidator func(context.Context, *evergreen.Settings, *model.Project, *model.ProjectRef, bool) ValidationErrors

type projectAliasValidator func(config *model.Project, aliases model.ProjectAliases) ValidationErrors

type ValidationErrorLevel int64

const (
	Error ValidationErrorLevel = iota
	Warning
	Notice
	EC2HostCreateTotalLimit    = 1000
	DockerHostCreateTotalLimit = 200
	HostCreateLimitPerTask     = 3
)

var (
	// Not a regex because these characters could be valid if unicoded.
	unauthorizedCharacters = []string{"|", "&", ";", "$", "`", "'", "*", "?", "#", "%", "^", "@", "{", "}", "(", ")", "<", ">"}
)

func (vel ValidationErrorLevel) String() string {
	switch vel {
	case Error:
		return "ERROR"
	case Warning:
		return "WARNING"
	case Notice:
		return "NOTICE"
	}
	return "?"
}

type ValidationError struct {
	Level   ValidationErrorLevel `json:"level"`
	Message string               `json:"message"`
}

type ValidationErrors []ValidationError

func (v ValidationErrors) Raw() any {
	return v
}
func (v ValidationErrors) Loggable() bool {
	return len(v) > 0
}
func (v ValidationErrors) String() string {
	out := ""
	for i, validationErr := range v {
		if i > 0 {
			out += "\n"
		}
		out += fmt.Sprintf("%s: %s", validationErr.Level.String(), validationErr.Message)
	}

	return out
}
func (v ValidationErrors) Annotate(key string, value any) error {
	return nil
}
func (v ValidationErrors) Priority() level.Priority {
	return level.Info
}
func (v ValidationErrors) SetPriority(_ level.Priority) error {
	return nil
}

// AtLevel returns all validation errors that match the given level.
func (v ValidationErrors) AtLevel(level ValidationErrorLevel) ValidationErrors {
	errs := ValidationErrors{}
	for _, err := range v {
		if err.Level == level {
			errs = append(errs, err)
		}
	}
	return errs
}

// Has returns if any of the errors are at the given level.
func (v ValidationErrors) Has(level ValidationErrorLevel) bool {
	return len(v.AtLevel(level)) > 0
}

type ValidationInput struct {
	ProjectYaml []byte `json:"project_yaml" yaml:"project_yaml"`
	Quiet       bool   `json:"quiet" yaml:"quiet"`
	ProjectID   string `json:"project_id" yaml:"project_id"`
}

// Functions used to validate the project configuration file for errors.
// These are expected to only return ValidationError's with
// a level of Error ValidationLevel. They must also explicitly return
// Error as opposed to leaving the field blank.
var projectErrorValidators = []projectValidator{
	validateBVFields,
	validateDependencyGraph,
	validateProjectFields,
	validateStatusesForTaskDependencies,
	validateTaskNames,
	validateBVNames,
	validateBVBatchTimes,
	validateDisplayTaskNames,
	validateBVTaskNames,
	validateAllDependenciesSpec,
	validateProjectTaskNames,
	validateProjectTaskIdsAndTags,
	validateParameters,
	validateTaskGroups,
	validateHostCreates,
	validateDuplicateBVTasks,
	validateGenerateTasks,
}

// Functions used to validate the syntax of project configs representing properties found on the project page.
var projectConfigErrorValidators = []projectConfigValidator{
	validateProjectConfigAliases,
	validateProjectConfigPlugins,
}

// Functions used to validate the project configuration file for warnings and
// notices. These are expected to only return ValidationError's with a level of
// Warning ValidationLevel or Notice ValidationLevel.
var projectWarningValidators = []projectValidator{
	checkTaskGroups,
	checkTaskRuns,
	checkModules,
	checkTasks,
	checkReferencesForTaskDependencies,
	checkRequestersForTaskDependencies,
	checkBuildVariants,
	checkTaskUsage,
}

// Functions used to validate the project configuration file at any level. This
// is useful for validation that could return a mix of errors and warnings and
// can be an optimization to avoid processing the same project configuration
// multiple times to perform validation checks at different levels.
var projectMixedValidators = []projectValidator{
	validatePluginCommands,
}

var projectAliasWarningValidators = []projectAliasValidator{
	validateAliasCoverage,
	validateCheckRuns,
}

// Functions used to validate a project configuration that requires additional
// info such as admin settings and project settings.
var projectSettingsValidators = []projectSettingsValidator{
	validateVersionControl,
	validateProjectLimits,
	validateIncludeLimits,
	validateTimeoutLimits,
	validateReferentialIntegrity,
}

func (vr ValidationError) Error() string {
	return vr.Message
}

func ValidationErrorsToString(ves ValidationErrors) string {
	var s bytes.Buffer
	if len(ves) == 0 {
		return s.String()
	}
	for _, ve := range ves {
		s.WriteString(ve.Error())
		s.WriteString("\n")
	}
	return s.String()
}

// getDistros creates a slice of all distro IDs and aliases.
func getDistros(ctx context.Context) (ids []string, aliases []string, distroWarnings map[string]string, err error) {
	ids, aliases, _, distroWarnings, err = getDistrosForProject(ctx, "")
	return ids, aliases, distroWarnings, err
}

// getDistrosForProject creates a slice of all valid distro IDs and a slice of
// all valid aliases for a project, as well as any distro warnings. If projectID is empty, it returns all distro
// IDs and all aliases.
func getDistrosForProject(ctx context.Context, projectID string) (ids, aliases, singleTaskDistroIDs []string, distroWarnings map[string]string, err error) {
	distroWarnings = map[string]string{}
	// create a slice of all known distros
	distros, err := distro.AllDistros(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	for _, d := range distros {
		if projectID == "" || len(d.ValidProjects) == 0 || utility.StringSliceContains(d.ValidProjects, projectID) {
			if d.SingleTaskDistro {
				singleTaskDistroIDs = append(singleTaskDistroIDs, d.Id)
			}
			ids = append(ids, d.Id)
			for _, alias := range d.Aliases {
				if !utility.StringSliceContains(aliases, alias) {
					aliases = append(aliases, alias)
				}
			}
			if d.WarningNote != "" {
				addDistroWarning(distroWarnings, d.Id, d.WarningNote)
				distroWarnings[d.Id] = d.WarningNote
				for _, alias := range d.Aliases {
					addDistroWarning(distroWarnings, alias, d.WarningNote)
				}
			}
		}
	}
	return ids, utility.UniqueStrings(aliases), singleTaskDistroIDs, distroWarnings, nil
}

func addDistroWarning(distroWarnings map[string]string, distroName, warningNote string) {
	if distroWarnings[distroName] == "" {
		distroWarnings[distroName] = warningNote
		return
	}
	distroWarnings[distroName] = fmt.Sprintf("\t%s\n\t%s", distroWarnings[distroName], warningNote)
}

// CheckProject calls the validating logic for a Project's configuration.
// That is, ProjectErrors, ProjectWarnings, ProjectConfigErrors,
// ProjectSettings, and AliasWarnings. If a respective item is nil,
// it will not check it (e.g. if the config is nil, it does not check the config).
// projectRefId is used to determine if there is a project specified and
// projectRefErr is used to determine if there was a problem retrieving
// the ref; both output different warnings for the project.
func CheckProject(ctx context.Context, project *model.Project, config *model.ProjectConfig, ref *model.ProjectRef, projectRefId string, projectRefErr error) ValidationErrors {
	isConfigDefined := config != nil
	verrs := CheckProjectErrors(ctx, project)
	verrs = append(verrs, CheckProjectMixedValidations(project)...)
	verrs = append(verrs, CheckProjectWarnings(project)...)
	if config != nil {
		verrs = append(verrs, CheckProjectConfigErrors(ctx, config)...)
	}

	if projectRefId == "" {
		verrs = append(verrs, ValidationError{
			Message: "no project specified; validation will proceed without checking project settings and alias coverage",
			Level:   Warning,
		})
		return verrs
	}
	if ref == nil {
		if projectRefErr != nil {
			return append(verrs, ValidationError{
				Message: "error finding project; validation will proceed without checking project settings and alias coverage",
				Level:   Warning,
			})
		}
		return append(verrs, ValidationError{
			Message: "project does not exist; validation will proceed without checking project settings and alias coverage",
			Level:   Warning,
		})
	}
	verrs = append(verrs, CheckProjectSettings(ctx, evergreen.GetEnvironment().Settings(), project, ref, isConfigDefined)...)
	// Check project aliases
	aliases, err := model.ConstructMergedAliasesByPrecedence(ctx, ref, config, ref.RepoRefId)
	if err != nil {
		return append(verrs, ValidationError{
			Message: "problem finding aliases; validation will not check alias coverage",
			Level:   Warning,
		})
	}
	return append(verrs, CheckAliasWarnings(project, aliases)...)
}

// CheckProjectWarnings returns warnings about the project configuration semantics
func CheckProjectWarnings(project *model.Project) ValidationErrors {
	validationErrs := ValidationErrors{}
	for _, projectWarningValidator := range projectWarningValidators {
		validationErrs = append(validationErrs,
			projectWarningValidator(project)...)
	}
	return validationErrs
}

// CheckProjectMixedValidations returns validation errors about the project
// configuration that can be at any validation level.
func CheckProjectMixedValidations(project *model.Project) ValidationErrors {
	validationErrs := ValidationErrors{}
	for _, mixedValidator := range projectMixedValidators {
		validationErrs = append(validationErrs,
			mixedValidator(project)...)
	}
	return validationErrs
}

// CheckAliasWarnings returns warnings related to the definition of tasks/variants matching the given aliases.
func CheckAliasWarnings(project *model.Project, aliases model.ProjectAliases) ValidationErrors {
	validationErrs := ValidationErrors{}
	for _, validator := range projectAliasWarningValidators {
		validationErrs = append(validationErrs,
			validator(project, aliases)...)
	}

	return validationErrs
}

// CheckProjectErrors returns errors about the project configuration syntax
func CheckProjectErrors(ctx context.Context, project *model.Project) ValidationErrors {
	validationErrs := ValidationErrors{}

	for _, projectErrorValidator := range projectErrorValidators {
		validationErrs = append(validationErrs,
			projectErrorValidator(project)...)
	}

	return validationErrs
}

// CheckPatchedProjectConfigErrors returns validation errors for the given patched project config.
func CheckPatchedProjectConfigErrors(ctx context.Context, patchedProjectConfig string) ValidationErrors {
	validationErrs := ValidationErrors{}
	if len(patchedProjectConfig) <= 0 {
		return validationErrs
	}
	projectConfig, err := model.CreateProjectConfig([]byte(patchedProjectConfig), "")
	if err != nil {
		validationErrs = append(validationErrs, ValidationError{
			Message: fmt.Sprintf("Error unmarshalling patched project config: %s", err.Error()),
		})
		return validationErrs
	}
	return CheckProjectConfigErrors(ctx, projectConfig)
}

// CheckProjectConfigErrors verifies that the project configuration syntax is valid
func CheckProjectConfigErrors(ctx context.Context, projectConfig *model.ProjectConfig) ValidationErrors {
	validationErrs := ValidationErrors{}
	if projectConfig == nil {
		return validationErrs
	}
	for _, projectConfigErrorValidator := range projectConfigErrorValidators {
		validationErrs = append(validationErrs,
			projectConfigErrorValidator(ctx, projectConfig)...)
	}

	return validationErrs
}

// CheckProjectSettings checks the project configuration against the project
// settings and referential integrity.
func CheckProjectSettings(ctx context.Context, settings *evergreen.Settings, p *model.Project, ref *model.ProjectRef, isConfigDefined bool) ValidationErrors {
	var validationErrs ValidationErrors
	for _, validateSettings := range projectSettingsValidators {
		validationErrs = append(validationErrs, validateSettings(ctx, settings, p, ref, isConfigDefined)...)
	}

	return validationErrs
}

// CheckProjectConfigurationIsValid checks if the project configuration has errors
func CheckProjectConfigurationIsValid(ctx context.Context, settings *evergreen.Settings, project *model.Project, pref *model.ProjectRef) error {
	_, span := tracer.Start(ctx, "check-configuration", trace.WithAttributes(
		attribute.String(evergreen.ProjectIDOtelAttribute, pref.Id),
		attribute.String(evergreen.ProjectIdentifierOtelAttribute, pref.Identifier),
	))
	defer span.End()
	catcher := grip.NewBasicCatcher()
	projectErrors := CheckProjectErrors(ctx, project)
	projectErrors = append(projectErrors, CheckProjectMixedValidations(project).AtLevel(Error)...)
	if len(projectErrors) != 0 {
		if errs := projectErrors.AtLevel(Error); len(errs) != 0 {
			catcher.Errorf("project contains errors: %s", ValidationErrorsToString(errs))
		}
	}

	if settingsErrs := CheckProjectSettings(ctx, settings, project, pref, false); len(settingsErrs) != 0 {
		if errs := settingsErrs.AtLevel(Error); len(errs) != 0 {
			catcher.Errorf("project contains errors related to project settings: %s", ValidationErrorsToString(errs))
		}
	}
	return catcher.Resolve()
}

// ensure that if any task spec references 'model.AllDependencies', it
// references no other dependency within the variant
func validateAllDependenciesSpec(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	for _, task := range project.Tasks {
		coveredVariants := map[string]bool{}
		if len(task.DependsOn) > 1 {
			for _, dependency := range task.DependsOn {
				if dependency.Name == model.AllDependencies {
					// incorrect if no variant specified or this variant has already been covered
					if dependency.Variant == "" || coveredVariants[dependency.Variant] {
						errs = append(errs,
							ValidationError{
								Level: Error,
								Message: fmt.Sprintf("task '%s' contains the all dependencies (%s)' "+
									"specification and other explicit dependencies or duplicate variants",
									task.Name, model.AllDependencies),
							},
						)
					}
					coveredVariants[dependency.Variant] = true
				}
			}
		}
	}
	return errs
}

// validateDependencyGraph returns a non-nil ValidationErrors if the dependency graph contains cycles.
func validateDependencyGraph(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	graph := project.DependencyGraph()
	for _, cycle := range graph.Cycles() {
		var nodeStrings []string
		for _, task := range cycle {
			nodeStrings = append(nodeStrings, task.String())
		}
		errs = append(errs, ValidationError{
			Level:   Error,
			Message: fmt.Sprintf("tasks [%s] form a dependency cycle", strings.Join(nodeStrings, ", ")),
		})
	}

	return errs
}

// tvToTaskUnit generates all task-variant pairs mapped to their corresponding
// task unit within a build variant.
func tvToTaskUnit(p *model.Project) map[model.TVPair]model.BuildVariantTaskUnit {
	// map of task name and variant -> BuildVariantTaskUnit
	tasksByNameAndVariant := map[model.TVPair]model.BuildVariantTaskUnit{}

	// generate task nodes for every task and variant combination

	taskGroups := map[string]struct{}{}
	for _, tg := range p.TaskGroups {
		taskGroups[tg.Name] = struct{}{}
	}
	for _, bv := range p.BuildVariants {
		tasksToAdd := []model.BuildVariantTaskUnit{}
		for _, t := range bv.Tasks {
			if _, ok := taskGroups[t.Name]; ok {
				tasksToAdd = append(tasksToAdd, model.CreateTasksFromGroup(t, p, "")...)
			} else {
				tasksToAdd = append(tasksToAdd, t)
			}
		}
		for _, t := range tasksToAdd {
			t.Variant = bv.Name
			node := model.TVPair{
				Variant:  bv.Name,
				TaskName: t.Name,
			}

			tasksByNameAndVariant[node] = t
		}
	}
	return tasksByNameAndVariant
}

func validateProjectConfigAliases(ctx context.Context, pc *model.ProjectConfig) ValidationErrors {
	errs := []string{}
	pc.SetInternalAliases()
	errs = append(errs, model.ValidateProjectAliases(pc.GitHubPRAliases, "GitHub PR Aliases")...)
	errs = append(errs, model.ValidateProjectAliases(pc.GitHubChecksAliases, "GitHub Checks Aliases")...)
	errs = append(errs, model.ValidateProjectAliases(pc.CommitQueueAliases, "Commit Queue Aliases")...)
	errs = append(errs, model.ValidateProjectAliases(pc.PatchAliases, "Patch Aliases")...)
	errs = append(errs, model.ValidateProjectAliases(pc.GitTagAliases, "Git Tag Aliases")...)

	validationErrs := ValidationErrors{}
	for _, errorMsg := range errs {
		validationErrs = append(validationErrs, ValidationError{
			Message: fmt.Sprintf("error validating aliases: %s", errorMsg),
			Level:   Error,
		})
	}
	return validationErrs
}

// validateAliasCoverage validates that all commit queue aliases defined match some variants/tasks.
func validateAliasCoverage(p *model.Project, aliases model.ProjectAliases) ValidationErrors {
	aliasMap := map[string]model.ProjectAlias{}
	for _, a := range aliases {
		aliasMap[a.ID.Hex()] = a
	}
	aliasNeedsVariant, aliasNeedsTask, err := getAliasCoverage(p, aliasMap)
	if err != nil {
		return ValidationErrors{
			{
				Message: "error checking alias coverage, continuing without validation",
				Level:   Warning,
			},
		}
	}
	return constructAliasWarnings(aliasMap, aliasNeedsVariant, aliasNeedsTask)
}

// constructAliasWarnings returns validation errors given a map of aliases, and whether they need variants/tasks to match.
func constructAliasWarnings(aliasMap map[string]model.ProjectAlias, aliasNeedsVariant, aliasNeedsTask map[string]bool) ValidationErrors {
	res := ValidationErrors{}
	errs := []string{}
	for aliasID, a := range aliasMap {
		needsVariant := aliasNeedsVariant[aliasID]
		needsTask := aliasNeedsTask[aliasID]
		if !needsVariant && !needsTask {
			continue
		}

		msgComponents := []string{}
		switch a.Alias {
		case evergreen.CommitQueueAlias:
			msgComponents = append(msgComponents, "Merge queue alias")
		case evergreen.GithubPRAlias:
			msgComponents = append(msgComponents, "GitHub PR alias")
		case evergreen.GitTagAlias:
			msgComponents = append(msgComponents, "Git tag alias")
		case evergreen.GithubChecksAlias:
			msgComponents = append(msgComponents, "GitHub check alias")
		default:
			msgComponents = append(msgComponents, fmt.Sprintf("Patch alias '%s'", a.Alias))
		}
		switch a.Source {
		case model.AliasSourceConfig:
			msgComponents = append(msgComponents, "(from the yaml)")
		case model.AliasSourceProject:
			msgComponents = append(msgComponents, "(from the project page)")
		case model.AliasSourceRepo:
			msgComponents = append(msgComponents, "(from the repo page)")
		}
		if len(a.VariantTags) > 0 {
			msgComponents = append(msgComponents, fmt.Sprintf("matching variant tags '%v'", a.VariantTags))
		} else {
			msgComponents = append(msgComponents, fmt.Sprintf("matching variant regexp '%s'", a.Variant))
		}
		if needsVariant {
			msgComponents = append(msgComponents, "has no matching variants")
		} else {
			// This is only relevant information if the alias matches the variant but not the task.
			if len(a.TaskTags) > 0 {
				msgComponents = append(msgComponents, fmt.Sprintf("and matching task tags '%v'", a.TaskTags))
			} else {
				msgComponents = append(msgComponents, fmt.Sprintf("and matching task regexp '%s'", a.Task))
			}
			msgComponents = append(msgComponents, "has no matching tasks")
		}
		errs = append(errs, strings.Join(msgComponents, " "))
	}
	sort.Strings(errs)
	for _, err := range errs {
		res = append(res, ValidationError{
			Message: err,
			Level:   Warning,
		})
	}

	return res
}

// getAliasCoverage returns a map of aliases that don't match variants and a map of aliases that don't match tasks.
func getAliasCoverage(p *model.Project, aliasMap map[string]model.ProjectAlias) (map[string]bool, map[string]bool, error) {
	type taskInfo struct {
		name string
		tags []string
	}
	aliasNeedsVariant := map[string]bool{}
	aliasNeedsTask := map[string]bool{}
	bvtCache := map[string]taskInfo{}
	for a := range aliasMap {
		aliasNeedsVariant[a] = true
		aliasNeedsTask[a] = true
	}
	for _, bv := range p.BuildVariants {
		for aliasID, alias := range aliasMap {
			if !aliasNeedsVariant[aliasID] && !aliasNeedsTask[aliasID] { // Have already found both variants and tasks.
				continue
			}
			// If we still need a task to match the variant, still check if the alias matches, so we know if checking tasks is needed.
			matchesThisVariant, err := alias.HasMatchingVariant(bv.Name, bv.Tags)
			if err != nil {
				return nil, nil, err
			}
			if !matchesThisVariant { // If the variant doesn't match, then there's no reason to keep checking tasks.
				continue
			}
			aliasNeedsVariant[aliasID] = false
			// Loop through all tasks to verify if there is task coverage.
			for _, t := range bv.Tasks {
				var name string
				var tags []string
				if info, ok := bvtCache[t.Name]; ok {
					name = info.name
					tags = info.tags
				} else {
					name, tags, _ = p.GetTaskNameAndTags(t)
					// Even if we can't find the name/tags, still store it, so we don't try again.
					bvtCache[t.Name] = taskInfo{name: name, tags: tags}
				}
				if name != "" {
					if t.IsGroup {
						matchesTaskGroupTask, err := aliasMatchesTaskGroupTask(p, alias, name)
						if err != nil {
							return nil, nil, err
						}
						if matchesTaskGroupTask {
							aliasNeedsTask[aliasID] = false
							break
						}
					}
					matchesThisTask, err := alias.HasMatchingTask(name, tags)
					if err != nil {
						return nil, nil, err
					}
					if matchesThisTask {
						aliasNeedsTask[aliasID] = false
						break
					}
				}
			}
		}
	}
	return aliasNeedsVariant, aliasNeedsTask, nil
}

// validateCheckRuns returns warnings if PR aliases are going to violate check run rules for the given config.
func validateCheckRuns(p *model.Project, aliases model.ProjectAliases) ValidationErrors {
	errs := ValidationErrors{}
	aliasMap := map[string][]model.ProjectAlias{} // map of alias name to aliases
	for _, a := range aliases {
		if _, ok := aliasMap[a.Alias]; !ok {
			aliasMap[a.Alias] = []model.ProjectAlias{}
		}
		aliasMap[a.Alias] = append(aliasMap[a.Alias], a)
	}

	tvPairs, err := p.BuildProjectTVPairsWithAlias(aliasMap[evergreen.GithubPRAlias], evergreen.GithubPRRequester)
	if err != nil {
		errs = append(errs, ValidationError{
			Message: "problem getting task variant pairs for PR aliases",
			Level:   Warning,
		})
		return errs
	}
	tvPairs.ExecTasks, err = model.IncludeDependencies(p, tvPairs.ExecTasks, evergreen.GithubPRRequester, nil)
	if err != nil {
		errs = append(errs, ValidationError{
			Message: "problem adding dependencies to PR alias tasks",
			Level:   Warning,
		})
		return errs
	}
	numCheckRuns := p.GetNumCheckRunsFromTaskVariantPairs(&tvPairs)
	checkRunLimit := evergreen.GetEnvironment().Settings().GitHubCheckRun.CheckRunLimit
	if numCheckRuns > checkRunLimit {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("total number of checkRuns (%d) exceeds maximum limit (%d)", numCheckRuns, checkRunLimit),
			Level:   Warning,
		})
	}
	return errs
}

func aliasMatchesTaskGroupTask(p *model.Project, alias model.ProjectAlias, tgName string) (bool, error) {
	tg := p.FindTaskGroup(tgName)
	if tg == nil {
		return false, errors.Errorf("definition for task group '%s' not found", tgName)
	}
	for _, tgTask := range tg.Tasks {
		t := p.FindProjectTask(tgTask)
		if t == nil {
			return false, errors.Errorf("task '%s' in task group '%s' not found", tgTask, tgName)
		}
		matchesTaskInTaskGroup, err := alias.HasMatchingTask(t.Name, t.Tags)
		if err != nil {
			return false, err
		}
		if matchesTaskInTaskGroup {
			return true, nil
		}
	}
	return false, nil
}

func validateProjectConfigPlugins(ctx context.Context, pc *model.ProjectConfig) ValidationErrors {
	errs := ValidationErrors{}
	annotationSettings := pc.TaskAnnotationSettings
	var webhook *evergreen.WebHook
	if annotationSettings != nil {
		webhook = &annotationSettings.FileTicketWebhook
	}
	// skip validation if no build baron configuration exists
	if pc.BuildBaronSettings == nil {
		return ValidationErrors{}
	}
	err := model.ValidateBbProject(ctx, pc.Project, *pc.BuildBaronSettings, webhook)
	if err != nil {
		errs = append(errs,
			ValidationError{
				Message: errors.Wrap(err, "error validating build baron config").Error(),
				Level:   Error,
			},
		)
	}
	return errs
}

func hasValidRunOn(runOn []string) bool {
	for _, d := range runOn {
		if d != "" {
			return true
		}
	}
	return false
}

// Ensures that the project has at least one buildvariant and also that all the
// fields required for any buildvariant definition are present
func validateBVFields(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	if len(project.BuildVariants) == 0 {
		return ValidationErrors{
			{
				Level:   Error,
				Message: "must specify at least one buildvariant",
			},
		}
	}

	for _, buildVariant := range project.BuildVariants {
		if buildVariant.Name == "" {
			errs = append(errs,
				ValidationError{
					Level:   Error,
					Message: "all buildvariants must have a name",
				},
			)
		}
		if len(buildVariant.Tasks) == 0 {
			errs = append(errs,
				ValidationError{
					Level: Error,
					Message: fmt.Sprintf("buildvariant '%s' must have at least one task",
						buildVariant.Name),
				},
			)
		}
		bvHasValidDistro := false
		for _, runOn := range buildVariant.RunOn {
			if runOn != "" {
				bvHasValidDistro = true
				break
			}
		}
		if bvHasValidDistro { // don't need to check if tasks have run_on defined since we have a variant default
			continue
		}

		for _, task := range buildVariant.Tasks {
			taskHasValidDistro := hasValidRunOn(task.RunOn)
			if taskHasValidDistro {
				break
			}
			if task.IsGroup {
				for _, t := range project.FindTaskGroup(task.Name).Tasks {
					pt := project.FindProjectTask(t)
					if pt != nil {
						if hasValidRunOn(pt.RunOn) {
							taskHasValidDistro = true
							break
						}
					}
				}
			} else {
				// check for a default in the task definition
				pt := project.FindProjectTask(task.Name)
				if pt != nil {
					taskHasValidDistro = hasValidRunOn(pt.RunOn)
				}
			}
			if !taskHasValidDistro {
				errs = append(errs,
					ValidationError{
						Level: Error,
						Message: fmt.Sprintf("buildvariant '%s' "+
							"must either specify run_on field or have every task specify run_on",
							buildVariant.Name),
					},
				)
				break
			}
		}
	}
	return errs
}

// Checks that the basic fields that are required by any project are present and
// valid.
func validateProjectFields(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}

	if project.CommandType != "" {
		if !utility.StringSliceContains(evergreen.ValidCommandTypes, project.CommandType) {
			errs = append(errs,
				ValidationError{
					Level:   Error,
					Message: fmt.Sprintf("invalid command type: %s", project.CommandType),
				},
			)
		}
	}
	return errs
}

func validateBuildVariantTaskNames(task string, variant string, allTaskNames map[string]bool, taskGroupTaskSet map[string]string) []ValidationError {
	var errs []ValidationError
	if _, ok := allTaskNames[task]; !ok {
		if task == "" {
			errs = append(errs, ValidationError{
				Message: fmt.Sprintf("tasks for buildvariant '%s' must each have a name field",
					variant),
				Level: Error,
			})

		} else {
			errs = append(errs, ValidationError{
				Message: fmt.Sprintf("buildvariant '%s' references a non-existent task '%s'",
					variant, task),
				Level: Error,
			})
		}
	}
	return errs
}

func matchTaskToAllowlist(allowlist []string, taskName string) (bool, []ValidationError) {
	errs := []ValidationError{}
	for _, allowedTask := range allowlist {
		matched, err := regexp.MatchString(allowedTask, taskName)
		if err != nil {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("invalid task regex '%s'", allowedTask),
					Level:   Warning,
				},
			)
		}
		if matched {
			return true, errs
		}
	}
	return false, errs
}

// ensureReferentialIntegrity checks all fields that reference other entities defined in the YAML and ensure that they are referring to valid names,
// and returns any relevant distro validation info.
// distroWarnings are considered validation notices.
func ensureReferentialIntegrity(project *model.Project, distroIDs, distroAliases, singleTaskDistroIDs []string, singleTaskDistroAllowlist evergreen.ProjectTasksPair, distroWarnings map[string]string) ValidationErrors {
	errs := ValidationErrors{}
	// create a set of all the task names
	allTaskNames := map[string]bool{}
	taskGroupTaskSet := map[string]string{}
	for _, task := range project.Tasks {
		allTaskNames[task.Name] = true
	}
	for _, taskGroup := range project.TaskGroups {
		allTaskNames[taskGroup.Name] = true
		for _, task := range taskGroup.Tasks {
			taskGroupTaskSet[task] = taskGroup.Name
		}
	}

	// create a list of tasks that call the generate task command
	generateTasksMap := project.TasksThatCallCommand(evergreen.GenerateTasksCommandName)
	generateTasks := slices.Collect(maps.Keys(generateTasksMap))

	for _, buildVariant := range project.BuildVariants {
		buildVariantTasks := map[string]bool{}
		for _, task := range buildVariant.Tasks {
			errs = append(errs, validateBuildVariantTaskNames(task.Name, buildVariant.Name, allTaskNames, taskGroupTaskSet)...)
			if _, ok := taskGroupTaskSet[task.Name]; ok {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("task '%s' in build variant '%s' is already referenced in task group '%s'",
							task.Name, buildVariant.Name, taskGroupTaskSet[task.Name]),
						Level: Warning,
					})
			}
			buildVariantTasks[task.Name] = true
			runOnHasDistro := false
			runOnHasContainer := false
			for _, name := range task.RunOn {
				if !utility.StringSliceContains(distroIDs, name) && !utility.StringSliceContains(distroAliases, name) {
					errs = append(errs,
						ValidationError{
							Message: fmt.Sprintf("task '%s' in buildvariant '%s' references a nonexistent distro or container named '%s'",
								task.Name, buildVariant.Name, name),
							Level: Warning,
						},
					)
				}

				validateSingleTaskDistros, allowAll, errorLevel, warnings := shouldValidateSingleTaskDistros(project.Identifier, singleTaskDistroAllowlist, singleTaskDistroIDs, name)
				errs = append(errs, warnings...)
				// Validate if the task uses a single task distro and not all tasks are allowed.
				if validateSingleTaskDistros && !allowAll {
					matchedTask, warnings := matchTaskToAllowlist(singleTaskDistroAllowlist.AllowedTasks, task.Name)
					errs = append(errs, warnings...)
					matchedBV, warnings := matchTaskToAllowlist(singleTaskDistroAllowlist.AllowedBVs, task.Variant)
					errs = append(errs, warnings...)
					if !(matchedTask || matchedBV) {
						errs = append(errs,
							ValidationError{
								Message: fmt.Sprintf("task '%s' in buildvariant '%s' references a single task distro '%s' that is not allowed for this task or variant",
									task.Name, buildVariant.Name, name),
								Level: errorLevel,
							},
						)
					}
				}
				if validateSingleTaskDistros {
					// Single task distros cannot use generate tasks
					if utility.StringSliceContains(generateTasks, task.Name) {
						errs = append(errs,
							ValidationError{
								Message: fmt.Sprintf("task '%s' in buildvariant '%s' runs on a single task distro '%s' and cannot use the generate tasks command",
									task.Name, buildVariant.Name, name),
								Level: errorLevel,
							},
						)
					}
				}

				if warning, ok := distroWarnings[name]; ok {
					errs = append(errs,
						ValidationError{
							Message: fmt.Sprintf("task '%s' in buildvariant '%s' "+
								"references distro '%s' with the following admin-defined warning(s): %s",
								task.Name, buildVariant.Name, name, warning),
							Level: Notice,
						},
					)
				}
				if utility.StringSliceContains(distroIDs, name) {
					runOnHasDistro = true
				}
			}
			errs = append(errs, checkRunOn(runOnHasDistro, runOnHasContainer, task.RunOn)...)
		}
		for _, dt := range buildVariant.DisplayTasks {
			for _, execTask := range dt.ExecTasks {
				if _, ok := allTaskNames[execTask]; !ok {
					errs = append(errs,
						ValidationError{
							Level: Error,
							Message: fmt.Sprintf("display task '%s' in buildvariant '%s' references a non-existent execution task '%s'",
								dt.Name, buildVariant.Name, execTask),
						},
					)
				}
			}
		}
		runOnHasDistro := false
		runOnHasContainer := false
		for _, name := range buildVariant.RunOn {
			if !utility.StringSliceContains(distroIDs, name) && !utility.StringSliceContains(distroAliases, name) {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("buildvariant '%s' references a nonexistent distro or container named '%s'",
							buildVariant.Name, name),
						Level: Warning,
					},
				)
			}

			shouldValidateSingleTaskDistros, allowAll, errorLevel, warnings := shouldValidateSingleTaskDistros(project.Identifier, singleTaskDistroAllowlist, singleTaskDistroIDs, name)
			errs = append(errs, warnings...)
			// Validate if the task uses a single task distro and not all tasks are allowed.
			if shouldValidateSingleTaskDistros && !allowAll {
				matched, warnings := matchTaskToAllowlist(singleTaskDistroAllowlist.AllowedBVs, buildVariant.Name)
				errs = append(errs, warnings...)
				if !matched {
					errs = append(errs,
						ValidationError{
							Message: fmt.Sprintf("buildvariant '%s' references a single task distro '%s' that is not allowed for this variant",
								buildVariant.Name, name),
							Level: errorLevel,
						},
					)
				}
			}
			if shouldValidateSingleTaskDistros {
				// Verify tasks that inherit the bv single task distros do not use generate tasks.
				for _, task := range buildVariant.Tasks {
					if task.RunOn == nil && utility.StringSliceContains(generateTasks, task.Name) {
						errs = append(errs,
							ValidationError{
								Message: fmt.Sprintf("task '%s' in buildvariant '%s' runs on a single task distro '%s' and cannot use the generate tasks command",
									task.Name, buildVariant.Name, name),
								Level: errorLevel,
							},
						)
					}
				}
			}

			if warning, ok := distroWarnings[name]; ok {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("buildvariant '%s' "+
							"references distro '%s' with the following admin-defined warning: %s",
							buildVariant.Name, name, warning),
						Level: Notice,
					},
				)
			}
			if utility.StringSliceContains(distroIDs, name) {
				runOnHasDistro = true
			}
		}
		errs = append(errs, checkRunOn(runOnHasDistro, runOnHasContainer, buildVariant.RunOn)...)
	}
	return errs
}

func checkRunOn(runOnHasDistro, runOnHasContainer bool, runOn []string) []ValidationError {
	if runOnHasContainer && runOnHasDistro {
		return []ValidationError{{
			Message: "run_on cannot contain a mixture of containers and distros",
			Level:   Error,
		}}

	} else if runOnHasContainer && len(runOn) > 1 {
		return []ValidationError{{
			Message: "only one container can be used from run_on; the first container in the list will be used",
			Level:   Warning,
		}}
	}
	return nil
}

func shouldValidateSingleTaskDistros(projectIdentifier string, singleTaskDistroAllowlist evergreen.ProjectTasksPair, singleTaskDistroIDs []string, distroName string) (bool, bool, ValidationErrorLevel, []ValidationError) {
	allowAll := singleTaskDistroAllowlist.AllowAll()
	shouldValidate := slices.Contains(singleTaskDistroIDs, distroName)
	if shouldValidate && projectIdentifier == "" {
		return shouldValidate, allowAll, Warning, []ValidationError{
			{
				Message: "project not specified, skipping single task distro validation",
				Level:   Warning,
			}}
	}

	return shouldValidate, allowAll, Error, []ValidationError{}
}

func validateTimeoutLimits(_ context.Context, settings *evergreen.Settings, project *model.Project, _ *model.ProjectRef, _ bool) ValidationErrors {
	errs := ValidationErrors{}
	if settings.TaskLimits.MaxExecTimeoutSecs > 0 {
		for _, task := range project.Tasks {
			if task.ExecTimeoutSecs > settings.TaskLimits.MaxExecTimeoutSecs {
				errs = append(errs, ValidationError{
					Message: fmt.Sprintf("task '%s' exec timeout (%d) is too high and will be set to maximum limit (%d)", task.Name, task.ExecTimeoutSecs, settings.TaskLimits.MaxExecTimeoutSecs),
					Level:   Error,
				})
			}
		}
	}
	return errs
}

func validateReferentialIntegrity(ctx context.Context, settings *evergreen.Settings, p *model.Project, ref *model.ProjectRef, _ bool) ValidationErrors {
	validationErrs := ValidationErrors{}
	singleTaskDistroAllowlist, err := GetAllowedSingleTaskDistroTasksForProject(ctx, ref.Id, settings)
	if err != nil {
		return []ValidationError{{Message: errors.Wrap(err, "problem getting allowed tasks for single task distros").Error()}}
	}
	// get distro IDs and aliases for ensureReferentialIntegrity validation
	distroIDs, distroAliases, singleTaskDistroIDs, distroWarnings, err := getDistrosForProject(ctx, ref.Id)
	if err != nil {
		validationErrs = append(validationErrs, ValidationError{Message: "can't get distros from database"})
	}
	validationErrs = append(validationErrs, ensureReferentialIntegrity(p, distroIDs, distroAliases, singleTaskDistroIDs, singleTaskDistroAllowlist, distroWarnings)...)
	return validationErrs
}

func validateIncludeLimits(_ context.Context, settings *evergreen.Settings, project *model.Project, _ *model.ProjectRef, _ bool) ValidationErrors {
	errs := ValidationErrors{}
	if settings.TaskLimits.MaxIncludesPerVersion > 0 && project.NumIncludes > settings.TaskLimits.MaxIncludesPerVersion {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("project's total number of includes (%d) exceeds maximum limit (%d)", project.NumIncludes, settings.TaskLimits.MaxIncludesPerVersion),
			Level:   Error,
		})
	}
	return errs
}

func validateProjectLimits(_ context.Context, settings *evergreen.Settings, project *model.Project, _ *model.ProjectRef, _ bool) ValidationErrors {
	bvTasks := project.FindAllBuildVariantTasks()
	errs := ValidationErrors{}
	if settings.TaskLimits.MaxTasksPerVersion > 0 && len(bvTasks) > settings.TaskLimits.MaxTasksPerVersion {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("project's total number of tasks (%d) exceeds maximum limit (%d)", len(bvTasks), settings.TaskLimits.MaxTasksPerVersion),
			Level:   Error,
		})
	}
	return errs
}

// validateTaskNames ensures the task names do not contain unauthorized characters.
func validateTaskNames(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}

	for _, task := range project.Tasks {
		// Add space to the list of unauthorized characters to prevent task names from containing spaces.
		if strings.ContainsAny(strings.TrimSpace(task.Name), strings.Join(unauthorizedCharacters, " ")) {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("task name '%s' contains unauthorized characters (%s)",
						task.Name, unauthorizedCharacters),
					Level: Error,
				})
		}
	}
	return errs
}

func checkTaskNames(project *model.Project, task *model.ProjectTask) ValidationErrors {
	errs := ValidationErrors{}
	// Warn against commas because the CLI allows users to specify
	// tasks separated by commas in their patches.
	if strings.Contains(task.Name, ",") {
		errs = append(errs, ValidationError{
			Level:   Warning,
			Message: fmt.Sprintf("task name '%s' should not contain commas", task.Name),
		})
	}
	// Warn against using "*" since it is ambiguous with the
	// all-dependencies specification (also "*").
	if task.Name == model.AllDependencies {
		errs = append(errs, ValidationError{
			Level:   Warning,
			Message: "task should not be named '*' because it is ambiguous with the all-dependencies '*' specification",
		})
	}
	// Warn against using "all" since it is ambiguous with the special "all"
	// task specifier when creating patches.
	if task.Name == "all" {
		errs = append(errs, ValidationError{
			Level:   Warning,
			Message: "task should not be named 'all' because it is ambiguous in task specifications for patches",
		})
	}
	return errs
}

// checkModules checks to make sure that the module's name, branch, and repo are correct.
func checkModules(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	moduleNames := map[string]bool{}

	for _, module := range project.Modules {
		// Warn if name is a duplicate or empty
		if module.Name == "" {
			errs = append(errs, ValidationError{
				Level:   Warning,
				Message: "module cannot have an empty name",
			})
		} else if moduleNames[module.Name] {
			errs = append(errs, ValidationError{
				Level:   Warning,
				Message: fmt.Sprintf("module '%s' already exists; the first module name defined will be used", module.Name),
			})
		} else {
			moduleNames[module.Name] = true
		}

		// Warn if branch is empty
		if module.Branch == "" {
			errs = append(errs, ValidationError{
				Level:   Warning,
				Message: fmt.Sprintf("module '%s' should have a set branch", module.Name),
			})
		}

		if module.Owner == "" {
			// Warn if repo is empty or does not conform to Git URL format
			owner, repo, err := thirdparty.ParseGitUrl(module.Repo)
			if err != nil {
				errs = append(errs, ValidationError{
					Level:   Warning,
					Message: errors.Wrapf(err, "module '%s' does not have a valid repo URL format", module.Name).Error(),
				})
			} else if owner == "" || repo == "" {
				errs = append(errs, ValidationError{
					Level:   Warning,
					Message: fmt.Sprintf("module '%s' repo '%s' is missing an owner or repo name", module.Name, module.Repo),
				})
			}
		} else if !strings.Contains(module.Repo, "git@github.com:") && module.Owner == "" {
			errs = append(errs, ValidationError{
				Level:   Warning,
				Message: fmt.Sprintf("module '%s' is missing an owner", module.Name),
			})

		}

		if module.Repo == "" {
			errs = append(errs, ValidationError{
				Level:   Warning,
				Message: fmt.Sprintf("module '%s' should have a set repo", module.Name),
			})
		}

	}

	return errs
}

// Ensures there aren't any duplicate buildvariant names specified in the given
// project and that the names do not contain unauthorized characters.
func validateBVNames(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	buildVariantNames := map[string]bool{}

	for _, buildVariant := range project.BuildVariants {
		if _, ok := buildVariantNames[buildVariant.Name]; ok {
			errs = append(errs,
				ValidationError{
					Level:   Error,
					Message: fmt.Sprintf("buildvariant '%s' already exists", buildVariant.Name),
				},
			)
		}
		buildVariantNames[buildVariant.Name] = true
		dispName := buildVariant.DisplayName
		if dispName == "" {
			errs = append(errs,
				ValidationError{
					Level:   Error,
					Message: fmt.Sprintf("buildvariant '%s' does not have a display name", buildVariant.Name),
				},
			)
		}

		if strings.ContainsAny(buildVariant.Name, strings.Join(unauthorizedCharacters, "")) {
			errs = append(errs,
				ValidationError{
					Level: Error,
					Message: fmt.Sprintf("buildvariant name '%s' contains unauthorized characters (%s)",
						buildVariant.Name, unauthorizedCharacters),
				})
		}

	}

	return errs
}

func checkBVNames(buildVariant *model.BuildVariant) ValidationErrors {
	errs := ValidationErrors{}

	// Warn against commas because the CLI allows users to specify
	// variants separated by commas in their patches.
	if strings.Contains(buildVariant.Name, ",") {
		errs = append(errs, ValidationError{
			Level:   Warning,
			Message: fmt.Sprintf("buildvariant name '%s' should not contains commas", buildVariant.Name),
		})
	}
	// Warn against using "*" since it is ambiguous with the
	// all-dependencies specification (also "*").
	if buildVariant.Name == model.AllVariants {
		errs = append(errs, ValidationError{
			Level:   Warning,
			Message: "buildvariant should not be named '*' because it is ambiguous with the all-variants '*' specification",
		})
	}
	// Warn against using "all" since it is ambiguous with the special "all"
	// task specifier when creating patches.
	if buildVariant.Name == "all" {
		errs = append(errs, ValidationError{
			Level:   Warning,
			Message: "buildvariant should not be named 'all' because it is ambiguous in buildvariant specifications for patches",
		})
	}

	return errs
}

// Ensures there aren't any duplicate task names specified for any buildvariant
// in this project
func validateBVTaskNames(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	for _, buildVariant := range project.BuildVariants {
		buildVariantTasks := map[string]bool{}
		for _, task := range buildVariant.Tasks {
			if _, ok := buildVariantTasks[task.Name]; ok {
				errs = append(errs,
					ValidationError{
						Level: Error,
						Message: fmt.Sprintf("task '%s' in buildvariant '%s' already exists",
							task.Name, buildVariant.Name),
					},
				)
			}
			buildVariantTasks[task.Name] = true
		}
	}
	return errs
}

func validateBVBatchTimes(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	for _, buildVariant := range project.BuildVariants {
		// check task batchtimes first
		for _, t := range buildVariant.Tasks {

			if t.CronBatchTime == "" {
				continue
			}
			// otherwise, cron batchtime is set
			if t.BatchTime != nil {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("task '%s' cannot specify cron and batchtime for variant '%s'", t.Name, buildVariant.Name),
						Level:   Error,
					})
			}
			if _, err := model.GetNextCronTime(time.Now(), t.CronBatchTime); err != nil {
				errs = append(errs,
					ValidationError{
						Message: errors.Wrapf(err, "task cron batchtime '%s' has invalid syntax for task '%s' for build variant '%s'",
							t.CronBatchTime, t.Name, buildVariant.Name).Error(),
						Level: Error,
					},
				)
			}
		}

		if buildVariant.CronBatchTime == "" {
			continue
		}
		if buildVariant.BatchTime != nil {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("variant '%s' cannot specify cron and batchtime", buildVariant.Name),
					Level:   Error,
				})
		}
		if _, err := model.GetNextCronTime(time.Now(), buildVariant.CronBatchTime); err != nil {
			errs = append(errs,
				ValidationError{
					Message: errors.Wrapf(err, "cron batchtime '%s' has invalid syntax", buildVariant.CronBatchTime).Error(),
					Level:   Error,
				},
			)
		}
	}
	return errs
}

func checkBVBatchTimes(buildVariant *model.BuildVariant) ValidationErrors {
	errs := ValidationErrors{}
	// check task batchtimes first
	for _, t := range buildVariant.Tasks {
		// setting activate explicitly to true with batchtime will use batchtime
		if utility.FromBoolPtr(t.Activate) && (t.CronBatchTime != "" || t.BatchTime != nil) {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("task '%s' for variant '%s' activation ignored since batchtime or cron specified",
						t.Name, buildVariant.Name),
					Level: Warning,
				})
		}

	}

	if utility.FromBoolPtr(buildVariant.Activate) && (buildVariant.CronBatchTime != "" || buildVariant.BatchTime != nil) {
		errs = append(errs,
			ValidationError{
				Message: fmt.Sprintf("variant '%s' activation ignored since batchtime specified", buildVariant.Name),
				Level:   Warning,
			})
	}

	return errs
}

func checkBVTaskPriority(buildVariant *model.BuildVariant) ValidationErrors {
	errs := ValidationErrors{}
	for _, t := range buildVariant.Tasks {
		if t.Priority > model.MaxConfigSetPriority {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("task '%s' has been set above %d priority in build variant '%s', in YAML, will default priority to %d",
						t.Name, model.MaxConfigSetPriority, buildVariant.Name, model.MaxConfigSetPriority),
					Level: Notice,
				})
		}
	}
	return errs
}

func validateDisplayTaskNames(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}

	// build a map of task names
	tn := map[string]struct{}{}
	for _, t := range project.Tasks {
		tn[t.Name] = struct{}{}
	}

	// check display tasks
	for _, bv := range project.BuildVariants {
		for _, dp := range bv.DisplayTasks {
			for _, etn := range dp.ExecTasks {
				if strings.HasPrefix(etn, "display_") {
					errs = append(errs,
						ValidationError{
							Level:   Error,
							Message: fmt.Sprintf("execution task '%s' has prefix 'display_' which is invalid", etn),
						})
				}
			}
		}
	}
	return errs
}

// Helper for validating a set of plugin commands given a project/registry
func validateCommands(section, taskName, tgName string, project *model.Project, commands []model.PluginCommandConf) ValidationErrors {
	errs := ValidationErrors{}
	var formattedTaskMsg string
	if taskName != "" {
		formattedTaskMsg = fmt.Sprintf(" for task '%s'", taskName)
	} else if tgName != "" {
		formattedTaskMsg += fmt.Sprintf(" for task group '%s'", tgName)
	}

	for i, cmd := range commands {
		hasFuncOrCommand := true
		if cmd.Function == "" && cmd.Command == "" {
			hasFuncOrCommand = false
			errs = append(errs, ValidationError{
				Level:   Error,
				Message: fmt.Sprintf("must specify either command or function%s", formattedTaskMsg),
			})
		}
		if cmd.Function != "" && cmd.Command != "" {
			errs = append(errs, ValidationError{
				Level:   Error,
				Message: fmt.Sprintf("cannot specify both command '%s' and function '%s'%s", cmd.Command, cmd.Function, formattedTaskMsg),
			})
		}
		if cmd.Function != "" && cmd.RetryOnFailure {
			errs = append(errs, ValidationError{
				Level:   Notice,
				Message: fmt.Sprintf("cannot specify retry_on_failure with function '%s'%s, can only specify retry_on_failure on individual commands", cmd.Function, formattedTaskMsg),
			})
		}
		if hasFuncOrCommand {
			commandName := fmt.Sprintf("'%s' command", cmd.Command)
			if cmd.Function != "" {
				commandName = fmt.Sprintf("'%s' function", cmd.Function)
			}
			blockInfo := command.BlockInfo{
				Block:     "",
				CmdNum:    i + 1,
				TotalCmds: len(commands),
			}
			_, err := command.Render(cmd, project, blockInfo)
			if err != nil {
				errs = append(errs, ValidationError{Message: fmt.Sprintf("%s section%s in %s: %s", section, formattedTaskMsg, commandName, err)})
			}
			if cmd.Type != "" {
				if !utility.StringSliceContains(evergreen.ValidCommandTypes, cmd.Type) {
					msg := fmt.Sprintf("%s section%s in %s: invalid command type: '%s'", section, formattedTaskMsg, commandName, cmd.Type)
					errs = append(errs, ValidationError{Message: msg})
				}
			}
		}
	}
	return errs
}

// Ensures there any plugin commands referenced in a project's configuration
// are specified in a valid format
func validatePluginCommands(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	seen := make(map[string]bool)

	// validate each function definition
	for funcName, commands := range project.Functions {
		if commands == nil || len(commands.List()) == 0 {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("'%s' function contains no commands", funcName),
					Level:   Error,
				},
			)
			continue
		}
		valErrs := validateCommands("functions", "", "", project, commands.List())
		for _, err := range valErrs {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("'%s' definition error: %s", funcName, err.Message),
					Level:   err.Level,
				},
			)
		}

		for _, c := range commands.List() {
			if c.Function != "" {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("can not reference a function within a "+
							"function: '%s' referenced within '%s'", c.Function, funcName),
					},
				)

			}
		}

		// this checks for duplicate function definitions in the project.
		if seen[funcName] {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf(`duplicate definition of "%s"`, funcName),
				},
			)
		}
		seen[funcName] = true
	}

	if project.Pre != nil {
		errs = append(errs, validateCommands("pre", "", "", project, project.Pre.List())...)
	}

	if project.Post != nil {
		errs = append(errs, validateCommands("post", "", "", project, project.Post.List())...)
	}

	if project.Timeout != nil {
		errs = append(errs, validateCommands("timeout", "", "", project, project.Timeout.List())...)
	}

	// validate project tasks section
	for _, task := range project.Tasks {
		errs = append(errs, validateCommands("tasks", task.Name, "", project, task.Commands)...)
	}

	for _, tg := range project.TaskGroups {
		if tg.SetupGroup != nil {
			errs = append(errs, validateCommands("setup_group", "", tg.Name, project, tg.SetupGroup.List())...)
		}
		if tg.SetupTask != nil {
			errs = append(errs, validateCommands("setup_task", "", tg.Name, project, tg.SetupTask.List())...)
		}
		if tg.TeardownTask != nil {
			errs = append(errs, validateCommands("teardown_task", "", tg.Name, project, tg.TeardownTask.List())...)
		}
		if tg.TeardownGroup != nil {
			errs = append(errs, validateCommands("teardown_group", "", tg.Name, project, tg.TeardownGroup.List())...)
		}
		if tg.Timeout != nil {
			errs = append(errs, validateCommands("timeout", "", tg.Name, project, tg.Timeout.List())...)
		}
	}

	return errs
}

// Ensures there aren't any duplicate task names for this project
func validateProjectTaskNames(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	// create a map to hold the task names
	taskNames := map[string]bool{}
	for _, task := range project.Tasks {
		if _, ok := taskNames[task.Name]; ok {
			errs = append(errs,
				ValidationError{
					Level:   Error,
					Message: fmt.Sprintf("task '%s' already exists", task.Name),
				},
			)
		}
		taskNames[task.Name] = true
	}
	return errs
}

// validateProjectTaskIdsAndTags ensures that task tags and ids only contain valid characters
func validateProjectTaskIdsAndTags(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	// create a map to hold the task names
	for _, task := range project.Tasks {
		// check task name
		if i := strings.IndexAny(task.Name, model.InvalidCriterionRunes); i == 0 {
			errs = append(errs, ValidationError{
				Level: Error,
				Message: fmt.Sprintf("task '%s' has invalid name: starts with invalid character %s",
					task.Name, strconv.QuoteRune(rune(task.Name[0])))})
		}
		// check tag names
		for _, tag := range task.Tags {
			if i := strings.IndexAny(tag, model.InvalidCriterionRunes); i == 0 {
				errs = append(errs, ValidationError{
					Level: Error,
					Message: fmt.Sprintf("task '%s' has invalid tag '%s': starts with invalid character %s",
						task.Name, tag, strconv.QuoteRune(rune(tag[0])))})
			}
			if i := util.IndexWhiteSpace(tag); i != -1 {
				errs = append(errs, ValidationError{
					Level: Error,
					Message: fmt.Sprintf("task '%s' has invalid tag '%s': tag contains white space",
						task.Name, tag)})
			}
		}
	}
	return errs
}

func checkTaskRuns(project *model.Project) ValidationErrors {
	var errs ValidationErrors
	for _, bvtu := range project.FindAllBuildVariantTasks() {
		hasValidAllowedRequester := len(bvtu.AllowedRequesters) == 0
		if len(bvtu.AllowedRequesters) != 0 {
			if bvtu.PatchOnly != nil {
				errs = append(errs, ValidationError{
					Level: Warning,
					Message: fmt.Sprintf("task '%s' in build variant '%s' specifies both allowed_requesters and patch_only, but allowed_requesters is always higher precedence",
						bvtu.Name, bvtu.Variant),
				})
			}
			if bvtu.Patchable != nil {
				errs = append(errs, ValidationError{
					Level: Warning,
					Message: fmt.Sprintf("task '%s' in build variant '%s' specifies both allowed_requesters and patchable, but allowed_requesters is always higher precedence",
						bvtu.Name, bvtu.Variant),
				})
			}
			if bvtu.GitTagOnly != nil {
				errs = append(errs, ValidationError{
					Level: Warning,
					Message: fmt.Sprintf("task '%s' in build variant '%s' specifies both allowed_requesters and git_tag_only, but allowed_requesters is always higher precedence",
						bvtu.Name, bvtu.Variant),
				})
			}
			if bvtu.AllowForGitTag != nil {
				errs = append(errs, ValidationError{
					Level: Warning,
					Message: fmt.Sprintf("task '%s' in build variant '%s' specifies both allowed_requesters and allow_for_git_tag, but allowed_requesters is always higher precedence",
						bvtu.Name, bvtu.Variant),
				})
			}
			for _, requester := range bvtu.AllowedRequesters {
				if requester.Validate() != nil {
					errs = append(errs, ValidationError{
						Level: Warning,
						Message: fmt.Sprintf("task '%s' in build variant '%s' specifies invalid allowed_requester '%s'",
							bvtu.Name, bvtu.Variant, requester),
					})
				} else {
					hasValidAllowedRequester = true
				}
			}
		}

		if hasValidAllowedRequester {
			if bvtu.SkipOnPatchBuild() && bvtu.SkipOnNonPatchBuild() {
				errs = append(errs, ValidationError{
					Level: Warning,
					Message: fmt.Sprintf("task '%s' in build variant '%s' will never run because it skips both patch builds and non-patch builds",
						bvtu.Name, bvtu.Variant),
				})
			}
			if bvtu.SkipOnGitTagBuild() && bvtu.SkipOnNonGitTagBuild() {
				errs = append(errs, ValidationError{
					Level: Warning,
					Message: fmt.Sprintf("task '%s' in build variant '%s' will never run because it skips both git tag builds and non git tag builds",
						bvtu.Name, bvtu.Variant),
				})
			}
			// Git-tag-only builds cannot run in patches.
			if bvtu.SkipOnNonGitTagBuild() && bvtu.SkipOnNonPatchBuild() {
				errs = append(errs, ValidationError{
					Level: Warning,
					Message: fmt.Sprintf("task '%s' in build variant '%s' will never run because it only runs for git tag builds but also is patch-only",
						bvtu.Name, bvtu.Variant),
				})
			}
			if bvtu.SkipOnNonGitTagBuild() && utility.FromBoolPtr(bvtu.Patchable) {
				errs = append(errs, ValidationError{
					Level: Warning,
					Message: fmt.Sprintf("task '%s' in build variant '%s' cannot be patchable if it only runs for git tag builds",
						bvtu.Name, bvtu.Variant),
				})
			}
		}
	}
	return errs
}

// validateStatusesForTaskDependencies checks that, for all tasks that have
// the status field is a valid status.
func validateStatusesForTaskDependencies(project *model.Project) ValidationErrors {
	var errs ValidationErrors
	for _, bvtu := range project.FindAllBuildVariantTasks() {
		for _, d := range bvtu.DependsOn {
			validDepStatuses := []string{evergreen.TaskSucceeded, evergreen.TaskFailed, model.AllStatuses, ""}
			if !utility.StringSliceContains(validDepStatuses, d.Status) {
				errs = append(errs,
					ValidationError{
						Level:   Error,
						Message: fmt.Sprintf("invalid dependency status '%s' for task '%s' in build variant '%s'", d.Status, d.Name, bvtu.Variant),
					},
				)
			}
		}
	}
	return errs
}

// checkReferencesForTaskDependencies checks that, for all tasks that have
// dependencies, those dependencies set the expected fields and all dependencies
// reference tasks that will actually run. For example, if task t1 in build
// variant bv1 depends on task t2, t2 should also be listed under bv1.
func checkReferencesForTaskDependencies(project *model.Project) ValidationErrors {
	bvtus := map[model.TVPair]model.BuildVariantTaskUnit{}
	bvs := map[string]struct{}{}
	tasks := map[string]struct{}{}
	for _, bvtu := range project.FindAllBuildVariantTasks() {
		bvtus[model.TVPair{Variant: bvtu.Variant, TaskName: bvtu.Name}] = bvtu
		bvs[bvtu.Variant] = struct{}{}
		tasks[bvtu.Name] = struct{}{}
	}

	var errs ValidationErrors
	for _, bvtu := range bvtus {
		for _, d := range bvtu.DependsOn {
			// Dependencies can be specified in different places, which can
			// overwrite each other. Each build variant task unit already takes
			// into account these precedence rules, so after resolving the
			// dependencies defined at the different levels, check that the
			// final dependencies all reference valid tasks.

			dep := model.TVPair{Variant: d.Variant, TaskName: d.Name}

			if dep.Variant == "" {
				// Implicit build variant - if no build variant is explicitly
				// stated, the task and dependency should both run in the same
				// build variant.
				dep.Variant = bvtu.Variant
			}

			if dep.TaskName == model.AllDependencies && dep.Variant == model.AllVariants {
				// If it depends on all other tasks, there's no referential
				// integrity to check because it can depend on any other task.
				continue
			}
			if dep.TaskName == model.AllDependencies {
				// If it depends on all tasks in a specific build variant, make
				// sure the build variant exists.
				if _, ok := bvs[dep.Variant]; !ok {
					errs = append(errs, ValidationError{
						Level:   Warning,
						Message: fmt.Sprintf("task '%s' in build variant '%s' depends on '%s' tasks in build variant '%s', but the build variant was not found", bvtu.Name, bvtu.Variant, dep.TaskName, dep.Variant),
					})
				}
				continue
			}
			if dep.Variant == model.AllDependencies {
				// If it depends on a specific task in all build variants, make
				// sure at least one build variant lists the task.
				if _, ok := tasks[dep.TaskName]; !ok {
					errs = append(errs, ValidationError{
						Level:   Warning,
						Message: fmt.Sprintf("task '%s' in build variant '%s' depends on task '%s' in '%s' build variants, but no build variant contains that task", bvtu.Name, bvtu.Variant, dep.TaskName, dep.Variant),
					})
				}
				continue
			}

			// If it depends on a specific task and build variant, check that
			// the build variant contains that task.
			if _, ok := bvtus[dep]; !ok {
				errs = append(errs, ValidationError{
					Level:   Warning,
					Message: fmt.Sprintf("task '%s' in build variant '%s' depends on task '%s' in build variant '%s', but it was not found", bvtu.Name, bvtu.Variant, dep.TaskName, dep.Variant),
				})
			}
		}
	}

	return errs
}

// checkRequestersForTaskDependencies checks that each task's dependencies will
// run for the same requesters. For example, a task that runs in a mainline
// commit cannot depend on a patch only task since the dependency will only be
// satisfiable in patches.
func checkRequestersForTaskDependencies(project *model.Project) ValidationErrors {
	bvtus := map[model.TVPair]model.BuildVariantTaskUnit{}
	bvs := map[string]struct{}{}
	tasks := map[string]struct{}{}
	for _, bvtu := range project.FindAllBuildVariantTasks() {
		bvtus[model.TVPair{Variant: bvtu.Variant, TaskName: bvtu.Name}] = bvtu
		bvs[bvtu.Variant] = struct{}{}
		tasks[bvtu.Name] = struct{}{}
	}

	var errs ValidationErrors
	for _, bvtu := range bvtus {
		for _, d := range bvtu.DependsOn {
			dep := model.TVPair{Variant: d.Variant, TaskName: d.Name}
			if dep.Variant == "" {
				// Implicit build variant - if no build variant is explicitly
				// stated, the task and dependency should both run in the same
				// build variant.
				dep.Variant = bvtu.Variant
			}
			dependent := project.FindTaskForVariant(dep.TaskName, dep.Variant)
			if dependent == nil {
				continue
			}
			if utility.FromBoolPtr(dependent.PatchOnly) && !utility.FromBoolPtr(bvtu.PatchOnly) {
				errs = append(errs, ValidationError{
					Level:   Warning,
					Message: fmt.Sprintf("Task '%s' depends on patch-only task '%s'. Both will only run in patches", bvtu.Name, d.Name),
				})
			}
			if !utility.FromBoolTPtr(dependent.Patchable) && utility.FromBoolTPtr(bvtu.Patchable) {
				errs = append(errs, ValidationError{
					Level:   Warning,
					Message: fmt.Sprintf("Task '%s' depends on non-patchable task '%s'. Neither will run in patches", bvtu.Name, d.Name),
				})
			}
			if utility.FromBoolPtr(dependent.GitTagOnly) && !utility.FromBoolPtr(bvtu.GitTagOnly) {
				errs = append(errs, ValidationError{
					Level:   Warning,
					Message: fmt.Sprintf("Task '%s' depends on git-tag-only task '%s'. Both will only run when pushing git tags", bvtu.Name, d.Name),
				})
			}
		}
	}

	return errs
}

func validateParameters(p *model.Project) ValidationErrors {
	errs := ValidationErrors{}

	names := map[string]bool{}
	for _, param := range p.Parameters {
		if _, ok := names[param.Parameter.Key]; ok {
			errs = append(errs, ValidationError{
				Level:   Error,
				Message: fmt.Sprintf("parameter '%s' is defined multiple times", param.Parameter.Key),
			})
			names[param.Parameter.Key] = true
		}
		if strings.Contains(param.Parameter.Key, "=") {
			errs = append(errs, ValidationError{
				Level:   Error,
				Message: fmt.Sprintf("parameter name '%s' cannot contain `=`", param.Parameter.Key),
			})
		}
		if param.Parameter.Key == "" {
			errs = append(errs, ValidationError{
				Level:   Error,
				Message: "parameter name is missing",
			})
		}
	}
	return errs
}

func validateTaskGroups(p *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	for _, tg := range p.TaskGroups {
		// validate that there is at least 1 task
		if len(tg.Tasks) < 1 {
			errs = append(errs, ValidationError{
				Message: fmt.Sprintf("task group %s must have at least 1 task", tg.Name),
				Level:   Error,
			})
		}
		// validate that the task group is not named the same as a task
		for _, t := range p.Tasks {
			if t.Name == tg.Name {
				errs = append(errs, ValidationError{
					Message: fmt.Sprintf("%s is used as a name for both a task and task group", t.Name),
					Level:   Error,
				})
			}
		}
		// validate that a task is not listed twice in a task group
		counts := make(map[string]int)
		for _, name := range tg.Tasks {
			counts[name]++
		}
		for name, count := range counts {
			if count > 1 {
				errs = append(errs, ValidationError{
					Message: fmt.Sprintf("%s is listed in task group %s %d times", name, tg.Name, count),
					Level:   Error,
				})
			}
		}
		// validate that attach commands aren't used in the teardown_group phase
		if tg.TeardownGroup != nil {
			for _, cmd := range tg.TeardownGroup.List() {
				if utility.StringSliceContains(evergreen.AttachCommands, cmd.Command) {
					errs = append(errs, ValidationError{
						Message: fmt.Sprintf("%s cannot be used in the group teardown stage", cmd.Command),
						Level:   Error,
					})
				}
			}
		}
	}

	return errs
}

func checkTaskGroups(p *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	tasksInTaskGroups := map[string]string{}
	names := map[string]bool{}
	for _, tg := range p.TaskGroups {
		// validate that teardown group timeout is not over MaxTeardownGroupTimeout
		if tg.TeardownGroupTimeoutSecs > int(globals.MaxTeardownGroupTimeout.Seconds()) {
			errs = append(errs, ValidationError{
				Level:   Warning,
				Message: fmt.Sprintf("task group '%s' has a teardown task timeout of %d seconds, which exceeds the maximum of %d seconds", tg.Name, tg.TeardownGroupTimeoutSecs, int(globals.MaxTeardownGroupTimeout.Seconds())),
			})
		}
		if _, ok := names[tg.Name]; ok {
			errs = append(errs, ValidationError{
				Level:   Warning,
				Message: fmt.Sprintf("task group '%s' is defined multiple times; only the first will be used", tg.Name),
			})
		}
		names[tg.Name] = true
		if tg.MaxHosts < -1 {
			errs = append(errs, ValidationError{
				Level:   Warning,
				Message: fmt.Sprintf("task group '%s' has number of hosts %d less than 1", tg.Name, tg.MaxHosts),
			})
		}
		if len(tg.Tasks) == 1 {
			continue
		}
		if tg.MaxHosts > len(tg.Tasks) {
			errs = append(errs, ValidationError{
				Level:   Warning,
				Message: fmt.Sprintf("task group '%s' has max number of hosts %d greater than the number of tasks %d", tg.Name, tg.MaxHosts, len(tg.Tasks)),
			})
		}
		for _, t := range tg.Tasks {
			tasksInTaskGroups[t] = tg.Name
		}
	}

	return errs
}

// validateDuplicateBVTasks ensures that no task is used multiple times
// in any given build variant.
func validateDuplicateBVTasks(p *model.Project) ValidationErrors {
	errors := []ValidationError{}

	for _, bv := range p.BuildVariants {
		tasksFound := map[string]any{}
		for _, t := range bv.Tasks {

			if t.IsGroup {
				tg := p.FindTaskGroup(t.Name)
				if tg == nil {
					continue
				}
				for _, tgTask := range tg.Tasks {
					err := checkOrAddTask(tgTask, bv.Name, tasksFound)
					if err != nil {
						errors = append(errors, *err)
					}
				}
			} else {
				err := checkOrAddTask(t.Name, bv.Name, tasksFound)
				if err != nil {
					errors = append(errors, *err)
				}
			}

		}
	}

	return errors
}

func checkOrAddTask(task, variant string, tasksFound map[string]any) *ValidationError {
	if _, found := tasksFound[task]; found {
		return &ValidationError{
			Level:   Error,
			Message: fmt.Sprintf("task '%s' in '%s' is listed more than once, likely through a task group", task, variant),
		}
	}
	tasksFound[task] = nil
	return nil
}

func validateHostCreates(p *model.Project) ValidationErrors {
	counts := tasksThatCallHostCreateByProvider(p)
	errs := validateTimesCalledPerTask(p, counts.All, evergreen.HostCreateCommandName, HostCreateLimitPerTask, Error)
	errs = append(errs, validateHostCreateTotals(p, counts)...)
	return errs
}

type hostCreateCounts struct {
	Docker map[string]int
	EC2    map[string]int
	All    map[string]int
}

// tasksThatCallHostCreateByProvider is similar to TasksThatCallCommand in the model package, except the output is
// split into host.create for Docker hosts and host.create for non-docker hosts, so limits can be validated separately.
func tasksThatCallHostCreateByProvider(p *model.Project) hostCreateCounts {
	// get all functions that call the command.
	ec2Fs := map[string]int{}
	dockerFs := map[string]int{}
	for f, cmds := range p.Functions {
		if cmds == nil {
			continue
		}
		for _, c := range cmds.List() {
			if c.Command == evergreen.HostCreateCommandName {
				provider, ok := c.Params["provider"]
				if ok && provider.(string) == evergreen.ProviderNameDocker {
					dockerFs[f] += 1
				} else {
					ec2Fs[f] += 1
				}
			}
		}
	}

	// get all tasks that call the command.
	counts := hostCreateCounts{
		Docker: map[string]int{},
		EC2:    map[string]int{},
		All:    map[string]int{},
	}
	for _, t := range p.Tasks {
		for _, c := range t.Commands {
			if c.Function != "" {
				if times, ok := ec2Fs[c.Function]; ok {
					counts.EC2[t.Name] += times
					counts.All[t.Name] += times
				}
				if times, ok := dockerFs[c.Function]; ok {
					counts.Docker[t.Name] += times
					counts.All[t.Name] += times
				}
			}
			if c.Command == evergreen.HostCreateCommandName {
				provider, ok := c.Params["provider"]
				if ok && provider.(string) == evergreen.ProviderNameDocker {
					counts.Docker[t.Name] += 1
					counts.All[t.Name] += 1
				} else {
					counts.EC2[t.Name] += 1
					counts.All[t.Name] += 1
				}
			}
		}
	}

	return counts
}

func validateTimesCalledPerTask(p *model.Project, ts map[string]int, commandName string, times int, level ValidationErrorLevel) ValidationErrors {
	errs := ValidationErrors{}
	for _, bv := range p.BuildVariants {
		for _, t := range bv.Tasks {
			if count, ok := ts[t.Name]; ok {
				if count > times {
					errs = append(errs, ValidationError{
						Level:   level,
						Message: fmt.Sprintf("build variant '%s' with task '%s' may only call %s %d time(s) but calls it %d time(s)", bv.Name, t.Name, commandName, times, count),
					})
				}
			}
		}
	}
	return errs
}

func validateHostCreateTotals(p *model.Project, counts hostCreateCounts) ValidationErrors {
	errs := ValidationErrors{}
	dockerTotal := 0
	ec2Total := 0
	errorFmt := "project config may only call %s %s %d time(s) but it is called %d time(s)"
	for _, bv := range p.BuildVariants {
		for _, t := range bv.Tasks {
			dockerTotal += counts.Docker[t.Name]
			ec2Total += counts.EC2[t.Name]
		}
	}
	if ec2Total > EC2HostCreateTotalLimit {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf(errorFmt, "ec2", evergreen.HostCreateCommandName, EC2HostCreateTotalLimit, ec2Total),
			Level:   Error,
		})
	}
	if dockerTotal > DockerHostCreateTotalLimit {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf(errorFmt, "docker", evergreen.HostCreateCommandName, DockerHostCreateTotalLimit, dockerTotal),
			Level:   Error,
		})
	}
	return errs
}

// validateGenerateTasks validates that no task calls 'generate.tasks' more than once, since if one
// does, the server will noop it.
func validateGenerateTasks(p *model.Project) ValidationErrors {
	ts := p.TasksThatCallCommand(evergreen.GenerateTasksCommandName)
	return validateTimesCalledPerTask(p, ts, evergreen.GenerateTasksCommandName, 1, Error)
}

// validateVersionControl checks if a project with defined project config fields has version control enabled on the project ref.
func validateVersionControl(_ context.Context, _ *evergreen.Settings, _ *model.Project, ref *model.ProjectRef, isConfigDefined bool) ValidationErrors {
	var errs ValidationErrors
	if ref.IsVersionControlEnabled() && !isConfigDefined {
		errs = append(errs, ValidationError{
			Level: Warning,
			Message: fmt.Sprintf("version control is enabled for project '%s' but no project config fields have been set.",
				ref.Identifier),
		})
	} else if !ref.IsVersionControlEnabled() && isConfigDefined {
		errs = append(errs, ValidationError{
			Level: Warning,
			Message: fmt.Sprintf("version control is disabled for project '%s'; the currently defined project config fields will not be picked up",
				ref.Identifier),
		})
	}
	return errs
}

// validateTVDependsOnTV checks that the dependent task always has a dependency on the depended on task.
// The dependedOnTask and every other task along the path must run on all the same requester types as the dependentTask
// and the dependency on the dependedOnTask must be with a status in statuses, if provided.
func validateTVDependsOnTV(dependentTask, dependedOnTask model.TVPair, statuses []string, project *model.Project) error {
	g := project.DependencyGraph()
	tvTaskUnitMap := tvToTaskUnit(project)

	startNode := task.TaskNode{Name: dependentTask.TaskName, Variant: dependentTask.Variant}
	targetNode := task.TaskNode{Name: dependedOnTask.TaskName, Variant: dependedOnTask.Variant}

	// The traversal function returns whether the current edge should be traversed by the DFS.
	traversal := func(edge task.DependencyEdge) bool {
		from := edge.From
		to := edge.To

		fromTaskUnit := tvTaskUnitMap[model.TVPair{TaskName: from.Name, Variant: from.Variant}]
		toTaskUnit := tvTaskUnitMap[model.TVPair{TaskName: to.Name, Variant: to.Variant}]

		var edgeInfo model.TaskUnitDependency
		for _, dependency := range fromTaskUnit.DependsOn {
			if dependency.Name == to.Name && dependency.Variant == to.Variant {
				edgeInfo = dependency
			}
		}

		// PatchOptional dependencies are skipped when the fromTaskUnit task is running on a patch.
		if edgeInfo.PatchOptional && !(fromTaskUnit.SkipOnPatchBuild() || fromTaskUnit.SkipOnNonGitTagBuild()) {
			return false
		}

		// The dependency is skipped if toTaskUnit doesn't run on all the same requester types that fromTaskUnit runs on.
		for _, rType := range evergreen.AllRequesterTypes {
			if !fromTaskUnit.SkipOnRequester(rType) && toTaskUnit.SkipOnRequester(rType) {
				return false
			}
		}

		// If statuses is specified we need to check the edge's status when the edge points to the target node.
		if statuses != nil && to == targetNode {
			return utility.StringSliceContains(statuses, edgeInfo.Status)
		}

		return true
	}

	if found := g.DepthFirstSearch(startNode, targetNode, traversal); !found {
		dependentBVTask := tvTaskUnitMap[dependentTask]
		runsOnPatches := !(dependentBVTask.SkipOnPatchBuild() || dependentBVTask.SkipOnNonGitTagBuild())
		runsOnNonPatches := !(dependentBVTask.SkipOnNonPatchBuild() || dependentBVTask.SkipOnNonGitTagBuild())
		runsOnGitTag := !(dependentBVTask.SkipOnNonPatchBuild() || dependentBVTask.SkipOnGitTagBuild())

		errMsg := "task '%s' in build variant '%s' must depend on" +
			" task '%s' in build variant '%s' completing"
		if runsOnPatches && runsOnNonPatches {
			errMsg += " for both patches and non-patches"
		} else if runsOnPatches {
			errMsg += " for patches"
		} else if runsOnNonPatches {
			errMsg += " for non-patches"
		} else if runsOnGitTag {
			errMsg += " for git-tag builds"
		}
		errMsg = fmt.Sprintf(errMsg, dependentTask.TaskName, dependentTask.Variant, dependedOnTask.TaskName, dependedOnTask.Variant)

		if statuses != nil {
			errMsg = fmt.Sprintf("%s with status in [%s]", errMsg, strings.Join(statuses, ", "))
		}

		return errors.New(errMsg)
	}
	return nil
}

// checkTasks checks whether project tasks contain warnings by checking if each task
// has commands, contains exec_timeout_sec, and has valid logger configs, dependencies and task names.
func checkTasks(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	execTimeoutWarningAdded := false
	for _, task := range project.Tasks {
		if len(task.Commands) == 0 {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("task '%s' does not contain any commands",
						task.Name),
					Level: Warning,
				},
			)
		}
		if task.Priority > model.MaxConfigSetPriority {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("task '%s' has been set above %d priority, in YAML, will default priority to %d",
						task.Name, model.MaxConfigSetPriority, model.MaxConfigSetPriority),
					Level: Notice,
				},
			)
		}
		if project.ExecTimeoutSecs == 0 && task.ExecTimeoutSecs == 0 && !execTimeoutWarningAdded {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("no exec_timeout_secs defined at the top-level or on one or more tasks; "+
						"these tasks will default to a timeout of %d hours",
						int(globals.DefaultExecTimeout.Hours())),
					Level: Warning,
				},
			)
			execTimeoutWarningAdded = true
		}
		errs = append(errs, checkTaskNames(project, &task)...)
	}
	return errs
}

// checkTaskUsage returns a notice for each task that is defined but unused by any (un-disabled) variant.
// TODO: upgrade to a warning in DEVPROD-8154
func checkTaskUsage(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	seen := map[string]bool{}
	for _, bvtu := range project.FindAllBuildVariantTasks() {
		if !utility.FromBoolPtr(bvtu.Disable) {
			seen[bvtu.Name] = true
		}
	}

	for _, pt := range project.Tasks {
		if utility.FromBoolPtr(pt.Disable) {
			continue
		}
		if !seen[pt.Name] {
			errs = append(errs, ValidationError{
				Message: fmt.Sprintf("task '%s' defined but not used by any variants; consider using or disabling",
					pt.Name),
				Level: Notice,
			})
		}
	}
	return errs
}

// checkBuildVariants checks whether project build variants contain warnings by checking if each variant
// has tasks, valid and non-duplicate names, and appropriate batch time settings.
func checkBuildVariants(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	displayNames := map[string]int{}
	for _, buildVariant := range project.BuildVariants {
		dispName := buildVariant.DisplayName
		displayNames[dispName] = displayNames[dispName] + 1

		if len(buildVariant.Tasks) == 0 {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("buildvariant '%s' contains no tasks", buildVariant.Name),
					Level:   Warning,
				},
			)
		}

		for _, warning := range buildVariant.TranslationWarnings {
			errs = append(errs, ValidationError{Message: warning, Level: Warning})
		}

		errs = append(errs, checkBVNames(&buildVariant)...)
		errs = append(errs, checkBVBatchTimes(&buildVariant)...)
		errs = append(errs, checkBVTaskPriority(&buildVariant)...)
	}

	for k, v := range displayNames {
		if v > 1 {
			errs = append(errs,
				ValidationError{
					Level:   Warning,
					Message: fmt.Sprintf("%d build variants share the same display name: '%s'", v, k),
				},
			)

		}
	}

	return errs
}

// GetAllowedSingleTaskDistroTasksForProject returns a struct with a list of tasks and bvs that is
// allowed for the given project and repo project. If both the project and
// repo project have allowed tasks/BVs, they are combined.
func GetAllowedSingleTaskDistroTasksForProject(ctx context.Context, identifier string, settings *evergreen.Settings) (evergreen.ProjectTasksPair, error) {
	allowList := evergreen.ProjectTasksPair{}

	projectsToLookFor := []string{}
	for _, pairs := range settings.SingleTaskDistro.ProjectTasksPairs {
		pRef, err := model.FindBranchProjectRef(ctx, identifier)
		if err != nil {
			return allowList, errors.Wrapf(err, "finding project ref '%s'", identifier)
		}

		// Look for allowed tasks for the project and its repo project.
		if pRef != nil {
			projectsToLookFor = append(projectsToLookFor, pRef.Id, pRef.Identifier, pRef.RepoRefId)
		} else {
			// If project ref is nil, it means the project is a repo project.
			repoRef, err := model.FindOneRepoRef(ctx, identifier)
			if err != nil {
				return allowList, errors.Wrapf(err, "finding repo ref '%s'", identifier)
			}
			if repoRef == nil {
				return allowList, errors.Errorf("project or repo ref '%s' not found", identifier)
			}
			projectsToLookFor = append(projectsToLookFor, repoRef.Id)
		}
		if utility.StringSliceContains(projectsToLookFor, pairs.ProjectID) {
			allowList.AllowedTasks = append(allowList.AllowedTasks, pairs.AllowedTasks...)
			allowList.AllowedBVs = append(allowList.AllowedBVs, pairs.AllowedBVs...)
		}
	}

	return allowList, nil
}
