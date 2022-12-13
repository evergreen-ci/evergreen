package model

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	ignore "github.com/sabhiram/go-gitignore"
	"gopkg.in/20210107192922/yaml.v3"
)

const (
	// DefaultCommandType is a system configuration option that is used to
	// differentiate between setup related commands and actual testing commands.
	DefaultCommandType = evergreen.CommandTypeTest

	waterfallTasksQueryMaxTime = 90 * time.Second
)

type GetProjectTasksOpts struct {
	Limit        int      `json:"num_versions"`
	BuildVariant string   `json:"build_variant"`
	StartAt      int      `json:"start_at"`
	Requesters   []string `json:"requesters"`
}

type Project struct {
	Enabled            bool                       `yaml:"enabled,omitempty" bson:"enabled"`
	Stepback           bool                       `yaml:"stepback,omitempty" bson:"stepback"`
	PreErrorFailsTask  bool                       `yaml:"pre_error_fails_task,omitempty" bson:"pre_error_fails_task,omitempty"`
	PostErrorFailsTask bool                       `yaml:"post_error_fails_task,omitempty" bson:"post_error_fails_task,omitempty"`
	OomTracker         bool                       `yaml:"oom_tracker,omitempty" bson:"oom_tracker"`
	BatchTime          int                        `yaml:"batchtime,omitempty" bson:"batch_time"`
	Owner              string                     `yaml:"owner,omitempty" bson:"owner_name"`
	Repo               string                     `yaml:"repo,omitempty" bson:"repo_name"`
	RemotePath         string                     `yaml:"remote_path,omitempty" bson:"remote_path"`
	Branch             string                     `yaml:"branch,omitempty" bson:"branch_name"`
	Identifier         string                     `yaml:"identifier,omitempty" bson:"identifier"`
	DisplayName        string                     `yaml:"display_name,omitempty" bson:"display_name"`
	CommandType        string                     `yaml:"command_type,omitempty" bson:"command_type"`
	Ignore             []string                   `yaml:"ignore,omitempty" bson:"ignore"`
	Parameters         []ParameterInfo            `yaml:"parameters,omitempty" bson:"parameters,omitempty"`
	Pre                *YAMLCommandSet            `yaml:"pre,omitempty" bson:"pre"`
	Post               *YAMLCommandSet            `yaml:"post,omitempty" bson:"post"`
	Timeout            *YAMLCommandSet            `yaml:"timeout,omitempty" bson:"timeout"`
	EarlyTermination   *YAMLCommandSet            `yaml:"early_termination,omitempty" bson:"early_termination,omitempty"`
	CallbackTimeout    int                        `yaml:"callback_timeout_secs,omitempty" bson:"callback_timeout_secs"`
	Modules            ModuleList                 `yaml:"modules,omitempty" bson:"modules"`
	Containers         []Container                `yaml:"containers,omitempty" bson:"containers"`
	BuildVariants      BuildVariants              `yaml:"buildvariants,omitempty" bson:"build_variants"`
	Functions          map[string]*YAMLCommandSet `yaml:"functions,omitempty" bson:"functions"`
	TaskGroups         []TaskGroup                `yaml:"task_groups,omitempty" bson:"task_groups"`
	Tasks              []ProjectTask              `yaml:"tasks,omitempty" bson:"tasks"`
	ExecTimeoutSecs    int                        `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs"`
	Loggers            *LoggerConfig              `yaml:"loggers,omitempty" bson:"loggers,omitempty"`

	// Flag that indicates a project as requiring user authentication
	Private bool `yaml:"private,omitempty" bson:"private"`
}

type ProjectInfo struct {
	Ref                 *ProjectRef
	Project             *Project
	IntermediateProject *ParserProject
	Config              *ProjectConfig
}

type PatchConfig struct {
	PatchedParserProject string
	PatchedProjectConfig string
}

func (p *ProjectInfo) NotPopulated() bool {
	return p.Ref == nil || p.IntermediateProject == nil
}

// Unmarshalled from the "tasks" list in an individual build variant. Can be either a task or task group
type BuildVariantTaskUnit struct {
	// Name has to match the name field of one of the tasks or groups specified at
	// the project level, or an error will be thrown
	Name string `yaml:"name,omitempty" bson:"name"`
	// IsGroup indicates that it is a task group or a task within a task group.
	IsGroup bool `yaml:"-" bson:"-"`
	// GroupName is the task group name if this is a task in a task group. If
	// it is the task group itself, it is not populated (Name is the task group
	// name).
	GroupName string `yaml:"-" bson:"-"`

	// fields to overwrite ProjectTask settings.
	Patchable      *bool                `yaml:"patchable,omitempty" bson:"patchable,omitempty"`
	PatchOnly      *bool                `yaml:"patch_only,omitempty" bson:"patch_only,omitempty"`
	Disable        *bool                `yaml:"disable,omitempty" bson:"disable,omitempty"`
	AllowForGitTag *bool                `yaml:"allow_for_git_tag,omitempty" bson:"allow_for_git_tag,omitempty"`
	GitTagOnly     *bool                `yaml:"git_tag_only,omitempty" bson:"git_tag_only,omitempty"`
	Priority       int64                `yaml:"priority,omitempty" bson:"priority"`
	DependsOn      []TaskUnitDependency `yaml:"depends_on,omitempty" bson:"depends_on"`

	// the distros that the task can be run on
	RunOn []string `yaml:"run_on,omitempty" bson:"run_on"`
	// currently unsupported (TODO EVG-578)
	ExecTimeoutSecs int   `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs"`
	Stepback        *bool `yaml:"stepback,omitempty" bson:"stepback,omitempty"`

	Variant string `yaml:"-" bson:"-"`

	CommitQueueMerge bool `yaml:"commit_queue_merge,omitempty" bson:"commit_queue_merge"`

	// Use a *int for 2 possible states
	// nil - not overriding the project setting
	// non-nil - overriding the project setting with this BatchTime
	BatchTime *int `yaml:"batchtime,omitempty" bson:"batchtime,omitempty"`
	// If CronBatchTime is not empty, then override the project settings with cron syntax,
	// with BatchTime and CronBatchTime being mutually exclusive.
	CronBatchTime string `yaml:"cron,omitempty" bson:"cron,omitempty"`
	// If Activate is set to false, then we don't initially activate the task.
	Activate *bool `yaml:"activate,omitempty" bson:"activate,omitempty"`
	// TaskGroup is set if an inline task group is defined on the build variant.
	TaskGroup *TaskGroup `yaml:"task_group,omitempty" bson:"task_group,omitempty"`
}

func (b BuildVariant) Get(name string) (BuildVariantTaskUnit, error) {
	for idx := range b.Tasks {
		if b.Tasks[idx].Name == name {
			return b.Tasks[idx], nil
		}
	}

	return BuildVariantTaskUnit{}, errors.Errorf("could not find task '%s' in build variant '%s'", name, b.Name)
}

func (b BuildVariant) GetDisplayTask(name string) *patch.DisplayTask {
	for _, dt := range b.DisplayTasks {
		if dt.Name == name {
			return &dt
		}
	}
	return nil
}

type BuildVariants []BuildVariant

func (b BuildVariants) Len() int           { return len(b) }
func (b BuildVariants) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b BuildVariants) Less(i, j int) bool { return b[i].DisplayName < b[j].DisplayName }
func (b BuildVariants) Get(name string) (BuildVariant, error) {
	for idx := range b {
		if b[idx].Name == name {
			return b[idx], nil
		}
	}

	return BuildVariant{}, errors.Errorf("could not find build variant named '%s'", name)
}

