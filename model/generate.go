package model

import (
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
	yaml "gopkg.in/yaml.v2"
)

const (
	maxGeneratedBuildVariants = 100
	maxGeneratedTasks         = 1000
	generateTasksCommand      = "generate.tasks"
)

// GeneratedProject is a subset of the Project type, and is generated from the
// JSON from a `generate.tasks` command.
type GeneratedProject struct {
	BuildVariants []parserBV                 `yaml:"buildvariants"`
	Tasks         []parserTask               `yaml:"tasks"`
	Functions     map[string]*YAMLCommandSet `yaml:"functions"`

	TaskID string
}

// MergeGeneratedProjects takes a slice of generated projects and returns a single, deduplicated project.
func MergeGeneratedProjects(projects []GeneratedProject) *GeneratedProject {
	catcher := grip.NewBasicCatcher()

	bvs := map[string]*parserBV{}
	tasks := map[string]*parserTask{}
	functions := map[string]*YAMLCommandSet{}

	for _, p := range projects {
		for _, bv := range p.BuildVariants {
			if len(bv.Tasks) == 0 {
				if _, ok := bvs[bv.Name]; ok {
					catcher.Add(errors.Errorf("found duplicate buildvariant (%s)", bv.Name))
				} else {
					bvs[bv.Name] = &bv
				}
			}
			if _, ok := bvs[bv.Name]; ok {
				bvs[bv.Name].Tasks = append(bvs[bv.Name].Tasks, bv.Tasks...)
			}
			bvs[bv.Name] = &bv
		}
		for _, t := range p.Tasks {
			if _, ok := tasks[t.Name]; ok {
				catcher.Add(errors.Errorf("found duplicate task (%s)", t.Name))
			} else {
				tasks[t.Name] = &t
			}
		}
		for f, val := range p.Functions {
			if _, ok := functions[f]; ok {
				catcher.Add(errors.Errorf("found duplicate function (%s)", f))
			}
			functions[f] = val
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

// AddGeneratedProjectToVersion adds the buildvariants, tasks, and functions
// from a generated project config to a project.
func (g *GeneratedProject) AddGeneratedProjectToVersion() error {
	// Get task, version, and project.
	t, err := task.FindOneId(g.TaskID)
	if err != nil {
		return errors.Wrapf(err, "error finding task %s", g.TaskID)
	}
	v, err := version.FindOneId(t.Version)
	if err != nil {
		return errors.Wrapf(err, "error finding version %s", t.Version)
	}
	p := &Project{}
	if err := LoadProjectInto([]byte(v.Config), t.Project, p); err != nil {
		return errors.Wrap(err, "error reading project yaml")
	}

	// Cache project data in maps for quick lookup
	cachedProject := cacheProjectData(p)

	// Validate generated project against original project.
	if err := g.validateGeneratedProject(p, cachedProject); err != nil {
		return errors.Wrap(err, "generated project is invalid")
	}

	// Create and save new config. This is definitely racy.
	config, err := g.addGeneratedProjectToConfig(v.Config, cachedProject)
	if err != nil {
		return errors.Wrap(err, "error creating config from generated config")
	}
	v.Config = config
	if err := version.UpdateOne(version.ById(v.Id), bson.M{"$set": bson.M{version.ConfigKey: config}}); err != nil {
		return errors.Wrapf(err, "error getting version %s", v.Id)
	}

	// Save new builds and tasks to the db.
	if err := LoadProjectInto([]byte(config), t.Project, p); err != nil {
		return errors.Wrap(err, "error reading project yaml")
	}
	if err := g.saveNewBuildsAndTasks(cachedProject, v, p); err != nil {
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
func (g *GeneratedProject) saveNewBuildsAndTasks(cachedProject projectMaps, v *version.Version, p *Project) error {
	newTVPairsForExistingVariants := TaskVariantPairs{}
	newTVPairsForNewVariants := TaskVariantPairs{}
	for _, bv := range g.BuildVariants {
		// If the buildvariant already exists, append tasks to it.
		if _, ok := cachedProject.buildVariants[bv.Name]; ok {
			for _, t := range bv.Tasks {
				newTVPairsForExistingVariants.ExecTasks = append(newTVPairsForExistingVariants.ExecTasks, TVPair{bv.Name, t.Name})
			}
		} else {
			// If the buildvariant does not exist, create it.
			for _, t := range bv.Tasks {
				newTVPairsForNewVariants.ExecTasks = append(newTVPairsForNewVariants.ExecTasks, TVPair{bv.Name, t.Name})
			}
		}
	}
	if err := AddNewTasks(true, v, p, newTVPairsForExistingVariants); err != nil {
		return errors.Wrap(err, "errors adding new builds")
	}
	if err := AddNewBuilds(true, v, p, newTVPairsForNewVariants); err != nil {
		return errors.Wrap(err, "errors adding new builds")
	}
	return nil
}

// addGeneratedProjectToConfig takes a YML config and returns a new one with the GeneratedProject included.
func (g *GeneratedProject) addGeneratedProjectToConfig(config string, cachedProject projectMaps) (string, error) {
	// Append buildvariants, tasks, and functions to the config.
	intermediateProject, errs := createIntermediateProject([]byte(config))
	if errs != nil {
		return "", errors.Wrap(errs[0], "error creating intermediate project")
	}
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
		bv.BatchTime != nil || bv.Stepback != nil || len(bv.RunOn) > 0 ||
		len(bv.DisplayTasks) > 0 {
		return true
	}
	return false
}

// validateNoRecursiveGenerateTasks validates that no 'generate.tasks' calls another 'generate.tasks'.
func (g *GeneratedProject) validateNoRecursiveGenerateTasks(cachedProject projectMaps, catcher grip.Catcher) {
	for _, t := range g.Tasks {
		for _, cmd := range t.Commands {
			if cmd.Command == generateTasksCommand {
				catcher.Add(errors.New("cannot define 'generate.tasks' from a 'generate.tasks' block"))
			}
		}
	}
	for _, f := range g.Functions {
		for _, cmd := range f.List() {
			if cmd.Command == generateTasksCommand {
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
		if cmd.Command == generateTasksCommand {
			catcher.Add(errors.Errorf("cannot assign a task that calls 'generate.tasks' from a 'generate.tasks' block (%s)", pvt.Name))
		}
		if cmd.Function != "" {
			if functionCmds, ok := cachedProject.functions[cmd.Function]; ok {
				for _, functionCmd := range functionCmds.List() {
					if functionCmd.Command == generateTasksCommand {
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
