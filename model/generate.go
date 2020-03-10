package model

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

const (
	maxGeneratedBuildVariants = 100
	maxGeneratedTasks         = 1000
)

// GeneratedProject is a subset of the Project type, and is generated from the
// JSON from a `generate.tasks` command.
type GeneratedProject struct {
	BuildVariants []parserBV                 `yaml:"buildvariants"`
	Tasks         []parserTask               `yaml:"tasks"`
	Functions     map[string]*YAMLCommandSet `yaml:"functions"`
	TaskGroups    []parserTaskGroup          `yaml:"task_groups"`

	TaskID string
}

// MergeGeneratedProjects takes a slice of generated projects and returns a single, deduplicated project.
func MergeGeneratedProjects(projects []GeneratedProject) *GeneratedProject {
	catcher := grip.NewBasicCatcher()

	bvs := map[string]*parserBV{}
	tasks := map[string]*parserTask{}
	functions := map[string]*YAMLCommandSet{}
	taskGroups := map[string]*parserTaskGroup{}

	for _, p := range projects {
	mergeBuildVariants:
		for i, bv := range p.BuildVariants {
			if len(bv.Tasks) == 0 {
				if _, ok := bvs[bv.Name]; ok {
					catcher.Add(errors.Errorf("found duplicate buildvariant (%s)", bv.Name))
				} else {
					bvs[bv.Name] = &p.BuildVariants[i]
					continue mergeBuildVariants
				}
			}
			if _, ok := bvs[bv.Name]; ok {
				bvs[bv.Name].Tasks = append(bvs[bv.Name].Tasks, bv.Tasks...)
				bvs[bv.Name].DisplayTasks = append(bvs[bv.Name].DisplayTasks, bv.DisplayTasks...)
			}
			bvs[bv.Name] = &p.BuildVariants[i]
		}
		for i, t := range p.Tasks {
			if _, ok := tasks[t.Name]; ok {
				catcher.Add(errors.Errorf("found duplicate task (%s)", t.Name))
			} else {
				tasks[t.Name] = &p.Tasks[i]
			}
		}
		for f, val := range p.Functions {
			if _, ok := functions[f]; ok {
				catcher.Add(errors.Errorf("found duplicate function (%s)", f))
			}
			functions[f] = val
		}
		for i, tg := range p.TaskGroups {
			if _, ok := taskGroups[tg.Name]; ok {
				catcher.Add(errors.Errorf("found duplicate task group (%s)", tg.Name))
			} else {
				taskGroups[tg.Name] = &p.TaskGroups[i]
			}
		}
	}

	g := &GeneratedProject{}
	for i := range bvs {
		g.BuildVariants = append(g.BuildVariants, *bvs[i])
	}
	for i := range tasks {
		g.Tasks = append(g.Tasks, *tasks[i])
	}
	g.Functions = functions
	for i := range taskGroups {
		g.TaskGroups = append(g.TaskGroups, *taskGroups[i])
	}
	return g
}

// ParseProjectFromJSON returns a GeneratedTasks type from JSON. We use the
// YAML parser instead of the JSON parser because the JSON parser will not
// properly unmarshal into a struct with multiple fields as options, like the YAMLCommandSet.
func ParseProjectFromJSONString(data string) (GeneratedProject, error) {
	g := GeneratedProject{}
	dataAsJSON := []byte(data)
	if err := yaml.Unmarshal(dataAsJSON, &g); err != nil {
		return g, errors.Wrap(err, "error unmarshaling into GeneratedTasks")
	}
	return g, nil
}

// ParseProjectFromJSON returns a GeneratedTasks type from JSON. We use the
// YAML parser instead of the JSON parser because the JSON parser will not
// properly unmarshal into a struct with multiple fields as options, like the YAMLCommandSet.
func ParseProjectFromJSON(data []byte) (GeneratedProject, error) {
	g := GeneratedProject{}
	if err := yaml.Unmarshal(data, &g); err != nil {
		return g, errors.Wrap(err, "error unmarshaling into GeneratedTasks")
	}
	return g, nil
}

