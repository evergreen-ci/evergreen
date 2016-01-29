package validator

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/plugin"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/evergreen-ci/evergreen/util"
	"strconv"
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
// TODO stop using a package variable for this
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
	ensureHasNecessaryProjectFields,
	verifyTaskDependencies,
	validateBVNames,
	validateBVTaskNames,
	checkAllDependenciesSpec,
	validateProjectTaskNames,
	validateProjectTaskIdsAndTags,
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

// Makes sure that the dependencies for the tasks in the project form a
// valid dependency graph (no cycles).
func checkDependencyGraph(project *model.Project) []ValidationError {
	errs := []ValidationError{}

	// map of task name and variant -> BuildVariantTask
	tasksByNameAndVariant := map[model.TVPair]model.BuildVariantTask{}

	// generate task nodes for every task and variant combination
	visited := map[model.TVPair]bool{}
	allNodes := []model.TVPair{}
	for _, bv := range project.BuildVariants {
		for _, t := range bv.Tasks {
			t.Populate(project.GetSpecForTask(t.Name))
			node := model.TVPair{bv.Name, t.Name}

			tasksByNameAndVariant[node] = t
			visited[node] = false
			allNodes = append(allNodes, node)
		}
	}

	// run through the task nodes, checking their dependency graphs for cycles
	for _, node := range allNodes {
		// the visited nodes
		if err := dependencyCycleExists(node, visited, tasksByNameAndVariant); err != nil {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf(
						"dependency error for '%v' task: %v", node.TaskName, err),
				},
			)
		}
	}

	return errs
}

// Helper for checking the dependency graph for cycles.
func dependencyCycleExists(node model.TVPair, visited map[model.TVPair]bool,
	tasksByNameAndVariant map[model.TVPair]model.BuildVariantTask) error {

	v, ok := visited[node]
	// if the node does not exist, the deps are broken
	if !ok {
		return fmt.Errorf("dependency %v is not present in the project config", node)
	}
	// if the task has already been visited, then a cycle certainly exists
	if v {
		return fmt.Errorf("dependency %v is part of a dependency cycle", node)
	}

	visited[node] = true

	task := tasksByNameAndVariant[node]
	depNodes := []model.TVPair{}
	// build a list of all possible dependency nodes for the task
	for _, dep := range task.DependsOn {
		if dep.Variant != model.AllVariants {
			// handle regular dependencies
			dn := model.TVPair{TaskName: dep.Name}
			if dep.Variant == "" {
				// use the current variant if none is specified
				dn.Variant = node.Variant
			} else {
				dn.Variant = dep.Variant
			}
			// handle * case by grabbing all the variant's tasks that aren't the current one
			if dn.TaskName == model.AllDependencies {
				for n, _ := range visited {
					if n.TaskName != node.TaskName && n.Variant == dn.Variant {
						depNodes = append(depNodes, n)
					}
				}
			} else {
				// normal case: just append the variant
				depNodes = append(depNodes, dn)
			}
		} else {
			// handle the all-variants case by adding all nodes that are
			// of the same task (but not the current node)
			if dep.Name != model.AllDependencies {
				for n, _ := range visited {
					if n.TaskName == dep.Name && (n != node) {
						depNodes = append(depNodes, n)
					}
				}
			} else {
				// edge case where variant and task name are both *
				for n, _ := range visited {
					if n != node {
						depNodes = append(depNodes, n)
					}
				}
			}
		}
	}

	// for each of the task's dependencies, make a recursive call
	for _, dn := range depNodes {
		if err := dependencyCycleExists(dn, visited, tasksByNameAndVariant); err != nil {
			return err
		}
	}

	// remove the task from the visited map so that higher-level calls do not see it
	visited[node] = false

	// no cycle found
	return nil
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

// Checks that the basic fields that are required by any project are present.
func ensureHasNecessaryProjectFields(project *model.Project) []ValidationError {
	errs := []ValidationError{}

	if project.BatchTime < 0 {
		errs = append(errs,
			ValidationError{
				Message: fmt.Sprintf("project '%v' must have a "+
					"non-negative 'batchtime' set", project.Identifier),
			},
		)
	}

	if project.CommandType != "" {
		if project.CommandType != model.SystemCommandType &&
			project.CommandType != model.TestCommandType {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("project '%v' contains an invalid "+
						"command type: %v", project.Identifier, project.CommandType),
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
							Level: Warning,
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
						Level: Warning,
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
		command := fmt.Sprintf("'%v' command", cmd.Command)
		_, err := registry.GetCommands(cmd, project.Functions)
		if err != nil {
			if cmd.Function != "" {
				command = fmt.Sprintf("'%v' function", cmd.Function)
			}
			errs = append(errs, ValidationError{Message: fmt.Sprintf("%v section in %v: %v", section, command, err)})
		}
		if cmd.Type != "" {
			if cmd.Type != model.SystemCommandType &&
				cmd.Type != model.TestCommandType {
				msg := fmt.Sprintf("%v section in '%v': invalid command type: '%v'", section, command, cmd.Type)
				errs = append(errs, ValidationError{Message: msg})
			}
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
	for _, pl := range plugin.CommandPlugins {
		if err := pluginRegistry.Register(pl); err != nil {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("failed to register plugin %v: %v", pl.Name(), err),
				},
			)
		}
	}

	seen := make(map[string]bool, 0)

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

		for _, c := range commands.List() {
			if c.Function != "" {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("can not reference a function within a "+
							"function: '%v' referenced within '%v'", c.Function, funcName),
					},
				)

			}
		}

		// this checks for duplicate function definitions in the project.
		if seen[funcName] {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf(`project '%v' has duplicate definition of "%v"`,
						project.Identifier, funcName),
				},
			)
		}
		seen[funcName] = true
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