// Populate updates the base fields of the BuildVariantTaskUnit with
// fields from the project task definition.
func (bvt *BuildVariantTaskUnit) Populate(pt ProjectTask) {
	// We never update "Name" or "Commands"
	if len(bvt.DependsOn) == 0 {
		bvt.DependsOn = pt.DependsOn
	}
	if len(bvt.RunOn) == 0 {
		bvt.RunOn = pt.RunOn
	}
	if bvt.Priority == 0 {
		bvt.Priority = pt.Priority
	}
	if bvt.Patchable == nil {
		bvt.Patchable = pt.Patchable
	}
	if bvt.PatchOnly == nil {
		bvt.PatchOnly = pt.PatchOnly
	}
	if bvt.Disable == nil {
		bvt.Disable = pt.Disable
	}
	if bvt.AllowForGitTag == nil {
		bvt.AllowForGitTag = pt.AllowForGitTag
	}
	if bvt.GitTagOnly == nil {
		bvt.GitTagOnly = pt.GitTagOnly
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
// and then falling back to the BuildVariantTaskUnit struct.
func (bvt *BuildVariantTaskUnit) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal just a selector string
	var onlySelector string
	if err := unmarshal(&onlySelector); err == nil {
		bvt.Name = onlySelector
		return nil
	}
	// we define a new type so that we can grab the yaml struct tags without the struct methods,
	// preventing infinte recursion on the UnmarshalYAML() method.
	type bvtCopyType BuildVariantTaskUnit
	var bvtc bvtCopyType
	err := unmarshal(&bvtc)
	if err != nil {
		return err
	}
	*bvt = BuildVariantTaskUnit(bvtc)
	return nil
}

func (bvt *BuildVariantTaskUnit) SkipOnRequester(requester string) bool {
	return evergreen.IsPatchRequester(requester) && bvt.SkipOnPatchBuild() ||
		!evergreen.IsPatchRequester(requester) && bvt.SkipOnNonPatchBuild() ||
		evergreen.IsGitTagRequester(requester) && bvt.SkipOnGitTagBuild() ||
		!evergreen.IsGitTagRequester(requester) && bvt.SkipOnNonGitTagBuild()
}

func (bvt *BuildVariantTaskUnit) SkipOnPatchBuild() bool {
	return !utility.FromBoolTPtr(bvt.Patchable)
}

func (bvt *BuildVariantTaskUnit) SkipOnNonPatchBuild() bool {
	return utility.FromBoolPtr(bvt.PatchOnly)
}

func (bvt *BuildVariantTaskUnit) SkipOnGitTagBuild() bool {
	return !utility.FromBoolTPtr(bvt.AllowForGitTag)
}

func (bvt *BuildVariantTaskUnit) SkipOnNonGitTagBuild() bool {
	return utility.FromBoolPtr(bvt.GitTagOnly)
}

func (bvt *BuildVariantTaskUnit) IsDisabled() bool {
	return utility.FromBoolPtr(bvt.Disable)
}

func (bvt *BuildVariantTaskUnit) toTaskNode() task.TaskNode {
	return task.TaskNode{
		Name:    bvt.Name,
		Variant: bvt.Variant,
	}
}

func (bvt *BuildVariantTaskUnit) ToTVPair() TVPair {
	return TVPair{
		TaskName: bvt.Name,
		Variant:  bvt.Variant,
	}
}

type BuildVariant struct {
	Name        string            `yaml:"name,omitempty" bson:"name"`
	DisplayName string            `yaml:"display_name,omitempty" bson:"display_name"`
	Expansions  map[string]string `yaml:"expansions,omitempty" bson:"expansions"`
	Modules     []string          `yaml:"modules,omitempty" bson:"modules"`
	Disabled    bool              `yaml:"disabled,omitempty" bson:"disabled"`
	Tags        []string          `yaml:"tags,omitempty" bson:"tags"`
	Push        bool              `yaml:"push,omitempty" bson:"push"`

	// Use a *int for 2 possible states
	// nil - not overriding the project setting
	// non-nil - overriding the project setting with this BatchTime
	BatchTime *int `yaml:"batchtime,omitempty" bson:"batchtime,omitempty"`
	// If CronBatchTime is not empty, then override the project settings with cron syntax,
	// with BatchTime and CronBatchTime being mutually exclusive.
	CronBatchTime string `yaml:"cron,omitempty" bson:"cron,omitempty"`

	// If Activate is set to false, then we don't initially activate the build variant.
	Activate *bool `yaml:"activate,omitempty" bson:"activate,omitempty"`

	// Use a *bool so that there are 3 possible states:
	//   1. nil   = not overriding the project setting (default)
	//   2. true  = overriding the project setting with true
	//   3. false = overriding the project setting with false
	Stepback *bool `yaml:"stepback,omitempty" bson:"stepback,omitempty"`

	// the default distros.  will be used to run a task if no distro field is
	// provided for the task
	RunOn []string `yaml:"run_on,omitempty" bson:"run_on"`

	// all of the tasks/groups to be run on the build variant, compile through tests.
	Tasks        []BuildVariantTaskUnit `yaml:"tasks,omitempty" bson:"tasks"`
	DisplayTasks []patch.DisplayTask    `yaml:"display_tasks,omitempty" bson:"display_tasks,omitempty"`
}

// ParameterInfo is used to provide extra information about a parameter.
type ParameterInfo struct {
	patch.Parameter `yaml:",inline" bson:",inline"`
	Description     string `yaml:"description" bson:"description"`
}

// Container holds all properties that are configurable when defining a container
// for tasks and build variants to run on in a project YAML file.
type Container struct {
	Name       string              `yaml:"name" bson:"name"`
	WorkingDir string              `yaml:"working_dir,omitempty" bson:"working_dir"`
	Image      string              `yaml:"image" bson:"image"`
	Size       string              `yaml:"size,omitempty" bson:"size"`
	Credential string              `yaml:"credential,omitempty" bson:"credential"`
	Resources  *ContainerResources `yaml:"resources,omitempty" bson:"resources"`
	System     ContainerSystem     `yaml:"system,omitempty" bson:"system"`
}

// ContainerSystem specifies the architecture and OS for the running container to use.
type ContainerSystem struct {
	CPUArchitecture evergreen.ContainerArch  `yaml:"cpu_architecture,omitempty" bson:"cpu_architecture"`
	OperatingSystem evergreen.ContainerOS    `yaml:"operating_system,omitempty" bson:"operating_system"`
	WindowsVersion  evergreen.WindowsVersion `yaml:"windows_version,omitempty" bson:"windows_version"`
}

type Module struct {
	Name       string `yaml:"name,omitempty" bson:"name"`
	Branch     string `yaml:"branch,omitempty" bson:"branch"`
	Repo       string `yaml:"repo,omitempty" bson:"repo"`
	Prefix     string `yaml:"prefix,omitempty" bson:"prefix"`
	Ref        string `yaml:"ref,omitempty" bson:"ref"`
	AutoUpdate bool   `yaml:"auto_update,omitempty" bson:"auto_update"`
}

type Include struct {
	FileName string `yaml:"filename,omitempty" bson:"filename,omitempty"`
	Module   string `yaml:"module,omitempty" bson:"module,omitempty"`
}

type ModuleList []Module

func (l *ModuleList) IsIdentical(m manifest.Manifest) bool {
	manifestModules := map[string]manifest.Module{}
	for name, module := range m.Modules {
		manifestModules[name] = manifest.Module{
			Branch: module.Branch,
			Repo:   module.Repo,
			Owner:  module.Owner,
		}
	}
	projectModules := map[string]manifest.Module{}
	for _, module := range *l {
		owner, repo := module.GetRepoOwnerAndName()
		projectModules[module.Name] = manifest.Module{
			Branch: module.Branch,
			Repo:   repo,
			Owner:  owner,
		}
	}

	return reflect.DeepEqual(manifestModules, projectModules)
}

func GetModuleByName(moduleList ModuleList, moduleName string) (*Module, error) {
	for _, module := range moduleList {
		if module.Name == moduleName {
			return &module, nil
		}
	}

	return nil, errors.Errorf("module '%s' doesn't exist", moduleName)
}

type TestSuite struct {
	Name  string `yaml:"name,omitempty"`
	Phase string `yaml:"phase,omitempty"`
}

type PluginCommandConf struct {
	Function string `yaml:"func,omitempty" bson:"func,omitempty"`
	// Type is used to differentiate between setup related commands and actual
	// testing commands.
	Type string `yaml:"type,omitempty" bson:"type,omitempty"`

	// DisplayName is a human readable description of the function of a given
	// command.
	DisplayName string `yaml:"display_name,omitempty" bson:"display_name,omitempty"`

	// Command is a unique identifier for the command configuration. It consists of a
	// plugin name and a command name.
	Command string `yaml:"command,omitempty" bson:"command,omitempty"`

	// Variants is used to enumerate the particular sets of buildvariants to run
	// this command configuration on. If it is empty, it is run on all defined
	// variants.
	Variants []string `yaml:"variants,omitempty" bson:"variants,omitempty"`

	// TimeoutSecs indicates the maximum duration the command is allowed to run for.
	TimeoutSecs int `yaml:"timeout_secs,omitempty" bson:"timeout_secs,omitempty"`

	// Params is used to define params in the yaml and parser project,
	// but is not stored in the DB (instead see ParamsYAML).
	Params map[string]interface{} `yaml:"params,omitempty" bson:"-"`

	// ParamsYAML is the marshalled Params to store in the database, to preserve nested interfaces.
	ParamsYAML string `yaml:"params_yaml,omitempty" bson:"params_yaml,omitempty"`

	// Vars defines variables that can be used within commands.
	Vars map[string]string `yaml:"vars,omitempty" bson:"vars,omitempty"`

	Loggers *LoggerConfig `yaml:"loggers,omitempty" bson:"loggers,omitempty"`
}

func (c *PluginCommandConf) resolveParams() error {
	out := map[string]interface{}{}
	if c == nil {
		return nil
	}
	if c.ParamsYAML != "" {
		if err := yaml.Unmarshal([]byte(c.ParamsYAML), &out); err != nil {
			return errors.Wrapf(err, "unmarshalling params from YAML")
		}
		c.Params = out
	}
	return nil
}

func (c *PluginCommandConf) UnmarshalYAML(unmarshal func(interface{}) error) error {
	temp := struct {
		Function    string                 `yaml:"func,omitempty" bson:"func,omitempty"`
		Type        string                 `yaml:"type,omitempty" bson:"type,omitempty"`
		DisplayName string                 `yaml:"display_name,omitempty" bson:"display_name,omitempty"`
		Command     string                 `yaml:"command,omitempty" bson:"command,omitempty"`
		Variants    []string               `yaml:"variants,omitempty" bson:"variants,omitempty"`
		TimeoutSecs int                    `yaml:"timeout_secs,omitempty" bson:"timeout_secs,omitempty"`
		Params      map[string]interface{} `yaml:"params,omitempty" bson:"params,omitempty"`
		ParamsYAML  string                 `yaml:"params_yaml,omitempty" bson:"params_yaml,omitempty"`
		Vars        map[string]string      `yaml:"vars,omitempty" bson:"vars,omitempty"`
		Loggers     *LoggerConfig          `yaml:"loggers,omitempty" bson:"loggers,omitempty"`
	}{}

	if err := unmarshal(&temp); err != nil {
		return errors.Wrap(err, "input fields may not match the command fields")
	}
	c.Function = temp.Function
	c.Type = temp.Type
	c.DisplayName = temp.DisplayName
	c.Command = temp.Command
	c.Variants = temp.Variants
	c.TimeoutSecs = temp.TimeoutSecs
	c.Vars = temp.Vars
	c.Loggers = temp.Loggers
	c.ParamsYAML = temp.ParamsYAML
	c.Params = temp.Params
	return c.unmarshalParams()
}

func (c *PluginCommandConf) UnmarshalBSON(in []byte) error {
	if err := mgobson.Unmarshal(in, c); err != nil {
		return err
	}
	return c.unmarshalParams()
}

// We read from YAML when available, as the given params could be corrupted from the roundtrip.
// If params is passed, then it means that we haven't yet stored this in the DB.
func (c *PluginCommandConf) unmarshalParams() error {
	if c.ParamsYAML != "" {
		out := map[string]interface{}{}
		if err := yaml.Unmarshal([]byte(c.ParamsYAML), &out); err != nil {
			return errors.Wrap(err, "unmarshalling params from YAML")
		}
		c.Params = out
		return nil
	}
	if len(c.Params) != 0 {
		bytes, err := yaml.Marshal(c.Params)
		if err != nil {
			return errors.Wrap(err, "marshalling params into YAML")
		}
		c.ParamsYAML = string(bytes)
	}
	return nil
}

type ArtifactInstructions struct {
	Include      []string `yaml:"include,omitempty" bson:"include"`
	ExcludeFiles []string `yaml:"excludefiles,omitempty" bson:"exclude_files"`
}

type YAMLCommandSet struct {
	SingleCommand *PluginCommandConf  `yaml:"single_command,omitempty" bson:"single_command,omitempty"`
	MultiCommand  []PluginCommandConf `yaml:"multi_command,omitempty" bson:"multi_command,omitempty"`
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
	res := c.List()
	for idx := range res {
		if err := res[idx].resolveParams(); err != nil {
			return nil, errors.Wrap(err, "resolving params for command set")
		}
	}
	return res, nil
}

func (c *YAMLCommandSet) UnmarshalYAML(unmarshal func(interface{}) error) error {
	err1 := unmarshal(&(c.MultiCommand))
	err2 := unmarshal(&(c.SingleCommand))
	if err1 == nil || err2 == nil {
		return nil
	}
	return err1
}

// TaskUnitDependency holds configuration information about a task/group that must finish before
// the task/group that contains the dependency can run.
type TaskUnitDependency struct {
	Name               string `yaml:"name,omitempty" bson:"name"`
	Variant            string `yaml:"variant,omitempty" bson:"variant,omitempty"`
	Status             string `yaml:"status,omitempty" bson:"status,omitempty"`
	PatchOptional      bool   `yaml:"patch_optional,omitempty" bson:"patch_optional,omitempty"`
	OmitGeneratedTasks bool   `yaml:"omit_generated_tasks,omitempty" bson:"omit_generated_tasks,omitempty"`
}

// UnmarshalYAML allows tasks to be referenced as single selector strings.
// This works by first attempting to unmarshal the YAML into a string
// and then falling back to the TaskUnitDependency struct.
func (td *TaskUnitDependency) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal just a selector string
	var onlySelector string
	if err := unmarshal(&onlySelector); err == nil {
		td.Name = onlySelector
		return nil
	}
	// we define a new type so that we can grab the yaml struct tags without the struct methods,
	// preventing infinte recursion on the UnmarshalYAML() method.
	type tdCopyType TaskUnitDependency
	var tdc tdCopyType
	err := unmarshal(&tdc)
	if err != nil {
		return err
	}
	*td = TaskUnitDependency(tdc)
	return nil
}

