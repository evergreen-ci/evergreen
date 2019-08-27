package model

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	ignore "github.com/sabhiram/go-git-ignore"
	yaml "gopkg.in/yaml.v2"
)

const (
	// DefaultCommandType is a system configuration option that is used to
	// differentiate between setup related commands and actual testing commands.
	DefaultCommandType = evergreen.CommandTypeTest
)

type Project struct {
	Enabled           bool                       `yaml:"enabled,omitempty" bson:"enabled"`
	Stepback          bool                       `yaml:"stepback,omitempty" bson:"stepback"`
	PreErrorFailsTask bool                       `yaml:"pre_error_fails_task,omitempty" bson:"pre_error_fails_task,omitempty"`
	BatchTime         int                        `yaml:"batchtime,omitempty" bson:"batch_time"`
	Owner             string                     `yaml:"owner,omitempty" bson:"owner_name"`
	Repo              string                     `yaml:"repo,omitempty" bson:"repo_name"`
	RemotePath        string                     `yaml:"remote_path,omitempty" bson:"remote_path"`
	RepoKind          string                     `yaml:"repokind,omitempty" bson:"repo_kind"`
	Branch            string                     `yaml:"branch,omitempty" bson:"branch_name"`
	Identifier        string                     `yaml:"identifier,omitempty" bson:"identifier"`
	DisplayName       string                     `yaml:"display_name,omitempty" bson:"display_name"`
	CommandType       string                     `yaml:"command_type,omitempty" bson:"command_type"`
	Ignore            []string                   `yaml:"ignore,omitempty" bson:"ignore"`
	Pre               *YAMLCommandSet            `yaml:"pre,omitempty" bson:"pre"`
	Post              *YAMLCommandSet            `yaml:"post,omitempty" bson:"post"`
	Timeout           *YAMLCommandSet            `yaml:"timeout,omitempty" bson:"timeout"`
	CallbackTimeout   int                        `yaml:"callback_timeout_secs,omitempty" bson:"callback_timeout_secs"`
	Modules           ModuleList                 `yaml:"modules,omitempty" bson:"modules"`
	BuildVariants     BuildVariants              `yaml:"buildvariants,omitempty" bson:"build_variants"`
	Functions         map[string]*YAMLCommandSet `yaml:"functions,omitempty" bson:"functions"`
	TaskGroups        []TaskGroup                `yaml:"task_groups,omitempty" bson:"task_groups"`
	Tasks             []ProjectTask              `yaml:"tasks,omitempty" bson:"tasks"`
	ExecTimeoutSecs   int                        `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs"`
	Loggers           *LoggerConfig              `yaml:"loggers,omitempty" bson:"loggers,omitempty"`

	// Flag that indicates a project as requiring user authentication
	Private bool `yaml:"private,omitempty" bson:"private"`
}

// Unmarshalled from the "tasks" list in an individual build variant. Can be either a task or task group
type BuildVariantTaskUnit struct {
	// Name has to match the name field of one of the tasks or groups specified at
	// the project level, or an error will be thrown
	Name      string `yaml:"name,omitempty" bson:"name"`
	IsGroup   bool   `yaml:"-" bson:"-"`
	GroupName string `yaml:"-" bson:"-"`

	// fields to overwrite ProjectTask settings.
	Patchable *bool                 `yaml:"patchable,omitempty" bson:"patchable,omitempty"`
	PatchOnly *bool                 `yaml:"patch_only,omitempty" bson:"patch_only,omitempty"`
	Priority  int64                 `yaml:"priority,omitempty" bson:"priority"`
	DependsOn []TaskUnitDependency  `yaml:"depends_on,omitempty" bson:"depends_on"`
	Requires  []TaskUnitRequirement `yaml:"requires,omitempty" bson:"requires"`

	// the distros that the task can be run on
	Distros []string `yaml:"distros,omitempty" bson:"distros"`

	// currently unsupported (TODO EVG-578)
	ExecTimeoutSecs int   `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs"`
	Stepback        *bool `yaml:"stepback,omitempty" bson:"stepback,omitempty"`

	Variant string `yaml:"-" bson:"-"`

	CommitQueueMerge bool `yaml:"commit_queue_merge,omitempty" bson:"commit_queue_merge"`
}

