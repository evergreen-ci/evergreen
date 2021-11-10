package validator

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
)

type projectValidator func(*model.Project) ValidationErrors

type projectSettingsValidator func(*model.Project, *model.ProjectRef) ValidationErrors

// bool indicates if we should still run the validator if the project is complex
type longValidator func(*model.Project, bool) ValidationErrors

type ValidationErrorLevel int64

const (
	Error ValidationErrorLevel = iota
	Warning
	unauthorizedCharacters                  = "|"
	EC2HostCreateTotalLimit                 = 1000
	DockerHostCreateTotalLimit              = 200
	HostCreateLimitPerTask                  = 3
	maxTaskSyncCommandsForDependenciesCheck = 300 // this should take about one second
)

func (vel ValidationErrorLevel) String() string {
	switch vel {
	case Error:
		return "ERROR"
	case Warning:
		return "WARNING"
	}
	return "?"
}

type ValidationError struct {
	Level   ValidationErrorLevel `json:"level"`
	Message string               `json:"message"`
}

type ValidationErrors []ValidationError

func (v ValidationErrors) Raw() interface{} {
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
func (v ValidationErrors) Annotate(key string, value interface{}) error {
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

type ValidationInput struct {
	ProjectYaml []byte `json:"project_yaml" yaml:"project_yaml"`
	Quiet       bool   `json:"quiet" yaml:"quiet"`
	IncludeLong bool   `json:"include_long" yaml:"include_long"`
}

// Functions used to validate the syntax of a project configuration file.
var projectSyntaxValidators = []projectValidator{
	ensureHasNecessaryBVFields,
	checkDependencyGraph,
	validatePluginCommands,
	ensureHasNecessaryProjectFields,
	validateTaskDependencies,
	validateTaskRuns,
	validateTaskNames,
	validateModules,
	validateBVNames,
	validateBVBatchTimes,
	validateDisplayTaskNames,
	validateBVTaskNames,
	validateBVsContainTasks,
	checkAllDependenciesSpec,
	validateProjectTaskNames,
	validateProjectTaskIdsAndTags,
	validateParameters,
	validateTaskGroups,
	validateHostCreates,
	validateDuplicateBVTasks,
	validateGenerateTasks,
	validateAliases,
}

// Functions used to validate the semantics of a project configuration file.
var projectSemanticValidators = []projectValidator{
	checkTaskCommands,
	checkTaskGroups,
	checkLoggerConfig,
}

var projectSettingsValidators = []projectSettingsValidator{
	validateTaskSyncSettings,
}

// These validators have the potential to be very long, and may not be fully run unless specified.
var longSyntaxValidators = []longValidator{
	validateTaskSyncCommands,
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
func getDistros() (ids []string, aliases []string, err error) {
	return getDistrosForProject("")
}

// getDistrosForProject creates a slice of all valid distro IDs and a slice of
// all valid aliases for a project. If projectID is empty, it returns all distro
// IDs and all aliases.
func getDistrosForProject(projectID string) (ids []string, aliases []string, err error) {
	// create a slice of all known distros
	distros, err := distro.Find(distro.All)
	if err != nil {
		return nil, nil, err
	}
	for _, d := range distros {
		if projectID != "" && len(d.ValidProjects) > 0 {
			if utility.StringSliceContains(d.ValidProjects, projectID) {
				ids = append(ids, d.Id)
				for _, alias := range d.Aliases {
					if !utility.StringSliceContains(aliases, alias) {
						aliases = append(aliases, alias)
					}
				}
			}
		} else {
			ids = append(ids, d.Id)
			for _, alias := range d.Aliases {
				if !utility.StringSliceContains(aliases, alias) {
					aliases = append(aliases, alias)
				}
			}
		}
	}
	return ids, aliases, nil
}

// verify that the project configuration semantics is valid
func CheckProjectSemantics(project *model.Project) ValidationErrors {
	validationErrs := ValidationErrors{}
	for _, projectSemanticValidator := range projectSemanticValidators {
		validationErrs = append(validationErrs,
			projectSemanticValidator(project)...)
	}
	return validationErrs
}

// verify that the project configuration syntax is valid
func CheckProjectSyntax(project *model.Project, includeLong bool) ValidationErrors {
	validationErrs := ValidationErrors{}
	for _, projectSyntaxValidator := range projectSyntaxValidators {
		validationErrs = append(validationErrs,
			projectSyntaxValidator(project)...)
	}
	for _, longSyntaxValidator := range longSyntaxValidators {
		validationErrs = append(validationErrs,
			longSyntaxValidator(project, includeLong)...)
	}

	// get distro IDs and aliases for ensureReferentialIntegrity validation
	distroIDs, distroAliases, err := getDistrosForProject(project.Identifier)
	if err != nil {
		validationErrs = append(validationErrs, ValidationError{Message: "can't get distros from database"})
	}
	validationErrs = append(validationErrs, ensureReferentialIntegrity(project, distroIDs, distroAliases)...)
	return validationErrs
}

// CheckProjectSettings checks the project configuration against the project
// settings.
func CheckProjectSettings(p *model.Project, ref *model.ProjectRef) ValidationErrors {
	var errs ValidationErrors
	for _, validateSettings := range projectSettingsValidators {
		errs = append(errs, validateSettings(p, ref)...)
	}
	return errs
}

func CheckYamlStrict(yamlBytes []byte) ValidationErrors {
	validationErrs := ValidationErrors{}
	// check strict yaml, i.e warn if there are missing fields
	strictProjectWithVariables := struct {
		model.ParserProject `yaml:"pp,inline"`
		// Variables is only used to suppress yaml unmarshalling errors related
		// to a non-existent variables field.
		Variables interface{} `yaml:"variables,omitempty" bson:"-"`
	}{}

	if err := util.UnmarshalYAMLStrictWithFallback(yamlBytes, &strictProjectWithVariables); err != nil {
		validationErrs = append(validationErrs, ValidationError{
			Level:   Warning,
			Message: err.Error(),
		})
	}
	return validationErrs
}

// verify that the project configuration semantics and configuration syntax is valid
func CheckProjectConfigurationIsValid(project *model.Project, pref *model.ProjectRef) error {
	catcher := grip.NewBasicCatcher()
	syntaxErrs := CheckProjectSyntax(project, false)
	if len(syntaxErrs) != 0 {
		if errs := syntaxErrs.AtLevel(Error); len(errs) != 0 {
			catcher.Errorf("project contains syntax errors: %s", ValidationErrorsToString(errs))
		}
	}
	semanticErrs := CheckProjectSemantics(project)
	if len(semanticErrs) != 0 {
		if errs := semanticErrs.AtLevel(Error); len(errs) != 0 {
			catcher.Errorf("project contains semantic errors: %s", ValidationErrorsToString(errs))
		}
	}
	if settingsErrs := CheckProjectSettings(project, pref); len(settingsErrs) != 0 {
		if errs := settingsErrs.AtLevel(Error); len(errs) != 0 {
			catcher.Errorf("project contains errors related to project settings: %s", ValidationErrorsToString(errs))
		}
	}
	return catcher.Resolve()
}

// ensure that if any task spec references 'model.AllDependencies', it
// references no other dependency within the variant
func checkAllDependenciesSpec(project *model.Project) ValidationErrors {
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
								Message: fmt.Sprintf("task '%s' in project '%s' "+
									"contains the all dependencies (%s)' "+
									"specification and other explicit dependencies or duplicate variants",
									task.Name, project.Identifier,
									model.AllDependencies),
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

// Makes sure that the dependencies for the tasks in the project form a
// valid dependency graph (no cycles).
func checkDependencyGraph(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}

	tvToTaskUnit := tvToTaskUnit(project)
	visited := map[model.TVPair]bool{}
	allNodes := []model.TVPair{}

	for node := range tvToTaskUnit {
		visited[node] = false
		allNodes = append(allNodes, node)
	}

	for node := range tvToTaskUnit {
		if err := dependencyCycleExists(node, allNodes, visited, tvToTaskUnit); err != nil {
			errs = append(errs, ValidationError{
				Level:   Error,
				Message: fmt.Sprintf("dependency error for '%s' task: %s", node.TaskName, err.Error()),
			})
		}
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
				tasksToAdd = append(tasksToAdd, model.CreateTasksFromGroup(t, p)...)
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

// Helper for checking the dependency graph for cycles.
func dependencyCycleExists(node model.TVPair, allNodes []model.TVPair, visited map[model.TVPair]bool,
	tasksByNameAndVariant map[model.TVPair]model.BuildVariantTaskUnit) error {

	v, ok := visited[node]
	// if the node does not exist, the deps are broken
	if !ok {
		return errors.Errorf("dependency %s is not present in the project config", node)
	}
	// if the task has already been visited, then a cycle certainly exists
	if v {
		return errors.Errorf("dependency %s is part of a dependency cycle", node)
	}

	visited[node] = true

	task := tasksByNameAndVariant[node]

	depsToNodes := dependenciesForTaskUnit(node, task, allNodes)
	for _, depNodes := range depsToNodes {
		// For each of the task's dependencies, recursively check for cycles.
		for _, dn := range depNodes {
			if err := dependencyCycleExists(dn, allNodes, visited, tasksByNameAndVariant); err != nil {
				return err
			}
		}
	}

	// remove the task from the visited map so that higher-level calls do not see it
	visited[node] = false

	// no cycle found
	return nil
}

func validateAliases(p *model.Project) ValidationErrors {
	errs := []string{}
	errs = append(errs, model.ValidateProjectAliases(p.GitHubPRAliases, "GitHub PR Aliases")...)
	errs = append(errs, model.ValidateProjectAliases(p.GitHubChecksAliases, "Github Checks Aliases")...)
	errs = append(errs, model.ValidateProjectAliases(p.CommitQueueAliases, "Commit Queue Aliases")...)
	errs = append(errs, model.ValidateProjectAliases(p.PatchAliases, "Patch Aliases")...)
	errs = append(errs, model.ValidateProjectAliases(p.GitTagAliases, "Git Tag Aliases")...)

	validationErrs := ValidationErrors{}
	for _, errorMsg := range errs {
		validationErrs = append(validationErrs, ValidationError{
			Message: errorMsg,
			Level:   Error,
		})
	}
	return validationErrs
}

// Ensures that the project has at least one buildvariant and also that all the
// fields required for any buildvariant definition are present
func ensureHasNecessaryBVFields(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	if len(project.BuildVariants) == 0 {
		return ValidationErrors{
			{
				Message: fmt.Sprintf("project '%s' must specify at least one "+
					"buildvariant", project.Identifier),
			},
		}
	}

	for _, buildVariant := range project.BuildVariants {
		if buildVariant.Name == "" {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("project '%s' buildvariant must "+
						"have a name", project.Identifier),
				},
			)
		}
		if len(buildVariant.Tasks) == 0 {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("buildvariant '%s' in project '%s' "+
						"must have at least one task", buildVariant.Name,
						project.Identifier),
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

		hasTaskWithoutDistro := false
		for _, task := range buildVariant.Tasks {
			taskHasValidDistro := false
			for _, d := range task.RunOn {
				if d != "" {
					taskHasValidDistro = true
					break
				}
			}
			if !taskHasValidDistro {
				// check for a default in the task definition
				pt := project.FindProjectTask(task.Name)
				if pt != nil {
					for _, d := range pt.RunOn {
						if d != "" {
							taskHasValidDistro = true
							break
						}
					}
				}
			}
			if !taskHasValidDistro {
				hasTaskWithoutDistro = true
				break
			}
		}

		if hasTaskWithoutDistro {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("buildvariant '%s' in project '%s' "+
						"must either specify run_on field or have every task "+
						"specify run_on.",
						buildVariant.Name, project.Identifier),
				},
			)
		}
	}
	return errs
}

// Checks that the basic fields that are required by any project are present and
// valid.
func ensureHasNecessaryProjectFields(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}

	if project.BatchTime < 0 {
		errs = append(errs,
			ValidationError{
				Message: fmt.Sprintf("project '%s' must have a "+
					"non-negative 'batchtime' set", project.Identifier),
			},
		)
	}

	if project.BatchTime > math.MaxInt32 {
		// Error level is warning for backwards compatibility with
		// existing projects. This value will be capped at MaxInt32
		// in ProjectRef.getBatchTime()
		errs = append(errs,
			ValidationError{
				Message: fmt.Sprintf("project '%s' field 'batchtime' should not exceed %d)",
					project.Identifier, math.MaxInt32),
				Level: Warning,
			},
		)
	}

	if project.CommandType != "" {
		if !utility.StringSliceContains(evergreen.ValidCommandTypes, project.CommandType) {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("project '%s' contains an invalid "+
						"command type: %s", project.Identifier, project.CommandType),
				},
			)
		}
	}
	return errs
}

