package model

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/shelman/angier"
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
	Enabled            bool                       `yaml:"enabled" bson:"enabled"`
	Stepback           bool                       `yaml:"stepback" bson:"stepback"`
	BatchTime          int                        `yaml:"batchtime" bson:"batch_time"`
	Owner              string                     `yaml:"owner" bson:"owner_name"`
	Repo               string                     `yaml:"repo" bson:"repo_name"`
	RemotePath         string                     `yaml:"remote_path" bson:"remote_path"`
	RepoKind           string                     `yaml:"repokind" bson:"repo_kind"`
	Branch             string                     `yaml:"branch" bson:"branch_name"`
	Identifier         string                     `yaml:"identifier" bson:"identifier"`
	DisplayName        string                     `yaml:"display_name" bson:"display_name"`
	CommandType        string                     `yaml:"command_type" bson:"command_type"`
	Pre                *YAMLCommandSet            `yaml:"pre" bson:"pre"`
	Post               *YAMLCommandSet            `yaml:"post" bson:"post"`
	Timeout            *YAMLCommandSet            `yaml:"timeout" bson:"timeout"`
	Modules            []Module                   `yaml:"modules" bson:"modules"`
	BuildVariants      []BuildVariant             `yaml:"buildvariants" bson:"build_variants"`
	Functions          map[string]*YAMLCommandSet `yaml:"functions" bson:"functions"`
	Tasks              []ProjectTask              `yaml:"tasks" bson:"tasks"`
	BuildVariantMatrix BuildVariantMatrix         `yaml:"build_variant_matrix" bson:"build_variant_matrix"`

	// Flag that indicates a project as requiring user authentication
	Private bool `yaml:"private" bson:"private"`
}

// Unmarshalled from the "tasks" list in an individual build variant
type BuildVariantTask struct {
	// this name HAS to match the name field of one of the tasks specified at
	// the project level, or an error will be thrown
	Name string `yaml:"name" bson:"name"`

	// the distros that the task can be run on
	Distros []string `yaml:"distros" bson:"distros"`
}

type BuildVariant struct {
	Name        string            `yaml:"name" bson:"name"`
	DisplayName string            `yaml:"display_name" bson:"display_name"`
	Expansions  map[string]string `yaml:"expansions" bson:"expansions"`
	Modules     []string          `yaml:"modules" bson:"modules"`
	Disabled    bool              `yaml:"disabled" bson:"disabled"`
	Push        bool              `yaml:"push" bson:"push"`

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
	RunOn []string `yaml:"run_on" bson:"run_on"`

	// all of the tasks to be run on the build variant, compile through tests.
	Tasks                 []BuildVariantTask `yaml:"tasks" bson:"tasks"`
	MatrixParameterValues map[string]string  `yaml:"matrix_parameter_values" bson:"matrix_parameter_values"`
}

type Module struct {
	Name   string `yaml:"name" bson:"name"`
	Branch string `yaml:"branch" bson:"branch"`
	Repo   string `yaml:"repo" bson:"repo"`
	Prefix string `yaml:"prefix" bson:"prefix"`
}

type TestSuite struct {
	Name  string `yaml:"name"`
	Phase string `yaml:"phase"`
}

type PluginCommandConf struct {
	Function string `yaml:"func" bson:"func"`
	// Type is used to differentiate between setup related commands and actual
	// testing commands.
	Type string `yaml:"type" bson:"type"`

	// DisplayName is a human readable description of the function of a given
	// command.
	DisplayName string `yaml:"display_name" bson:"display_name"`

	// Command is a unique identifier for the command configuration. It consists of a
	// plugin name and a command name.
	Command string `yaml:"command" bson:"command"`

	// Variants is used to enumerate the particular sets of buildvariants to run
	// this command configuration on. If it is empty, it is run on all defined
	// variants.
	Variants []string `yaml:"variants" bson:"variants"`

	// TimeoutSecs indicates the maximum duration the command is allowed to run
	// for. If undefined, it is unbounded.
	TimeoutSecs int `yaml:"timeout_secs" bson:"timeout_secs"`

	// Params are used to supply configuratiion specific information.
	Params map[string]interface{} `yaml:"params" bson:"params"`

	// Vars defines variables that can be used within commands.
	Vars map[string]string `yaml:"vars" bson:"vars"`
}

type ArtifactInstructions struct {
	Include      []string `yaml:"include" bson:"include"`
	ExcludeFiles []string `yaml:"excludefiles" bson:"exclude_files"`
}

type MatrixParameterValue struct {
	Value      string            `yaml:"value" bson:"value"`
	Expansions map[string]string `yaml:"expansions" bson:"expansions"`
}

type MatrixParameter struct {
	Name   string                 `yaml:"name" bson:"name"`
	Values []MatrixParameterValue `yaml:"values" bson:"values"`
}

type BuildVariantMatrix struct {
	MatrixParameters []MatrixParameter `yaml:"matrix_parameters" bson:"matrix_parameters"`
	Template         BuildVariant      `yaml:"template" bson:"template"`
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
	Name    string `yaml:"name" bson:"name"`
	Variant string `yaml:"variant" bson:"variant,omitempty"`
	Status  string `yaml:"status" bson:"status,omitempty"`
}

// Unmarshalled from the "tasks" list in the project file
type ProjectTask struct {
	Name        string              `yaml:"name" bson:"name"`
	ExecTimeout int                 `yaml:"exec_timeout" bson:"exec_timeout"`
	DependsOn   []TaskDependency    `yaml:"depends_on" bson:"depends_on"`
	Commands    []PluginCommandConf `yaml:"commands" bson:"commands"`

	// Use a *bool so that there are 3 possible states:
	//   1. nil   = not overriding the project setting (default)
	//   2. true  = overriding the project setting with true
	//   3. false = overriding the project setting with false
	Stepback *bool `yaml:"stepback,omitempty" bson:"stepback,omitempty"`
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
	expansions.Put("task_id", t.Id)
	expansions.Put("task_name", t.DisplayName)
	expansions.Put("build_id", t.BuildId)
	expansions.Put("build_variant", t.BuildVariant)
	expansions.Put("workdir", d.WorkDir)
	expansions.Put("revision", t.Revision)
	expansions.Put("project", t.Project)
	expansions.Put("branch_name", t.Project)
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
	newBv := BuildVariant{}
	err := angier.TransferByFieldNames(&current, &newBv)
	if err != nil {
		return nil, err
	}
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

func (self *Project) GetVariantMappings() map[string]string {
	mappings := make(map[string]string)
	for _, buildVariant := range self.BuildVariants {
		mappings[buildVariant.Name] = buildVariant.DisplayName
	}
	return mappings
}

func (self *Project) GetVariantsWithTask(taskName string) []string {
	var variantsList []string
	for _, buildVariant := range self.BuildVariants {
		for _, task := range buildVariant.Tasks {
			if task.Name == taskName {
				variantsList = append(variantsList, buildVariant.Name)
			}
		}
	}
	return variantsList
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
					"last good version for project, %v: %v", lastGoodVersion.Project, err)
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
// and sets the project's identifier field to identifier
func LoadProjectInto(data []byte, identifier string, project *Project) error {
	if err := yaml.Unmarshal(data, project); err != nil {
		return fmt.Errorf("Parse error unmarshalling project: %v", err)
	}
	project.Identifier = identifier
	return addMatrixVariants(project)
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
