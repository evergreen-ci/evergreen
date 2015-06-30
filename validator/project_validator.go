package validator

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/plugin"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/evergreen-ci/evergreen/util"
	"strings"
)

type projectValidator func(*model.Project) []ValidationError

type ValidationErrorLevel int64

const (
	Error ValidationErrorLevel = iota
	Warning
)

type ValidationError struct {
	Level   ValidationErrorLevel `json:"level"`
	Message string               `json:"message"`
}

// mapping of all valid distros
var (
	distroIds []string
)

// Functions used to validate the syntax of a project configuration file. Any
// validation errors here for remote configuration files are fatal and will
// cause stubs to be created for the project.
var projectSyntaxValidators = []projectValidator{
	ensureHasNecessaryBVFields,
	checkDependencyGraph,
	validatePluginCommands,
	verifyTaskDependencies,
	validateBVNames,
	validateBVTaskNames,
	checkAllDependenciesSpec,
	validateProjectTaskNames,
	ensureReferentialIntegrity,
}

// Functions used to validate the semantics of a project configuration file.
// Validations errors here are not fatal. However, it is recommended that the
// suggested corrections are applied.
var projectSemanticValidators = []projectValidator{
	checkTaskCommands,
}

func (vr ValidationError) Error() string {
	return vr.Message
}

// create a slice of all valid distro names
func populateDistroIds() *ValidationError {
	// create a slice of all known distros
	distros, err := distro.Find(distro.All)
	if err != nil {
		return &ValidationError{
			Message: fmt.Sprintf("error finding distros: %v", err),
			Level:   Error,
		}
	}
	distroIds = []string{}
	for _, d := range distros {
		if !util.SliceContains(distroIds, d.Id) {
			distroIds = append(distroIds, d.Id)
		}
	}
	return nil
}

// verify that the project configuration semantics is valid
func CheckProjectSemantics(project *model.Project) []ValidationError {

	if err := populateDistroIds(); err != nil {
		return []ValidationError{*err}
	}

	validationErrs := []ValidationError{}
	for _, projectSemanticValidator := range projectSemanticValidators {
		validationErrs = append(validationErrs,
			projectSemanticValidator(project)...)
	}
	return validationErrs
}

// verify that the project configuration syntax is valid
func CheckProjectSyntax(project *model.Project) []ValidationError {

	if err := populateDistroIds(); err != nil {
		return []ValidationError{*err}
	}

	validationErrs := []ValidationError{}
	for _, projectSyntaxValidator := range projectSyntaxValidators {
		validationErrs = append(validationErrs,
			projectSyntaxValidator(project)...)
	}
	return validationErrs
}

// ensure that if any task spec references 'model.AllDependencies', it
// references no other dependency
func checkAllDependenciesSpec(project *model.Project) []ValidationError {
	errs := []ValidationError{}
	for _, task := range project.Tasks {
		if len(task.DependsOn) > 1 {
			for _, dependency := range task.DependsOn {
				if dependency.Name == model.AllDependencies {
					errs = append(errs,
						ValidationError{
							Message: fmt.Sprintf("task '%v' in project '%v' "+
								"contains the all dependencies (%v)' "+
								"specification and other explicit dependencies",
								project.Identifier, task.Name,
								model.AllDependencies),
						},
					)
				}
			}
		}
	}
	return errs
}

// Makes sure that the dependencies for the tasks in the project forms a
// valid dependency graph (no cycles).
func checkDependencyGraph(project *model.Project) []ValidationError {
	errs := []ValidationError{}

	// map of task name -> task
	tasksByName := map[string]model.ProjectTask{}
	for _, task := range project.Tasks {
		tasksByName[task.Name] = task
	}

	// run through the tasks, checking their dependency graphs for cycles
	for _, task := range project.Tasks {
		// the visited nodes
		if dependencyCycleExists(task, map[string]bool{}, tasksByName) {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("a cycle exists in the dependency "+
						"graph for task %v", task.Name),
				},
			)
		}
	}

	return errs
}

// Helper for checking the dependency graph for cycles.
func dependencyCycleExists(task model.ProjectTask, visited map[string]bool,
	tasksByName map[string]model.ProjectTask) bool {

	// if the task has already been visited, then a cycle certainly exists
	if visited[task.Name] {
		return true
	}
	visited[task.Name] = true

	// for each of the task's dependencies, make a recursive call
	for _, dep := range task.DependsOn {
		if dependencyCycleExists(tasksByName[dep.Name], visited, tasksByName) {
			return true
		}
	}

	// remove the task from the visited map so that higher-level calls do not
	// see it
	visited[task.Name] = false

	// no cycle found
	return false
}

