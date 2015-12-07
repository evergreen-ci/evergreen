package model

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"gopkg.in/yaml.v2"
	"reflect"
	"strings"
)

const (
	TestCommandType   = "test"
	SystemCommandType = "system"
)

const (
	// DefaultCommandType is a system configuration option that is used to
	// differentiate between setup related commands and actual testing commands.
	DefaultCommandType = TestCommandType
)

type Project struct {
	Enabled            bool                       `yaml:"enabled,omitempty" bson:"enabled"`
	Stepback           bool                       `yaml:"stepback,omitempty" bson:"stepback"`
	BatchTime          int                        `yaml:"batchtime,omitempty" bson:"batch_time"`
	Owner              string                     `yaml:"owner,omitempty" bson:"owner_name"`
	Repo               string                     `yaml:"repo,omitempty" bson:"repo_name"`
	RemotePath         string                     `yaml:"remote_path,omitempty" bson:"remote_path"`
	RepoKind           string                     `yaml:"repokind,omitempty" bson:"repo_kind"`
	Branch             string                     `yaml:"branch,omitempty" bson:"branch_name"`
	Identifier         string                     `yaml:"identifier,omitempty" bson:"identifier"`
	DisplayName        string                     `yaml:"display_name,omitempty" bson:"display_name"`
	CommandType        string                     `yaml:"command_type,omitempty" bson:"command_type"`
	Pre                *YAMLCommandSet            `yaml:"pre,omitempty" bson:"pre"`
	Post               *YAMLCommandSet            `yaml:"post,omitempty" bson:"post"`
	Timeout            *YAMLCommandSet            `yaml:"timeout,omitempty" bson:"timeout"`
	CallbackTimeout    int                        `yaml:"callback_timeout_secs,omitempty" bson:"callback_timeout_secs"`
	Modules            []Module                   `yaml:"modules,omitempty" bson:"modules"`
	BuildVariants      []BuildVariant             `yaml:"buildvariants,omitempty" bson:"build_variants"`
	Functions          map[string]*YAMLCommandSet `yaml:"functions,omitempty" bson:"functions"`
	Tasks              []ProjectTask              `yaml:"tasks,omitempty" bson:"tasks"`
	BuildVariantMatrix BuildVariantMatrix         `yaml:"build_variant_matrix,omitempty" bson:"build_variant_matrix"`

	// Flag that indicates a project as requiring user authentication
	Private bool `yaml:"private,omitempty" bson:"private"`
}

// Unmarshalled from the "tasks" list in an individual build variant
type BuildVariantTask struct {
	// Name has to match the name field of one of the tasks specified at
	// the project level, or an error will be thrown
	Name string `yaml:"name,omitempty" bson:"name"`

	// fields to overwrite ProjectTask settings
	Patchable *bool            `yaml:"patchable,omitempty" bson:"patchable,omitempty"`
	Priority  int              `yaml:"priority,omitempty" bson:"priority"`
	DependsOn []TaskDependency `yaml:"depends_on,omitempty" bson:"depends_on"`

	// currently unsupported (TODO EVG-578)
	ExecTimeoutSecs int   `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs"`
	Stepback        *bool `yaml:"stepback,omitempty" bson:"stepback,omitempty"`

	// the distros that the task can be run on
	Distros []string `yaml:"distros,omitempty" bson:"distros"`
}

// Populate updates the base fields of the BuildVariantTask with
// fields from the project task definition
func (bvt *BuildVariantTask) Populate(pt ProjectTask) {
	// We never update "Name" or "Commands"
	if len(bvt.DependsOn) == 0 {
		bvt.DependsOn = pt.DependsOn
	}
	if bvt.Priority == 0 {
		bvt.Priority = pt.Priority
	}
	if bvt.Patchable == nil {
		bvt.Patchable = pt.Patchable
	}
	// TODO these are copied but unused until EVG-578 is completed
	if bvt.ExecTimeoutSecs == 0 {
		bvt.ExecTimeoutSecs = pt.ExecTimeoutSecs
	}
	if bvt.Stepback == nil {
		bvt.Stepback = pt.Stepback
	}
}

