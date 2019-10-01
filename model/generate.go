package model

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
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
func ParseProjectFromJSON(data []byte) (GeneratedProject, error) {
	g := GeneratedProject{}
	if err := yaml.Unmarshal(data, &g); err != nil {
		return g, errors.Wrap(err, "error unmarshaling into GeneratedTasks")
	}
	return g, nil
}

// NewVersion adds the buildvariants, tasks, and functions
// from a generated project config to a project, and returns the previous config number.
func (g *GeneratedProject) NewVersion() (*Project, *Version, *task.Task, *projectMaps, error) {
	// Get task, version, and project.
	t, err := task.FindOneId(g.TaskID)
	if err != nil {
		return nil, nil, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: errors.Wrapf(err, "error finding task %s", g.TaskID).Error()}
	}
	if t == nil {
		return nil, nil, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: fmt.Sprintf("unable to find task %s", g.TaskID)}
	}
	v, err := VersionFindOneId(t.Version)
	if err != nil {
		return nil, nil, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: errors.Wrapf(err, "error finding version %s", t.Version).Error()}
	}
	if v == nil {
		return nil, nil, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: fmt.Sprintf("unable to find version %s", t.Version)}
	}
	if v.Config == "" {
		return nil, nil, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: fmt.Sprintf("unable to find config string for version %s", t.Version)}
	}
	p := &Project{}
	if err = LoadProjectInto([]byte(v.Config), t.Project, p); err != nil {
		return nil, nil, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: errors.Wrapf(err, "error reading project yaml for version %s", t.Version).Error()}
	}

	// Cache project data in maps for quick lookup
	cachedProject := cacheProjectData(p)

	// Validate generated project against original project.
	if err = g.validateGeneratedProject(p, cachedProject); err != nil {
		return nil, nil, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: errors.Wrap(err, "generated project is invalid").Error()}
	}

	newConfig, err := g.addGeneratedProjectToConfig(v.Config, cachedProject)
	if err != nil {
		return nil, nil, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: errors.Wrap(err, "error creating config from generated config").Error()}
	}
	v.Config = newConfig
	if err := LoadProjectInto([]byte(v.Config), t.Project, p); err != nil {
		return nil, nil, nil, nil,
			gimlet.ErrorResponse{StatusCode: http.StatusBadRequest, Message: errors.Wrap(err, "error reading project yaml").Error()}
	}
	return p, v, t, &cachedProject, nil
}

func (g *GeneratedProject) Save(ctx context.Context, p *Project, v *Version, t *task.Task, pm *projectMaps) error {
	query := bson.M{
		VersionIdKey:           v.Id,
		VersionConfigNumberKey: v.ConfigUpdateNumber,
	}
	update := bson.M{
		"$set": bson.M{VersionConfigKey: v.Config},
		"$inc": bson.M{VersionConfigNumberKey: 1},
	}

	err := VersionUpdateOne(query, update)
	if err != nil {
		return errors.Wrapf(err, "error updating version %s", v.Id)
	}

	if v.Requester == evergreen.MergeTestRequester {
		mergeTask, err := task.FindMergeTaskForVersion(v.Id)
		if err != nil && !adb.ResultsNotFound(err) {
			return errors.Wrap(err, "error finding merge task")
		}
		// if a merge task exists then update its dependencies
		if !adb.ResultsNotFound(err) {
			if err = v.UpdateMergeTaskDependencies(p, mergeTask); err != nil {
				return errors.Wrap(err, "error updating merge task")
			}
		}
	}

	v.ConfigUpdateNumber += 1
	if err := g.saveNewBuildsAndTasks(ctx, pm, v, p, t.Priority); err != nil {
		return errors.Wrap(err, "error savings new builds and tasks")
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
func (g *GeneratedProject) saveNewBuildsAndTasks(ctx context.Context, cachedProject *projectMaps, v *Version, p *Project, parentPriority int64) error {
	newTVPairsForExistingVariants := TaskVariantPairs{}
	newTVPairsForNewVariants := TaskVariantPairs{}
	builds, err := build.Find(build.ByVersion(v.Id).WithFields(build.IdKey, build.BuildVariantKey))
	if err != nil {
		return errors.Wrap(err, "problem finding builds for version")
	}
	buildSet := map[string]struct{}{}
	for _, b := range builds {
		buildSet[b.BuildVariant] = struct{}{}
	}
	for _, bv := range g.BuildVariants {
		// If the buildvariant already exists, append tasks to it.
		if _, ok := buildSet[bv.Name]; ok {
			newTVPairsForExistingVariants = appendTasks(newTVPairsForExistingVariants, bv, p)
		} else {
			// If the buildvariant does not exist, create it.
			newTVPairsForNewVariants = appendTasks(newTVPairsForNewVariants, bv, p)
		}
	}

	// inherit priority from the parent task
	projBvs := []BuildVariant(p.BuildVariants)
	for i, projBv := range projBvs {
		for j, _ := range projBv.Tasks {
			projBvs[i].Tasks[j].Priority = parentPriority
		}
	}

	dependencies := IncludePatchDependencies(p, newTVPairsForExistingVariants.ExecTasks)
	newTVPairsForExistingVariants.ExecTasks = append(newTVPairsForExistingVariants.ExecTasks, dependencies...)
	if err := AddNewTasks(ctx, true, v, p, newTVPairsForExistingVariants, g.TaskID); err != nil {
		return errors.Wrap(err, "errors adding new tasks")
	}
	dependencies = IncludePatchDependencies(p, newTVPairsForNewVariants.ExecTasks)
	newTVPairsForNewVariants.ExecTasks = append(newTVPairsForNewVariants.ExecTasks, dependencies...)
	if err := AddNewBuilds(true, v, p, newTVPairsForNewVariants, g.TaskID); err != nil {
		return errors.Wrap(err, "errors adding new builds")
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

// addGeneratedProjectToConfig takes a YML config and returns a new one with the GeneratedProject included.
func (g *GeneratedProject) addGeneratedProjectToConfig(config string, cachedProject projectMaps) (string, error) {
	// Append buildvariants, tasks, and functions to the config.
	intermediateProject, errs := createIntermediateProject([]byte(config))
	if errs != nil {
		return "", errors.Wrap(errs[0], "error creating intermediate project")
	}
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
	byteConfig, err := yaml.Marshal(intermediateProject)
	if err != nil {
		return "", errors.Wrap(err, "error marshalling new project config")
	}
	return string(byteConfig), nil
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