// Ensures that the project has at least one buildvariant and also that all the
// fields required for any buildvariant definition are present
func ensureHasNecessaryBVFields(project *model.Project) []ValidationError {
	errs := []ValidationError{}

	if len(project.BuildVariants) == 0 {
		return []ValidationError{
			ValidationError{
				Message: fmt.Sprintf("project '%v' must specify at least one "+
					"buildvariant", project.Identifier),
			},
		}
	}

	for _, buildVariant := range project.BuildVariants {
		hasTaskWithoutDistro := false
		if buildVariant.Name == "" {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("project '%v' buildvariant must "+
						"have a name", project.Identifier),
				},
			)
		}
		if len(buildVariant.Tasks) == 0 {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("buildvariant '%v' in project '%v' "+
						"must have at least one task", buildVariant.Name,
						project.Identifier),
				},
			)
		}
		for _, task := range buildVariant.Tasks {
			if len(task.Distros) == 0 {
				hasTaskWithoutDistro = true
				break
			}
		}
		if hasTaskWithoutDistro && len(buildVariant.RunOn) == 0 {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("buildvariant '%v' in project '%v' "+
						"must either specify run_on field or have every task "+
						"specify a distro.\nValid distros include: \n\t- %v",
						buildVariant.Name, project.Identifier, strings.Join(
							distroIds, "\n\t- ")),
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
func ensureReferentialIntegrity(project *model.Project) []ValidationError {
	errs := []ValidationError{}
	// create a set of all the task names
	allTaskNames := map[string]bool{}
	for _, task := range project.Tasks {
		allTaskNames[task.Name] = true
	}

	for _, buildVariant := range project.BuildVariants {
		buildVariantTasks := map[string]bool{}
		for _, task := range buildVariant.Tasks {
			if _, ok := allTaskNames[task.Name]; !ok {
				if task.Name == "" {
					errs = append(errs,
						ValidationError{
							Message: fmt.Sprintf("tasks for buildvariant '%v' "+
								"in project '%v' must each have a name field",
								project.Identifier, buildVariant.Name),
						},
					)
				} else {
					errs = append(errs,
						ValidationError{
							Message: fmt.Sprintf("buildvariant '%v' in "+
								"project '%v' references a non-existent "+
								"task '%v'", buildVariant.Name,
								project.Identifier, task.Name),
						},
					)
				}
			}
			buildVariantTasks[task.Name] = true
			for _, distroId := range task.Distros {
				if !util.SliceContains(distroIds, distroId) {
					errs = append(errs,
						ValidationError{
							Message: fmt.Sprintf("task '%v' in buildvariant "+
								"'%v' in project '%v' references a "+
								"non-existent distro '%v'.\nValid distros "+
								"include: \n\t- %v", task.Name,
								buildVariant.Name, project.Identifier,
								distroId, strings.Join(distroIds, "\n\t- ")),
						},
					)
				}
			}
		}
		for _, distroId := range buildVariant.RunOn {
			if !util.SliceContains(distroIds, distroId) {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("buildvariant '%v' in project "+
							"'%v' references a non-existent distro '%v'.\n"+
							"Valid distros include: \n\t- %v",
							buildVariant.Name, project.Identifier, distroId,
							strings.Join(distroIds, "\n\t- ")),
					},
				)
			}
		}
	}
	return errs
}

// Ensures there aren't any duplicate buildvariant names specified in the given
// project
func validateBVNames(project *model.Project) []ValidationError {
	errs := []ValidationError{}
	buildVariantNames := map[string]bool{}
	for _, buildVariant := range project.BuildVariants {
		if _, ok := buildVariantNames[buildVariant.Name]; ok {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("project '%v' buildvariant "+
						"'%v' already exists", project.Identifier,
						buildVariant.Name),
				},
			)
		}
		buildVariantNames[buildVariant.Name] = true
	}
	return errs
}