type TaskGroup struct {
	Name string `yaml:"name" bson:"name"`

	// data about the task group
	MaxHosts                int             `yaml:"max_hosts" bson:"max_hosts"`
	SetupGroupFailTask      bool            `yaml:"setup_group_can_fail_task" bson:"setup_group_can_fail_task"`
	SetupGroupTimeoutSecs   int             `yaml:"setup_group_timeout_secs" bson:"setup_group_timeout_secs"`
	SetupGroup              *YAMLCommandSet `yaml:"setup_group" bson:"setup_group"`
	TeardownTaskCanFailTask bool            `yaml:"teardown_task_can_fail_task" bson:"teardown_task_can_fail_task"`
	TeardownGroup           *YAMLCommandSet `yaml:"teardown_group" bson:"teardown_group"`
	SetupTask               *YAMLCommandSet `yaml:"setup_task" bson:"setup_task"`
	TeardownTask            *YAMLCommandSet `yaml:"teardown_task" bson:"teardown_task"`
	Timeout                 *YAMLCommandSet `yaml:"timeout,omitempty" bson:"timeout"`
	Tasks                   []string        `yaml:"tasks" bson:"tasks"`
	Tags                    []string        `yaml:"tags,omitempty" bson:"tags"`
	// ShareProcs causes processes to persist between task group tasks.
	ShareProcs bool `yaml:"share_processes" bson:"share_processes"`
}

// Unmarshalled from the "tasks" list in the project file
type ProjectTask struct {
	Name            string               `yaml:"name,omitempty" bson:"name"`
	Priority        int64                `yaml:"priority,omitempty" bson:"priority"`
	ExecTimeoutSecs int                  `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs"`
	DependsOn       []TaskUnitDependency `yaml:"depends_on,omitempty" bson:"depends_on"`
	Commands        []PluginCommandConf  `yaml:"commands,omitempty" bson:"commands"`
	Tags            []string             `yaml:"tags,omitempty" bson:"tags"`
	RunOn           []string             `yaml:"run_on,omitempty" bson:"run_on"`
	// Use a *bool so that there are 3 possible states:
	//   1. nil   = not overriding the project setting (default)
	//   2. true  = overriding the project setting with true
	//   3. false = overriding the project setting with false
	Patchable       *bool `yaml:"patchable,omitempty" bson:"patchable,omitempty"`
	PatchOnly       *bool `yaml:"patch_only,omitempty" bson:"patch_only,omitempty"`
	Disable         *bool `yaml:"disable,omitempty" bson:"disable,omitempty"`
	AllowForGitTag  *bool `yaml:"allow_for_git_tag,omitempty" bson:"allow_for_git_tag,omitempty"`
	GitTagOnly      *bool `yaml:"git_tag_only,omitempty" bson:"git_tag_only,omitempty"`
	Stepback        *bool `yaml:"stepback,omitempty" bson:"stepback,omitempty"`
	MustHaveResults *bool `yaml:"must_have_test_results,omitempty" bson:"must_have_test_results,omitempty"`
}

type LoggerConfig struct {
	Agent  []LogOpts `yaml:"agent,omitempty" bson:"agent,omitempty"`
	System []LogOpts `yaml:"system,omitempty" bson:"system,omitempty"`
	Task   []LogOpts `yaml:"task,omitempty" bson:"task,omitempty"`
}

type LogOpts struct {
	Type         string `yaml:"type,omitempty" bson:"type,omitempty"`
	SplunkServer string `yaml:"splunk_server,omitempty" bson:"splunk_server,omitempty"`
	SplunkToken  string `yaml:"splunk_token,omitempty" bson:"splunk_token,omitempty"`
	LogDirectory string `yaml:"log_directory,omitempty" bson:"log_directory,omitempty"`
}

func (c *LoggerConfig) IsValid() error {
	if c == nil {
		return nil
	}
	catcher := grip.NewBasicCatcher()
	for _, opts := range c.Agent {
		catcher.Wrap(opts.IsValid(), "invalid agent logger config")
	}
	for _, opts := range c.System {
		catcher.Wrap(opts.IsValid(), "invalid system logger config")
		if opts.Type == FileLogSender {
			catcher.New("file logger is disallowed for system logs; will use Evergreen logger")
		} else if opts.Type == LogkeeperLogSender {
			catcher.New("logkeeper is disallowed for system logs; will use Evergreen logger")
		}
	}
	for _, opts := range c.Task {
		catcher.Wrap(opts.IsValid(), "invalid task logger config")
	}

	return catcher.Resolve()
}

func (o *LogOpts) IsValid() error {
	catcher := grip.NewBasicCatcher()
	if !utility.StringSliceContains(ValidLogSenders, o.Type) {
		catcher.Errorf("'%s' is not a valid log sender", o.Type)
	}
	if o.Type == SplunkLogSender && o.SplunkServer == "" {
		catcher.New("Splunk logger requires a server URL")
	}
	if o.Type == SplunkLogSender && o.SplunkToken == "" {
		catcher.New("Splunk logger requires a token")
	}

	return catcher.Resolve()
}

func mergeAllLogs(main, add *LoggerConfig) *LoggerConfig {
	if main == nil {
		return add
	} else if add == nil {
		return main
	} else {
		main.Agent = append(main.Agent, add.Agent...)
		main.System = append(main.System, add.System...)
		main.Task = append(main.Task, add.Task...)
	}
	return main
}

const (
	EvergreenLogSender   = "evergreen"
	FileLogSender        = "file"
	LogkeeperLogSender   = "logkeeper"
	BuildloggerLogSender = "buildlogger"
	SplunkLogSender      = "splunk"
)

// IsValidDefaultLogger returns whether the given logger, set either globally
// or at the project level, is a valid default logger. Default loggers must be
// configured globally or not require configuration and must be valid for use
// with system logs.
func IsValidDefaultLogger(logger string) bool {
	for _, validLogger := range ValidDefaultLoggers {
		if logger == validLogger {
			return true
		}
	}
	return false
}

var ValidDefaultLoggers = []string{
	EvergreenLogSender,
	BuildloggerLogSender,
}

var ValidLogSenders = []string{
	EvergreenLogSender,
	FileLogSender,
	LogkeeperLogSender,
	SplunkLogSender,
	BuildloggerLogSender,
}

