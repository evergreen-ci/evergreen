package validator

import (
	"bytes"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
)

type projectValidator func(*model.Project) ValidationErrors

type ValidationErrorLevel int64

const (
	Error ValidationErrorLevel = iota
	Warning
	unauthorizedCharacters = "|"
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

// Functions used to validate the syntax of a project configuration file.
var projectSyntaxValidators = []projectValidator{
	ensureHasNecessaryBVFields,
	checkDependencyGraph,
	validatePluginCommands,
	ensureHasNecessaryProjectFields,
	verifyTaskDependencies,
	verifyTaskRequirements,
	validateTaskNames,
	validateBVNames,
	validateBVBatchTimes,
	validateDisplayTaskNames,
	validateBVTaskNames,
	validateBVsContainTasks,
	checkAllDependenciesSpec,
	validateProjectTaskNames,
	validateProjectTaskIdsAndTags,
	validateTaskGroups,
	validateCreateHosts,
	validateDuplicateTaskDefinition,
	validateGenerateTasks,
}

// Functions used to validate the semantics of a project configuration file.
var projectSemanticValidators = []projectValidator{
	checkTaskCommands,
	checkTaskGroups,
	checkLoggerConfig,
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

// create a slice of all distro names
func getDistroIds() ([]string, error) {
	return getDistroIdsForProject("")
}

// create a slice of all valid distro names for a project
func getDistroIdsForProject(projectID string) ([]string, error) {
	// create a slice of all known distros
	distros, err := distro.Find(distro.All)
	if err != nil {
		return nil, err
	}
	distroIds := []string{}
	for _, d := range distros {
		if projectID != "" && len(d.ValidProjects) > 0 {
			if util.StringSliceContains(d.ValidProjects, projectID) {
				distroIds = append(distroIds, d.Id)
			}
		} else {
			distroIds = append(distroIds, d.Id)
		}
	}
	return distroIds, nil
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
func CheckProjectSyntax(project *model.Project) ValidationErrors {
	validationErrs := ValidationErrors{}
	for _, projectSyntaxValidator := range projectSyntaxValidators {
		validationErrs = append(validationErrs,
			projectSyntaxValidator(project)...)
	}

	// get distroIds for ensureReferentialIntegrity validation
	distroIds, err := getDistroIdsForProject(project.Identifier)
	if err != nil {
		validationErrs = append(validationErrs, ValidationError{Message: "can't get distros from database"})
	}
	validationErrs = append(validationErrs, ensureReferentialIntegrity(project, distroIds)...)
	return validationErrs
}

// verify that the project configuration semantics and configuration syntax is valid
func CheckProjectConfigurationIsValid(project *model.Project) error {
	syntaxErrs := CheckProjectSyntax(project)
	if len(syntaxErrs) > 0 {
		syntaxErrsAtErrorLevel := ValidationErrors{}
		for _, err := range syntaxErrs {
			if err.Level == Error {
				syntaxErrsAtErrorLevel = append(syntaxErrsAtErrorLevel, err)
			}
		}
		if len(syntaxErrsAtErrorLevel) > 0 {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("project syntax is invalid: %s", ValidationErrorsToString(syntaxErrs)),
			}
		}
	}
	semanticErrs := CheckProjectSemantics(project)
	if len(semanticErrs) > 0 {
		semanticErrsAtErrorLevel := ValidationErrors{}
		for _, err := range semanticErrs {
			if err.Level == Error {
				semanticErrsAtErrorLevel = append(semanticErrsAtErrorLevel, err)
			}
		}
		if len(semanticErrsAtErrorLevel) > 0 {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("project semantics is invalid: %s", ValidationErrorsToString(semanticErrs)),
			}
		}
	}
	return nil
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

	// map of task name and variant -> BuildVariantTaskUnit
	tasksByNameAndVariant := map[model.TVPair]model.BuildVariantTaskUnit{}

	// generate task nodes for every task and variant combination
	visited := map[model.TVPair]bool{}
	allNodes := []model.TVPair{}

	taskGroups := map[string]struct{}{}
	for _, tg := range project.TaskGroups {
		taskGroups[tg.Name] = struct{}{}
	}
	for _, bv := range project.BuildVariants {
		tasksToAdd := []model.BuildVariantTaskUnit{}
		for _, t := range bv.Tasks {
			if _, ok := taskGroups[t.Name]; ok {
				tasksToAdd = append(tasksToAdd, model.CreateTasksFromGroup(t, project)...)
			} else {
				tasksToAdd = append(tasksToAdd, t)
			}
		}
		for _, t := range tasksToAdd {
			t.Populate(project.GetSpecForTask(t.Name))
			node := model.TVPair{
				Variant:  bv.Name,
				TaskName: t.Name,
			}

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
						"dependency error for '%s' task: %s", node.TaskName, err),
				},
			)
		}
	}

	return errs
}