// NewVersion adds the buildvariants, tasks, and functions
// from a generated project config to a project, and returns the previous config number.
func (g *GeneratedProject) NewVersion() (*Project, *ParserProject, *Version, *task.Task, *projectMaps, error) {
	// Get task, version, and project.
	t, err := task.FindOneId(g.TaskID)
	if err != nil {
		return nil, nil, nil, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: errors.Wrapf(err, "error finding task %s", g.TaskID).Error()}
	}
	if t == nil {
		return nil, nil, nil, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: fmt.Sprintf("unable to find task %s", g.TaskID)}
	}
	v, err := VersionFindOneId(t.Version)
	if err != nil {
		return nil, nil, nil, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: errors.Wrapf(err, "error finding version %s", t.Version).Error()}
	}
	if v == nil {
		return nil, nil, nil, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: fmt.Sprintf("unable to find version %s", t.Version)}
	}
	p, pp, err := LoadProjectForVersion(v, t.Project, false)
	if err != nil {
		return nil, nil, nil, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: errors.Wrapf(err, "error getting project for version %s", t.Version).Error()}
	}

	// Cache project data in maps for quick lookup
	cachedProject := cacheProjectData(p)

	// Validate generated project against original project.
	if err = g.validateGeneratedProject(p, cachedProject); err != nil {
		// Return version in this error case for handleError, which checks for a race. We only need to do this in cases where there is a validation check.
		return nil, pp, v, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: errors.Wrap(err, "generated project is invalid").Error()}
	}

	newPP, err := g.addGeneratedProjectToConfig(pp, v.Config, cachedProject)
	if err != nil {
		return nil, nil, nil, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: errors.Wrap(err, "error creating config from generated config").Error()}
	}
	newPP.Id = v.Id
	p, err = TranslateProject(newPP)
	if err != nil {
		return nil, nil, nil, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: errors.Wrap(err, "error translating project").Error()}
	}
	return p, newPP, v, t, &cachedProject, nil
}

func (g *GeneratedProject) Save(ctx context.Context, p *Project, pp *ParserProject, v *Version, t *task.Task, pm *projectMaps) error {
	if err := updateParserProject(v, pp); err != nil {
		return errors.WithStack(err)
	}

	if err := g.saveNewBuildsAndTasks(ctx, pm, v, p, t); err != nil {
		return errors.Wrap(err, "error savings new builds and tasks")
	}
	return nil
}

// update the parser project using the newest config number (if using legacy version config, this comes from version)
func updateParserProject(v *Version, pp *ParserProject) error {
	updateNum := pp.ConfigUpdateNumber + 1
	// legacy: most likely a version for which no parser project exists
	if pp.ConfigUpdateNumber < v.ConfigUpdateNumber {
		updateNum = v.ConfigUpdateNumber + 1
	}

	if err := pp.UpsertWithConfigNumber(updateNum); err != nil {
		return errors.Wrapf(err, "error upserting parser project '%s'", pp.Id)
	}
	return nil
}

func cacheProjectData(p *Project) projectMaps {
	cachedProject := projectMaps{
		buildVariants: map[string]struct{}{},
		tasks:         map[string]*ProjectTask{},
		functions:     map[string]*YAMLCommandSet{},
	}
	// use a set because we never need to look up buildvariants
	for _, bv := range p.BuildVariants {
		cachedProject.buildVariants[bv.Name] = struct{}{}
	}
	for _, t := range p.Tasks {
		cachedProject.tasks[t.Name] = &t
	}
	// functions is already a map, cache it anyway for convenience
	cachedProject.functions = p.Functions
	return cachedProject
}

