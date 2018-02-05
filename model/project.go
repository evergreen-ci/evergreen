package model

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	ignore "github.com/sabhiram/go-git-ignore"
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
	Enabled         bool                       `yaml:"enabled,omitempty" bson:"enabled"`
	Stepback        bool                       `yaml:"stepback,omitempty" bson:"stepback"`
	BatchTime       int                        `yaml:"batchtime,omitempty" bson:"batch_time"`
	Owner           string                     `yaml:"owner,omitempty" bson:"owner_name"`
	Repo            string                     `yaml:"repo,omitempty" bson:"repo_name"`
	RemotePath      string                     `yaml:"remote_path,omitempty" bson:"remote_path"`
	RepoKind        string                     `yaml:"repokind,omitempty" bson:"repo_kind"`
	Branch          string                     `yaml:"branch,omitempty" bson:"branch_name"`
	Identifier      string                     `yaml:"identifier,omitempty" bson:"identifier"`
	DisplayName     string                     `yaml:"display_name,omitempty" bson:"display_name"`
	CommandType     string                     `yaml:"command_type,omitempty" bson:"command_type"`
	Ignore          []string                   `yaml:"ignore,omitempty" bson:"ignore"`
	Pre             *YAMLCommandSet            `yaml:"pre,omitempty" bson:"pre"`
	Post            *YAMLCommandSet            `yaml:"post,omitempty" bson:"post"`
	Timeout         *YAMLCommandSet            `yaml:"timeout,omitempty" bson:"timeout"`
	CallbackTimeout int                        `yaml:"callback_timeout_secs,omitempty" bson:"callback_timeout_secs"`
	Modules         []Module                   `yaml:"modules,omitempty" bson:"modules"`
	BuildVariants   []BuildVariant             `yaml:"buildvariants,omitempty" bson:"build_variants"`
	Functions       map[string]*YAMLCommandSet `yaml:"functions,omitempty" bson:"functions"`
	TaskGroups      []TaskGroup                `yaml:"task_groups,omitempty" bson:"task_groups"`
	Tasks           []ProjectTask              `yaml:"tasks,omitempty" bson:"tasks"`
	ExecTimeoutSecs int                        `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs"`

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
	Priority  int64                 `yaml:"priority,omitempty" bson:"priority"`
	DependsOn []TaskUnitDependency  `yaml:"depends_on,omitempty" bson:"depends_on"`
	Requires  []TaskUnitRequirement `yaml:"requires,omitempty" bson:"requires"`

	// the distros that the task can be run on
	Distros []string `yaml:"distros,omitempty" bson:"distros"`

	// currently unsupported (TODO EVG-578)
	ExecTimeoutSecs int   `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs"`
	Stepback        *bool `yaml:"stepback,omitempty" bson:"stepback,omitempty"`
}

type DisplayTask struct {
	Name           string   `yaml:"name,omitempty" bson:"name,omitempty"`
	ExecutionTasks []string `yaml:"execution_tasks,omitempty" bson:"execution_tasks,omitempty"`
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

	// Params are used to supply configuratiion specific information.
	Params map[string]interface{} `yaml:"params,omitempty" bson:"params"`

	// Vars defines variables that can be used within commands.
	Vars map[string]string `yaml:"vars,omitempty" bson:"vars"`
}