// TaskIdTable is a map of [variant, task display name]->[task id].
type TaskIdTable map[TVPair]string

type TaskIdConfig struct {
	ExecutionTasks TaskIdTable
	DisplayTasks   TaskIdTable
}

// TVPair is a helper type for mapping bv/task pairs to ids.
type TVPair struct {
	Variant  string
	TaskName string
}

type TVPairSet []TVPair

// ByVariant returns a list of TVPairs filtered to include only those
// for the given variant
func (tvps TVPairSet) ByVariant(variant string) TVPairSet {
	p := []TVPair{}
	for _, pair := range tvps {
		if pair.Variant != variant {
			continue
		}
		p = append(p, pair)
	}
	return TVPairSet(p)
}

// TaskNames extracts the unique set of task names for a given variant in the set of task/variant pairs.
func (tvps TVPairSet) TaskNames(variant string) []string {
	taskSet := map[string]bool{}
	taskNames := []string{}
	for _, pair := range tvps {
		// skip over any pairs that aren't for the given variant
		if pair.Variant != variant {
			continue
		}
		// skip over tasks we already picked up
		if _, ok := taskSet[pair.TaskName]; ok {
			continue
		}
		taskSet[pair.TaskName] = true
		taskNames = append(taskNames, pair.TaskName)
	}
	return taskNames
}

// String returns the pair's name in a readable form.
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