// UnmarshalYAML allows tasks to be referenced as single selector strings.
// This works by first attempting to unmarshal the YAML into a string
// and then falling back to the BuildVariantTask struct.
func (bvt *BuildVariantTask) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal just a selector string
	var onlySelector string
	if err := unmarshal(&onlySelector); err == nil {
		bvt.Name = onlySelector
		return nil
	}
	// we define a new type so that we can grab the yaml struct tags without the struct methods,
	// preventing infinte recursion on the UnmarshalYAML() method.
	type bvtCopyType BuildVariantTask
	var bvtc bvtCopyType
	err := unmarshal(&bvtc)
	if err != nil {
		return err
	}
	*bvt = BuildVariantTask(bvtc)
	return nil
}

type BuildVariant struct {
	Name        string            `yaml:"name,omitempty" bson:"name"`
	DisplayName string            `yaml:"display_name,omitempty" bson:"display_name"`
	Expansions  map[string]string `yaml:"expansions,omitempty" bson:"expansions"`
	Modules     []string          `yaml:"modules,omitempty" bson:"modules"`
	Disabled    bool              `yaml:"disabled,omitempty" bson:"disabled"`
	Push        bool              `yaml:"push,omitempty" bson:"push"`

	// Use a *int for 2 possible states
	// nil - not overriding the project setting
	// non-nil - overriding the project setting with this BatchTime
	BatchTime *int `yaml:"batchtime,omitempty" bson:"batchtime,omitempty"`

	// Use a *bool so that there are 3 possible states:
	//   1. nil   = not overriding the project setting (default)
	//   2. true  = overriding the project setting with true
	//   3. false = overriding the project setting with false
	Stepback *bool `yaml:"stepback,omitempty" bson:"stepback,omitempty"`

	// the default distros.  will be used to run a task if no distro field is
	// provided for the task
	RunOn []string `yaml:"run_on,omitempty" bson:"run_on"`

	// all of the tasks to be run on the build variant, compile through tests.
	Tasks                 []BuildVariantTask `yaml:"tasks,omitempty" bson:"tasks"`
	MatrixParameterValues map[string]string  `yaml:"matrix_parameter_values,omitempty" bson:"matrix_parameter_values"`
}

type Module struct {
	Name   string `yaml:"name,omitempty" bson:"name"`
	Branch string `yaml:"branch,omitempty" bson:"branch"`
	Repo   string `yaml:"repo,omitempty" bson:"repo"`
	Prefix string `yaml:"prefix,omitempty" bson:"prefix"`
}

type TestSuite struct {
	Name  string `yaml:"name,omitempty"`
	Phase string `yaml:"phase,omitempty"`
}

type PluginCommandConf struct {
	Function string `yaml:"func,omitempty" bson:"func"`
	// Type is used to differentiate between setup related commands and actual
	// testing commands.
	Type string `yaml:"type,omitempty" bson:"type"`

	// DisplayName is a human readable description of the function of a given
	// command.
	DisplayName string `yaml:"display_name,omitempty" bson:"display_name"`

	// Command is a unique identifier for the command configuration. It consists of a
	// plugin name and a command name.
	Command string `yaml:"command,omitempty" bson:"command"`

	// Variants is used to enumerate the particular sets of buildvariants to run
	// this command configuration on. If it is empty, it is run on all defined
	// variants.
	Variants []string `yaml:"variants,omitempty" bson:"variants"`

	// TimeoutSecs indicates the maximum duration the command is allowed to run
	// for. If undefined, it is unbounded.
	TimeoutSecs int `yaml:"timeout_secs,omitempty" bson:"timeout_secs"`

	// Params are used to supply configuratiion specific information.
	Params map[string]interface{} `yaml:"params,omitempty" bson:"params"`

	// Vars defines variables that can be used within commands.
	Vars map[string]string `yaml:"vars,omitempty" bson:"vars"`
}

type ArtifactInstructions struct {
	Include      []string `yaml:"include,omitempty" bson:"include"`
	ExcludeFiles []string `yaml:"excludefiles,omitempty" bson:"exclude_files"`
}

