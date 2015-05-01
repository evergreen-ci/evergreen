package model

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/shelman/angier"
	"gopkg.in/yaml.v2"
	"reflect"
	"strings"
)

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
	// TODO: include push
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
	Function       string                 `yaml:"func" bson:"func"`
	Command        string                 `yaml:"command" bson:"command"`
	Variants       []string               `yaml:"variants" bson:"variants"`
	TimeoutSecs    int                    `yaml:"timeout_secs" bson:"timeout_secs"`
	ResetOnTimeout bool                   `yaml:"reset_on_timeout" bson:"reset_on_timeout"`
	Params         map[string]interface{} `yaml:"params" bson:"params"`
	Vars           map[string]string      `yaml:"vars" bson:"vars"`
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

// The information about a task's dependency
type TaskDependency struct {
	Name string `yaml:"name" bson:"name"`
}

// Unmarshalled from the "tasks" list in the project file
type ProjectTask struct {
	Name      string              `yaml:"name" bson:"name"`
	DependsOn []TaskDependency    `yaml:"depends_on" bson:"depends_on"`
	Commands  []PluginCommandConf `yaml:"commands" bson:"commands"`

	// Use a *bool so that there are 3 possible states:
	//   1. nil   = not overriding the project setting (default)
	//   2. true  = overriding the project setting with true
	//   3. false = overriding the project setting with false
	Stepback *bool `yaml:"stepback,omitempty" bson:"stepback,omitempty"`
}

type TaskConfig struct {
	Distro       *distro.Distro
	Project      *Project
	Task         *Task
	BuildVariant *BuildVariant
	Expansions   *command.Expansions
	WorkDir      string
}

var (
	// bson fields for the project struct
	ProjectEnabledKey       = bsonutil.MustHaveTag(Project{}, "Enabled")
	ProjectBatchTimeKey     = bsonutil.MustHaveTag(Project{}, "BatchTime")
	ProjectOwnerNameKey     = bsonutil.MustHaveTag(Project{}, "Owner")
	ProjectRepoKey          = bsonutil.MustHaveTag(Project{}, "Repo")
	ProjectRepoKindKey      = bsonutil.MustHaveTag(Project{}, "RepoKind")
	ProjectBranchKey        = bsonutil.MustHaveTag(Project{}, "Branch")
	ProjectIdentifierKey    = bsonutil.MustHaveTag(Project{}, "Identifier")
	ProjectDisplayNameKey   = bsonutil.MustHaveTag(Project{}, "DisplayName")
	ProjectPreKey           = bsonutil.MustHaveTag(Project{}, "Pre")
	ProjectPostKey          = bsonutil.MustHaveTag(Project{}, "Post")
	ProjectModulesKey       = bsonutil.MustHaveTag(Project{}, "Modules")
	ProjectBuildVariantsKey = bsonutil.MustHaveTag(Project{}, "BuildVariants")
	ProjectFunctionsKey     = bsonutil.MustHaveTag(Project{}, "Functions")
	ProjectStepbackKey      = bsonutil.MustHaveTag(Project{}, "Stepback")
	ProjectTasksKey         = bsonutil.MustHaveTag(Project{}, "Tasks")
	ProjectBVMatrixKey      = bsonutil.MustHaveTag(Project{}, "BuildVariantMatrix")
)

func NewTaskConfig(d *distro.Distro, p *Project, t *Task) (*TaskConfig, error) {
	bv := p.FindBuildVariant(t.BuildVariant)
	if bv == nil {
		return nil, fmt.Errorf("couldn't find buildvariant: '%v'", t.BuildVariant)
	}
	e := populateExpansions(d, p, bv, t)
	return &TaskConfig{d, p, t, bv, e, d.WorkDir}, nil
}

func populateExpansions(d *distro.Distro, p *Project, bv *BuildVariant, t *Task) *command.Expansions {
	expansions := command.NewExpansions(map[string]string{})
	// TODO: Some of these expansions aren't strictly needed for Evergreen (MCI-1072)
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

//TODO: refactor this once we get better error handling into the waterfall
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
		return nil, fmt.Errorf("invalid projectRef")
	}
	if projectRef.Identifier == "" {
		return nil, fmt.Errorf("Invalid project with blank identifier")
	}

	project := &Project{}
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
			err = LoadProjectInto([]byte(lastGoodVersion.Config), project)
			if err != nil {
				return nil, fmt.Errorf("Error loading project from "+
					"last good version for project, %v: %v", lastGoodVersion.Project, err)
			}
		} else {
			// Check to see if there is a local configuration in the project ref
			if projectRef.LocalConfig != "" {
				err = LoadProjectInto([]byte(projectRef.LocalConfig), project)
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
		if err = LoadProjectInto([]byte(version.Config), project); err != nil {
			return nil, fmt.Errorf("Error loading project from version: %v", err)
		}
	}

	project.Identifier = projectRef.Identifier
	project.Owner = projectRef.Owner
	project.Repo = projectRef.Repo
	project.Branch = projectRef.Branch
	project.RepoKind = projectRef.RepoKind
	project.Enabled = projectRef.Enabled
	project.Private = projectRef.Private
	project.BatchTime = projectRef.BatchTime
	project.RemotePath = projectRef.RemotePath
	project.DisplayName = projectRef.DisplayName
	return project, nil
}

func LoadProjectInto(data []byte, project *Project) error {
	if err := yaml.Unmarshal(data, project); err != nil {
		return fmt.Errorf("Parse error unmarshalling project: %v", err)
	}
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

func (p *Project) GetBatchTime(variant *BuildVariant) int {
	if variant.BatchTime != nil {
		return *variant.BatchTime
	}
	return p.BatchTime
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

// Generate the URL to the repo.
func (p *Project) Location() (string, error) {
	return fmt.Sprintf("git@github.com:%v/%v.git", p.Owner, p.Repo), nil
}