// Ensures that:
// 1. a referenced task within a buildvariant task object exists in
// the set of project tasks
// 2. any referenced distro exists within the current setting's distro directory
func ensureReferentialIntegrity(project *model.Project, distroIDs []string, distroAliases []string) ValidationErrors {
	errs := ValidationErrors{}
	// create a set of all the task names
	allTaskNames := map[string]bool{}
	for _, task := range project.Tasks {
		allTaskNames[task.Name] = true
	}
	for _, taskGroup := range project.TaskGroups {
		allTaskNames[taskGroup.Name] = true
	}

	for _, buildVariant := range project.BuildVariants {
		buildVariantTasks := map[string]bool{}
		for _, task := range buildVariant.Tasks {
			if _, ok := allTaskNames[task.Name]; !ok {
				if task.Name == "" {
					errs = append(errs,
						ValidationError{
							Message: fmt.Sprintf("tasks for buildvariant '%s' "+
								"in project '%s' must each have a name field",
								project.Identifier, buildVariant.Name),
						},
					)
				} else {
					errs = append(errs,
						ValidationError{
							Message: fmt.Sprintf("buildvariant '%s' in "+
								"project '%s' references a non-existent "+
								"task '%s'", buildVariant.Name,
								project.Identifier, task.Name),
						},
					)
				}
			}
			buildVariantTasks[task.Name] = true
			for _, distro := range task.RunOn {
				if !utility.StringSliceContains(distroIDs, distro) && !utility.StringSliceContains(distroAliases, distro) {
					errs = append(errs,
						ValidationError{
							Message: fmt.Sprintf("task '%s' in buildvariant '%s' in project "+
								"'%s' references a nonexistent distro '%s'.\n",
								task.Name, buildVariant.Name,
								project.Identifier, distro),
							Level: Warning,
						},
					)
				}
			}
		}
		for _, distro := range buildVariant.RunOn {
			if !utility.StringSliceContains(distroIDs, distro) && !utility.StringSliceContains(distroAliases, distro) {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("buildvariant '%s' in project "+
							"'%s' references a nonexistent distro '%s'.\n",
							buildVariant.Name,
							project.Identifier, distro),
						Level: Warning,
					},
				)
			}
		}
	}
	return errs
}