type MatrixParameterValue struct {
	Value      string            `yaml:"value,omitempty" bson:"value"`
	Expansions map[string]string `yaml:"expansions,omitempty" bson:"expansions"`
}

type MatrixParameter struct {
	Name   string                 `yaml:"name,omitempty" bson:"name"`
	Values []MatrixParameterValue `yaml:"values,omitempty" bson:"values"`
}

type BuildVariantMatrix struct {
	MatrixParameters []MatrixParameter `yaml:"matrix_parameters,omitempty" bson:"matrix_parameters"`
	Template         BuildVariant      `yaml:"template,omitempty" bson:"template"`
}

type YAMLCommandSet struct {
	SingleCommand *PluginCommandConf
	MultiCommand  []PluginCommandConf
}

func (c *YAMLCommandSet) List() []PluginCommandConf {
	if len(c.MultiCommand) > 0 {
		return c.MultiCommand
	}
	if c.SingleCommand != nil && (c.SingleCommand.Command != "" || c.SingleCommand.Function != "") {
		return []PluginCommandConf{*c.SingleCommand}
	}
	return []PluginCommandConf{}
}

func (c *YAMLCommandSet) MarshalYAML() (interface{}, error) {
	if c == nil {
		return nil, nil
	}
	return c.List(), nil
}

func (c *YAMLCommandSet) UnmarshalYAML(unmarshal func(interface{}) error) error {
	err1 := unmarshal(&(c.MultiCommand))
	err2 := unmarshal(&(c.SingleCommand))
	if err1 == nil || err2 == nil {
		return nil
	}
	return err1
}

// The information about a task's dependency
type TaskDependency struct {
	Name    string `yaml:"name,omitempty" bson:"name"`
	Variant string `yaml:"variant,omitempty" bson:"variant,omitempty"`
	Status  string `yaml:"status,omitempty" bson:"status,omitempty"`
}

// UnmarshalYAML allows tasks to be referenced as single selector strings.
// This works by first attempting to unmarshal the YAML into a string
// and then falling back to the TaskDependency struct.
func (td *TaskDependency) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal just a selector string
	var onlySelector string
	if err := unmarshal(&onlySelector); err == nil {
		td.Name = onlySelector
		return nil
	}
	// we define a new type so that we can grab the yaml struct tags without the struct methods,
	// preventing infinte recursion on the UnmarshalYAML() method.
	type tdCopyType TaskDependency
	var tdc tdCopyType
	err := unmarshal(&tdc)
	if err != nil {
		return err
	}
	*td = TaskDependency(tdc)
	return nil
}

// Unmarshalled from the "tasks" list in the project file
type ProjectTask struct {
	Name            string              `yaml:"name,omitempty" bson:"name"`
	Priority        int                 `yaml:"priority,omitempty" bson:"priority"`
	ExecTimeoutSecs int                 `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs"`
	DependsOn       []TaskDependency    `yaml:"depends_on,omitempty" bson:"depends_on"`
	Commands        []PluginCommandConf `yaml:"commands,omitempty" bson:"commands"`
	Tags            []string            `yaml:"tags,omitempty" bson:"tags"`

	// Use a *bool so that there are 3 possible states:
	//   1. nil   = not overriding the project setting (default)
	//   2. true  = overriding the project setting with true
	//   3. false = overriding the project setting with false
	Patchable *bool `yaml:"patchable,omitempty" bson:"patchable,omitempty"`
	Stepback  *bool `yaml:"stepback,omitempty" bson:"stepback,omitempty"`
}

type TaskConfig struct {
	Distro       *distro.Distro
	ProjectRef   *ProjectRef
	Project      *Project
	Task         *Task
	BuildVariant *BuildVariant
	Expansions   *command.Expansions
	WorkDir      string
}

// TaskIdTable is a map of [variant, task display name]->[task id].
type TaskIdTable map[TVPair]string

// TVPair is a helper type for mapping bv/task pairs to ids.
type TVPair struct {
	Variant  string
	TaskName string
}

// String returns the pair's name in a readible form.
func (p TVPair) String() string {
	return fmt.Sprintf("%v/%v", p.Variant, p.TaskName)
}