// Helper for checking the dependency graph for cycles.
func dependencyCycleExists(node model.TVPair, visited map[model.TVPair]bool,
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
				for n := range visited {
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
				for n := range visited {
					if n.TaskName == dep.Name && (n != node) {
						depNodes = append(depNodes, n)
					}
				}
			} else {
				// edge case where variant and task name are both *
				for n := range visited {
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
		hasTaskWithoutDistro := false
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
		for _, task := range buildVariant.Tasks {
			if len(task.Distros) == 0 {
				hasTaskWithoutDistro = true
				break
			}
		}
		if hasTaskWithoutDistro && len(buildVariant.RunOn) == 0 {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("buildvariant '%s' in project '%s' "+
						"must either specify run_on field or have every task "+
						"specify a distro.",
						buildVariant.Name, project.Identifier),
				},
			)
		}
	}
	return errs
}

// Checks that the basic fields that are required by any project are present.
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
		if !util.StringSliceContains(evergreen.ValidCommandTypes, project.CommandType) {
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
func ensureReferentialIntegrity(project *model.Project, distroIds []string) ValidationErrors {
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
			for _, distroId := range task.Distros {
				if !util.StringSliceContains(distroIds, distroId) {
					errs = append(errs,
						ValidationError{
							Message: fmt.Sprintf("task '%s' in buildvariant "+
								"'%s' in project '%s' references an "+
								"invalid distro '%s'.\nValid distros "+
								"include: \n\t- %s", task.Name,
								buildVariant.Name, project.Identifier,
								distroId, strings.Join(distroIds, "\n\t- ")),
							Level: Warning,
						},
					)
				}
			}
		}
		for _, distroId := range buildVariant.RunOn {
			if !util.StringSliceContains(distroIds, distroId) {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("buildvariant '%s' in project "+
							"'%s' references an invalid distro '%s'.\n"+
							"Valid distros include: \n\t- %s",
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

// validateTaskNames ensures the task names do not contain unauthorized characters.
func validateTaskNames(project *model.Project) ValidationErrors {
	unauthorizedTaskCharacters := unauthorizedCharacters + " "
	errs := ValidationErrors{}
	for _, task := range project.Tasks {
		if strings.ContainsAny(strings.TrimSpace(task.Name), unauthorizedTaskCharacters) {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("task name %s contains unauthorized characters ('%s')",
						task.Name, unauthorizedTaskCharacters),
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
		if dispName == "" { // Default display name to the actual name (identifier)
			dispName = buildVariant.Name
		}
		displayNames[dispName] = displayNames[dispName] + 1

		if strings.ContainsAny(buildVariant.Name, unauthorizedCharacters) {
			errs = append(errs,
				ValidationError{
					Message: fmt.Sprintf("buildvariant name %s contains unauthorized characters (%s)",
						buildVariant.Name, unauthorizedCharacters),
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
			for _, etn := range dp.ExecutionTasks {
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
			if !util.StringSliceContains(evergreen.ValidCommandTypes, cmd.Type) {
				msg := fmt.Sprintf("%s section in '%s': invalid command type: '%s'", section, commandName, cmd.Type)
				errs = append(errs, ValidationError{Message: msg})
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
		// validate project pre section
		errs = append(errs, validateCommands("pre", project, project.Pre.List())...)
	}

	if project.Post != nil {
		// validate project post section
		errs = append(errs, validateCommands("post", project, project.Post.List())...)
	}

	if project.Timeout != nil {
		// validate project timeout section
		errs = append(errs, validateCommands("timeout", project, project.Timeout.List())...)
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

// Makes sure that the dependencies for the tasks have the correct fields,
// and that the fields reference valid tasks.
func verifyTaskRequirements(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
	for _, bvt := range project.FindAllBuildVariantTasks() {
		for _, r := range bvt.Requires {
			if project.FindProjectTask(r.Name) == nil {
				if r.Name == model.AllDependencies {
					errs = append(errs, ValidationError{Message: fmt.Sprintf(
						"task '%s': * is not supported for requirement selectors", bvt.Name)})
				} else {
					errs = append(errs,
						ValidationError{Message: fmt.Sprintf(
							"task '%s' requires non-existent task '%s'", bvt.Name, r.Name)})
				}
			}
			if r.Variant != "" && r.Variant != model.AllVariants && project.FindBuildVariant(r.Variant) == nil {
				errs = append(errs, ValidationError{Message: fmt.Sprintf(
					"task '%s' requires non-existent variant '%s'", bvt.Name, r.Variant)})
			}
			vs := project.FindVariantsWithTask(r.Name)
			if r.Variant != "" && r.Variant != model.AllVariants {
				if !util.StringSliceContains(vs, r.Variant) {
					errs = append(errs, ValidationError{Message: fmt.Sprintf(
						"task '%s' requires task '%s' on variant '%s'", bvt.Name, r.Name, r.Variant)})
				}
			} else {
				if !util.StringSliceContains(vs, bvt.Variant) {
					errs = append(errs, ValidationError{Message: fmt.Sprintf(
						"task '%s' requires task '%s' on variant '%s'", bvt.Name, r.Name, bvt.Variant)})
				}
			}
		}
	}
	return errs
}

// Makes sure that the dependencies for the tasks have the correct fields,
// and that the fields have valid values
func verifyTaskDependencies(project *model.Project) ValidationErrors {
	errs := ValidationErrors{}
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
			if depNames[model.TVPair{TaskName: dep.Name, Variant: dep.Variant}] {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("project '%s' contains a "+
							"duplicate dependency '%s' specified for task '%s'",
							project.Identifier, dep.Name, task.Name),
					},
				)
			}
			depNames[model.TVPair{TaskName: dep.Name, Variant: dep.Variant}] = true

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
			if dep.Name != model.AllDependencies && !taskNames[dep.Name] {
				errs = append(errs,
					ValidationError{
						Message: fmt.Sprintf("project '%s' contains a "+
							"non-existent task name '%s' in dependencies for "+
							"task '%s'", project.Identifier, dep.Name,
							task.Name),
					},
				)
			}
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
					Message: fmt.Sprintf("%s is listed in task group %s more than once", name, tg.Name),
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
		if tg.MaxHosts > (len(tg.Tasks) / 2) {
			errs = append(errs, ValidationError{
				Message: fmt.Sprintf("task group %s has max number of hosts %d greater than half the number of tasks %d", tg.Name, tg.MaxHosts, len(tg.Tasks)),
				Level:   Warning,
			})
		}
		for _, t := range tg.Tasks {
			tasksInTaskGroups[t] = tg.Name
		}
	}
	for t, tg := range tasksInTaskGroups {
		spec := p.GetSpecForTask(t)
		if len(spec.DependsOn) > 0 {
			dependencies := make([]string, 0, len(spec.DependsOn))
			for _, dependsOn := range spec.DependsOn {
				dependencies = append(dependencies, dependsOn.Name)
			}
			errs = append(errs, ValidationError{
				Message: fmt.Sprintf("task %s in task group %s has a dependency on another task (%s), "+
					"which can cause task group tasks to be scheduled out of order", t, tg, dependencies),
				Level: Warning,
			})
		}
	}
	return errs
}

func validateDuplicateTaskDefinition(p *model.Project) ValidationErrors {
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

func validateCreateHosts(p *model.Project) ValidationErrors {
	ts := p.TasksThatCallCommand(evergreen.CreateHostCommandName)
	errs := validateTimesCalledPerTask(p, ts, evergreen.CreateHostCommandName, 3)
	errs = append(errs, validateTimesCalledTotal(p, ts, evergreen.CreateHostCommandName, 50)...)
	return errs
}

func validateTimesCalledPerTask(p *model.Project, ts map[string]int, commandName string, times int) (errs ValidationErrors) {
	for _, bv := range p.BuildVariants {
		for _, t := range bv.Tasks {
			if count, ok := ts[t.Name]; ok {
				if count > times {
					errs = append(errs, ValidationError{
						Message: fmt.Sprintf("variant %s task %s may only call %s %d times but calls it %d times", bv.Name, t.Name, commandName, times, count),
						Level:   Error,
					})
				}
			}
		}
	}
	return errs
}

func validateTimesCalledTotal(p *model.Project, ts map[string]int, commandName string, times int) (errs ValidationErrors) {
	total := 0
	for _, bv := range p.BuildVariants {
		for _, t := range bv.Tasks {
			if count, ok := ts[t.Name]; ok {
				total += count
			}
		}
	}
	if total > times {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("may only call %s %d times but it is called %d times", commandName, times, total),
			Level:   Error,
		})
	}
	return errs
}

// validateGenerateTasks validates that no task calls 'generate.tasks' more than once, since if one
// does, the server will noop it.
func validateGenerateTasks(p *model.Project) ValidationErrors {
	ts := p.TasksThatCallCommand(evergreen.GenerateTasksCommandName)
	return validateTimesCalledPerTask(p, ts, evergreen.GenerateTasksCommandName, 1)
}