// validateTaskNames ensures the task names do not contain unauthorized characters.
func validateTaskNames(project *model.Project) ValidationErrors {
	unauthorizedTaskCharacters := unauthorizedCharacters + " "
	errs := ValidationErrors{}
	for _, task := range project.Tasks {
		if strings.ContainsAny(strings.TrimSpace(task.Name), unauthorizedTaskCharacters) {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("task name '%s' contains unauthorized characters ('%s')",
						task.Name, unauthorizedTaskCharacters),
				})
		}
		// Warn against commas because the CLI allows users to specify
		// tasks separated by commas in their patches.
		if strings.Contains(task.Name, ",") {
			errs = append(errs, ValidationError{
				Level:   Warning,
				Message: fmt.Sprintf("task name '%s' should not contains commas", task.Name),
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
	}
	return errs
}

// validateModules checks to make sure that the module's name, branch, and repo are correct.
func validateModules(project *model.Project) ValidationErrors {
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
				Message: fmt.Sprintf("module '%s' already exists", module.Name),
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
	}

	return errs
}

// Ensures there aren't any duplicate buildvariant names specified in the given
// project and that the names do not contain unauthorized characters.
func validateBVNames(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	buildVariantNames := map[string]bool{}
	displayNames := map[string]int{}

	for _, buildVariant := range project.BuildVariants {
		if _, ok := buildVariantNames[buildVariant.Name]; ok {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("project '%s' buildvariant '%s' already exists",
						project.Identifier, buildVariant.Name),
				},
			)
		}
		buildVariantNames[buildVariant.Name] = true
		dispName := buildVariant.DisplayName
		if dispName == "" {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("project '%s' buildvariant '%s' does not have a display name",
						project.Identifier, buildVariant.Name),
				},
			)
		} else if dispName == evergreen.MergeTaskVariant {
			errs = append(errs, ValidationError{
				Message: fmt.Sprintf("the variant name '%s' is reserved for the commit queue", evergreen.MergeTaskVariant),
			})
		}
		displayNames[dispName] = displayNames[dispName] + 1

		if strings.ContainsAny(buildVariant.Name, unauthorizedCharacters) {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("buildvariant name '%s' contains unauthorized characters (%s)",
						buildVariant.Name, unauthorizedCharacters),
				})
		}

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
	}
	// don't bother checking for the warnings if we already found errors
	if len(errs) > 0 {
		return errs
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

func checkLoggerConfig(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	if project.Loggers != nil {
		if err := project.Loggers.IsValid(); err != nil {
			errs = append(errs, ValidationError{
				Message: errors.Wrap(err, "error in project-level logger config").Error(),
				Level:   Warning,
			})
		}

		for _, task := range project.Tasks {
			for _, command := range task.Commands {
				if err := command.Loggers.IsValid(); err != nil {
					errs = append(errs, ValidationError{
						Message: errors.Wrapf(err, "error in logger config for command %s in task %s", command.DisplayName, task.Name).Error(),
						Level:   Warning,
					})
				}
			}
		}
	}

	return errs
}

// Checks each task definitions to determine if a command is specified
func checkTaskCommands(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	for _, task := range project.Tasks {
		if len(task.Commands) == 0 {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("task '%s' in project '%s' does not "+
						"contain any commands",
						task.Name, project.Identifier),
					Level: Warning,
				},
			)
		}
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
						Message: fmt.Sprintf("task '%s' in buildvariant '%s' "+
							"in project '%s' already exists",
							task.Name, buildVariant.Name, project.Identifier),
					},
				)
			}
			buildVariantTasks[task.Name] = true
		}
	}
	return errs
}