// saveNewBuildsAndTasks saves new builds and tasks to the db.
func (g *GeneratedProject) saveNewBuildsAndTasks(ctx context.Context, cachedProject *projectMaps, v *Version, p *Project, t *task.Task) error {
	// inherit priority from the parent task
	for i, projBv := range p.BuildVariants {
		for j := range projBv.Tasks {
			p.BuildVariants[i].Tasks[j].Priority = t.Priority
		}
	}

	newTVPairs := TaskVariantPairs{}
	for _, bv := range g.BuildVariants {
		newTVPairs = appendTasks(newTVPairs, bv, p)
	}
	newTVPairs.ExecTasks = IncludePatchDependencies(p, newTVPairs.ExecTasks)

	// group into new builds and new tasks for existing builds
	builds, err := build.Find(build.ByVersion(v.Id).WithFields(build.IdKey, build.BuildVariantKey))
	if err != nil {
		return errors.Wrap(err, "problem finding builds for version")
	}
	buildSet := map[string]struct{}{}
	for _, b := range builds {
		buildSet[b.BuildVariant] = struct{}{}
	}
	newTVPairsForExistingVariants := TaskVariantPairs{}
	newTVPairsForNewVariants := TaskVariantPairs{}
	for _, execTask := range newTVPairs.ExecTasks {
		if _, ok := buildSet[execTask.Variant]; ok {
			newTVPairsForExistingVariants.ExecTasks = append(newTVPairsForExistingVariants.ExecTasks, execTask)
		} else {
			newTVPairsForNewVariants.ExecTasks = append(newTVPairsForNewVariants.ExecTasks, execTask)
		}
	}
	for _, dispTask := range newTVPairs.DisplayTasks {
		if _, ok := buildSet[dispTask.Variant]; ok {
			newTVPairsForExistingVariants.DisplayTasks = append(newTVPairsForExistingVariants.DisplayTasks, dispTask)
		} else {
			newTVPairsForNewVariants.DisplayTasks = append(newTVPairsForNewVariants.DisplayTasks, dispTask)
		}
	}

	tasksInExistingBuilds, err := AddNewTasks(ctx, true, v, p, newTVPairsForExistingVariants, g.TaskID)
	if err != nil {
		return errors.Wrap(err, "errors adding new tasks")
	}

	_, tasksInNewBuilds, err := AddNewBuilds(ctx, true, v, p, newTVPairsForNewVariants, g.TaskID)
	if err != nil {
		return errors.Wrap(err, "errors adding new builds")
	}

	if err = addDependencies(t, append(tasksInExistingBuilds, tasksInNewBuilds...)); err != nil {
		return errors.Wrap(err, "error adding dependencies")
	}

	return nil
}

func addDependencies(t *task.Task, newTaskIds []string) error {
	statuses := []string{evergreen.TaskSucceeded, task.AllStatuses}
	for _, status := range statuses {
		if err := t.UpdateDependsOn(status, newTaskIds); err != nil {
			return errors.Wrapf(err, "can't update tasks depending on '%s'", t.Id)
		}
	}

	return nil
}

func appendTasks(pairs TaskVariantPairs, bv parserBV, p *Project) TaskVariantPairs {
	taskGroups := map[string]TaskGroup{}
	for _, tg := range p.TaskGroups {
		taskGroups[tg.Name] = tg
	}
	for _, t := range bv.Tasks {
		if tg, ok := taskGroups[t.Name]; ok {
			for _, taskInGroup := range tg.Tasks {
				pairs.ExecTasks = append(pairs.ExecTasks, TVPair{bv.Name, taskInGroup})
			}
		} else {
			pairs.ExecTasks = append(pairs.ExecTasks, TVPair{bv.Name, t.Name})
		}
	}
	for _, dt := range bv.DisplayTasks {
		pairs.DisplayTasks = append(pairs.DisplayTasks, TVPair{bv.Name, dt.Name})
	}
	return pairs
}

// addGeneratedProjectToConfig takes a ParserProject and a YML config and returns a new one with the GeneratedProject included.
// support for YML config will be degraded.
func (g *GeneratedProject) addGeneratedProjectToConfig(intermediateProject *ParserProject, config string, cachedProject projectMaps) (*ParserProject, error) {
	var err error
	if intermediateProject == nil {
		intermediateProject, err = createIntermediateProject([]byte(config))
		if err != nil {
			return nil, errors.Wrapf(err, "error creating intermediate project")
		}
	}

	// Append buildvariants, tasks, and functions to the config.
	intermediateProject.TaskGroups = append(intermediateProject.TaskGroups, g.TaskGroups...)
	intermediateProject.Tasks = append(intermediateProject.Tasks, g.Tasks...)
	for key, val := range g.Functions {
		intermediateProject.Functions[key] = val
	}
	for _, bv := range g.BuildVariants {
		// If the buildvariant already exists, append tasks to it.
		if _, ok := cachedProject.buildVariants[bv.Name]; ok {
			for i, intermediateProjectBV := range intermediateProject.BuildVariants {
				if intermediateProjectBV.Name == bv.Name {
					intermediateProject.BuildVariants[i].Tasks = append(intermediateProject.BuildVariants[i].Tasks, bv.Tasks...)
					intermediateProject.BuildVariants[i].DisplayTasks = append(intermediateProject.BuildVariants[i].DisplayTasks, bv.DisplayTasks...)
				}
			}
		} else {
			// If the buildvariant does not exist, create it.
			intermediateProject.BuildVariants = append(intermediateProject.BuildVariants, bv)
		}
	}
	return intermediateProject, nil
}