// validateProjectTaskIdsAndTags ensures that task tags and ids only contain valid characters
func validateProjectTaskIdsAndTags(project *model.Project) []ValidationError {
	errs := []ValidationError{}
	// create a map to hold the task names
	for _, task := range project.Tasks {
		// check task name
		if i := strings.IndexAny(task.Name, model.InvalidCriterionRunes); i == 0 {
			errs = append(errs, ValidationError{
				Message: fmt.Sprintf("task '%v' has invalid name: starts with invalid character %v",
					task.Name, strconv.QuoteRune(rune(task.Name[0])))})
		}
		// check tag names
		for _, tag := range task.Tags {
			if i := strings.IndexAny(tag, model.InvalidCriterionRunes); i == 0 {
				errs = append(errs, ValidationError{
					Message: fmt.Sprintf("task '%v' has invalid tag '%v': starts with invalid character %v",
						task.Name, tag, strconv.QuoteRune(rune(tag[0])))})
			}
			if i := util.IndexWhiteSpace(tag); i != -1 {
				errs = append(errs, ValidationError{
					Message: fmt.Sprintf("task '%v' has invalid tag '%v': tag contains white space",
						task.Name, tag)})
			}
		}
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
		depNames := map[model.TVPair]bool{}

		for _, dep := range task.DependsOn {
			// make sure the dependency is not specified more than once
			if depNames[model.TVPair{dep.Name, dep.Variant}] {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("project '%v' contains a "+
							"duplicate dependency '%v' specified for task '%v'",
							project.Identifier, dep.Name, task.Name),
					},
				)
			}
			depNames[model.TVPair{dep.Name, dep.Variant}] = true

			// check that the status is valid
			switch dep.Status {
			case evergreen.TaskSucceeded, evergreen.TaskFailed, model.AllStatuses, "":
				// these are all valid
			default:
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("project '%v' contains an invalid dependency status for task '%v': %v",
							project.Identifier, task.Name, dep.Status)})
			}

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