// Ensure there are no buildvariants without tasks
func validateBVsContainTasks(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	for _, buildVariant := range project.BuildVariants {
		if len(buildVariant.Tasks) == 0 {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("buildvariant '%s' contains no tasks", buildVariant.Name),
					Level:   Warning,
				},
			)
		}
	}
	return errs
}

func validateBVBatchTimes(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	for _, buildVariant := range project.BuildVariants {
		// check task batchtimes first
		for _, t := range buildVariant.Tasks {
			// setting explicitly to true with batchtime will use batchtime
			if utility.FromBoolPtr(t.Activate) && (t.CronBatchTime != "" || t.BatchTime != nil) {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("task '%s' for variant '%s' activation ignored since batchtime specified",
							t.Name, buildVariant.Name),
						Level: Warning,
					})
			}
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
			if _, err := model.GetActivationTimeWithCron(time.Now(), t.CronBatchTime); err != nil {
				errs = append(errs,
					ValidationError{
						Message: errors.Wrapf(err, "task cron batchtime '%s' has invalid syntax for task '%s' for build variant '%s'",
							t.CronBatchTime, t.Name, buildVariant.Name).Error(),
						Level: Error,
					},
				)
			}
		}

		if utility.FromBoolPtr(buildVariant.Activate) && (buildVariant.CronBatchTime != "" || buildVariant.BatchTime != nil) {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("variant '%s' activation ignored since batchtime specified", buildVariant.Name),
					Level:   Warning,
				})
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
		if _, err := model.GetActivationTimeWithCron(time.Now(), buildVariant.CronBatchTime); err != nil {
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
func validateCommands(section string, project *model.Project,
	commands []model.PluginCommandConf) ValidationErrors {
	errs := ValidationErrors{}

	for _, cmd := range commands {
		commandName := fmt.Sprintf("'%s' command", cmd.Command)
		_, err := command.Render(cmd, project.Functions)
		if err != nil {
			if cmd.Function != "" {
				commandName = fmt.Sprintf("'%s' function", cmd.Function)
			}
			errs = append(errs, ValidationError{Message: fmt.Sprintf("%s section in %s: %s", section, commandName, err)})
		}
		if cmd.Type != "" {
			if !utility.StringSliceContains(evergreen.ValidCommandTypes, cmd.Type) {
				msg := fmt.Sprintf("%s section in '%s': invalid command type: '%s'", section, commandName, cmd.Type)
				errs = append(errs, ValidationError{Message: msg})
			}
		}
		if cmd.Function != "" && cmd.Command != "" {
			errs = append(errs, ValidationError{
				Level:   Error,
				Message: fmt.Sprintf("cannot specify both command '%s' and function '%s'", cmd.Command, cmd.Function),
			})
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
					Message: fmt.Sprintf("'%s' project's '%s' function contains no commands",
						project.Identifier, funcName),
					Level: Error,
				},
			)
			continue
		}
		valErrs := validateCommands("functions", project, commands.List())
		for _, err := range valErrs {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("'%s' project's '%s' definition: %s",
						project.Identifier, funcName, err),
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
					Message: fmt.Sprintf(`project '%s' has duplicate definition of "%s"`,
						project.Identifier, funcName),
				},
			)
		}
		seen[funcName] = true
	}

	if project.Pre != nil {
		errs = append(errs, validateCommands("pre", project, project.Pre.List())...)
	}

	if project.Post != nil {
		errs = append(errs, validateCommands("post", project, project.Post.List())...)
	}

	if project.Timeout != nil {
		errs = append(errs, validateCommands("timeout", project, project.Timeout.List())...)
	}

	if project.EarlyTermination != nil {
		errs = append(errs, validateCommands("early termination", project, project.EarlyTermination.List())...)
	}

	// validate project tasks section
	for _, task := range project.Tasks {
		errs = append(errs, validateCommands("tasks", project, task.Commands)...)
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
					Message: fmt.Sprintf("task '%s' in project '%s' "+
						"already exists", task.Name, project.Identifier),
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
				Message: fmt.Sprintf("task '%s' has invalid name: starts with invalid character %s",
					task.Name, strconv.QuoteRune(rune(task.Name[0])))})
		}
		// check tag names
		for _, tag := range task.Tags {
			if i := strings.IndexAny(tag, model.InvalidCriterionRunes); i == 0 {
				errs = append(errs, ValidationError{
					Message: fmt.Sprintf("task '%s' has invalid tag '%s': starts with invalid character %s",
						task.Name, tag, strconv.QuoteRune(rune(tag[0])))})
			}
			if i := util.IndexWhiteSpace(tag); i != -1 {
				errs = append(errs, ValidationError{
					Message: fmt.Sprintf("task '%s' has invalid tag '%s': tag contains white space",
						task.Name, tag)})
			}
		}
	}
	return errs
}