func (b BuildVariant) Get(name string) (BuildVariantTaskUnit, error) {
	for idx := range b.Tasks {
		if b.Tasks[idx].Name == name {
			return b.Tasks[idx], nil
		}
	}

	return BuildVariantTaskUnit{}, errors.Errorf("could not find task %s in build variant %s",
		name, b.Name)
}

type DisplayTask struct {
	Name           string   `yaml:"name,omitempty" bson:"name,omitempty"`
	ExecutionTasks []string `yaml:"execution_tasks,omitempty" bson:"execution_tasks,omitempty"`
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

	return BuildVariant{}, errors.Errorf("could not find build variant named %s", name)
}

// Populate updates the base fields of the BuildVariantTaskUnit with
// fields from the project task definition.
func (bvt *BuildVariantTaskUnit) Populate(pt ProjectTask) {
	// We never update "Name" or "Commands"
	if len(bvt.DependsOn) == 0 {
		bvt.DependsOn = pt.DependsOn
	}
	if len(bvt.Requires) == 0 {
		bvt.Requires = pt.Requires
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

func (bvt *BuildVariantTaskUnit) SkipOnPatchBuild() bool {
	return (bvt.Patchable != nil && !*bvt.Patchable) || bvt.Name == evergreen.PushStage
}

func (bvt *BuildVariantTaskUnit) SkipOnNonPatchBuild() bool {
	return bvt.PatchOnly != nil && *bvt.PatchOnly
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
	DisplayTasks []DisplayTask          `yaml:"display_tasks,omitempty" bson:"display_tasks,omitempty"`
}

type Module struct {
	Name   string `yaml:"name,omitempty" bson:"name"`
	Branch string `yaml:"branch,omitempty" bson:"branch"`
	Repo   string `yaml:"repo,omitempty" bson:"repo"`
	Prefix string `yaml:"prefix,omitempty" bson:"prefix"`
	Ref    string `yaml:"ref,omitempty" bson:"ref"`
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

	// TimeoutSecs indicates the maximum duration the command is allowed to run for.
	TimeoutSecs int `yaml:"timeout_secs,omitempty" bson:"timeout_secs"`

	// Params are used to supply configuration specific information.
	// map[string]interface{} is stored as a JSON string.
	Params map[string]interface{} `yaml:"params,omitempty" bson:"params"`

	// YAML string of Params to store in database
	ParamsYAML string `yaml:"params_yaml,omitempty" bson:"params_yaml"`

	// Vars defines variables that can be used within commands.
	Vars map[string]string `yaml:"vars,omitempty" bson:"vars"`

	Loggers *LoggerConfig `yaml:"loggers,omitempty" bson:"loggers,omitempty"`
}

func (c *PluginCommandConf) resolveParams() error {
	out := map[string]interface{}{}
	if c == nil {
		return nil
	}
	if c.ParamsYAML != "" {
		if err := yaml.Unmarshal([]byte(c.ParamsYAML), &out); err != nil {
			return errors.Wrapf(err, "error unmarshalling params")
		}
		c.Params = out
	}
	return nil
}

func (c *PluginCommandConf) UnmarshalYAML(unmarshal func(interface{}) error) error {
	temp := struct {
		Function    string                 `yaml:"func,omitempty" bson:"func"`
		Type        string                 `yaml:"type,omitempty" bson:"type"`
		DisplayName string                 `yaml:"display_name,omitempty" bson:"display_name"`
		Command     string                 `yaml:"command,omitempty" bson:"command"`
		Variants    []string               `yaml:"variants,omitempty" bson:"variants"`
		TimeoutSecs int                    `yaml:"timeout_secs,omitempty" bson:"timeout_secs"`
		Params      map[string]interface{} `yaml:"params,omitempty" bson:"params"`
		ParamsYAML  string                 `yaml:"params_yaml,omitempty" bson:"params_yaml"`
		Vars        map[string]string      `yaml:"vars,omitempty" bson:"vars"`
		Loggers     *LoggerConfig          `yaml:"loggers,omitempty" bson:"loggers,omitempty"`
	}{}

	if err := unmarshal(&temp); err != nil {
		return errors.Wrap(err, "error unmarshalling into temp structure")
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

// we maintain Params for backwards compatibility, but we read from YAML when available, as the
// given params could be corrupted from the roundtrip
func (c *PluginCommandConf) unmarshalParams() error {
	if c.ParamsYAML != "" {
		out := map[string]interface{}{}
		if err := yaml.Unmarshal([]byte(c.ParamsYAML), &out); err != nil {
			return errors.Wrapf(err, "error unmarshalling params from yaml")
		}
		c.Params = out
		return nil
	}
	if len(c.Params) != 0 {
		bytes, err := yaml.Marshal(c.Params)
		if err != nil {
			return errors.Wrap(err, "error marshalling params into yaml")
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
	SingleCommand *PluginCommandConf  `bson:"single_command"`
	MultiCommand  []PluginCommandConf `bson:"multi_command"`
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
			return nil, errors.Wrap(err, "error resolving params for command set")
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
	Name          string `yaml:"name,omitempty" bson:"name"`
	Variant       string `yaml:"variant,omitempty" bson:"variant,omitempty"`
	Status        string `yaml:"status,omitempty" bson:"status,omitempty"`
	PatchOptional bool   `yaml:"patch_optional,omitempty" bson:"patch_optional,omitempty"`
}

// TaskUnitRequirement represents tasks/groups that must exist along with
// the requirement's holder. This is only used when configuring patches.
type TaskUnitRequirement struct {
	Name    string `yaml:"name,omitempty" bson:"name"`
	Variant string `yaml:"variant,omitempty" bson:"variant,omitempty"`
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
	MaxHosts              int             `yaml:"max_hosts" bson:"max_hosts"`
	SetupGroupFailTask    bool            `yaml:"setup_group_can_fail_task" bson:"setup_group_can_fail_task"`
	SetupGroupTimeoutSecs int             `yaml:"setup_group_timeout_secs" bson:"setup_group_timeout_secs"`
	SetupGroup            *YAMLCommandSet `yaml:"setup_group" bson:"setup_group"`
	TeardownGroup         *YAMLCommandSet `yaml:"teardown_group" bson:"teardown_group"`
	SetupTask             *YAMLCommandSet `yaml:"setup_task" bson:"setup_task"`
	TeardownTask          *YAMLCommandSet `yaml:"teardown_task" bson:"teardown_task"`
	Timeout               *YAMLCommandSet `yaml:"timeout,omitempty" bson:"timeout"`
	Tasks                 []string        `yaml:"tasks" bson:"tasks"`
	Tags                  []string        `yaml:"tags,omitempty" bson:"tags"`
	// ShareProcs causes processes to persist between task group tasks.
	ShareProcs bool `yaml:"share_processes" bson:"share_processes"`
}

// Unmarshalled from the "tasks" list in the project file
type ProjectTask struct {
	Name            string                `yaml:"name,omitempty" bson:"name"`
	Priority        int64                 `yaml:"priority,omitempty" bson:"priority"`
	ExecTimeoutSecs int                   `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs"`
	DependsOn       []TaskUnitDependency  `yaml:"depends_on,omitempty" bson:"depends_on"`
	Requires        []TaskUnitRequirement `yaml:"requires,omitempty" bson:"requires"`
	Commands        []PluginCommandConf   `yaml:"commands,omitempty" bson:"commands"`
	Tags            []string              `yaml:"tags,omitempty" bson:"tags"`
	// Use a *bool so that there are 3 possible states:
	//   1. nil   = not overriding the project setting (default)
	//   2. true  = overriding the project setting with true
	//   3. false = overriding the project setting with false
	Patchable *bool `yaml:"patchable,omitempty" bson:"patchable,omitempty"`
	Stepback  *bool `yaml:"stepback,omitempty" bson:"stepback,omitempty"`
	PatchOnly *bool `yaml:"patch_only,omitempty" bson:"patch_only,omitempty"`
}

type LoggerConfig struct {
	Agent  []LogOpts `yaml:"agent" bson:"agent"`
	System []LogOpts `yaml:"system" bson:"system"`
	Task   []LogOpts `yaml:"task" bson:"task"`
}

type LogOpts struct {
	Type         string `yaml:"type" bson:"type"`
	SplunkServer string `yaml:"splunk_server,omitempty" bson:"splunk_server,omitempty"`
	SplunkToken  string `yaml:"splunk_token,omitempty" bson:"splunk_token,omitempty"`
	LogDirectory string `yaml:"log_directory" bson:"log_directory"`
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
			catcher.New("logkeepr is disallowed for system logs; will use Evergreen logger")
		}
	}
	for _, opts := range c.Task {
		catcher.Wrap(opts.IsValid(), "invalid task logger config")
	}

	return catcher.Resolve()
}

func (o *LogOpts) IsValid() error {
	catcher := grip.NewBasicCatcher()
	if !util.StringSliceContains(ValidLogSenders, o.Type) {
		catcher.Errorf("%s is not a valid log sender", o.Type)
	}
	if o.Type == SplunkLogSender && o.SplunkServer == "" {
		catcher.New("Splunk logger requires a server URL")
	}
	if o.Type == SplunkLogSender && o.SplunkToken == "" {
		catcher.New("Splunk logger requires a token")
	}

	return catcher.Resolve()
}

const (
	EvergreenLogSender = "evergreen"
	FileLogSender      = "file"
	LogkeeperLogSender = "logkeeper"
	SplunkLogSender    = "splunk"
)

var ValidLogSenders = []string{
	EvergreenLogSender,
	FileLogSender,
	LogkeeperLogSender,
	SplunkLogSender,
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

// GetIdsForAllVariants returns all task Ids for taskName on all variants.
func (tt TaskIdTable) GetIdsForAllVariants(taskName string) []string {
	return tt.GetIdsForAllVariantsExcluding(taskName, TVPair{})
}

// GetIdsForAllVariants returns all task Ids for taskName on all variants, excluding
// the specific task denoted by the task/variant pair.
func (tt TaskIdTable) GetIdsForAllVariantsExcluding(taskName string, exclude TVPair) []string {
	ids := []string{}
	for pair := range tt {
		if pair.TaskName == taskName && pair != exclude {
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
	for pair := range tt {
		if !(pair.TaskName == taskName && pair.Variant == currentVariant) {
			if id := tt[pair]; id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// TaskIdTable builds a TaskIdTable for the given version and project
func NewTaskIdTable(p *Project, v *Version, sourceRev, defID string) TaskIdConfig {
	// init the variant map
	execTable := TaskIdTable{}
	displayTable := TaskIdTable{}

	sort.Stable(p.BuildVariants)

	for _, bv := range p.BuildVariants {
		rev := v.Revision
		if evergreen.IsPatchRequester(v.Requester) {
			rev = fmt.Sprintf("patch_%s_%s", v.Revision, v.Id)
		} else if v.Requester == evergreen.TriggerRequester {
			rev = fmt.Sprintf("%s_%s", sourceRev, defID)
		} else if v.Requester == evergreen.AdHocRequester {
			rev = v.Id
		}
		for _, t := range bv.Tasks {
			if tg := p.FindTaskGroup(t.Name); tg != nil {
				for _, groupTask := range tg.Tasks {
					taskId := generateId(groupTask, p, &bv, rev, v)
					execTable[TVPair{bv.Name, groupTask}] = util.CleanName(taskId)
				}
			} else {
				// create a unique Id for each task
				taskId := generateId(t.Name, p, &bv, rev, v)
				execTable[TVPair{bv.Name, t.Name}] = util.CleanName(taskId)
			}
		}

		for _, dt := range bv.DisplayTasks {
			name := fmt.Sprintf("display_%s", dt.Name)
			taskId := generateId(name, p, &bv, rev, v)
			displayTable[TVPair{bv.Name, dt.Name}] = util.CleanName(taskId)
		}
	}
	return TaskIdConfig{ExecutionTasks: execTable, DisplayTasks: displayTable}
}

// NewPatchTaskIdTable constructs a new TaskIdTable (map of [variant, task display name]->[task  id])
func NewPatchTaskIdTable(proj *Project, v *Version, tasks TaskVariantPairs) TaskIdConfig {
	config := TaskIdConfig{}
	processedVariants := map[string]bool{}

	// resolve task groups to exec tasks
	tgMap := map[string]TaskGroup{}
	for _, tg := range proj.TaskGroups {
		tgMap[tg.Name] = tg
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
		config.ExecutionTasks = generateIdsForVariant(vt, proj, v, tasks.ExecTasks, config.ExecutionTasks, tgMap)
	}
	processedVariants = map[string]bool{}
	for _, vt := range tasks.DisplayTasks {
		// don't hit the same variant more than once
		if _, ok := processedVariants[vt.Variant]; ok {
			continue
		}
		processedVariants[vt.Variant] = true
		config.DisplayTasks = generateIdsForVariant(vt, proj, v, tasks.DisplayTasks, config.DisplayTasks, tgMap)
	}
	return config
}

func generateIdsForVariant(vt TVPair, proj *Project, v *Version, tasks TVPairSet, table TaskIdTable, tgMap map[string]TaskGroup) TaskIdTable {
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
		if util.StringSliceContains(taskNamesForVariant, t.Name) {
			table[TVPair{vt.Variant, t.Name}] = util.CleanName(generateId(t.Name, proj, projBV, rev, v))
		} else if tg, ok := tgMap[t.Name]; ok {
			for _, name := range tg.Tasks {
				table[TVPair{vt.Variant, name}] = util.CleanName(generateId(name, proj, projBV, rev, v))
			}
		}
	}
	for _, t := range projBV.DisplayTasks {
		// create Ids for each task that can run on the variant and is requested by the patch.
		if util.StringSliceContains(taskNamesForVariant, t.Name) {
			table[TVPair{vt.Variant, t.Name}] = util.CleanName(generateId(fmt.Sprintf("display_%s", t.Name), proj, projBV, rev, v))
		}
	}

	return table
}

func generateId(name string, proj *Project, projBV *BuildVariant, rev string, v *Version) string {
	return fmt.Sprintf("%s_%s_%s_%s_%s",
		proj.Identifier,
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
	if h == nil {
		return nil, errors.New("host cannot be nil")
	}

	expansions := util.Expansions{}
	expansions.Put("execution", fmt.Sprintf("%v", t.Execution))
	expansions.Put("version_id", t.Version)
	expansions.Put("task_id", t.Id)
	expansions.Put("task_name", t.DisplayName)
	expansions.Put("build_id", t.BuildId)
	expansions.Put("build_variant", t.BuildVariant)
	expansions.Put("revision", t.Revision)
	expansions.Put("project", t.Project)
	expansions.Put(evergreen.GlobalGitHubTokenExpansion, oauthToken)
	expansions.Put("distro_id", h.Distro.Id)

	if t.TriggerID != "" {
		expansions.Put("trigger_event_identifier", t.TriggerID)
		expansions.Put("trigger_event_type", t.TriggerType)
		expansions.Put("trigger_id", t.TriggerEvent)
		var upstreamProjectID string
		if t.TriggerType == ProjectTriggerLevelTask {
			upstreamTask, err := task.FindOneId(t.TriggerID)
			if err != nil {
				return nil, errors.Wrap(err, "error finding task")
			}
			if upstreamTask == nil {
				return nil, errors.New("upstream task not found")
			}
			expansions.Put("trigger_status", upstreamTask.Status)
			expansions.Put("trigger_revision", upstreamTask.Revision)
			upstreamProjectID = upstreamTask.Project
		} else if t.TriggerType == ProjectTriggerLevelBuild {
			upstreamBuild, err := build.FindOneId(t.TriggerID)
			if err != nil {
				return nil, errors.Wrap(err, "error finding build")
			}
			if upstreamBuild == nil {
				return nil, errors.New("upstream build not found")
			}
			expansions.Put("trigger_status", upstreamBuild.Status)
			expansions.Put("trigger_revision", upstreamBuild.Revision)
			upstreamProjectID = upstreamBuild.Project
		}
		upstreamProject, err := FindOneProjectRef(upstreamProjectID)
		if err != nil {
			return nil, errors.Wrap(err, "error finding project")
		}
		if upstreamProject == nil {
			return nil, errors.Errorf("upstream project %s not found", t.Project)
		}
		expansions.Put("trigger_repo_owner", upstreamProject.Owner)
		expansions.Put("trigger_repo_name", upstreamProject.Repo)
		expansions.Put("trigger_branch", upstreamProject.Branch)
	}

	v, err := VersionFindOneId(t.Version)
	if err != nil {
		return nil, errors.Wrap(err, "error finding version")
	}
	expansions.Put("branch_name", v.Branch)
	expansions.Put("author", v.Author)
	expansions.Put("created_at", v.CreateTime.Format(build.IdTimeLayout))

	if evergreen.IsPatchRequester(v.Requester) {
		var p *patch.Patch
		expansions.Put("is_patch", "true")
		expansions.Put("revision_order_id", fmt.Sprintf("%s_%d", v.Author, v.RevisionOrderNumber))
		if v.Requester == evergreen.MergeTestRequester {
			expansions.Put("is_commit_queue", "true")
		}

		p, err = patch.FindOne(patch.ByVersion(t.Version))
		if err != nil {
			return nil, errors.Wrap(err, "error finding patch")
		}

		if v.Requester == evergreen.GithubPRRequester && p != nil {
			expansions.Put("github_pr_number", fmt.Sprintf("%d", p.GithubPatchData.PRNumber))
			expansions.Put("github_org", p.GithubPatchData.BaseOwner)
			expansions.Put("github_repo", p.GithubPatchData.BaseRepo)
			expansions.Put("github_author", p.GithubPatchData.Author)
		}
	} else {
		expansions.Put("revision_order_id", strconv.Itoa(v.RevisionOrderNumber))
	}

	for _, e := range h.Distro.Expansions {
		expansions.Put(e.Key, e.Value)
	}
	proj := &Project{}
	err = LoadProjectInto([]byte(v.Config), t.Project, proj)
	if err != nil {
		return nil, errors.Wrap(err, "error unmarshaling project")
	}
	bv := proj.FindBuildVariant(t.BuildVariant)
	expansions.Update(bv.Expansions)

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

// RunOnVariant returns true if the plugin command should run on variant; returns false otherwise
func (p PluginCommandConf) RunOnVariant(variant string) bool {
	return len(p.Variants) == 0 || util.StringSliceContains(p.Variants, variant)
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
	return nil
}

// GetTaskGroup returns the task group for a given task from its project
func GetTaskGroup(taskGroup string, tc *TaskConfig) (*TaskGroup, error) {
	if tc == nil {
		return nil, errors.New("unable to get task group: TaskConfig is nil")
	}
	if tc.Task == nil {
		return nil, errors.New("unable to get task group: task is nil")
	}
	if tc.Task.Version == "" {
		return nil, errors.New("task has no version")
	}
	if tc.Version == nil {
		return nil, errors.New("version is nil")
	}
	var p Project
	if err := LoadProjectInto([]byte(tc.Version.Config), tc.Task.Project, &p); err != nil {
		return nil, errors.Wrap(err, "error retrieving project for task group")
	}
	if taskGroup == "" {
		// if there is no named task group, fall back to project definitions
		return &TaskGroup{
			SetupTask:          p.Pre,
			TeardownTask:       p.Post,
			Timeout:            p.Timeout,
			SetupGroupFailTask: p.Pre == nil || p.PreErrorFailsTask,
		}, nil
	}
	tg := p.FindTaskGroup(taskGroup)
	if tg == nil {
		return nil, errors.Errorf("couldn't find task group %s", tc.Task.TaskGroup)
	}
	return tg, nil
}

func FindProjectFromVersionID(versionStr string) (*Project, error) {
	ver, err := VersionFindOne(VersionById(versionStr))
	if err != nil {
		return nil, err
	}
	if ver == nil {
		return nil, errors.Errorf("nil version returned for version '%s'", versionStr)
	}

	project := &Project{}
	err = LoadProjectInto([]byte(ver.Config), ver.Identifier, project)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to load project config for version %s", versionStr)
	}
	return project, nil
}

func (p *Project) FindDistroNameForTask(t *task.Task) (string, error) {
	bv, err := p.BuildVariants.Get(t.BuildVariant)
	if err != nil {
		return "", errors.Wrapf(err, "problem finding buildvariant for task '%s'", t.Id)
	}

	bvt, err := bv.Get(t.DisplayName)
	if err != nil {
		return "", errors.Wrapf(err, "problem finding buildvarianttask for task '%s'", t.Id)
	}

	var distro string

	if len(bvt.Distros) > 0 {
		distro = bvt.Distros[0]
	} else if len(bv.RunOn) > 0 {
		distro = bv.RunOn[0]
	} else {
		return "", errors.Errorf("cannot find the distro for %s", t.Id)
	}

	return distro, nil
}

func FindLastKnownGoodProject(identifier string) (*Project, error) {
	if identifier == "" {
		return nil, errors.WithStack(errors.New("cannot pass empty identifier to FindLastKnownGoodProject"))
	}
	project := &Project{
		Identifier: identifier,
	}

	lastGoodVersion, err := VersionFindOne(VersionByLastKnownGoodConfig(identifier))
	if err != nil {
		return nil, errors.Wrapf(err, "Error finding recent valid version for '%s'", identifier)
	}
	if lastGoodVersion != nil {
		err = LoadProjectInto([]byte(lastGoodVersion.Config), identifier, project)
		if err != nil {
			return nil, errors.Wrapf(err, "Error loading project from "+
				"last good version for project, %s", lastGoodVersion.Identifier)
		}
	}
	return project, nil
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

	for _, bvt := range bv.Tasks {
		if bvt.Name == task {
			if projectTask := p.FindProjectTask(task); projectTask != nil {
				bvt.Populate(*projectTask)
				return &bvt
			}
		}
		if tg, ok := tgMap[bvt.Name]; ok {
			for _, t := range tg.Tasks {
				if t == task {
					bvt.Populate(*p.FindProjectTask(task))
					return &bvt
				}
			}
		}
	}
	return nil
}

func (bv *BuildVariant) GetDisplayTaskNamek(execTask string) string {
	for _, dt := range bv.DisplayTasks {
		for _, et := range dt.ExecutionTasks {
			if et == execTask {
				return dt.Name
			}
		}
	}
	return ""
}

func (p *Project) FindBuildVariant(build string) *BuildVariant {
	for _, b := range p.BuildVariants {
		if b.Name == build {
			return &b
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
	return nil, errors.New("No such module on this project.")
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

// FindAllBuildVariantTasks returns every BuildVariantTaskUnit, fully populated,
// for all variants of a project.
func (p *Project) FindAllBuildVariantTasks() []BuildVariantTaskUnit {
	tasksByName := map[string]ProjectTask{}
	taskGroups := map[string]TaskGroup{}
	for _, t := range p.Tasks {
		tasksByName[t.Name] = t
	}
	for _, tg := range p.TaskGroups {
		taskGroups[tg.Name] = tg
	}
	allBVTs := []BuildVariantTaskUnit{}
	for _, b := range p.BuildVariants {
		for _, t := range b.Tasks {
			tasksToPopulate := []ProjectTask{}
			if t.IsGroup {
				group, exists := taskGroups[t.Name]
				if !exists {
					continue
				}
				for _, groupTask := range group.Tasks {
					tasksToPopulate = append(tasksToPopulate, tasksByName[groupTask])
				}
			} else {
				tasksToPopulate = append(tasksToPopulate, tasksByName[t.Name])
			}
			for _, pTask := range tasksToPopulate {
				t.Populate(pTask)
				t.Variant = b.Name
				allBVTs = append(allBVTs, t)
			}
		}
	}
	return allBVTs
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
	ignorer, _ := ignore.CompileIgnoreLines(p.Ignore...)
	for _, f := range files {
		if !ignorer.MatchesPath(f) {
			return false
		}
	}
	return true
}

func (p *Project) BuildProjectTVPairs(patchDoc *patch.Patch, alias string) {
	//expand tasks and build variants and include dependencies
	if len(patchDoc.BuildVariants) == 1 && patchDoc.BuildVariants[0] == "all" {
		patchDoc.BuildVariants = []string{}
		for _, buildVariant := range p.BuildVariants {
			if buildVariant.Disabled {
				continue
			}
			patchDoc.BuildVariants = append(patchDoc.BuildVariants, buildVariant.Name)
		}
	}

	if len(patchDoc.Tasks) == 1 && patchDoc.Tasks[0] == "all" {
		patchDoc.Tasks = []string{}
		for _, t := range p.Tasks {
			if t.Patchable != nil && !(*t.Patchable) {
				continue
			}
			patchDoc.Tasks = append(patchDoc.Tasks, t.Name)
		}
	}

	var pairs []TVPair
	for _, v := range patchDoc.BuildVariants {
		for _, t := range patchDoc.Tasks {
			if p.FindTaskForVariant(t, v) != nil {
				pairs = append(pairs, TVPair{v, t})
			}
		}
	}

	if alias != "" {
		aliasPairs, displayTaskPairs, err := p.BuildProjectTVPairsWithAlias(alias)
		if err != nil {
			grip.Error(errors.Wrap(err, "failed to get task/variant pairs for alias"))
		} else {
			pairs = append(pairs, aliasPairs...)
			for _, pair := range displayTaskPairs {
				if !util.StringSliceContains(patchDoc.BuildVariants, pair.Variant) {
					patchDoc.BuildVariants = append(patchDoc.BuildVariants, pair.Variant)
				}
				if !util.StringSliceContains(patchDoc.Tasks, pair.TaskName) {
					patchDoc.Tasks = append(patchDoc.Tasks, pair.TaskName)
				}
			}
		}
	}

	tasks := extractDisplayTasks(pairs, patchDoc.Tasks, patchDoc.BuildVariants, p)

	// update variant and tasks to include dependencies
	tasks.ExecTasks = IncludePatchDependencies(p, tasks.ExecTasks)

	patchDoc.SyncVariantsTasks(tasks.TVPairsToVariantTasks())
}

// TasksThatCallCommand returns a map of tasks that call a given command.
func (p *Project) TasksThatCallCommand(find string) map[string]int {
	// get all functions that call `generate.tasks`
	fs := map[string]int{}
	for f, cmds := range p.Functions {
		for _, c := range cmds.List() {
			if c.Command == find {
				fs[f] = fs[f] + 1
			}
		}
	}

	// get all tasks that call `generate.tasks`
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

func extractDisplayTasks(pairs []TVPair, tasks []string, variants []string, p *Project) TaskVariantPairs {
	displayTasks := []TVPair{}
	alreadyAdded := map[string]bool{}
	for _, bv := range p.BuildVariants {
		if !util.StringSliceContains(variants, bv.Name) {
			continue
		}
		for _, taskName := range tasks {
			dt := bv.GetDisplayTaskNamek(taskName)
			if dt != "" && !alreadyAdded[dt] {
				alreadyAdded[dt] = true
				tasks = append(tasks, dt)
			}
		}
		for _, dt := range bv.DisplayTasks {
			if util.StringSliceContains(tasks, dt.Name) {
				displayTasks = append(displayTasks, TVPair{Variant: bv.Name, TaskName: dt.Name})
				for _, et := range dt.ExecutionTasks {
					pairs = append(pairs, TVPair{Variant: bv.Name, TaskName: et})
				}
			}
		}
	}

	return TaskVariantPairs{ExecTasks: pairs, DisplayTasks: displayTasks}
}

// BuildProjectTVPairsWithAlias returns variants and tasks for a project alias.
func (p *Project) BuildProjectTVPairsWithAlias(alias string) ([]TVPair, []TVPair, error) {
	vars, err := FindAliasInProject(p.Identifier, alias)
	if err != nil || vars == nil {
		return nil, nil, err
	}

	pairs := []TVPair{}
	displayTaskPairs := []TVPair{}
	for _, v := range vars {
		var variantRegex *regexp.Regexp
		variantRegex, err = regexp.Compile(v.Variant)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "Error compiling regex: %s", v.Variant)
		}

		var taskRegex *regexp.Regexp
		taskRegex, err = regexp.Compile(v.Task)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "Error compiling regex: %s", v.Task)
		}

		for _, variant := range p.BuildVariants {
			if isValidRegexOrTag(variant.Name, v.Variant, variant.Tags, v.VariantTags, variantRegex) {
				for _, task := range p.Tasks {
					if task.Patchable != nil && !(*task.Patchable) {
						continue
					}
					if !isValidRegexOrTag(task.Name, v.Task, task.Tags, v.TaskTags, taskRegex) {
						continue
					}

					if p.FindTaskForVariant(task.Name, variant.Name) != nil {
						pairs = append(pairs, TVPair{variant.Name, task.Name})
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

	return pairs, displayTaskPairs, err
}

// FetchVersionsAndAssociatedBuilds is a helper function to fetch a group of versions and their associated builds.
// Returns the versions themselves, as well as a map of version id -> the
// builds that are a part of the version (unsorted).
func FetchVersionsAndAssociatedBuilds(project *Project, skip int, numVersions int, showTriggered bool) ([]Version, map[string][]build.Build, error) {

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
		).Sort([]string{"-" + VersionCreateTimeKey}).Skip(skip).Limit(numVersions))

	if err != nil {
		return nil, nil, errors.Wrap(err, "error fetching versions from database")
	}

	// create a slice of the version ids (used to fetch the builds)
	versionIds := make([]string, 0, len(versionsFromDB))
	for _, v := range versionsFromDB {
		versionIds = append(versionIds, v.Id)
	}

	// fetch all of the builds (with only relevant fields)
	buildsFromDb, err := build.Find(
		build.ByVersions(versionIds).
			WithFields(build.BuildVariantKey, build.TasksKey, build.VersionKey))
	if err != nil {
		return nil, nil, errors.Wrap(err, "error fetching builds from database")
	}

	// group the builds by version
	buildsByVersion := map[string][]build.Build{}
	for _, build := range buildsFromDb {
		buildsByVersion[build.Version] = append(buildsByVersion[build.Version], build)
	}

	return versionsFromDB, buildsByVersion, nil
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