// Checks each task definitions to determine if a command is specified
func checkTaskCommands(project *model.Project) []ValidationError {
	errs := []ValidationError{}
	for _, task := range project.Tasks {
		if len(task.Commands) == 0 {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("task '%v' in project '%v' does not "+
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
func validateBVTaskNames(project *model.Project) []ValidationError {
	errs := []ValidationError{}
	for _, buildVariant := range project.BuildVariants {
		buildVariantTasks := map[string]bool{}
		for _, task := range buildVariant.Tasks {
			if _, ok := buildVariantTasks[task.Name]; ok {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("task '%v' in buildvariant '%v' "+
							"in project '%v' already exists",
							task.Name, buildVariant.Name, project.Identifier),
					},
				)
			}
			buildVariantTasks[task.Name] = true
		}
	}
	return errs
}

// Helper for validating a set of plugin commands given a project/registry
func validateCommands(section string, project *model.Project, registry plugin.Registry,
	commands []model.PluginCommandConf) []ValidationError {
	errs := []ValidationError{}
	for _, cmd := range commands {
		_, err := registry.GetCommands(cmd, project.Functions)
		if err != nil {
			command := fmt.Sprintf("'%v' command", cmd.Command)
			if cmd.Function != "" {
				command = fmt.Sprintf("'%v' function", cmd.Function)
			}
			errs = append(errs, ValidationError{Message: fmt.Sprintf("%v section in %v: %v", section, command, err)})
		}
	}
	return errs
}

// Ensures there any plugin commands referenced in a project's configuration
// are specified in a valid format
func validatePluginCommands(project *model.Project) []ValidationError {
	errs := []ValidationError{}
	pluginRegistry := plugin.NewSimpleRegistry()

	// register the published plugins
	for _, pl := range plugin.Published {
		if err := pluginRegistry.Register(pl); err != nil {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("failed to register plugin %v: %v", pl.Name(), err),
				},
			)
		}
	}

	// validate each function definition
	for funcName, commands := range project.Functions {
		valErrs := validateCommands("functions", project, pluginRegistry, commands.List())
		for _, err := range valErrs {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("'%v' project's '%v' definition: %v",
						project.Identifier, funcName, err),
				},
			)
		}
	}

	if project.Pre != nil {
		// validate project pre section
		errs = append(errs, validateCommands("pre", project, pluginRegistry, project.Pre.List())...)
	}

	if project.Post != nil {
		// validate project post section
		errs = append(errs, validateCommands("post", project, pluginRegistry, project.Post.List())...)
	}

	if project.Timeout != nil {
		// validate project timeout section
		errs = append(errs, validateCommands("timeout", project, pluginRegistry, project.Timeout.List())...)
	}

	// validate project tasks section
	for _, task := range project.Tasks {
		errs = append(errs, validateCommands("tasks", project, pluginRegistry, task.Commands)...)
	}
	return errs
}

// Ensures there aren't any duplicate task names for this project
func validateProjectTaskNames(project *model.Project) []ValidationError {
	errs := []ValidationError{}
	// create a map to hold the task names
	taskNames := map[string]bool{}
	for _, task := range project.Tasks {
		if _, ok := taskNames[task.Name]; ok {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("task '%v' in project '%v' "+
						"already exists", task.Name, project.Identifier),
				},
			)
		}
		taskNames[task.Name] = true
	}
	return errs
}

// Makes sure that the dependencies for the tasks have the correct fields,
// and that the fields have valid values
func verifyTaskDependencies(project *model.Project) []ValidationError {
	errs := []ValidationError{}
	// create a set of all the task names
	taskNames := map[string]bool{}
	for _, task := range project.Tasks {
		taskNames[task.Name] = true
	}

	for _, task := range project.Tasks {
		// create a set of the dependencies, to check for duplicates
		depNames := map[string]bool{}

		for _, dep := range task.DependsOn {
			// make sure the dependency is not specified more than once
			if depNames[dep.Name] {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("project '%v' contains a "+
							"duplicate dependency '%v' specified for task '%v'",
							project.Identifier, dep.Name, task.Name),
					},
				)
			}
			depNames[dep.Name] = true

			// check that name of the dependency task is valid
			if dep.Name != model.AllDependencies && !taskNames[dep.Name] {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("project '%v' contains a "+
							"non-existent task name '%v' in dependencies for "+
							"task '%v'", project.Identifier, dep.Name,
							task.Name),
					},
				)
			}
		}
	}
	return errs
}