// GetIdsForTaskInAllVariants returns all task Ids for taskName on all variants
func (tt TaskIdTable) GetIdsForTaskInAllVariants(taskName string) []string {
	ids := []string{}
	for pair, id := range tt {
		if pair.TaskName != taskName || id == "" {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// GetIdsForAllTasksInVariant returns all task Ids for all tasks on a variant
func (tt TaskIdTable) GetIdsForAllTasksInVariant(variantName string) []string {
	ids := []string{}
	for pair, id := range tt {
		if pair.Variant != variantName || id == "" {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// GetIdsForAllTasks returns every id in the table
func (tt TaskIdTable) GetIdsForAllTasks() []string {
	ids := make([]string, 0, len(tt))
	for _, id := range tt {
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// TaskIdTable builds a TaskIdTable for the given version and project
func NewTaskIdTable(p *Project, v *Version, sourceRev, defID string) TaskIdConfig {
	// init the variant map
	execTable := TaskIdTable{}
	displayTable := TaskIdTable{}

	sort.Stable(p.BuildVariants)

	projectIdentifier, err := GetIdentifierForProject(p.Identifier)
	if err != nil { // default to ID
		projectIdentifier = p.Identifier
	}
	for _, bv := range p.BuildVariants {
		rev := v.Revision
		if evergreen.IsPatchRequester(v.Requester) {
			rev = fmt.Sprintf("patch_%s_%s", v.Revision, v.Id)
		} else if v.Requester == evergreen.TriggerRequester {
			rev = fmt.Sprintf("%s_%s", sourceRev, defID)
		} else if v.Requester == evergreen.AdHocRequester {
			rev = v.Id
		} else if v.Requester == evergreen.GitTagRequester {
			rev = fmt.Sprintf("%s_%s", sourceRev, v.TriggeredByGitTag.Tag)
		}
		for _, t := range bv.Tasks {
			// omit tasks excluded from the version
			if t.IsDisabled() || t.SkipOnRequester(v.Requester) {
				continue
			}
			tg := t.TaskGroup
			if tg == nil {
				tg = p.FindTaskGroup(t.Name)
			}
			if tg != nil {
				for _, groupTask := range tg.Tasks {
					taskId := generateId(groupTask, projectIdentifier, &bv, rev, v)
					execTable[TVPair{bv.Name, groupTask}] = util.CleanName(taskId)
				}
			} else {
				// create a unique Id for each task
				taskId := generateId(t.Name, projectIdentifier, &bv, rev, v)
				execTable[TVPair{bv.Name, t.Name}] = util.CleanName(taskId)
			}
		}

		for _, dt := range bv.DisplayTasks {
			name := fmt.Sprintf("display_%s", dt.Name)
			taskId := generateId(name, projectIdentifier, &bv, rev, v)
			displayTable[TVPair{bv.Name, dt.Name}] = util.CleanName(taskId)
		}
	}
	return TaskIdConfig{ExecutionTasks: execTable, DisplayTasks: displayTable}
}

// NewPatchTaskIdTable constructs a new TaskIdTable (map of [variant, task display name]->[task  id])
func NewPatchTaskIdTable(proj *Project, v *Version, tasks TaskVariantPairs, projectIdentifier string) TaskIdConfig {
	config := TaskIdConfig{ExecutionTasks: TaskIdTable{}, DisplayTasks: TaskIdTable{}}
	processedVariants := map[string]bool{}

	// resolve task groups to exec tasks
	tgMap := map[string]TaskGroup{}
	for _, tg := range proj.TaskGroups {
		tgMap[tg.Name] = tg
	}
	for _, variant := range proj.BuildVariants {
		for _, t := range variant.Tasks {
			if t.TaskGroup != nil {
				tgMap[t.Name] = *t.TaskGroup
			}
		}
	}
	execTasksWithTaskGroupTasks := TVPairSet{}
	for _, vt := range tasks.ExecTasks {
		if _, ok := tgMap[vt.TaskName]; ok {
			if tg := proj.FindTaskGroup(vt.TaskName); tg != nil {
				for _, t := range tg.Tasks {
					execTasksWithTaskGroupTasks = append(execTasksWithTaskGroupTasks, TVPair{vt.Variant, t})
				}
			}
		} else {
			execTasksWithTaskGroupTasks = append(execTasksWithTaskGroupTasks, vt)
		}
	}
	tasks.ExecTasks = execTasksWithTaskGroupTasks

	for _, vt := range tasks.ExecTasks {
		// don't hit the same variant more than once
		if _, ok := processedVariants[vt.Variant]; ok {
			continue
		}
		processedVariants[vt.Variant] = true
		config.ExecutionTasks = generateIdsForVariant(vt, proj, v, tasks.ExecTasks, config.ExecutionTasks, tgMap, projectIdentifier)
	}
	processedVariants = map[string]bool{}
	for _, vt := range tasks.DisplayTasks {
		// don't hit the same variant more than once
		if _, ok := processedVariants[vt.Variant]; ok {
			continue
		}
		processedVariants[vt.Variant] = true
		config.DisplayTasks = generateIdsForVariant(vt, proj, v, tasks.DisplayTasks, config.DisplayTasks, tgMap, projectIdentifier)
	}
	return config
}

func generateIdsForVariant(vt TVPair, proj *Project, v *Version, tasks TVPairSet, table TaskIdTable,
	tgMap map[string]TaskGroup, projectIdentifier string) TaskIdTable {
	if table == nil {
		table = map[TVPair]string{}
	}

	// we must track the project's variants definitions as well,
	// so that we don't create Ids for variants that don't exist.
	projBV := proj.FindBuildVariant(vt.Variant)
	taskNamesForVariant := tasks.TaskNames(vt.Variant)
	rev := v.Revision
	if evergreen.IsPatchRequester(v.Requester) {
		rev = fmt.Sprintf("patch_%s_%s", v.Revision, v.Id)
	}
	for _, t := range projBV.Tasks { // create Ids for each task that can run on the variant and is requested by the patch.
		if utility.StringSliceContains(taskNamesForVariant, t.Name) {
			table[TVPair{vt.Variant, t.Name}] = util.CleanName(generateId(t.Name, projectIdentifier, projBV, rev, v))
		} else if tg, ok := tgMap[t.Name]; ok {
			for _, name := range tg.Tasks {
				table[TVPair{vt.Variant, name}] = util.CleanName(generateId(name, projectIdentifier, projBV, rev, v))
			}
		}
	}
	for _, t := range projBV.DisplayTasks {
		// create Ids for each task that can run on the variant and is requested by the patch.
		if utility.StringSliceContains(taskNamesForVariant, t.Name) {
			table[TVPair{vt.Variant, t.Name}] = util.CleanName(generateId(fmt.Sprintf("display_%s", t.Name), projectIdentifier, projBV, rev, v))
		}
	}

	return table
}

func generateId(name string, projectIdentifier string, projBV *BuildVariant, rev string, v *Version) string {
	return fmt.Sprintf("%s_%s_%s_%s_%s",
		projectIdentifier,
		projBV.Name,
		name,
		rev,
		v.CreateTime.Format(build.IdTimeLayout))
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
)

func PopulateExpansions(t *task.Task, h *host.Host, oauthToken string) (util.Expansions, error) {
	if t == nil {
		return nil, errors.New("task cannot be nil")
	}

	projectRef, err := FindBranchProjectRef(t.Project)
	if err != nil {
		return nil, errors.Wrap(err, "finding project ref")
	}

	expansions := util.Expansions{}
	expansions.Put("execution", fmt.Sprintf("%v", t.Execution))
	expansions.Put("version_id", t.Version)
	expansions.Put("task_id", t.Id)
	expansions.Put("task_name", t.DisplayName)
	expansions.Put("build_id", t.BuildId)
	expansions.Put("build_variant", t.BuildVariant)
	expansions.Put("revision", t.Revision)
	expansions.Put("github_commit", t.Revision)
	expansions.Put(evergreen.GlobalGitHubTokenExpansion, oauthToken)
	expansions.Put("project", projectRef.Identifier)
	expansions.Put("project_identifier", projectRef.Identifier) // TODO: deprecate
	expansions.Put("project_id", projectRef.Id)
	if h != nil {
		expansions.Put("distro_id", h.Distro.Id)
	}
	if t.ActivatedBy == evergreen.StepbackTaskActivator {
		expansions.Put("is_stepback", "true")
	}
	if t.TriggerID != "" {
		expansions.Put("trigger_event_identifier", t.TriggerID)
		expansions.Put("trigger_event_type", t.TriggerType)
		expansions.Put("trigger_id", t.TriggerEvent)
		var upstreamProjectID string
		if t.TriggerType == ProjectTriggerLevelTask {
			var upstreamTask *task.Task
			upstreamTask, err = task.FindOneId(t.TriggerID)
			if err != nil {
				return nil, errors.Wrap(err, "finding task")
			}
			if upstreamTask == nil {
				return nil, errors.New("upstream task not found")
			}
			expansions.Put("trigger_status", upstreamTask.Status)
			expansions.Put("trigger_revision", upstreamTask.Revision)
			upstreamProjectID = upstreamTask.Project
		} else if t.TriggerType == ProjectTriggerLevelBuild {
			var upstreamBuild *build.Build
			upstreamBuild, err = build.FindOneId(t.TriggerID)
			if err != nil {
				return nil, errors.Wrap(err, "finding build")
			}
			if upstreamBuild == nil {
				return nil, errors.New("upstream build not found")
			}
			expansions.Put("trigger_status", upstreamBuild.Status)
			expansions.Put("trigger_revision", upstreamBuild.Revision)
			upstreamProjectID = upstreamBuild.Project
		}
		var upstreamProject *ProjectRef
		upstreamProject, err = FindBranchProjectRef(upstreamProjectID)
		if err != nil {
			return nil, errors.Wrap(err, "finding project")
		}
		if upstreamProject == nil {
			return nil, errors.Errorf("upstream project '%s' not found", t.Project)
		}
		expansions.Put("trigger_repo_owner", upstreamProject.Owner)
		expansions.Put("trigger_repo_name", upstreamProject.Repo)
		expansions.Put("trigger_branch", upstreamProject.Branch)
	}

	v, err := VersionFindOneId(t.Version)
	if err != nil {
		return nil, errors.Wrap(err, "finding version")
	}
	if v == nil {
		return nil, errors.Wrapf(err, "version '%s' doesn't exist", v.Id)
	}

	expansions.Put("branch_name", v.Branch)
	expansions.Put("author", v.Author)
	expansions.Put("author_email", v.AuthorEmail)
	expansions.Put("created_at", v.CreateTime.Format(build.IdTimeLayout))

	requesterExpansion := ""
	switch v.Requester {
	case evergreen.PatchVersionRequester:
		requesterExpansion = "patch"
	case evergreen.GithubPRRequester:
		requesterExpansion = "github_pr"
	case evergreen.GitTagRequester:
		requesterExpansion = "github_tag"
	case evergreen.RepotrackerVersionRequester:
		requesterExpansion = "commit"
	case evergreen.TriggerRequester:
		requesterExpansion = "trigger"
	case evergreen.MergeTestRequester:
		requesterExpansion = "commit_queue"
	case evergreen.AdHocRequester:
		requesterExpansion = "ad_hoc"
	default:
		requesterExpansion = "unknown_requester"
	}
	expansions.Put("requester", requesterExpansion)

	if evergreen.IsGitTagRequester(v.Requester) {
		expansions.Put("triggered_by_git_tag", v.TriggeredByGitTag.Tag)
	}
	if evergreen.IsPatchRequester(v.Requester) {
		var p *patch.Patch
		p, err = patch.FindOne(patch.ByVersion(t.Version))
		if err != nil {
			return nil, errors.Wrapf(err, "finding patch for version '%s'", t.Version)
		}
		if p == nil {
			return nil, errors.Errorf("no patch found for version '%s'", t.Version)
		}

		expansions.Put("is_patch", "true")
		expansions.Put("revision_order_id", fmt.Sprintf("%s_%d", v.Author, v.RevisionOrderNumber))
		expansions.Put("alias", p.Alias)

		if v.Requester == evergreen.MergeTestRequester {
			expansions.Put("is_commit_queue", "true")
			expansions.Put("commit_message", p.Description)
		}

		if v.Requester == evergreen.GithubPRRequester {
			expansions.Put("github_pr_number", fmt.Sprintf("%d", p.GithubPatchData.PRNumber))
			expansions.Put("github_org", p.GithubPatchData.BaseOwner)
			expansions.Put("github_repo", p.GithubPatchData.BaseRepo)
			expansions.Put("github_author", p.GithubPatchData.Author)
			expansions.Put("github_commit", p.GithubPatchData.HeadHash)
		}
	} else {
		expansions.Put("is_patch", "")
		expansions.Put("revision_order_id", strconv.Itoa(v.RevisionOrderNumber))
	}

	if h != nil {
		for _, e := range h.Distro.Expansions {
			expansions.Put(e.Key, e.Value)
		}
	}

	bvExpansions, err := FindExpansionsForVariant(v, t.BuildVariant)
	if err != nil {
		return nil, errors.Wrap(err, "getting expansions for variant")
	}
	expansions.Update(bvExpansions)
	return expansions, nil
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

func (p *Project) GetParameters() []patch.Parameter {
	res := []patch.Parameter{}
	for _, param := range p.Parameters {
		res = append(res, param.Parameter)
	}
	return res
}

// RunOnVariant returns true if the plugin command should run on variant; returns false otherwise
func (p PluginCommandConf) RunOnVariant(variant string) bool {
	return len(p.Variants) == 0 || utility.StringSliceContains(p.Variants, variant)
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

// GetRepoOwnerAndName returns the owner and repo name (in that order) of a module
func (m *Module) GetRepoOwnerAndName() (string, string) {
	parts := strings.Split(m.Repo, ":")
	basename := parts[len(parts)-1]
	ownerAndName := strings.TrimSuffix(basename, ".git")
	ownersplit := strings.Split(ownerAndName, "/")
	if len(ownersplit) != 2 {
		return "", ""
	} else {
		return ownersplit[0], ownersplit[1]
	}
}

// FindTaskGroup returns a specific task group from a project
func (p *Project) FindTaskGroup(name string) *TaskGroup {
	for _, tg := range p.TaskGroups {
		if tg.Name == name {
			return &tg
		}
	}
	for _, bv := range p.BuildVariants {
		for _, t := range bv.Tasks {
			if t.TaskGroup != nil && t.Name == name {
				return t.TaskGroup
			}
		}
	}
	return nil
}

// FindContainerFromProject finds the container configuration associated with a given task's Container field.
func FindContainerFromProject(t task.Task) (*Container, error) {
	project, _, err := FindAndTranslateProjectForVersion(t.Version, t.Project)
	if err != nil {
		return nil, errors.Wrapf(err, "getting project for version '%s'", t.Version)
	}
	for _, container := range project.Containers {
		if container.Name == t.Container {
			return &container, nil
		}
	}
	return nil, errors.Errorf("no such container '%s' defined on project '%s'", t.Container, t.Project)
}

func FindProjectFromVersionID(versionStr string) (*Project, error) {
	ver, err := VersionFindOne(VersionById(versionStr))
	if err != nil {
		return nil, err
	}
	if ver == nil {
		return nil, errors.Errorf("version '%s' not found", versionStr)
	}

	project, _, err := FindAndTranslateProjectForVersion(ver.Id, ver.Identifier)
	if err != nil {
		return nil, errors.Wrapf(err, "loading project config for version '%s'", versionStr)
	}
	return project, nil
}

func (p *Project) FindDistroNameForTask(t *task.Task) (string, error) {
	bv, err := p.BuildVariants.Get(t.BuildVariant)
	if err != nil {
		return "", errors.Wrapf(err, "finding build variant for task '%s'", t.Id)
	}

	bvt, err := bv.Get(t.DisplayName)
	if err != nil {
		return "", errors.Wrapf(err, "finding build variant task unit for task '%s'", t.Id)
	}

	var distro string

	if len(bvt.RunOn) > 0 {
		distro = bvt.RunOn[0]
	} else if len(bv.RunOn) > 0 {
		distro = bv.RunOn[0]
	} else {
		return "", errors.Errorf("cannot find the distro for task '%s'", t.Id)
	}

	return distro, nil
}

func FindLatestVersionWithValidProject(projectId string) (*Version, *Project, error) {
	const retryCount = 5
	if projectId == "" {
		return nil, nil, errors.WithStack(errors.New("cannot pass empty projectId to FindLatestVersionWithValidProject"))
	}
	project := &Project{
		Identifier: projectId,
	}

	revisionOrderNum := -1 // only specify in the event of failure
	var err error
	var lastGoodVersion *Version
	for i := 0; i < retryCount; i++ {
		lastGoodVersion, err = FindVersionByLastKnownGoodConfig(projectId, revisionOrderNum)
		if err != nil {
			// database error, don't log critical
			continue
		}
		if lastGoodVersion != nil {
			project, _, err = FindAndTranslateProjectForVersion(lastGoodVersion.Id, projectId)
			if err != nil {
				grip.Critical(message.WrapError(err, message.Fields{
					"message": "last known good version has malformed config",
					"version": lastGoodVersion.Id,
					"project": projectId,
				}))
				revisionOrderNum = lastGoodVersion.RevisionOrderNumber // look for an older version if the returned version is malformed
				continue
			}
		}
		return lastGoodVersion, project, nil
	}

	if lastGoodVersion == nil {
		return nil, nil, errors.Wrapf(err, "finding a valid version for project '%s'", projectId)
	}

	return nil, nil, errors.Wrapf(err, "loading project from "+
		"last good version for project '%s'", lastGoodVersion.Identifier)
}

// HasSpecificActivation returns if the build variant task specifies an activation condition that
// overrides the default, such as cron/batchtime, disabling the task, or explicitly activating it.
func (bvt *BuildVariantTaskUnit) HasSpecificActivation() bool {
	return bvt.CronBatchTime != "" || bvt.BatchTime != nil || bvt.Activate != nil || bvt.IsDisabled()
}

func (p *Project) FindTaskForVariant(task, variant string) *BuildVariantTaskUnit {
	bv := p.FindBuildVariant(variant)
	if bv == nil {
		return nil
	}

	tgMap := map[string]TaskGroup{}
	for _, tg := range p.TaskGroups {
		tgMap[tg.Name] = tg
	}
	for _, t := range bv.Tasks {
		if t.TaskGroup != nil {
			tgMap[t.Name] = *t.TaskGroup
		}
	}

	for _, bvt := range bv.Tasks {
		if bvt.Name == task {
			bvt.Variant = variant
			if projectTask := p.FindProjectTask(task); projectTask != nil {
				return &bvt
			} else if _, exists := tgMap[task]; exists {
				return &bvt
			}
		}
		if tg, ok := tgMap[bvt.Name]; ok {
			for _, t := range tg.Tasks {
				if t == task {
					bvt.Variant = variant
					// task group tasks need to be repopulated from the task list
					bvt.Populate(*p.FindProjectTask(task))
					return &bvt
				}
			}
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

// findMatchingBuildVariants returns a list of build variant names in a project that match the given regexp.
func (p *Project) findMatchingBuildVariants(bvRegex *regexp.Regexp) []string {
	var res []string
	for _, b := range p.BuildVariants {
		if bvRegex.MatchString(b.Name) {
			res = append(res, b.Name)
		}
	}
	return res
}

func (p *Project) findBuildVariantsWithTag(tags []string) []string {
	var res []string
	for _, b := range p.BuildVariants {
		if len(utility.StringSliceIntersection(b.Tags, tags)) > 0 {
			res = append(res, b.Name)
		}
	}
	return res
}

// GetTaskNameAndTags checks the project for a task or task group matching the
// build variant task unit, and returns the name and tags
func (p *Project) GetTaskNameAndTags(bvt BuildVariantTaskUnit) (string, []string, bool) {
	if bvt.IsGroup {
		ptg := bvt.TaskGroup
		if ptg == nil {
			ptg = p.FindTaskGroup(bvt.Name)
		}
		if ptg == nil {
			return "", nil, false
		}
		return ptg.Name, ptg.Tags, true
	}

	pt := p.FindProjectTask(bvt.Name)
	if pt == nil {
		return "", nil, false
	}
	return pt.Name, pt.Tags, true
}

func (p *Project) FindProjectTask(name string) *ProjectTask {
	for _, t := range p.Tasks {
		if t.Name == name {
			return &t
		}
	}
	return nil
}

// findMatchingProjectTasks returns a list of tasks in a project that match the given regexp.
func (p *Project) findMatchingProjectTasks(tRegex *regexp.Regexp) []string {
	var res []string
	for _, t := range p.Tasks {
		if tRegex.MatchString(t.Name) {
			res = append(res, t.Name)
		}
	}
	return res
}

func (p *Project) findProjectTasksWithTag(tags []string) []string {
	var res []string
	for _, t := range p.Tasks {
		if len(utility.StringSliceIntersection(t.Tags, tags)) > 0 {
			res = append(res, t.Name)
		}
	}
	return res
}

func (p *Project) GetModuleByName(name string) (*Module, error) {
	for _, v := range p.Modules {
		if v.Name == name {
			return &v, nil
		}
	}
	return nil, errors.New("no such module on this project")
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

func (p *Project) FindDisplayTasksForVariant(build string) []string {
	for _, b := range p.BuildVariants {
		if b.Name == build {
			displayTasks := make([]string, 0, len(b.DisplayTasks))
			for _, dt := range b.DisplayTasks {
				displayTasks = append(displayTasks, dt.Name)
			}
			return displayTasks
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

// FindAllBuildVariantTasks returns every BuildVariantTaskUnit, fully populated,
// for all variants of a project.
func (p *Project) FindAllBuildVariantTasks() []BuildVariantTaskUnit {
	tasksByName := map[string]ProjectTask{}
	for _, t := range p.Tasks {
		tasksByName[t.Name] = t
	}
	allBVTs := []BuildVariantTaskUnit{}
	for _, b := range p.BuildVariants {
		for _, t := range b.Tasks {
			t.Variant = b.Name
			if t.IsGroup {
				allBVTs = append(allBVTs, p.tasksFromGroup(t)...)
			} else {
				t.Populate(tasksByName[t.Name])
				allBVTs = append(allBVTs, t)
			}
		}
	}
	return allBVTs
}

// tasksFromGroup returns a slice of the task group's tasks.
// Settings missing from the group task are populated from the task definition.
func (p *Project) tasksFromGroup(bvTaskGroup BuildVariantTaskUnit) []BuildVariantTaskUnit {
	tg := bvTaskGroup.TaskGroup
	if tg == nil {
		tg = p.FindTaskGroup(bvTaskGroup.Name)
	}
	if tg == nil {
		return nil
	}

	tasks := []BuildVariantTaskUnit{}
	taskMap := map[string]ProjectTask{}
	for _, projTask := range p.Tasks {
		taskMap[projTask.Name] = projTask
	}

	for _, t := range tg.Tasks {
		bvt := BuildVariantTaskUnit{
			Name: t,
			// IsGroup is not persisted, and indicates here that the
			// task is a member of a task group.
			IsGroup:          true,
			TaskGroup:        bvTaskGroup.TaskGroup,
			Variant:          bvTaskGroup.Variant,
			GroupName:        bvTaskGroup.Name,
			Patchable:        bvTaskGroup.Patchable,
			PatchOnly:        bvTaskGroup.PatchOnly,
			Disable:          bvTaskGroup.Disable,
			AllowForGitTag:   bvTaskGroup.AllowForGitTag,
			GitTagOnly:       bvTaskGroup.GitTagOnly,
			Priority:         bvTaskGroup.Priority,
			DependsOn:        bvTaskGroup.DependsOn,
			RunOn:            bvTaskGroup.RunOn,
			ExecTimeoutSecs:  bvTaskGroup.ExecTimeoutSecs,
			Stepback:         bvTaskGroup.Stepback,
			Activate:         bvTaskGroup.Activate,
			CommitQueueMerge: bvTaskGroup.CommitQueueMerge,
		}
		// Default to project task settings when unspecified
		bvt.Populate(taskMap[t])
		tasks = append(tasks, bvt)
	}
	return tasks
}

func (p *Project) FindAllTasksMap() map[string]ProjectTask {
	allTasks := map[string]ProjectTask{}
	for _, task := range p.Tasks {
		allTasks[task.Name] = task
	}

	return allTasks
}

// FindVariantsWithTask returns the name of each variant containing
// the given task name.
func (p *Project) FindVariantsWithTask(task string) []string {
	variants := make([]string, 0, len(p.BuildVariants))
	for _, b := range p.BuildVariants {
		for _, t := range b.Tasks {
			if t.Name == task {
				variants = append(variants, b.Name)
			}
		}
	}
	return variants
}

// IgnoresAllFiles takes in a slice of filepaths and checks to see if
// all files are matched by the project's Ignore regular expressions.
func (p *Project) IgnoresAllFiles(files []string) bool {
	if len(p.Ignore) == 0 || len(files) == 0 {
		return false
	}
	// CompileIgnoreLines has a silly API: it always returns a nil error.
	ignorer := ignore.CompileIgnoreLines(p.Ignore...)
	for _, f := range files {
		if !ignorer.MatchesPath(f) {
			return false
		}
	}
	return true
}

// BuildProjectTVPairs resolves the build variants and tasks into which build
// variants will run and which tasks will run on each build variant.
func (p *Project) BuildProjectTVPairs(patchDoc *patch.Patch, alias string) {
	patchDoc.BuildVariants, patchDoc.Tasks, patchDoc.VariantsTasks = p.ResolvePatchVTs(patchDoc, patchDoc.GetRequester(), alias, true)
}

// ResolvePatchVTs resolves a list of build variants and tasks into a list of
// all build variants that will run, a list of all tasks that will run, and a
// mapping of the build variant to the tasks that will run on that build
// variant. If includeDeps is set, it will also resolve task dependencies.
func (p *Project) ResolvePatchVTs(patchDoc *patch.Patch, requester, alias string, includeDeps bool) (resolvedBVs []string, resolvedTasks []string, vts []patch.VariantTasks) {
	var bvs, bvTags, tasks, taskTags []string
	for _, bv := range patchDoc.BuildVariants {
		// Tags should start with "."
		if strings.HasPrefix(bv, ".") {
			bvTags = append(bvTags, bv[1:])
		} else {
			bvs = append(bvs, bv)
		}
	}
	for _, t := range patchDoc.Tasks {
		// Tags should start with "."
		if strings.HasPrefix(t, ".") {
			taskTags = append(taskTags, t[1:])
		} else {
			tasks = append(tasks, t)
		}
	}

	if len(bvs) == 1 && bvs[0] == "all" {
		bvs = []string{}
		for _, bv := range p.BuildVariants {
			if bv.Disabled {
				continue
			}
			bvs = append(bvs, bv.Name)
		}
	} else {
		if len(bvTags) > 0 {
			bvs = append(bvs, p.findBuildVariantsWithTag(bvTags)...)
		}
		for _, bv := range patchDoc.RegexBuildVariants {
			bvRegex, err := regexp.Compile(bv)
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":   "compiling buildvariant regex",
					"regex":     bv,
					"project":   p.Identifier,
					"patch_doc": patchDoc.Id,
				}))
				continue
			}
			bvs = append(bvs, p.findMatchingBuildVariants(bvRegex)...)
		}
	}
	if len(tasks) == 1 && tasks[0] == "all" {
		tasks = []string{}
		for _, t := range p.Tasks {
			if !utility.FromBoolTPtr(t.Patchable) || utility.FromBoolPtr(t.GitTagOnly) {
				continue
			}
			tasks = append(tasks, t.Name)
		}
	} else {
		if len(taskTags) > 0 {
			tasks = append(tasks, p.findProjectTasksWithTag(taskTags)...)
		}
		for _, t := range patchDoc.RegexTasks {
			tRegex, err := regexp.Compile(t)
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":   "compiling task regex",
					"regex":     t,
					"project":   p.Identifier,
					"patch_doc": patchDoc.Id,
				}))
				continue
			}
			tasks = append(tasks, p.findMatchingProjectTasks(tRegex)...)
		}
	}
	var pairs TaskVariantPairs
	for _, v := range bvs {
		for _, t := range tasks {
			if p.FindTaskForVariant(t, v) != nil {
				pairs.ExecTasks = append(pairs.ExecTasks, TVPair{Variant: v, TaskName: t})
			} else if p.GetDisplayTask(v, t) != nil {
				pairs.DisplayTasks = append(pairs.DisplayTasks, TVPair{Variant: v, TaskName: t})
			}
		}
	}

	if alias != "" {
		catcher := grip.NewBasicCatcher()
		vars, err := findAliasesForPatch(p.Identifier, alias, patchDoc)
		catcher.Wrapf(err, "retrieving alias '%s' for patched project config '%s'", alias, patchDoc.Id.Hex())

		var aliasPairs, displayTaskPairs []TVPair
		if !catcher.HasErrors() {
			aliasPairs, displayTaskPairs, err = p.BuildProjectTVPairsWithAlias(vars)
			catcher.Wrap(err, "getting task/variant pairs for alias")
		}
		grip.Error(message.WrapError(catcher.Resolve(), message.Fields{
			"message": "problem adding variants/tasks for alias",
			"alias":   alias,
			"project": p.Identifier,
		}))

		if !catcher.HasErrors() {
			pairs.ExecTasks = append(pairs.ExecTasks, aliasPairs...)
			pairs.DisplayTasks = append(pairs.DisplayTasks, displayTaskPairs...)
		}
	}

	pairs = p.extractDisplayTasks(pairs)
	if includeDeps {
		var err error
		pairs.ExecTasks, err = IncludeDependencies(p, pairs.ExecTasks, requester, nil)
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "error including dependencies",
			"project": p.Identifier,
		}))
	}

	vts = pairs.TVPairsToVariantTasks()
	bvs, tasks = patch.ResolveVariantTasks(vts)
	return bvs, tasks, vts
}

// GetVariantTasks returns all the build variants and all tasks specified for
// each build variant.
func (p *Project) GetAllVariantTasks() []patch.VariantTasks {
	var vts []patch.VariantTasks
	for _, bv := range p.BuildVariants {
		vt := patch.VariantTasks{Variant: bv.Name}
		for _, t := range bv.Tasks {
			vt.Tasks = append(vt.Tasks, t.Name)
		}
		for _, dt := range bv.DisplayTasks {
			vt.DisplayTasks = append(vt.DisplayTasks, patch.DisplayTask{
				Name:      dt.Name,
				ExecTasks: dt.ExecTasks,
			})
		}
		vts = append(vts, vt)
	}
	return vts
}

// TasksThatCallCommand returns a map of tasks that call a given command to the
// number of times the command is called in the task.
func (p *Project) TasksThatCallCommand(find string) map[string]int {
	// get all functions that call the command.
	fs := map[string]int{}
	for f, cmds := range p.Functions {
		if cmds == nil {
			continue
		}
		for _, c := range cmds.List() {
			if c.Command == find {
				fs[f] = fs[f] + 1
			}
		}
	}

	// get all tasks that call the command.
	ts := map[string]int{}
	for _, t := range p.Tasks {
		for _, c := range t.Commands {
			if c.Function != "" {
				if times, ok := fs[c.Function]; ok {
					ts[t.Name] = ts[t.Name] + times
				}
			}
			if c.Command == find {
				ts[t.Name] = ts[t.Name] + 1
			}
		}
	}
	return ts
}

// IsGenerateTask indicates that the task generates other tasks, which the
// scheduler will use to prioritize this task.
func (p *Project) IsGenerateTask(taskName string) bool {
	_, ok := p.TasksThatCallCommand(evergreen.GenerateTasksCommandName)[taskName]
	return ok
}

func findAliasesForPatch(projectId, alias string, patchDoc *patch.Patch) ([]ProjectAlias, error) {
	vars, shouldExit, err := findAliasInProjectOrRepoFromDb(projectId, alias)
	if err != nil {
		return nil, errors.Wrap(err, "getting alias from project")
	}
	if !shouldExit && len(vars) == 0 {
		pRef, err := FindMergedProjectRef(projectId, "", false)
		if err != nil {
			return nil, errors.Wrap(err, "getting project ref")
		}
		if pRef == nil || !pRef.IsVersionControlEnabled() {
			return vars, nil
		}
		if len(patchDoc.PatchedProjectConfig) > 0 {
			projectConfig, err := CreateProjectConfig([]byte(patchDoc.PatchedProjectConfig), projectId)
			if err != nil {
				return nil, errors.Wrap(err, "retrieving aliases from patched config")
			}
			vars, err = findAliasFromProjectConfig(projectConfig, alias)
			if err != nil {
				return nil, errors.Wrapf(err, "retrieving alias '%s' from project config", alias)
			}
		} else if patchDoc.Version != "" {
			vars, err = getMatchingAliasForVersion(patchDoc.Version, alias)
			if err != nil {
				return nil, errors.Wrapf(err, "retrieving alias '%s' from project config", alias)
			}
		}
	}
	return vars, nil
}

// extractDisplayTasks adds display tasks and all their execution tasks when
// pairs.DisplayTasks includes the display task or when
// one constituent execution task is included
func (p *Project) extractDisplayTasks(pairs TaskVariantPairs) TaskVariantPairs {
	displayTasksToExecTasks := make(map[TVPair][]TVPair)
	execTaskToDisplayTask := make(map[TVPair]TVPair)
	for _, bv := range p.BuildVariants {
		for _, dt := range bv.DisplayTasks {
			displayTV := TVPair{Variant: bv.Name, TaskName: dt.Name}
			execTVPairs := make([]TVPair, 0, len(dt.ExecTasks))
			for _, et := range dt.ExecTasks {
				exexTV := TVPair{Variant: bv.Name, TaskName: et}
				execTaskToDisplayTask[exexTV] = displayTV
				execTVPairs = append(execTVPairs, exexTV)
			}
			displayTasksToExecTasks[displayTV] = execTVPairs
		}
	}

	// slices -> sets
	displayTaskSet := make(map[TVPair]bool)
	for _, dt := range pairs.DisplayTasks {
		displayTaskSet[dt] = true
	}
	executionTaskSet := make(map[TVPair]bool)
	for _, et := range pairs.ExecTasks {
		executionTaskSet[et] = true
	}

	// include a display task when one of its execution tasks was specified
	for et := range executionTaskSet {
		dt, ok := execTaskToDisplayTask[et]
		if ok {
			displayTaskSet[dt] = true
		}
	}

	// include every execution task of the all the display tasks
	for dt := range displayTaskSet {
		for _, et := range displayTasksToExecTasks[dt] {
			executionTaskSet[et] = true
		}
	}

	// sets -> slices
	displayTasks := make(TVPairSet, 0, len(displayTaskSet))
	execTasks := make(TVPairSet, 0, len(executionTaskSet))
	for dt := range displayTaskSet {
		displayTasks = append(displayTasks, dt)
	}
	for et := range executionTaskSet {
		execTasks = append(execTasks, et)
	}

	return TaskVariantPairs{ExecTasks: execTasks, DisplayTasks: displayTasks}
}

// BuildProjectTVPairsWithAlias returns variants and tasks for a project alias.
func (p *Project) BuildProjectTVPairsWithAlias(vars []ProjectAlias) ([]TVPair, []TVPair, error) {
	pairs := []TVPair{}
	displayTaskPairs := []TVPair{}
	for _, v := range vars {
		var variantRegex *regexp.Regexp
		variantRegex, err := v.getVariantRegex()
		if err != nil {
			return nil, nil, err
		}

		var taskRegex *regexp.Regexp
		taskRegex, err = v.getTaskRegex()
		if err != nil {
			return nil, nil, err
		}

		for _, variant := range p.BuildVariants {
			if isValidRegexOrTag(variant.Name, variant.Tags, v.VariantTags, variantRegex) {
				for _, t := range p.Tasks {
					if t.Patchable != nil && !(*t.Patchable) {
						continue
					}
					if !isValidRegexOrTag(t.Name, t.Tags, v.TaskTags, taskRegex) {
						continue
					}

					if p.FindTaskForVariant(t.Name, variant.Name) != nil {
						pairs = append(pairs, TVPair{variant.Name, t.Name})
					}
				}

				if v.Task == "" {
					continue
				}
				for _, displayTask := range variant.DisplayTasks {
					if !taskRegex.MatchString(displayTask.Name) {
						continue
					}
					displayTaskPairs = append(displayTaskPairs, TVPair{variant.Name, displayTask.Name})
				}
			}
		}
	}

	return pairs, displayTaskPairs, nil
}

func (p *Project) VariantTasksForSelectors(definitions []patch.PatchTriggerDefinition, requester string) ([]patch.VariantTasks, error) {
	projectAliases := []ProjectAlias{}
	for _, definition := range definitions {
		for _, specifier := range definition.TaskSpecifiers {
			if specifier.PatchAlias != "" {
				aliases, err := FindAliasInProjectRepoOrConfig(p.Identifier, specifier.PatchAlias)
				if err != nil {
					return nil, errors.Wrap(err, "getting alias from project")
				}
				projectAliases = append(projectAliases, aliases...)
			} else {
				projectAliases = append(projectAliases, ProjectAlias{Variant: specifier.VariantRegex, Task: specifier.TaskRegex})
			}
		}
	}

	var err error
	pairs := TaskVariantPairs{}
	pairs.ExecTasks, pairs.DisplayTasks, err = p.BuildProjectTVPairsWithAlias(projectAliases)
	if err != nil {
		return nil, errors.Wrap(err, "getting pairs matching patch aliases")
	}
	pairs = p.extractDisplayTasks(pairs)
	pairs.ExecTasks, err = IncludeDependencies(p, pairs.ExecTasks, requester, nil)
	grip.Warning(message.WrapError(err, message.Fields{
		"message": "error including dependencies",
		"project": p.Identifier,
	}))

	return pairs.TVPairsToVariantTasks(), nil
}

// CommandsRunOnTV returns the list of matching commands that match the given
// command name on the given task in a build variant.
func (p *Project) CommandsRunOnTV(tv TVPair, cmd string) ([]PluginCommandConf, error) {
	task := p.FindProjectTask(tv.TaskName)
	if task == nil {
		return nil, errors.Errorf("definition of task '%s' not found", tv.TaskName)
	}
	return p.CommandsRunOnBV(task.Commands, cmd, tv.Variant), nil
}

// CommandsRunOnBV returns the list of matching commands from cmds that will run
// the named command on the build variant.
func (p *Project) CommandsRunOnBV(cmds []PluginCommandConf, cmd, bv string) []PluginCommandConf {
	var matchingCmds []PluginCommandConf
	for _, c := range cmds {
		if c.Function != "" {
			f, ok := p.Functions[c.Function]
			if !ok || f == nil {
				continue
			}
			for _, funcCmd := range f.List() {
				if funcCmd.Command == cmd && funcCmd.RunOnVariant(bv) {
					matchingCmds = append(matchingCmds, funcCmd)
				}
			}
		} else if c.Command == cmd && c.RunOnVariant(bv) {
			matchingCmds = append(matchingCmds, c)
		}
	}
	return matchingCmds
}

func (p *Project) GetDisplayTask(variant, name string) *patch.DisplayTask {
	bv := p.FindBuildVariant(variant)
	if bv == nil {
		return nil
	}
	for _, dt := range bv.DisplayTasks {
		if dt.Name == name {
			return &dt
		}
	}

	return nil
}

// DependencyGraph returns a task.DependencyGraph populated with the tasks in the project.
func (p *Project) DependencyGraph() task.DependencyGraph {
	tasks := p.FindAllBuildVariantTasks()
	g := task.NewDependencyGraph(false)

	for _, t := range tasks {
		g.AddTaskNode(t.toTaskNode())
	}

	for _, dependencyEdge := range dependenciesForTaskUnit(tasks) {
		g.AddEdge(dependencyEdge.From, dependencyEdge.To, dependencyEdge.Status)
	}

	return g

}

// dependenciesForTaskUnit returns a slice of dependencies between tasks in the project.
func dependenciesForTaskUnit(taskUnits []BuildVariantTaskUnit) []task.DependencyEdge {
	var dependencies []task.DependencyEdge
	for _, dependentTask := range taskUnits {
		for _, dep := range dependentTask.DependsOn {
			// Use the current variant if none is specified.
			if dep.Variant == "" {
				dep.Variant = dependentTask.Variant
			}

			for _, dependedOnTask := range taskUnits {
				if dependedOnTask.ToTVPair() != dependentTask.ToTVPair() &&
					(dep.Variant == AllVariants || dependedOnTask.Variant == dep.Variant) &&
					(dep.Name == AllDependencies || dependedOnTask.Name == dep.Name) {
					dependencies = append(dependencies, task.DependencyEdge{
						Status: dep.Status,
						From:   dependentTask.toTaskNode(),
						To:     dependedOnTask.toTaskNode(),
					})
				}
			}
		}
	}

	return dependencies
}

// FetchVersionsBuildsAndTasks is a helper function to fetch a group of versions and their associated builds and tasks.
// Returns the versions themselves, a map of version id -> the builds that are a part of the version (unsorted)
// and a map of build ID -> each build's tasks
func FetchVersionsBuildsAndTasks(project *Project, skip int, numVersions int, showTriggered bool) ([]Version, map[string][]build.Build, map[string][]task.Task, error) {
	// fetch the versions from the db
	versionsFromDB, err := VersionFind(VersionByProjectAndTrigger(project.Identifier, showTriggered).
		WithFields(
			VersionRevisionKey,
			VersionErrorsKey,
			VersionWarningsKey,
			VersionIgnoredKey,
			VersionMessageKey,
			VersionAuthorKey,
			VersionRevisionOrderNumberKey,
			VersionCreateTimeKey,
			VersionTriggerIDKey,
			VersionTriggerTypeKey,
			VersionGitTagsKey,
		).Sort([]string{"-" + VersionCreateTimeKey}).Skip(skip).Limit(numVersions))

	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "fetching versions from database")
	}

	// create a slice of the version ids (used to fetch the builds)
	versionIds := make([]string, 0, len(versionsFromDB))
	for _, v := range versionsFromDB {
		versionIds = append(versionIds, v.Id)
	}

	// fetch all of the builds (with only relevant fields)
	buildsFromDb, err := build.Find(
		build.ByVersions(versionIds).
			WithFields(
				build.BuildVariantKey,
				bsonutil.GetDottedKeyName(build.TasksKey, build.TaskCacheIdKey),
				build.VersionKey,
				build.DisplayNameKey,
				build.RevisionKey,
			))
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "fetching builds from database")
	}

	// group the builds by version
	buildsByVersion := map[string][]build.Build{}
	for _, build := range buildsFromDb {
		buildsByVersion[build.Version] = append(buildsByVersion[build.Version], build)
	}

	// Filter out execution tasks because they'll be dropped when iterating through the build task cache anyway.
	// maxTime ensures the query won't go on indefinitely when the request is cancelled.
	tasksFromDb, err := task.FindAll(db.Query(task.NonExecutionTasksByVersions(versionIds)).WithFields(task.StatusFields...).MaxTime(waterfallTasksQueryMaxTime))
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "fetching tasks from database")
	}
	taskMap := task.TaskSliceToMap(tasksFromDb)

	tasksByBuild := map[string][]task.Task{}
	for _, b := range buildsFromDb {
		for _, t := range b.Tasks {
			tasksByBuild[b.Id] = append(tasksByBuild[b.Id], taskMap[t.Id])
		}
	}

	return versionsFromDB, buildsByVersion, tasksByBuild, nil
}

