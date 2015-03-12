package model

import (
	"10gen.com/mci"
	"10gen.com/mci/command"
	"10gen.com/mci/db/bsonutil"
	"10gen.com/mci/model/distro"
	"10gen.com/mci/util"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/shelman/angier"
	"gopkg.in/yaml.v1"
	"io/ioutil"
	"os"
	"path/filepath"
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

type Project struct {
	Enabled            bool                         `yaml:"enabled" bson:"enabled"`
	Remote             bool                         `yaml:"remote" bson:"remote"`
	Stepback           bool                         `yaml:"stepback" bson:"stepback"`
	BatchTime          int                          `yaml:"batchtime" bson:"batch_time"`
	Owner              string                       `yaml:"owner" bson:"owner_name"`
	Repo               string                       `yaml:"repo" bson:"repo_name"`
	RemotePath         string                       `yaml:"remote_path" bson:"remote_path"`
	RepoKind           string                       `yaml:"repokind" bson:"repo_kind"`
	Branch             string                       `yaml:"branch" bson:"branch_name"`
	Identifier         string                       `yaml:"identifier" bson:"identifier"`
	DisplayName        string                       `yaml:"display_name" bson:"display_name"`
	Pre                []PluginCommandConf          `yaml:"pre" bson:"pre"`
	Post               []PluginCommandConf          `yaml:"post" bson:"post"`
	Timeout            []PluginCommandConf          `yaml:"timeout" bson:"timeout"`
	Modules            []Module                     `yaml:"modules" bson:"modules"`
	BuildVariants      []BuildVariant               `yaml:"buildvariants" bson:"build_variants"`
	Functions          map[string]PluginCommandConf `yaml:"functions" bson:"functions"`
	Tasks              []ProjectTask                `yaml:"tasks" bson:"tasks"`
	BuildVariantMatrix BuildVariantMatrix           `yaml:"build_variant_matrix" bson:"build_variant_matrix"`

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
	SourceDir    string
}

var (
	// bson fields for the project struct
	ProjectEnabledKey       = bsonutil.MustHaveTag(Project{}, "Enabled")
	ProjectRemoteKey        = bsonutil.MustHaveTag(Project{}, "Remote")
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

func NewTaskConfig(distro *distro.Distro, project *Project, task *Task, workDir string) (*TaskConfig, error) {
	buildVariant := project.FindBuildVariant(task.BuildVariant)
	if buildVariant == nil {
		return nil, fmt.Errorf("Couldn't find buildvariant: %v", task.BuildVariant)
	}
	expansions := populateExpansions(distro, project, buildVariant, task)
	workDirPart := project.Name()
	workDirPart = strings.Replace(workDirPart, ":", "", -1)
	workDirPart = strings.Replace(workDirPart, "//", "/", -1)
	sourceDir := filepath.Join(workDir, workDirPart)
	expansions.Put("workdir", sourceDir)

	return &TaskConfig{distro, project, task, buildVariant, expansions, workDir, sourceDir}, nil
}

func populateExpansions(distro *distro.Distro, project *Project,
	buildVariant *BuildVariant, task *Task) *command.Expansions {
	expansions := command.NewExpansions(map[string]string{})
	expansions.Update(distro.Expansions)

	if buildVariant != nil {
		expansions.Update(buildVariant.Expansions)
	}

	buildNum := task.RemoteArgs.Options["build_num"]
	expansions.Put("builder_num", buildNum)
	expansions.Put("builder", fmt.Sprintf("mci_0.9_%s", task.BuildVariant))
	expansions.Put("builder_phase", fmt.Sprintf("%v_%v", task.DisplayName, task.Execution))

	// this is done this way since we don't have a good way of having the agent
	// interact with buildlogger. Once we have it rewritten, this should be
	// cleaner. Follow MCI-1072 for progress on this
	expansions.Put("execution", fmt.Sprintf("%v", task.Execution))
	expansions.Put("task_id", task.Id)
	expansions.Put("task_name", task.DisplayName)
	expansions.Put("build_id", task.BuildId)
	expansions.Put("build_variant", task.BuildVariant)
	expansions.Put("revision", task.Revision)

	//DEPRECATED - eliminate uses of branch_name expansion in favor of 'project'
	expansions.Put("branch_name", task.Project)

	expansions.Put("project", task.Project)
	return expansions
}

// Creates a copy of `current`, with instances of ${matrixParameterName} in
// top level string and slice of strings fields replaced by the value specified
// in matrixParameterValues
func expandBuildVariantMatrixParameters(project *Project, current BuildVariant,
	matrixParameterValues []MatrixParameterValue) (*BuildVariant, error) {
	// Create a new build variant with the same parameters
	newBv := BuildVariant{}
	angier.TransferByFieldNames(&current, &newBv)
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

func FindProject(revision, identifier, configName string) (*Project, error) {
	if identifier == "" {
		return nil, fmt.Errorf("empty project name")
	}

	configRoot, err := mci.FindMCIConfig(configName)
	if err != nil {
		return nil, err
	}

	return NewProjectByName(revision, identifier, configRoot)
}

// getVersionProjectConfig attempts to retrieve the most recent valid version of
// a project from the database. If none is found, it falls back to using the
// most recent version of the project's configuration it finds
func getVersionProjectConfig(identifier string) (*Version, error) {
	versions, err := LastKnownGoodConfig(identifier)
	if err != nil {
		return nil, fmt.Errorf("Error finding recent "+
			"valid version for %v: %v", identifier, err)
	}
	if len(versions) == 0 {
		return nil, mci.Logger.Errorf(slogger.WARN, "No recent valid version "+
			"found for %v; falling back to any recent version", identifier)
	}
	return &versions[0], nil
}

func NewProjectByName(revision, identifier, configRoot string) (*Project, error) {
	if identifier == "" {
		return nil, fmt.Errorf("empty project identifier")
	}

	// we want to eventually move from reading anything directly from
	// configuration files on disk. everything from here to when
	// we check for revision == "" should go away once we move completely
	// to getting this data from the database. leaving it in now for those
	// projects that don't have any revision in the database - so we get some
	// skeletal data to use
	fileName := filepath.Join(configRoot, "project", identifier+".yml")
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		} else {
			return nil, err
		}
	}
	project := &Project{}
	if err = LoadProjectInto(data, project); err != nil {
		return nil, err
	}

	// for remotely tracked projects that don't require a specific version of
	// the configuration, get the most recent valid configuration from the db
	if project.Remote && revision == "" {
		version, err := getVersionProjectConfig(identifier)
		// for new repositories, we don't want to error out when we don't have
		// any versions stored in the database so we default to the skeletal
		// information we already have from the project file on disk
		if err == nil {
			project = &Project{}
			err = LoadProjectInto([]byte(version.Config), project)
			if err != nil {
				return nil, fmt.Errorf("Error loading project from "+
					"version: %v", err)
			}
		}
	}
	if revision != "" {
		project = &Project{}
		// we immediately return an error if the repotracker version isn't found
		// for the given project at the given revision
		version, err := FindVersionByIdAndRevision(identifier, revision)
		if err != nil {
			return nil, fmt.Errorf("error fetching version document for "+
				"project %v at revision %v: %v", identifier, revision, err)
		}
		if version == nil {
			return nil, fmt.Errorf("version for revision %v of project %v "+
				"does not exist in the db. Perhaps history was rewritten?",
				revision, identifier)
		}
		if err = LoadProjectInto([]byte(version.Config), project); err != nil {
			return nil, fmt.Errorf("Error loading project from "+
				"version: %v", err)
		}
	}
	return project, nil
}

func LoadProjectInto(data []byte, project *Project) error {
	if err := yaml.Unmarshal(data, project); err != nil {
		return fmt.Errorf("Parse error unmarshalling project: %v", err)
	}
	return addMatrixVariants(project)
}

func (self *Project) FindBuildVariant(build string) *BuildVariant {
	for _, b := range self.BuildVariants {
		if b.Name == build {
			return &b
		}
	}
	return nil
}

func (self *Project) GetBatchTime(variant *BuildVariant) int {
	if variant.BatchTime != nil {
		return *variant.BatchTime
	}
	return self.BatchTime
}

func (self *Project) FindTestSuite(name string) *ProjectTask {
	for _, ts := range self.Tasks {
		if ts.Name == name {
			return &ts
		}
	}
	return nil
}

func (self *Project) FindProjectTask(name string) *ProjectTask {
	for _, pa := range self.Tasks {
		if pa.Name == name {
			return &pa
		}
	}
	return nil
}

func (self *Project) GetModuleByName(name string) (*Module, error) {
	for _, v := range self.Modules {
		if v.Name == name {
			return &v, nil
		}
	}
	return nil, fmt.Errorf("No such module on this project.")
}

// Generate the URL to the repo.
func (project *Project) Location() (string, error) {
	switch project.RepoKind {
	case GithubRepoType:
		return fmt.Sprintf("git@github.com:%v/%v.git", project.Owner,
			project.Repo), nil
	default:
		return "", mci.Logger.Errorf(slogger.ERROR, "Unknown value of RepoKind"+
			" (“%v”)", project.RepoKind)
	}
}

func (b *Project) Name() string {
	return b.Identifier
}