// projectMaps is a struct of maps of project fields, which allows efficient comparisons of generated projects to projects.
type projectMaps struct {
	buildVariants map[string]struct{}
	tasks         map[string]*ProjectTask
	functions     map[string]*YAMLCommandSet
}

// validateMaxTasksAndVariants validates that the GeneratedProject contains fewer than 100 variants and 1000 tasks.
func (g *GeneratedProject) validateMaxTasksAndVariants(catcher grip.Catcher) {
	if len(g.BuildVariants) > maxGeneratedBuildVariants {
		catcher.Add(errors.Errorf("it is illegal to generate more than %d buildvariants", maxGeneratedBuildVariants))
	}
	if len(g.Tasks) > maxGeneratedTasks {
		catcher.Add(errors.Errorf("it is illegal to generate more than %d tasks", maxGeneratedTasks))
	}
}

// validateNoRedefine validates that buildvariants, tasks, or functions, are not redefined
// except to add a task to a buildvariant.
func (g *GeneratedProject) validateNoRedefine(cachedProject projectMaps, catcher grip.Catcher) {
	for _, bv := range g.BuildVariants {
		if _, ok := cachedProject.buildVariants[bv.Name]; ok {
			{
				if isNonZeroBV(bv) {
					catcher.Add(errors.Errorf("cannot redefine buildvariants in 'generate.tasks' (%s), except to add tasks", bv.Name))
				}
			}
		}
	}
	for _, t := range g.Tasks {
		if _, ok := cachedProject.tasks[t.Name]; ok {
			catcher.Add(errors.Errorf("cannot redefine tasks in 'generate.tasks' (%s)", t.Name))
		}
	}
	for f := range g.Functions {
		if _, ok := cachedProject.functions[f]; ok {
			catcher.Add(errors.Errorf("cannot redefine functions in 'generate.tasks' (%s)", f))
		}
	}
}

func isNonZeroBV(bv parserBV) bool {
	if bv.DisplayName != "" || len(bv.Expansions) > 0 || len(bv.Modules) > 0 ||
		bv.Disabled || len(bv.Tags) > 0 || bv.Push ||
		bv.BatchTime != nil || bv.Stepback != nil || len(bv.RunOn) > 0 {
		return true
	}
	return false
}

// validateNoRecursiveGenerateTasks validates that no 'generate.tasks' calls another 'generate.tasks'.
func (g *GeneratedProject) validateNoRecursiveGenerateTasks(cachedProject projectMaps, catcher grip.Catcher) {
	for _, t := range g.Tasks {
		for _, cmd := range t.Commands {
			if cmd.Command == evergreen.GenerateTasksCommandName {
				catcher.Add(errors.New("cannot define 'generate.tasks' from a 'generate.tasks' block"))
			}
		}
	}
	for _, f := range g.Functions {
		for _, cmd := range f.List() {
			if cmd.Command == evergreen.GenerateTasksCommandName {
				catcher.Add(errors.New("cannot define 'generate.tasks' from a 'generate.tasks' block"))
			}
		}
	}
	for _, bv := range g.BuildVariants {
		for _, t := range bv.Tasks {
			if projectTask, ok := cachedProject.tasks[t.Name]; ok {
				validateCommands(projectTask, cachedProject, t, catcher)
			}
		}
	}
}

func validateCommands(projectTask *ProjectTask, cachedProject projectMaps, pvt parserBVTaskUnit, catcher grip.Catcher) {
	for _, cmd := range projectTask.Commands {
		if cmd.Command == evergreen.GenerateTasksCommandName {
			catcher.Add(errors.Errorf("cannot assign a task that calls 'generate.tasks' from a 'generate.tasks' block (%s)", pvt.Name))
		}
		if cmd.Function != "" {
			if functionCmds, ok := cachedProject.functions[cmd.Function]; ok {
				for _, functionCmd := range functionCmds.List() {
					if functionCmd.Command == evergreen.GenerateTasksCommandName {
						catcher.Add(errors.Errorf("cannot assign a task that calls 'generate.tasks' from a 'generate.tasks' block (%s)", cmd.Function))
					}
				}
			}
		}
	}
}

func (g *GeneratedProject) validateGeneratedProject(p *Project, cachedProject projectMaps) error {
	catcher := grip.NewBasicCatcher()

	g.validateMaxTasksAndVariants(catcher)
	g.validateNoRedefine(cachedProject, catcher)
	g.validateNoRecursiveGenerateTasks(cachedProject, catcher)

	return errors.WithStack(catcher.Resolve())
}