func (tg *TaskGroup) InjectInfo(t *task.Task) {
	t.TaskGroup = tg.Name
	t.TaskGroupMaxHosts = tg.MaxHosts

	for idx, n := range tg.Tasks {
		if n == t.DisplayName {
			t.TaskGroupOrder = idx + 1
			break
		}
	}
}

type VariantsAndTasksFromProject struct {
	Variants map[string]BuildVariant
	Tasks    []struct{ Name string }
	Project  Project
}

// GetVariantsAndTasksFromPatchProject formats variants and tasks as used by the UI pages.
func GetVariantsAndTasksFromPatchProject(ctx context.Context, p *patch.Patch) (*VariantsAndTasksFromProject, error) {
	project, _, err := FindAndTranslateProjectForPatch(ctx, p)
	if err != nil {
		return nil, err
	}

	// retrieve tasks and variant mappings' names
	variantMappings := make(map[string]BuildVariant)
	for _, variant := range project.BuildVariants {
		tasksForVariant := []BuildVariantTaskUnit{}
		for _, taskFromVariant := range variant.Tasks {
			// add a task name to the list if it's patchable and not restricted to git tags and not disabled
			if !taskFromVariant.IsDisabled() && utility.FromBoolTPtr(taskFromVariant.Patchable) && !utility.FromBoolPtr(taskFromVariant.GitTagOnly) {
				if taskFromVariant.IsGroup {

					tasksForVariant = append(tasksForVariant, CreateTasksFromGroup(taskFromVariant, project, evergreen.PatchVersionRequester)...)
				} else {
					tasksForVariant = append(tasksForVariant, taskFromVariant)
				}
			}
		}
		if len(tasksForVariant) > 0 {
			variant.Tasks = tasksForVariant
			variantMappings[variant.Name] = variant
		}
	}

	tasksList := []struct{ Name string }{}
	for _, task := range project.Tasks {
		// add a task name to the list if it's patchable and not restricted to git tags and not disabled
		if !utility.FromBoolPtr(task.Disable) && utility.FromBoolTPtr(task.Patchable) && !utility.FromBoolPtr(task.GitTagOnly) {
			tasksList = append(tasksList, struct{ Name string }{task.Name})
		}
	}

	variantsAndTasksFromProject := VariantsAndTasksFromProject{
		Variants: variantMappings,
		Tasks:    tasksList,
		Project:  *project,
	}
	return &variantsAndTasksFromProject, nil
}