func validateTaskRuns(project *model.Project) ValidationErrors {
	var errs ValidationErrors
	for _, bvtu := range project.FindAllBuildVariantTasks() {
		if bvtu.SkipOnPatchBuild() && bvtu.SkipOnNonPatchBuild() {
			errs = append(errs, ValidationError{
				Level: Warning,
				Message: fmt.Sprintf("task '%s' will never run because it skips both patch builds and non-patch builds",
					bvtu.Name),
			})
		}
		if bvtu.SkipOnGitTagBuild() && bvtu.SkipOnNonGitTagBuild() {
			errs = append(errs, ValidationError{
				Level: Warning,
				Message: fmt.Sprintf("task '%s' will never run because it skips both git tag builds and non git tag builds",
					bvtu.Name),
			})
		}
		// Git-tag-only builds cannot run in patches.
		if bvtu.SkipOnNonGitTagBuild() && bvtu.SkipOnNonPatchBuild() {
			errs = append(errs, ValidationError{
				Level: Warning,
				Message: fmt.Sprintf("task '%s' will never run because it only runs for git tag builds but also is patch-only",
					bvtu.Name),
			})
		}
		if bvtu.SkipOnNonGitTagBuild() && utility.FromBoolPtr(bvtu.Patchable) {
			errs = append(errs, ValidationError{
				Level: Warning,
				Message: fmt.Sprintf("task '%s' cannot be patchable if it only runs for git tag builds",
					bvtu.Name),
			})
		}
	}
	return errs
}