// AddId adds the Id for the task/variant combination to the table.
func (tt TaskIdTable) AddId(variant, taskName, id string) {
	tt[TVPair{variant, taskName}] = id
}

// GetId returns the Id for the given task on the given variant.
// Returns the empty string if the task/variant does not exist.
func (tt TaskIdTable) GetId(variant, taskName string) string {
	return tt[TVPair{variant, taskName}]
}

// GetIdsForAllVariants returns all task Ids for taskName on all variants != currentVariant.
// The current variant must be passed in to avoid cycle generation.
func (tt TaskIdTable) GetIdsForAllVariants(currentVariant, taskName string) []string {
	ids := []string{}
	for pair, _ := range tt {
		if pair.TaskName == taskName && pair.Variant != currentVariant {
			if id := tt[pair]; id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// GetIdsForTasks returns all task Ids for tasks on all variants != the current task.
// The current variant and task must be passed in to avoid cycle generation.
func (tt TaskIdTable) GetIdsForAllTasks(currentVariant, taskName string) []string {
	ids := []string{}
	for pair, _ := range tt {
		if !(pair.TaskName == taskName && pair.Variant == currentVariant) {
			if id := tt[pair]; id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// TaskIdTable builds a TaskIdTable for the given version and project
func BuildTaskIdTable(p *Project, v *version.Version) TaskIdTable {
	// init the variant map
	table := TaskIdTable{}
	for _, bv := range p.BuildVariants {
		if bv.Disabled {
			continue
		}
		for _, t := range bv.Tasks {
			// create a unique Id for each task
			taskId := util.CleanName(
				fmt.Sprintf("%v_%v_%v_%v_%v",
					p.Identifier, bv.Name, t.Name, v.Revision, v.CreateTime.Format(build.IdTimeLayout)))
			table[TVPair{bv.Name, t.Name}] = taskId
		}
	}
	return table
}

var (
	// bson fields for the project struct
	ProjectIdentifierKey    = bsonutil.MustHaveTag(Project{}, "Identifier")
	ProjectPreKey           = bsonutil.MustHaveTag(Project{}, "Pre")
	ProjectPostKey          = bsonutil.MustHaveTag(Project{}, "Post")
	ProjectModulesKey       = bsonutil.MustHaveTag(Project{}, "Modules")
	ProjectBuildVariantsKey = bsonutil.MustHaveTag(Project{}, "BuildVariants")
	ProjectFunctionsKey     = bsonutil.MustHaveTag(Project{}, "Functions")
	ProjectStepbackKey      = bsonutil.MustHaveTag(Project{}, "Stepback")
	ProjectTasksKey         = bsonutil.MustHaveTag(Project{}, "Tasks")
	ProjectBVMatrixKey      = bsonutil.MustHaveTag(Project{}, "BuildVariantMatrix")
)

func NewTaskConfig(d *distro.Distro, p *Project, t *Task, r *ProjectRef) (*TaskConfig, error) {
	// do a check on if the project is empty
	if p == nil {
		return nil, fmt.Errorf("project for task with branch %v is empty", t.Project)
	}

	// check on if the project ref is empty
	if r == nil {
		return nil, fmt.Errorf("Project ref with identifier: %v was empty", p.Identifier)
	}

	bv := p.FindBuildVariant(t.BuildVariant)
	if bv == nil {
		return nil, fmt.Errorf("couldn't find buildvariant: '%v'", t.BuildVariant)
	}

	e := populateExpansions(d, bv, t)
	return &TaskConfig{d, r, p, t, bv, e, d.WorkDir}, nil
}

func populateExpansions(d *distro.Distro, bv *BuildVariant, t *Task) *command.Expansions {
	expansions := command.NewExpansions(map[string]string{})
	expansions.Put("execution", fmt.Sprintf("%v", t.Execution))
	expansions.Put("version_id", t.Version)
	expansions.Put("task_id", t.Id)
	expansions.Put("task_name", t.DisplayName)
	expansions.Put("build_id", t.BuildId)
	expansions.Put("build_variant", t.BuildVariant)
	expansions.Put("workdir", d.WorkDir)
	expansions.Put("revision", t.Revision)
	expansions.Put("project", t.Project)
	expansions.Put("branch_name", t.Project)
	if t.Requester == evergreen.PatchVersionRequester {
		expansions.Put("is_patch", "true")
	}
	for _, e := range d.Expansions {
		expansions.Put(e.Key, e.Value)
	}
	expansions.Update(bv.Expansions)
	return expansions
}

// Creates a copy of `current`, with instances of ${matrixParameterName} in
// top level string and slice of strings fields replaced by the value specified
// in matrixParameterValues
func expandBuildVariantMatrixParameters(project *Project, current BuildVariant,
	matrixParameterValues []MatrixParameterValue) (*BuildVariant, error) {
	// Create a new build variant with the same parameters
	newBv := current
	newBv.Expansions = make(map[string]string)

	// Make sure to copy over expansions
	for k, v := range current.Expansions {
		newBv.Expansions[k] = v
	}

	// Convert parameter value state into a map for use with expansions
	matrixParameterMap := make(map[string]string)
	for i, parameter := range project.BuildVariantMatrix.MatrixParameters {
		matrixParameterMap[parameter.Name] = matrixParameterValues[i].Value
	}
	matrixParameterExpansions := command.NewExpansions(matrixParameterMap)

	// Iterate over all fields
	numFields := reflect.TypeOf(newBv).NumField()
	for fieldIndex := 0; fieldIndex < numFields; fieldIndex++ {
		// Expand matrix parameters in top level string fields
		if reflect.TypeOf(newBv).Field(fieldIndex).Type.Kind() == reflect.String {
			val := reflect.ValueOf(&newBv).Elem().Field(fieldIndex).String()
			val, err := matrixParameterExpansions.ExpandString(val)
			if err != nil {
				return nil, err
			}
			reflect.ValueOf(&newBv).Elem().Field(fieldIndex).SetString(val)
		}

		// Expand matrix parameters in top level slices of strings
		if reflect.TypeOf(newBv).Field(fieldIndex).Type == reflect.SliceOf(reflect.TypeOf("")) {
			slice := reflect.ValueOf(&newBv).Elem().Field(fieldIndex)
			newSlice := []string{}
			for arrayIndex := 0; arrayIndex < slice.Len(); arrayIndex++ {
				// Expand matrix parameters for each individual element of the slice
				val := slice.Index(arrayIndex).String()
				val, err := matrixParameterExpansions.ExpandString(val)
				if err != nil {
					return nil, err
				}
				newSlice = append(newSlice, val)
			}

			reflect.ValueOf(&newBv).Elem().Field(fieldIndex).Set(reflect.ValueOf(newSlice))
		}
	}

	// First, attach all conditional expansions (i.e. expansions associated with
	// a given parameter value)
	for _, value := range matrixParameterValues {
		for k, v := range value.Expansions {
			newBv.Expansions[k] = v
		}
	}

	// Then, expand matrix parameters in all expansions
	for key, expansion := range newBv.Expansions {
		expansion, err := matrixParameterExpansions.ExpandString(expansion)
		if err != nil {
			return nil, err
		}
		newBv.Expansions[key] = expansion
	}

	// Build variant matrix parameter values are stored in the build variant
	buildVariantMatrixParameterValues := make(map[string]string)
	for i, matrixParameter := range project.BuildVariantMatrix.MatrixParameters {
		buildVariantMatrixParameterValues[matrixParameter.Name] =
			matrixParameterValues[i].Value
	}
	newBv.MatrixParameterValues = buildVariantMatrixParameterValues

	return &newBv, nil
}

// Recursively builds up build variants specified in build variant matrix.
// Should not be called directly, use addMatrixVariants() below instead.
func addMatrixVariantsRecursion(project *Project,
	valueState []MatrixParameterValue, current BuildVariant) error {
	parameterIndex := len(valueState)
	values := project.BuildVariantMatrix.MatrixParameters[parameterIndex].Values

	// Recursion by values specified for each parameter
	for _, value := range values {
		valueState = append(valueState, value)
		// If we're at the bottom of the recursion, create new build variant
		if parameterIndex >= len(project.BuildVariantMatrix.MatrixParameters)-1 {
			newBv, err := expandBuildVariantMatrixParameters(project, current,
				valueState)
			if err != nil {
				return err
			}
			project.BuildVariants = append(project.BuildVariants, *newBv)
		} else {
			// Otherwise, continue on to next parameter
			err := addMatrixVariantsRecursion(project, valueState, current)
			if err != nil {
				return err
			}
		}
		valueState = valueState[:len(valueState)-1]
	}

	return nil
}

// Append new build variants based on the project's build variant matrix
func addMatrixVariants(project *Project) error {
	if len(project.BuildVariantMatrix.MatrixParameters) > 0 {
		err := addMatrixVariantsRecursion(project, []MatrixParameterValue{},
			project.BuildVariantMatrix.Template)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetSpecForTask returns a ProjectTask spec for the given name.
// Returns an empty ProjectTask if none exists.
func (p Project) GetSpecForTask(name string) ProjectTask {
	for _, pt := range p.Tasks {
		if pt.Name == name {
			return pt
		}
	}
	return ProjectTask{}
}

func (p *Project) GetVariantMappings() map[string]string {
	mappings := make(map[string]string)
	for _, buildVariant := range p.BuildVariants {
		mappings[buildVariant.Name] = buildVariant.DisplayName
	}
	return mappings
}

func (p *Project) GetVariantsWithTask(taskName string) []string {
	var variantsList []string
	for _, buildVariant := range p.BuildVariants {
		for _, task := range buildVariant.Tasks {
			if task.Name == taskName {
				variantsList = append(variantsList, buildVariant.Name)
			}
		}
	}
	return variantsList
}

// EvaluateTags replaces all tag selectors with actual tasks objects that
// can be used by the rest of Evergreen's code.
func (p *Project) EvaluateTags() (err error) {
	tse := NewTaskSelectorEvaluator(p.Tasks)
	for i, bv := range p.BuildVariants {
		p.BuildVariants[i].Tasks, err = tse.EvaluateTasks(bv.Tasks)
		if err != nil {
			return fmt.Errorf("error evaluating tags on variant '%v': %v", bv.Name, err)
		}
	}
	for i, t := range p.Tasks {
		p.Tasks[i].DependsOn, err = tse.EvaluateDeps(t.DependsOn)
		if err != nil {
			return fmt.Errorf("error evaluating dependency tags on task '%v': %v", t.Name, err)
		}
	}
	return nil
}

// RunOnVariant returns true if the plugin command should run on variant; returns false otherwise
func (p PluginCommandConf) RunOnVariant(variant string) bool {
	return len(p.Variants) == 0 || util.SliceContains(p.Variants, variant)
}

// GetDisplayName returns the  display name of the plugin command. If none is
// defined, it returns the command's identifier.
func (p PluginCommandConf) GetDisplayName() string {
	if p.DisplayName != "" {
		return p.DisplayName
	}
	return p.Command
}

// GetType returns the type of this command if one is explicitly specified. If
// no type is specified, it checks the default command type of the project. If
// one is specified, it returns that, if not, it returns the DefaultCommandType.
func (p PluginCommandConf) GetType(prj *Project) string {
	if p.Type != "" {
		return p.Type
	}
	if prj.CommandType != "" {
		return prj.CommandType
	}
	return DefaultCommandType
}

func (m *Module) GetRepoOwnerAndName() (string, string) {
	parts := strings.Split(string(m.Repo), ":")
	basename := parts[len(parts)-1]
	ownerAndName := util.RemoveSuffix(basename, ".git")
	ownersplit := strings.Split(ownerAndName, "/")
	if len(ownersplit) != 2 {
		return "", ""
	} else {
		return ownersplit[0], ownersplit[1]
	}
}

func FindProject(revision string, projectRef *ProjectRef) (*Project, error) {
	if projectRef == nil {
		return nil, fmt.Errorf("projectRef given is nil")
	}
	if projectRef.Identifier == "" {
		return nil, fmt.Errorf("Invalid project with blank identifier")
	}

	project := &Project{}
	project.Identifier = projectRef.Identifier
	// when the revision is empty we find the last known good configuration from the versions
	// If the last known good configuration does not exist,
	// load the configuration from the local config in the project ref.
	if revision == "" {
		lastGoodVersion, err := version.FindOne(version.ByLastKnownGoodConfig(projectRef.Identifier))
		if err != nil {
			return nil, fmt.Errorf("Error finding recent valid version for %v: %v", projectRef.Identifier, err)
		}
		if lastGoodVersion != nil {
			// for new repositories, we don't want to error out when we don't have
			// any versions stored in the database so we default to the skeletal
			// information we already have from the project file on disk
			err = LoadProjectInto([]byte(lastGoodVersion.Config), projectRef.Identifier, project)
			if err != nil {
				return nil, fmt.Errorf("Error loading project from "+
					"last good version for project, %v: %v", lastGoodVersion.Identifier, err)
			}
		} else {
			// Check to see if there is a local configuration in the project ref
			if projectRef.LocalConfig != "" {
				err = LoadProjectInto([]byte(projectRef.LocalConfig), projectRef.Identifier, project)
				if err != nil {
					return nil, fmt.Errorf("Error loading local config for project ref, %v : %v", projectRef.Identifier, err)
				}
			}
		}
	}

	if revision != "" {
		// we immediately return an error if the repotracker version isn't found
		// for the given project at the given revision
		version, err := version.FindOne(version.ByProjectIdAndRevision(projectRef.Identifier, revision))
		if err != nil {
			return nil, fmt.Errorf("error fetching version for project %v revision %v: %v", projectRef.Identifier, revision, err)
		}
		if version == nil {
			// fall back to the skeletal project
			return project, nil
		}

		project = &Project{}
		if err = LoadProjectInto([]byte(version.Config), projectRef.Identifier, project); err != nil {
			return nil, fmt.Errorf("Error loading project from version: %v", err)
		}
	}
	return project, nil
}

// LoadProjectInto loads the raw data from the config file into project
// and sets the project's identifier field to identifier. Tags and matrix variants
// are expanded.
func LoadProjectInto(data []byte, identifier string, project *Project) error {
	if err := yaml.Unmarshal(data, project); err != nil {
		return fmt.Errorf("parse error unmarshalling project: %v", err)
	}
	// expand task definitions
	if err := project.EvaluateTags(); err != nil {
		return fmt.Errorf("error evaluating project tags: %v", err)
	}
	project.Identifier = identifier
	return addMatrixVariants(project)
}

func (p *Project) FindTaskForVariant(task, variant string) *BuildVariantTask {
	bv := p.FindBuildVariant(variant)
	if bv == nil {
		return nil
	}
	for _, bvt := range bv.Tasks {
		if bvt.Name == task {
			bvt.Populate(*p.FindProjectTask(task))
			return &bvt
		}
	}
	return nil
}

func (p *Project) FindBuildVariant(build string) *BuildVariant {
	for _, b := range p.BuildVariants {
		if b.Name == build {
			return &b
		}
	}
	return nil
}

func (p *Project) FindTestSuite(name string) *ProjectTask {
	for _, ts := range p.Tasks {
		if ts.Name == name {
			return &ts
		}
	}
	return nil
}

func (p *Project) FindProjectTask(name string) *ProjectTask {
	for _, t := range p.Tasks {
		if t.Name == name {
			return &t
		}
	}
	return nil
}

func (p *Project) GetModuleByName(name string) (*Module, error) {
	for _, v := range p.Modules {
		if v.Name == name {
			return &v, nil
		}
	}
	return nil, fmt.Errorf("No such module on this project.")
}

func (p *Project) FindTasksForVariant(build string) []string {
	for _, b := range p.BuildVariants {
		if b.Name == build {
			tasks := make([]string, 0, len(b.Tasks))
			for _, task := range b.Tasks {
				tasks = append(tasks, task.Name)
			}
			return tasks
		}
	}
	return nil
}

func (p *Project) FindAllVariants() []string {
	variants := make([]string, 0, len(p.BuildVariants))
	for _, b := range p.BuildVariants {
		variants = append(variants, b.Name)
	}
	return variants
}