type ArtifactInstructions struct {
	Include      []string `yaml:"include,omitempty" bson:"include"`
	ExcludeFiles []string `yaml:"excludefiles,omitempty" bson:"exclude_files"`
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

	// fields that will be copied to all tasks
	Priority  int64                 `yaml:"priority,omitempty" bson:"priority"`
	Patchable *bool                 `yaml:"patchable,omitempty" bson:"patchable,omitempty"`
	DependsOn []TaskUnitDependency  `yaml:"depends_on,omitempty" bson:"depends_on"`
	Requires  []TaskUnitRequirement `yaml:"requires,omitempty" bson:"requires"`
	// currently unsupported (TODO EVG-578)
	ExecTimeoutSecs int   `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs"`
	Stepback        *bool `yaml:"stepback,omitempty" bson:"stepback,omitempty"`

	// data about the task group
	MaxHosts      int             `yaml:"max_hosts" bson:"max_hosts"`
	SetupGroup    *YAMLCommandSet `yaml:"setup_group" bson:"setup_group"`
	TeardownGroup *YAMLCommandSet `yaml:"teardown_group" bson:"teardown_group"`
	SetupTask     *YAMLCommandSet `yaml:"setup_task" bson:"setup_task"`
	TeardownTask  *YAMLCommandSet `yaml:"teardown_task" bson:"teardown_task"`
	Timeout       *YAMLCommandSet `yaml:"timeout,omitempty" bson:"timeout"`
	Tasks         []string        `yaml:"tasks" bson:"tasks"`
	Tags          []string        `yaml:"tags,omitempty" bson:"tags"`
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
func NewTaskIdTable(p *Project, v *version.Version) TaskIdConfig {
	// init the variant map
	execTable := TaskIdTable{}
	displayTable := TaskIdTable{}
	for _, bv := range p.BuildVariants {
		rev := v.Revision
		if evergreen.IsPatchRequester(v.Requester) {
			rev = fmt.Sprintf("patch_%s_%s", v.Revision, v.Id)
		}
		for _, t := range bv.Tasks {
			if t.IsGroup {
				tg := p.FindTaskGroup(t.Name)
				if tg == nil {
					continue
				}
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
func NewPatchTaskIdTable(proj *Project, v *version.Version, tasks TaskVariantPairs) TaskIdConfig {
	config := TaskIdConfig{}
	processedVariants := map[string]bool{}
	for _, vt := range tasks.ExecTasks {
		// don't hit the same variant more than once
		if _, ok := processedVariants[vt.Variant]; ok {
			continue
		}
		processedVariants[vt.Variant] = true
		config.ExecutionTasks = generateIdsForVariant(vt, proj, v, tasks.ExecTasks, config.ExecutionTasks)
	}
	processedVariants = map[string]bool{}
	for _, vt := range tasks.DisplayTasks {
		// don't hit the same variant more than once
		if _, ok := processedVariants[vt.Variant]; ok {
			continue
		}
		processedVariants[vt.Variant] = true
		config.DisplayTasks = generateIdsForVariant(vt, proj, v, tasks.DisplayTasks, config.DisplayTasks)
	}
	return config
}

func generateIdsForVariant(vt TVPair, proj *Project, v *version.Version, tasks TVPairSet, table TaskIdTable) TaskIdTable {
	if table == nil {
		table = map[TVPair]string{}
	}

	// we must track the project's variants definitions as well,
	// so that we don't create Ids for variants that don't exist.
	projBV := proj.FindBuildVariant(vt.Variant)
	taskNamesForVariant := tasks.TaskNames(vt.Variant)

	rev := v.Revision
	if v.Requester == evergreen.PatchVersionRequester {
		rev = fmt.Sprintf("patch_%s_%s", v.Revision, v.Id)
	}
	for _, t := range projBV.Tasks {
		// create Ids for each task that can run on the variant and is requested by the patch.
		if util.StringSliceContains(taskNamesForVariant, t.Name) {
			table[TVPair{vt.Variant, t.Name}] = util.CleanName(generateId(t.Name, proj, projBV, rev, v))
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

func generateId(name string, proj *Project, projBV *BuildVariant, rev string, v *version.Version) string {
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

func populateExpansions(d *distro.Distro, v *version.Version, bv *BuildVariant, t *task.Task, p *patch.Patch) *util.Expansions {
	expansions := util.NewExpansions(map[string]string{})
	expansions.Put("execution", fmt.Sprintf("%v", t.Execution))
	expansions.Put("version_id", t.Version)
	expansions.Put("task_id", t.Id)
	expansions.Put("task_name", t.DisplayName)
	expansions.Put("build_id", t.BuildId)
	expansions.Put("build_variant", t.BuildVariant)
	expansions.Put("workdir", d.WorkDir)
	expansions.Put("revision", t.Revision)
	expansions.Put("project", t.Project)
	expansions.Put("branch_name", v.Branch)
	expansions.Put("author", v.Author)
	expansions.Put("distro_id", d.Id)
	expansions.Put("created_at", v.CreateTime.Format(build.IdTimeLayout))

	if evergreen.IsPatchRequester(v.Requester) {
		expansions.Put("is_patch", "true")
		expansions.Put("revision_order_id", fmt.Sprintf("%s_%d", v.Author, v.RevisionOrderNumber))

		if v.Requester == evergreen.GithubPRRequester && p != nil {
			expansions.Put("github_pr_number", fmt.Sprintf("%d", p.GithubPatchData.PRNumber))
			expansions.Put("github_org", p.GithubPatchData.BaseOwner)
			expansions.Put("github_repo", p.GithubPatchData.BaseRepo)
			expansions.Put("github_author", p.GithubPatchData.Author)
		}

	} else {
		expansions.Put("revision_order_id", strconv.Itoa(v.RevisionOrderNumber))
	}

	for _, e := range d.Expansions {
		expansions.Put(e.Key, e.Value)
	}
	expansions.Update(bv.Expansions)
	return expansions
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
func GetTaskGroup(tc *TaskConfig) (*TaskGroup, error) {
	if tc.Task == nil {
		return nil, errors.New("unable to getc.Task task group: task is nil")
	}
	if tc.Task.Version == "" {
		return nil, errors.New("task has no version")
	}
	if tc.Version == nil {
		return nil, errors.New("version is nil")
	}
	var p Project
	if err := LoadProjectInto([]byte(tc.Version.Config), tc.Task.Project, &p); err != nil {
		return nil, errors.Wrap(err, "error retrieving projectc.Task for task group")
	}
	if tc.Task.TaskGroup == "" {
		// if there is no named task group, fall back to projectc.Task definitions
		return &TaskGroup{
			SetupTask:    p.Pre,
			TeardownTask: p.Post,
			Timeout:      p.Timeout,
		}, nil
	}
	tg := p.FindTaskGroup(tc.Task.TaskGroup)
	if tg != nil {
		return tg, nil
	}
	return nil, errors.Errorf("couldn'tc.Task find task group %s", tc.Task.TaskGroup)
}

func FindProject(revision string, projectRef *ProjectRef) (*Project, error) {
	if projectRef == nil {
		return nil, errors.New("projectRef given is nil")
	}
	if projectRef.Identifier == "" {
		return nil, errors.New("Invalid project with blank identifier")
	}

	project := &Project{}
	project.Identifier = projectRef.Identifier
	// when the revision is empty we find the last known good configuration from the versions
	// If the last known good configuration does not exist,
	// load the configuration from the local config in the project ref.
	if revision == "" {
		lastGoodVersion, err := version.FindOne(version.ByLastKnownGoodConfig(projectRef.Identifier))
		if err != nil {
			return nil, errors.Wrapf(err, "Error finding recent valid version for %v: %v", projectRef.Identifier)
		}
		if lastGoodVersion != nil {
			// for new repositories, we don't want to error out when we don't have
			// any versions stored in the database so we default to the skeletal
			// information we already have from the project file on disk
			err = LoadProjectInto([]byte(lastGoodVersion.Config), projectRef.Identifier, project)
			if err != nil {
				return nil, errors.Wrapf(err, "Error loading project from "+
					"last good version for project, %v", lastGoodVersion.Identifier)
			}
		} else {
			// Check to see if there is a local configuration in the project ref
			if projectRef.LocalConfig != "" {
				err = LoadProjectInto([]byte(projectRef.LocalConfig), projectRef.Identifier, project)
				if err != nil {
					return nil, errors.Wrapf(err, "Error loading local config for project ref, %v", projectRef.Identifier)
				}
			}
		}
	}

	if revision != "" {
		// we immediately return an error if the repotracker version isn't found
		// for the given project at the given revision
		v, err := version.FindOne(version.ByProjectIdAndRevision(projectRef.Identifier, revision))
		if err != nil {
			return nil, errors.Wrapf(err, "error fetching version for project %v revision %v", projectRef.Identifier, revision)
		}
		if v == nil {
			// fall back to the skeletal project
			return project, nil
		}

		project = &Project{}
		if err = LoadProjectInto([]byte(v.Config), projectRef.Identifier, project); err != nil {
			return nil, errors.Wrap(err, "Error loading project from version")
		}
	}
	return project, nil
}

func (p *Project) FindTaskForVariant(task, variant string) *BuildVariantTaskUnit {
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
	tasksByName := map[string]*ProjectTask{}
	for i, t := range p.Tasks {
		tasksByName[t.Name] = &p.Tasks[i]
	}
	allBVTs := []BuildVariantTaskUnit{}
	for _, b := range p.BuildVariants {
		for _, t := range b.Tasks {
			if pTask := tasksByName[t.Name]; pTask != nil {
				t.Populate(*pTask)
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
		aliasPairs, err := p.BuildProjectTVPairsWithAlias(alias)
		if err != nil {
			grip.Error(errors.Wrap(err, "failed to get task/variant pairs for alias"))
		} else {
			pairs = append(pairs, aliasPairs...)
		}
	}

	tasks := extractDisplayTasks(pairs, patchDoc.Tasks, patchDoc.BuildVariants, p)

	// update variant and tasks to include dependencies
	tasks.ExecTasks = IncludePatchDependencies(p, tasks.ExecTasks)

	patchDoc.SyncVariantsTasks(tasks.TVPairsToVariantTasks())
}

func extractDisplayTasks(pairs []TVPair, tasks []string, variants []string, p *Project) TaskVariantPairs {
	displayTasks := []TVPair{}
	for _, bv := range p.BuildVariants {
		if !util.StringSliceContains(variants, bv.Name) {
			continue
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
func (p *Project) BuildProjectTVPairsWithAlias(alias string) ([]TVPair, error) {
	var pairs []TVPair
	var err error
	var vars []PatchDefinition
	vars, err = FindOneProjectAlias(p.Identifier, alias)
	if err != nil || vars == nil {
		return pairs, err
	}
	for _, v := range vars {
		var variantRegex *regexp.Regexp
		variantRegex, err = regexp.Compile(v.Variant)
		if err != nil {
			return pairs, errors.Wrapf(err, "Error compiling regex: %s", v.Variant)
		}
		var taskRegex *regexp.Regexp
		taskRegex, err = regexp.Compile(v.Task)
		if err != nil {
			return nil, errors.Wrapf(err, "Error compiling regex: %s", v.Task)
		}

		for _, variant := range p.BuildVariants {
			if variantRegex.MatchString(variant.Name) {
				for _, task := range p.Tasks {
					if ((v.Task != "" && taskRegex.MatchString(task.Name)) ||
						(len(v.Tags) > 0 && len(util.StringSliceIntersection(task.Tags, v.Tags)) > 0)) &&
						(p.FindTaskForVariant(task.Name, variant.Name) != nil) {
						pairs = append(pairs, TVPair{variant.Name, task.Name})
					}
				}
			}
		}
	}
	return pairs, err
}