// validateTaskDependencies ensures that the dependencies for the tasks have the
// correct fields, and that the fields have valid values
func validateTaskDependencies(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}

	allTasks := project.FindAllTasksMap()
	for _, task := range project.Tasks {
		// create a set of the dependencies, to check for duplicates
		depNames := map[model.TVPair]bool{}

		for _, dep := range task.DependsOn {
			pair := model.TVPair{TaskName: dep.Name, Variant: dep.Variant}
			// make sure the dependency is not specified more than once
			if depNames[pair] {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("project '%s' contains a "+
							"duplicate dependency '%s' specified for task '%s'",
							project.Identifier, dep.Name, task.Name),
					},
				)
			}
			depNames[pair] = true

			// check that the status is valid
			switch dep.Status {
			case evergreen.TaskSucceeded, evergreen.TaskFailed, model.AllStatuses, "":
				// these are all valid
			default:
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("project '%s' contains an invalid dependency status for task '%s': %s",
							project.Identifier, task.Name, dep.Status)})
			}

			// check that name of the dependency task is valid
			if dep.Name != model.AllDependencies && project.FindProjectTask(dep.Name) == nil {
				errs = append(errs,
					ValidationError{
						Level: Error,
						Message: fmt.Sprintf("project '%s' contains a "+
							"non-existent task name '%s' in dependencies for "+
							"task '%s'", project.Identifier, dep.Name,
							task.Name),
					},
				)
			}
			if dep.Variant != "" && dep.Variant != model.AllVariants && project.FindBuildVariant(dep.Variant) == nil {
				errs = append(errs, ValidationError{
					Level: Error,
					Message: fmt.Sprintf("project '%s' contains a non-existent variant name '%s' in dependencies for task '%s'",
						project.Identifier, dep.Variant, task.Name),
				})
			}

			dependent, exists := allTasks[dep.Name]
			if !exists {
				continue
			}
			if utility.FromBoolPtr(dependent.PatchOnly) && !utility.FromBoolPtr(task.PatchOnly) {
				errs = append(errs, ValidationError{
					Level:   Warning,
					Message: fmt.Sprintf("Task '%s' depends on patch-only task '%s'. Both will only run in patches", task.Name, dep.Name),
				})
			}
			if !utility.FromBoolTPtr(dependent.Patchable) && utility.FromBoolTPtr(task.Patchable) {
				errs = append(errs, ValidationError{
					Level:   Warning,
					Message: fmt.Sprintf("Task '%s' depends on non-patchable task '%s'. Neither will run in patches", task.Name, dep.Name),
				})
			}
			if utility.FromBoolPtr(dependent.GitTagOnly) && !utility.FromBoolPtr(task.GitTagOnly) {
				errs = append(errs, ValidationError{
					Level:   Warning,
					Message: fmt.Sprintf("Task '%s' depends on git-tag-only task '%s'. Both will only run when pushing git tags", task.Name, dep.Name),
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

	names := map[string]bool{}
	for _, tg := range p.TaskGroups {
		if _, ok := names[tg.Name]; ok {
			errs = append(errs, ValidationError{
				Level:   Warning,
				Message: fmt.Sprintf("task group '%s' is defined multiple times", tg.Name),
			})
		}
		names[tg.Name] = true

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
				if cmd.Command == "attach.results" || cmd.Command == "attach.artifacts" {
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
	for _, tg := range p.TaskGroups {
		if tg.MaxHosts < 1 {
			errs = append(errs, ValidationError{
				Message: fmt.Sprintf("task group %s has number of hosts %d less than 1", tg.Name, tg.MaxHosts),
				Level:   Warning,
			})
		}
		if len(tg.Tasks) == 1 {
			continue
		}
		if tg.MaxHosts > len(tg.Tasks) {
			errs = append(errs, ValidationError{
				Message: fmt.Sprintf("task group %s has max number of hosts %d greater than the number of tasks %d", tg.Name, tg.MaxHosts, len(tg.Tasks)),
				Level:   Warning,
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
		tasksFound := map[string]interface{}{}
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

func checkOrAddTask(task, variant string, tasksFound map[string]interface{}) *ValidationError {
	if _, found := tasksFound[task]; found {
		return &ValidationError{
			Message: fmt.Sprintf("task '%s' in '%s' is listed more than once, likely through a task group", task, variant),
			Level:   Error,
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
						Message: fmt.Sprintf("build variant '%s' with task '%s' may only call %s %d time(s) but calls it %d time(s)", bv.Name, t.Name, commandName, times, count),
						Level:   level,
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

// validateTaskSyncSettings checks that task sync in the project settings have
// enabled task sync for the config.
func validateTaskSyncSettings(p *model.Project, ref *model.ProjectRef) ValidationErrors {
	if ref.TaskSync.IsConfigEnabled() {
		return nil
	}
	var errs ValidationErrors
	if s3PushCalls := p.TasksThatCallCommand(evergreen.S3PushCommandName); len(s3PushCalls) != 0 {
		errs = append(errs, ValidationError{
			Level:   Error,
			Message: fmt.Sprintf("cannot use %s command in project config when it is disabled by project settings", evergreen.S3PushCommandName),
		})
	}
	if s3PullCalls := p.TasksThatCallCommand(evergreen.S3PullCommandName); len(s3PullCalls) != 0 {
		errs = append(errs, ValidationError{
			Level:   Error,
			Message: fmt.Sprintf("cannot use %s command in project config when it is disabled by project settings", evergreen.S3PullCommandName),
		})
	}
	return errs
}

// bvsWithTasksThatCallCommand creates a mapping from build variants to tasks
// that run the given command cmd, including the list of matching commands for
// each task. Returns the total number of commands in the map.
func bvsWithTasksThatCallCommand(p *model.Project, cmd string) (map[string]map[string][]model.PluginCommandConf, int, error) {
	// build variant -> tasks that run cmd -> all matching commands
	bvToTasksWithCmds := map[string]map[string][]model.PluginCommandConf{}
	numCmds := 0
	catcher := grip.NewBasicCatcher()

	// addCmdsForTaskInBV adds commands that run for a task in a build variant
	// to the mapping.
	addCmdsForTaskInBV := func(bvToTaskWithCmds map[string]map[string][]model.PluginCommandConf, bv, taskUnit string, cmds []model.PluginCommandConf) {
		if len(cmds) == 0 {
			return
		}
		if _, ok := bvToTaskWithCmds[bv]; !ok {
			bvToTasksWithCmds[bv] = map[string][]model.PluginCommandConf{}
		}
		bvToTasksWithCmds[bv][taskUnit] = append(bvToTasksWithCmds[bv][taskUnit], cmds...)
		numCmds += len(cmds)
	}

	for _, bv := range p.BuildVariants {
		var preAndPostCmds []model.PluginCommandConf
		if p.Pre != nil {
			preAndPostCmds = append(preAndPostCmds, p.CommandsRunOnBV(p.Pre.List(), cmd, bv.Name)...)
		}
		if p.Post != nil {
			preAndPostCmds = append(preAndPostCmds, p.CommandsRunOnBV(p.Post.List(), cmd, bv.Name)...)
		}

		for _, bvtu := range bv.Tasks {
			if bvtu.IsGroup {
				tg := p.FindTaskGroup(bvtu.Name)
				if tg == nil {
					catcher.Errorf("cannot find definition of task group '%s' used in build variant '%s'", bvtu.Name, bv.Name)
					continue
				}
				// All setup/teardown commands that apply for this build variant
				// will run for this task.
				var setupAndTeardownCmds []model.PluginCommandConf
				if tg.SetupGroup != nil {
					setupAndTeardownCmds = append(setupAndTeardownCmds, p.CommandsRunOnBV(tg.SetupGroup.List(), cmd, bv.Name)...)
				}
				if tg.SetupTask != nil {
					setupAndTeardownCmds = append(setupAndTeardownCmds, p.CommandsRunOnBV(tg.SetupTask.List(), cmd, bv.Name)...)
				}
				if tg.TeardownGroup != nil {
					setupAndTeardownCmds = append(setupAndTeardownCmds, p.CommandsRunOnBV(tg.TeardownGroup.List(), cmd, bv.Name)...)
				}
				if tg.TeardownTask != nil {
					setupAndTeardownCmds = append(setupAndTeardownCmds, p.CommandsRunOnBV(tg.TeardownTask.List(), cmd, bv.Name)...)
				}
				for _, tgTask := range model.CreateTasksFromGroup(bvtu, p) {
					addCmdsForTaskInBV(bvToTasksWithCmds, bv.Name, tgTask.Name, setupAndTeardownCmds)
					if projTask := p.FindProjectTask(tgTask.Name); projTask != nil {
						cmds := p.CommandsRunOnBV(projTask.Commands, cmd, bv.Name)
						addCmdsForTaskInBV(bvToTasksWithCmds, bv.Name, tgTask.Name, cmds)
					} else {
						catcher.Errorf("cannot find definition of task '%s' used in task group '%s'", tgTask.Name, tg.Name)
					}
				}
			} else {
				// All pre/post commands that apply for this build variant will
				// run for this task.
				addCmdsForTaskInBV(bvToTasksWithCmds, bv.Name, bvtu.Name, preAndPostCmds)

				projTask := p.FindProjectTask(bvtu.Name)
				if projTask == nil {
					catcher.Errorf("cannot find definition of task '%s'", bvtu.Name)
					continue
				}
				cmds := p.CommandsRunOnBV(projTask.Commands, cmd, bv.Name)
				addCmdsForTaskInBV(bvToTasksWithCmds, bv.Name, bvtu.Name, cmds)
			}
		}
	}
	return bvToTasksWithCmds, numCmds, catcher.Resolve()
}

// validateTaskSyncCommands validates project's task sync commands.  In
// particular, s3.push should be called at most once per task and s3.pull should
// refer to a valid task running s3.push.  It does not check that the project
// settings allow task syncing - see validateTaskSyncSettings. If run long isn't set,
// we don't validate dependencies if there are too many commands.
func validateTaskSyncCommands(p *model.Project, runLong bool) ValidationErrors {
	errs := ValidationErrors{}

	// A task should not call s3.push multiple times.
	s3PushCalls := p.TasksThatCallCommand(evergreen.S3PushCommandName)
	errs = append(errs, validateTimesCalledPerTask(p, s3PushCalls, evergreen.S3PushCommandName, 1, Warning)...)

	bvToTaskCmds, numCmds, err := bvsWithTasksThatCallCommand(p, evergreen.S3PullCommandName)
	if err != nil {
		errs = append(errs, ValidationError{
			Level:   Error,
			Message: fmt.Sprintf("could not generate map of build variants with tasks that call command '%s': %s", evergreen.S3PullCommandName, err.Error()),
		})
	}

	checkDependencies := numCmds <= maxTaskSyncCommandsForDependenciesCheck || runLong
	if !checkDependencies {
		errs = append(errs, ValidationError{
			Level:   Warning,
			Message: fmt.Sprintf("too many commands using '%s' to check dependencies by default", evergreen.S3PullCommandName),
		})
	}
	var taskDefMapping map[model.TVPair]model.BuildVariantTaskUnit
	if checkDependencies {
		taskDefMapping = tvToTaskUnit(p)
	}
	for bv, taskCmds := range bvToTaskCmds {
		for task, cmds := range taskCmds {
			for _, cmd := range cmds {
				// This is only possible because we disallow expansions for the
				// task and build variant for s3.pull, which would prevent
				// evaluation of dependencies.
				s3PushTaskName, s3PushBVName, parseErr := parseS3PullParameters(cmd)
				if parseErr != nil {
					errs = append(errs, ValidationError{
						Level:   Error,
						Message: fmt.Sprintf("could not parse parameters for command '%s': %s", cmd.Command, parseErr.Error()),
					})
					continue
				}

				// If no build variant is explicitly stated, the build variant
				// is the same as the build variant of the task running s3.pull.
				if s3PushBVName == "" {
					s3PushBVName = bv
				}

				// Since s3.pull depends on the task running s3.push to run
				// first, ensure that this task for this build variant has a
				// dependency on the referenced task and build variant.
				s3PushTaskNode := model.TVPair{TaskName: s3PushTaskName, Variant: s3PushBVName}
				if checkDependencies {
					s3PullTaskNode := model.TVPair{TaskName: task, Variant: bv}
					if err := validateTVDependsOnTV(s3PullTaskNode, s3PushTaskNode, taskDefMapping); err != nil {
						errs = append(errs, ValidationError{
							Level: Error,
							Message: fmt.Sprintf("problem validating that task running command '%s' depends on task running command '%s': %s",
								evergreen.S3PullCommandName, evergreen.S3PushCommandName, err.Error()),
						})
					}
				}

				// Find the task referenced by s3.pull and ensure that it exists
				// and calls s3.push.
				cmds, err := p.CommandsRunOnTV(s3PushTaskNode, evergreen.S3PushCommandName)
				if err != nil {
					errs = append(errs, ValidationError{
						Level: Error,
						Message: fmt.Sprintf("problem validating that task '%s' runs command '%s': %s",
							s3PushTaskName, evergreen.S3PushCommandName, err.Error()),
					})
				} else if len(cmds) == 0 {
					errs = append(errs, ValidationError{
						Level: Error,
						Message: fmt.Sprintf("task '%s' in build variant '%s' does not run command '%s'",
							s3PushTaskName, s3PushBVName, evergreen.S3PushCommandName),
					})
				}
			}
		}
	}

	return errs
}

// validateTVDependsOnTV checks that the task in the given build variant has a
// dependency on the task in the given build variant.
func validateTVDependsOnTV(source, target model.TVPair, tvToTaskUnit map[model.TVPair]model.BuildVariantTaskUnit) error {
	if source == target {
		return errors.Errorf("task '%s' in build variant '%s' cannot depend on itself",
			source.TaskName, source.Variant)
	}
	visited := map[model.TVPair]bool{}
	var allTVs []model.TVPair
	for tv := range tvToTaskUnit {
		visited[tv] = false
		allTVs = append(allTVs, tv)
	}

	sourceTask, ok := tvToTaskUnit[source]
	if !ok {
		return errors.Errorf("could not find task '%s' in build variant '%s'",
			source.TaskName, source.Variant)
	}

	// patches and mainline builds shouldn't depend on anything that's git tag only,
	// while something that could run in a git tag build can't depend on something that's patchOnly.
	// requireOnNonGitTag is just requireOnPatches & requireOnNonPatches so we don't consider this case.
	depReqs := dependencyRequirements{
		lastDepNeedsSuccess: true,
		requireOnPatches:    !sourceTask.SkipOnPatchBuild() && !sourceTask.SkipOnNonGitTagBuild(),
		requireOnNonPatches: !sourceTask.SkipOnNonPatchBuild() && !sourceTask.SkipOnNonGitTagBuild(),
		requireOnGitTag:     !sourceTask.SkipOnNonPatchBuild() && !sourceTask.SkipOnGitTagBuild(),
	}
	depFound, err := dependencyMustRun(target, source, depReqs, allTVs, visited, tvToTaskUnit)
	if err != nil {
		return errors.Wrapf(err, "error searching for dependency of task '%s' in build variant '%s'"+
			" on task '%s' in build variant '%s'",
			source.TaskName, source.Variant,
			target.TaskName, target.Variant)
	}
	if !depFound {
		errMsg := "task '%s' on build variant '%s' must depend on" +
			" task '%s' in build variant '%s' running and succeeding"
		if depReqs.requireOnPatches && depReqs.requireOnNonPatches {
			errMsg += " for both patches and non-patches"
		} else if depReqs.requireOnPatches {
			errMsg += " for patches"
		} else if depReqs.requireOnNonPatches {
			errMsg += " for non-patches"
		} else if depReqs.requireOnGitTag {
			errMsg += " for git-tag builds"
		}
		return errors.Errorf(errMsg, source.TaskName, source.Variant, target.TaskName, target.Variant)
	}
	return nil
}

type dependencyRequirements struct {
	lastDepNeedsSuccess bool
	requireOnPatches    bool
	requireOnNonPatches bool
	requireOnGitTag     bool
}

// dependencyMustRun checks whether or not the current task in a build
// variant depends on the success of the target task in the build variant.
func dependencyMustRun(target model.TVPair, current model.TVPair, depReqs dependencyRequirements, allNodes []model.TVPair, visited map[model.TVPair]bool, tvToTaskUnit map[model.TVPair]model.BuildVariantTaskUnit) (bool, error) {
	isVisited, ok := visited[current]
	// If the node is missing, the dependency graph is malformed.
	if !ok {
		return false, errors.Errorf("dependency '%s' in variant '%s' is not defined", current.TaskName, current.Variant)
	}
	// If a node is revisited on this DFS, the dependency graph cannot be
	// checked because it has a cycle.
	if isVisited {
		return false, errors.Errorf("dependency '%s' in variant '%s' is in a dependency cycle", current.TaskName, current.Variant)
	}

	taskUnit := tvToTaskUnit[current]
	// Even if current depends on target according to the dependency graph, if
	// the current task will not run in the same cases as the source (e.g. the
	// source task runs on patches but current task does not, or if the current task
	// is only available to git tag builds), the dependency is
	// not reachable from this branch.
	if depReqs.requireOnPatches && (taskUnit.SkipOnPatchBuild() || taskUnit.SkipOnNonGitTagBuild()) {
		return false, nil
	}
	if depReqs.requireOnNonPatches && (taskUnit.SkipOnNonPatchBuild() || taskUnit.SkipOnNonGitTagBuild()) {
		return false, nil
	}
	if depReqs.requireOnGitTag && (taskUnit.SkipOnNonPatchBuild() || taskUnit.SkipOnGitTagBuild()) {
		return false, nil
	}

	if current == target {
		return depReqs.lastDepNeedsSuccess, nil
	}

	visited[current] = true

	depsToNodes := dependenciesForTaskUnit(current, taskUnit, allNodes)
	for dep, depNodes := range depsToNodes {
		// If the task must run on patches but this dependency is optional on
		// patches, we cannot traverse this dependency branch.
		if depReqs.requireOnPatches && dep.PatchOptional {
			continue
		}
		depReqs.lastDepNeedsSuccess = dep.Status == "" || dep.Status == evergreen.TaskSucceeded

		for _, depNode := range depNodes {
			reachable, err := dependencyMustRun(target, depNode, depReqs, allNodes, visited, tvToTaskUnit)
			if err != nil {
				return false, errors.Wrap(err, "dependency graph has problems")
			}
			if reachable {
				return true, nil
			}
		}
	}

	visited[current] = false

	return false, nil
}

// dependenciesForTaskUnit returns a map of this task unit's dependencies to
// and all the task-build variant pairs the task unit depends on.
func dependenciesForTaskUnit(tv model.TVPair, taskUnit model.BuildVariantTaskUnit, allTVs []model.TVPair) map[model.TaskUnitDependency][]model.TVPair {
	depsToNodes := map[model.TaskUnitDependency][]model.TVPair{}
	for _, dep := range taskUnit.DependsOn {
		if dep.Variant != model.AllVariants {
			// Handle dependencies with one variant.

			depTV := model.TVPair{TaskName: dep.Name, Variant: dep.Variant}
			if depTV.Variant == "" {
				// Use the current variant if none is specified.
				depTV.Variant = tv.Variant
			}

			if depTV.TaskName == model.AllDependencies {
				// Handle dependencies with all-dependencies by adding all the
				// variant's tasks except the current one.
				for _, currTV := range allTVs {
					if currTV.TaskName != tv.TaskName && currTV.Variant == depTV.Variant {
						depsToNodes[dep] = append(depsToNodes[dep], currTV)
					}
				}
			} else {
				// Normal case: just append the dependency with its task and
				// variant.
				depsToNodes[dep] = append(depsToNodes[dep], depTV)
			}
		} else {
			// Handle dependencies with all-variants.

			if dep.Name != model.AllDependencies {
				// Handle dependencies with all-variants by adding the task from
				// all variants except the current task-variant.
				for _, currTV := range allTVs {
					if currTV.TaskName == dep.Name && (currTV != tv) {
						depsToNodes[dep] = append(depsToNodes[dep], currTV)
					}
				}
			} else {
				// Handle dependencies with all-variants and all-dependencies by
				// adding all the tasks except the current one.
				for _, currTV := range allTVs {
					if currTV != tv {
						depsToNodes[dep] = append(depsToNodes[dep], currTV)
					}
				}
			}
		}
	}

	return depsToNodes
}

// parseS3PullParameters returns the parameters from the s3.pull command that
// references the push task.
func parseS3PullParameters(c model.PluginCommandConf) (task, bv string, err error) {
	if len(c.Params) == 0 {
		return "", "", errors.Errorf("command '%s' has no parameters", c.Command)
	}
	var i interface{}
	var ok bool
	var paramName string

	paramName = "task"
	i, ok = c.Params[paramName]
	if !ok {
		return "", "", errors.Errorf("command '%s' needs parameter '%s' defined", c.Command, paramName)
	} else {
		task, ok = i.(string)
		if !ok {
			return "", "", errors.Errorf("command '%s' was supplied parameter '%s' but is not a string argument, got %T", c.Command, paramName, i)
		}
	}

	paramName = "from_build_variant"
	i, ok = c.Params[paramName]
	if !ok {
		return task, "", nil
	}
	bv, ok = i.(string)
	if !ok {
		return "", "", errors.Errorf("command '%s' was supplied parameter '%s' but is not a string argument, got %T", c.Command, paramName, i)
	}
	return task, bv, nil
}
