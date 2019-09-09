package model

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

const LoadProjectError = "load project error(s)"

// This file contains the infrastructure for turning a YAML project configuration
// into a usable Project struct. A basic overview of the project parsing process is:
//
// First, the YAML bytes are unmarshalled into an intermediary parserProject.
// The parserProject's internal types define custom YAML unmarshal hooks, allowing
// users to do things like offer a single definition where we expect a list, e.g.
//   `tags: "single_tag"` instead of the more verbose `tags: ["single_tag"]`
// or refer to task by a single selector. Custom YAML handling allows us to
// add other useful features like detecting fatal errors and reporting them
// through the YAML parser's error code, which supplies helpful line number information
// that we would lose during validation against already-parsed data. In the future,
// custom YAML hooks will allow us to add even more helpful features, like alerting users
// when they use fields that aren't actually defined.
//
// Once the intermediary project is created, we crawl it to evaluate tag selectors
// and  matrix definitions. This step recursively crawls variants, tasks, their
// dependencies, and so on, to replace selectors with the tasks they reference and return
// a populated Project type.
//
// Code outside of this file should never have to consider selectors or parser* types
// when handling project code.

// parserProject serves as an intermediary struct for parsing project
// configuration YAML. It implements the Unmarshaler interface
// to allow for flexible handling.
type parserProject struct {
	Enabled           bool                       `yaml:"enabled,omitempty" bson:"enabled,omitempty"`
	Stepback          bool                       `yaml:"stepback,omitempty" bson:"stepback,omitempty"`
	PreErrorFailsTask bool                       `yaml:"pre_error_fails_task,omitempty" bson:"pre_error_fails_task,omitempty"`
	BatchTime         int                        `yaml:"batchtime,omitempty" bson:"batchtime,omitempty"`
	Owner             string                     `yaml:"owner,omitempty" bson:"owner,omitempty"`
	Repo              string                     `yaml:"repo,omitempty" bson:"repo,omitempty"`
	RemotePath        string                     `yaml:"remote_path,omitempty" bson:"remote_path,omitempty"`
	RepoKind          string                     `yaml:"repokind,omitempty" bson:"repokind,omitempty"`
	Branch            string                     `yaml:"branch,omitempty" bson:"branch,omitempty"`
	Identifier        string                     `yaml:"identifier,omitempty" bson:"identifier,omitempty"`
	DisplayName       string                     `yaml:"display_name,omitempty" bson:"display_name,omitempty"`
	CommandType       string                     `yaml:"command_type,omitempty" bson:"command_type,omitempty"`
	Ignore            parserStringSlice          `yaml:"ignore,omitempty" bson:"ignore,omitempty"`
	Pre               *YAMLCommandSet            `yaml:"pre,omitempty" bson:"pre,omitempty"`
	Post              *YAMLCommandSet            `yaml:"post,omitempty" bson:"post,omitempty"`
	Timeout           *YAMLCommandSet            `yaml:"timeout,omitempty" bson:"timeout,omitempty"`
	CallbackTimeout   int                        `yaml:"callback_timeout_secs,omitempty" bson:"callback_timeout_secs,omitempty"`
	Modules           []Module                   `yaml:"modules,omitempty" bson:"modules,omitempty"`
	BuildVariants     []parserBV                 `yaml:"buildvariants,omitempty" bson:"buildvariants,omitempty"`
	Functions         map[string]*YAMLCommandSet `yaml:"functions,omitempty" bson:"functions,omitempty"`
	TaskGroups        []parserTaskGroup          `yaml:"task_groups,omitempty" bson:"task_groups,omitempty"`
	Tasks             []parserTask               `yaml:"tasks,omitempty" bson:"tasks,omitempty"`
	ExecTimeoutSecs   int                        `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs,omitempty"`
	Loggers           *LoggerConfig              `yaml:"loggers,omitempty" bson:"loggers:omitempty"`

	// Matrix code
	Axes []matrixAxis `yaml:"axes,omitempty" bson:"axes,omitempty"`
}

type parserTaskGroup struct {
	Name                  string             `yaml:"name,omitempty"`
	Priority              int64              `yaml:"priority,omitempty"`
	Patchable             *bool              `yaml:"patchable,omitempty"`
	PatchOnly             *bool              `yaml:"patch_only,omitempty"`
	ExecTimeoutSecs       int                `yaml:"exec_timeout_secs,omitempty"`
	Stepback              *bool              `yaml:"stepback,omitempty"`
	MaxHosts              int                `yaml:"max_hosts,omitempty"`
	SetupGroupFailTask    bool               `yaml:"setup_group_can_fail_task,omitempty"`
	SetupGroupTimeoutSecs int                `yaml:"setup_group_timeout_secs,omitempty"`
	SetupGroup            *YAMLCommandSet    `yaml:"setup_group,omitempty"`
	TeardownGroup         *YAMLCommandSet    `yaml:"teardown_group,omitempty"`
	SetupTask             *YAMLCommandSet    `yaml:"setup_task,omitempty"`
	TeardownTask          *YAMLCommandSet    `yaml:"teardown_task,omitempty"`
	Timeout               *YAMLCommandSet    `yaml:"timeout,omitempty"`
	Tasks                 []string           `yaml:"tasks,omitempty"`
	DependsOn             parserDependencies `yaml:"depends_on,omitempty"`
	Requires              taskSelectors      `yaml:"requires,omitempty"`
	Tags                  parserStringSlice  `yaml:"tags,omitempty"`
	ShareProcs            bool               `yaml:"share_processes,omitempty"`
}

func (ptg *parserTaskGroup) name() string   { return ptg.Name }
func (ptg *parserTaskGroup) tags() []string { return ptg.Tags }

// parserTask represents an intermediary state of task definitions.
type parserTask struct {
	Name            string              `yaml:"name,omitempty"`
	Priority        int64               `yaml:"priority,omitempty"`
	ExecTimeoutSecs int                 `yaml:"exec_timeout_secs,omitempty"`
	DependsOn       parserDependencies  `yaml:"depends_on,omitempty"`
	Requires        taskSelectors       `yaml:"requires,omitempty"`
	Commands        []PluginCommandConf `yaml:"commands,omitempty"`
	Tags            parserStringSlice   `yaml:"tags,omitempty"`
	Patchable       *bool               `yaml:"patchable,omitempty"`
	PatchOnly       *bool               `yaml:"patch_only,omitempty"`
	Stepback        *bool               `yaml:"stepback,omitempty"`
}

func (pp *parserProject) MarshalYAML() (interface{}, error) {
	for i, pt := range pp.Tasks {
		for j := range pt.Commands {
			if err := pp.Tasks[i].Commands[j].resolveParams(); err != nil {
				return nil, errors.Wrapf(err, "error marshalling commands for task")
			}
		}
	}

	return pp, nil
}

type displayTask struct {
	Name           string   `yaml:"name,omitempty"`
	ExecutionTasks []string `yaml:"execution_tasks,omitempty"`
}

// helper methods for task tag evaluations
func (pt *parserTask) name() string   { return pt.Name }
func (pt *parserTask) tags() []string { return pt.Tags }

// parserDependency represents the intermediary state for referencing dependencies.
type parserDependency struct {
	TaskSelector  taskSelector `yaml:",inline"`
	Status        string       `yaml:"status,omitempty"`
	PatchOptional bool         `yaml:"patch_optional,omitempty"`
}

// parserDependencies is a type defined for unmarshalling both a single
// dependency or multiple dependencies into a slice.
type parserDependencies []parserDependency

// UnmarshalYAML reads YAML into an array of parserDependency. It will
// successfully unmarshal arrays of dependency entries or single dependency entry.
func (pds *parserDependencies) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first check if we are handling a single dep that is not in an array.
	pd := parserDependency{}
	if err := unmarshal(&pd); err == nil {
		*pds = parserDependencies([]parserDependency{pd})
		return nil
	}
	var slice []parserDependency
	if err := unmarshal(&slice); err != nil {
		return err
	}
	*pds = parserDependencies(slice)
	return nil
}

// UnmarshalYAML reads YAML into a parserDependency. A single selector string
// will be also be accepted.
func (pd *parserDependency) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if err := unmarshal(&pd.TaskSelector); err != nil {
		return err
	}
	otherFields := struct {
		Status        string `yaml:"status"`
		PatchOptional bool   `yaml:"patch_optional"`
	}{}
	// ignore any errors here; if we're using a single-string selector, this is expected to fail
	grip.Debug(unmarshal(&otherFields))
	pd.Status = otherFields.Status
	pd.PatchOptional = otherFields.PatchOptional
	return nil
}

// TaskSelector handles the selection of specific task/variant combinations
// in the context of dependencies and requirements fields. //TODO no export?
type taskSelector struct {
	Name    string           `yaml:"name,omitempty"`
	Variant *variantSelector `yaml:"variant,omitempty" bson:"variant"`
}

// TaskSelectors is a helper type for parsing arrays of TaskSelector.
type taskSelectors []taskSelector

// VariantSelector handles the selection of a variant, either by a id/tag selector
// or by matching against matrix axis values.
type variantSelector struct {
	StringSelector string           `yaml:"string_selector" bson:"string_selector"`
	MatrixSelector matrixDefinition `yaml:"matrix_selector" bson:"matrix_selector"`
}

// UnmarshalYAML allows variants to be referenced as single selector strings or
// as a matrix definition. This works by first attempting to unmarshal the YAML
// into a string and then falling back to the matrix.
func (vs *variantSelector) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal just a selector string
	var onlySelector string
	if err := unmarshal(&onlySelector); err == nil {
		if onlySelector != "" {
			vs.StringSelector = onlySelector
			return nil
		}
	}

	md := matrixDefinition{}
	if err := unmarshal(&md); err != nil {
		return err
	}
	if len(md) == 0 {
		return errors.New("variant selector must not be empty")
	}
	vs.MatrixSelector = md
	return nil
}

func (vs *variantSelector) MarshalYAML() (interface{}, error) {
	if vs == nil || vs.StringSelector == "" {
		return nil, nil
	}
	// Note: Generate tasks will not work with matrix variant selectors,
	// since this will only marshal the string part of a variant selector.
	return vs.StringSelector, nil
}

// UnmarshalYAML reads YAML into an array of TaskSelector. It will
// successfully unmarshal arrays of dependency selectors or a single selector.
func (tss *taskSelectors) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal a single selector
	var single taskSelector
	if err := unmarshal(&single); err == nil {
		*tss = taskSelectors([]taskSelector{single})
		return nil
	}
	var slice []taskSelector
	if err := unmarshal(&slice); err != nil {
		return err
	}
	*tss = taskSelectors(slice)
	return nil
}

// UnmarshalYAML allows tasks to be referenced as single selector strings.
// This works by first attempting to unmarshal the YAML into a string
// and then falling back to the TaskSelector struct.
func (ts *taskSelector) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal just a selector string
	var onlySelector string
	if err := unmarshal(&onlySelector); err == nil {
		if onlySelector != "" {
			ts.Name = onlySelector
			return nil
		}
	}
	// we define a new type so that we can grab the yaml struct tags without the struct methods,
	// preventing infinite recursion on the UnmarshalYAML() method.
	type copyType taskSelector
	var tsc copyType
	if err := unmarshal(&tsc); err != nil {
		return err
	}
	if tsc.Name == "" {
		return errors.WithStack(errors.New("task selector must have a name"))
	}
	*ts = taskSelector(tsc)
	return nil
}

// parserBV is a helper type storing intermediary variant definitions.
type parserBV struct {
	Name         string             `yaml:"name,omitempty"`
	DisplayName  string             `yaml:"display_name,omitempty"`
	Expansions   util.Expansions    `yaml:"expansions,omitempty"`
	Tags         parserStringSlice  `yaml:"tags,omitempty,omitempty"`
	Modules      parserStringSlice  `yaml:"modules,omitempty"`
	Disabled     bool               `yaml:"disabled,omitempty"`
	Push         bool               `yaml:"push,omitempty"`
	BatchTime    *int               `yaml:"batchtime,omitempty"`
	Stepback     *bool              `yaml:"stepback,omitempty"`
	RunOn        parserStringSlice  `yaml:"run_on,omitempty"`
	Tasks        parserBVTaskUnits  `yaml:"tasks,omitempty"`
	DisplayTasks []displayTask      `yaml:"display_tasks,omitempty"`
	DependsOn    parserDependencies `yaml:"depends_on,omitempty"`
	Requires     taskSelectors      `yaml:"requires,omitempty"`

	// internal matrix stuff
	matrixId  string
	matrixVal matrixValue
	matrix    *matrix

	matrixRules []ruleAction
}

// helper methods for variant tag evaluations
func (pbv *parserBV) name() string   { return pbv.Name }
func (pbv *parserBV) tags() []string { return pbv.Tags }

func (pbv *parserBV) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first attempt to unmarshal into a matrix
	m := matrix{}
	merr := unmarshal(&m)
	if merr == nil {
		if m.Id != "" {
			*pbv = parserBV{matrix: &m}
			return nil
		}
	}
	// otherwise use a BV copy type to skip this Unmarshal method
	type copyType parserBV
	var bv copyType
	if err := unmarshal(&bv); err != nil {
		return errors.WithStack(err)
	}
	if bv.Name == "" {
		// if we're here, it's very likely that the user was building a matrix but broke
		// the syntax, so we try and surface the matrix error if they used "matrix_name".
		if m.Id != "" {
			return errors.Wrap(merr, "parsing matrix")
		}
		return errors.New("buildvariant missing name")
	}
	*pbv = parserBV(bv)
	return nil
}

// parserBVTaskUnit is a helper type storing intermediary variant task configurations.
type parserBVTaskUnit struct {
	Name             string             `yaml:"name,omitempty"`
	Patchable        *bool              `yaml:"patchable,omitempty"`
	PatchOnly        *bool              `yaml:"patch_only,omitempty"`
	Priority         int64              `yaml:"priority,omitempty"`
	DependsOn        parserDependencies `yaml:"depends_on,omitempty"`
	Requires         taskSelectors      `yaml:"requires,omitempty"`
	ExecTimeoutSecs  int                `yaml:"exec_timeout_secs,omitempty"`
	Stepback         *bool              `yaml:"stepback,omitempty"`
	Distros          parserStringSlice  `yaml:"distros,omitempty"`
	RunOn            parserStringSlice  `yaml:"run_on,omitempty"` // Alias for "Distros" TODO: deprecate Distros
	CommitQueueMerge bool               `yaml:"commit_queue_merge,omitempty"`
}

// UnmarshalYAML allows the YAML parser to read both a single selector string or
// a fully defined parserBVTaskUnit.
func (pbvt *parserBVTaskUnit) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal just a selector string
	var onlySelector string
	if err := unmarshal(&onlySelector); err == nil {
		if onlySelector != "" {
			pbvt.Name = onlySelector
			return nil
		}
	}
	// we define a new type so that we can grab the YAML struct tags without the struct methods,
	// preventing infinite recursion on the UnmarshalYAML() method.
	type copyType parserBVTaskUnit
	var copy copyType
	if err := unmarshal(&copy); err != nil {
		return err
	}
	if copy.Name == "" {
		return errors.New("buildvariant task selector must have a name")
	}
	// logic for aliasing the "run_on" field to "distros"
	if len(copy.RunOn) > 0 {
		if len(copy.Distros) > 0 {
			return errors.New("cannot use both 'run_on' and 'distros' fields")
		}
		copy.Distros, copy.RunOn = copy.RunOn, nil
	}
	*pbvt = parserBVTaskUnit(copy)
	return nil
}

// parserBVTaskUnits is a helper type for handling arrays of parserBVTaskUnit.
type parserBVTaskUnits []parserBVTaskUnit

// UnmarshalYAML allows the YAML parser to read both a single parserBVTaskUnit or
// an array of them into a slice.
func (pbvts *parserBVTaskUnits) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal just a selector string
	var single parserBVTaskUnit
	if err := unmarshal(&single); err == nil {
		*pbvts = parserBVTaskUnits([]parserBVTaskUnit{single})
		return nil
	}
	var slice []parserBVTaskUnit
	if err := unmarshal(&slice); err != nil {
		return err
	}
	*pbvts = parserBVTaskUnits(slice)
	return nil
}

// parserStringSlice is YAML helper type that accepts both an array of strings
// or single string value during unmarshalling.
type parserStringSlice []string

// UnmarshalYAML allows the YAML parser to read both a single string or
// an array of them into a slice.
func (pss *parserStringSlice) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var single string
	if err := unmarshal(&single); err == nil {
		*pss = []string{single}
		return nil
	}
	var slice []string
	if err := unmarshal(&slice); err != nil {
		return err
	}
	*pss = slice
	return nil
}

// LoadProjectInto loads the raw data from the config file into project
// and sets the project's identifier field to identifier. Tags are evaluated.
func LoadProjectInto(data []byte, identifier string, project *Project) error {
	p, errs := projectFromYAML(data)
	if len(errs) > 0 {
		// create a human-readable error list
		buf := bytes.Buffer{}
		for _, e := range errs {
			if len(errs) > 1 {
				buf.WriteString("\n\t") //only newline if we have multiple errs
			}
			buf.WriteString(e.Error())
		}
		return errors.Errorf("%s: %s", LoadProjectError, buf.String())
	}
	*project = *p
	project.Identifier = identifier
	return nil
}

// projectFromYAML reads and evaluates project YAML, returning a project and warnings and
// errors encountered during parsing or evaluation.
func projectFromYAML(yml []byte) (*Project, []error) {
	intermediateProject, errs := createIntermediateProject(yml)
	if len(errs) > 0 {
		return nil, errs
	}
	p, errs := translateProject(intermediateProject)
	return p, errs
}

// createIntermediateProject marshals the supplied YAML into our
// intermediate project representation (i.e. before selectors or
// matrix logic has been evaluated).
func createIntermediateProject(yml []byte) (*parserProject, []error) {
	p := &parserProject{}
	err := yaml.Unmarshal(yml, p)
	if err != nil {
		return nil, []error{err}
	}
	if p.Functions == nil {
		p.Functions = map[string]*YAMLCommandSet{}
	}

	return p, nil
}

// translateProject converts our intermediate project representation into
// the Project type that Evergreen actually uses. Errors are added to
// pp.errors and pp.warnings and must be checked separately.
func translateProject(pp *parserProject) (*Project, []error) {
	// Transfer top level fields
	proj := &Project{
		Enabled:           pp.Enabled,
		Stepback:          pp.Stepback,
		PreErrorFailsTask: pp.PreErrorFailsTask,
		BatchTime:         pp.BatchTime,
		Owner:             pp.Owner,
		Repo:              pp.Repo,
		RemotePath:        pp.RemotePath,
		RepoKind:          pp.RepoKind,
		Branch:            pp.Branch,
		Identifier:        pp.Identifier,
		DisplayName:       pp.DisplayName,
		CommandType:       pp.CommandType,
		Ignore:            pp.Ignore,
		Pre:               pp.Pre,
		Post:              pp.Post,
		Timeout:           pp.Timeout,
		CallbackTimeout:   pp.CallbackTimeout,
		Modules:           pp.Modules,
		Functions:         pp.Functions,
		ExecTimeoutSecs:   pp.ExecTimeoutSecs,
		Loggers:           pp.Loggers,
	}
	tse := NewParserTaskSelectorEvaluator(pp.Tasks)
	tgse := newTaskGroupSelectorEvaluator(pp.TaskGroups)
	ase := NewAxisSelectorEvaluator(pp.Axes)
	regularBVs, matrices := sieveMatrixVariants(pp.BuildVariants)

	var evalErrs, errs []error
	matrixVariants, errs := buildMatrixVariants(pp.Axes, ase, matrices)
	evalErrs = append(evalErrs, errs...)
	buildVariants := append(regularBVs, matrixVariants...)
	vse := NewVariantSelectorEvaluator(buildVariants, ase)

	proj.Tasks, proj.TaskGroups, errs = evaluateTaskUnits(tse, tgse, vse, pp.Tasks, pp.TaskGroups)
	evalErrs = append(evalErrs, errs...)

	proj.BuildVariants, errs = evaluateBuildVariants(tse, tgse, vse, buildVariants, pp.Tasks, proj.TaskGroups)
	evalErrs = append(evalErrs, errs...)

	return proj, evalErrs
}

// sieveMatrixVariants takes a set of parserBVs and groups them into regular
// buildvariant matrix definitions and matrix definitions.
func sieveMatrixVariants(bvs []parserBV) (regular []parserBV, matrices []matrix) {
	for _, bv := range bvs {
		if bv.matrix != nil {
			matrices = append(matrices, *bv.matrix)
		} else {
			regular = append(regular, bv)
		}
	}
	return regular, matrices
}

// evaluateTaskUnits translates intermediate tasks into true ProjectTask types,
// evaluating any selectors in the DependsOn or Requires fields.
func evaluateTaskUnits(tse *taskSelectorEvaluator, tgse *tagSelectorEvaluator, vse *variantSelectorEvaluator,
	pts []parserTask, tgs []parserTaskGroup) ([]ProjectTask, []TaskGroup, []error) {
	tasks := []ProjectTask{}
	groups := []TaskGroup{}
	var evalErrs, errs []error
	for _, pt := range pts {
		t := ProjectTask{
			Name:            pt.Name,
			Priority:        pt.Priority,
			ExecTimeoutSecs: pt.ExecTimeoutSecs,
			Commands:        pt.Commands,
			Tags:            pt.Tags,
			Patchable:       pt.Patchable,
			PatchOnly:       pt.PatchOnly,
			Stepback:        pt.Stepback,
		}
		t.DependsOn, errs = evaluateDependsOn(tse.tagEval, tgse, vse, pt.DependsOn)
		evalErrs = append(evalErrs, errs...)
		t.Requires, errs = evaluateRequires(tse.tagEval, tgse, vse, pt.Requires)
		evalErrs = append(evalErrs, errs...)
		tasks = append(tasks, t)
	}
	for _, ptg := range tgs {
		tg := TaskGroup{
			Name:                  ptg.Name,
			SetupGroupFailTask:    ptg.SetupGroupFailTask,
			SetupGroupTimeoutSecs: ptg.SetupGroupTimeoutSecs,
			SetupGroup:            ptg.SetupGroup,
			TeardownGroup:         ptg.TeardownGroup,
			SetupTask:             ptg.SetupTask,
			TeardownTask:          ptg.TeardownTask,
			Tags:                  ptg.Tags,
			MaxHosts:              ptg.MaxHosts,
			Timeout:               ptg.Timeout,
			ShareProcs:            ptg.ShareProcs,
		}
		if tg.MaxHosts < 1 {
			tg.MaxHosts = 1
		}
		// expand, validate that tasks defined in a group are listed in the project tasks
		var taskNames []string
		for _, taskName := range ptg.Tasks {
			names, err := tse.evalSelector(ParseSelector(taskName))
			if err != nil {
				evalErrs = append(evalErrs, err)
			}
			taskNames = append(taskNames, names...)
		}
		tg.Tasks = taskNames
		groups = append(groups, tg)
	}
	return tasks, groups, evalErrs
}

// evaluateBuildsVariants translates intermediate tasks into true BuildVariant types,
// evaluating any selectors in the Tasks fields.
func evaluateBuildVariants(tse *taskSelectorEvaluator, tgse *tagSelectorEvaluator, vse *variantSelectorEvaluator,
	pbvs []parserBV, tasks []parserTask, tgs []TaskGroup) ([]BuildVariant, []error) {
	bvs := []BuildVariant{}
	var evalErrs, errs []error
	for _, pbv := range pbvs {
		bv := BuildVariant{
			DisplayName: pbv.DisplayName,
			Name:        pbv.Name,
			Expansions:  pbv.Expansions,
			Modules:     pbv.Modules,
			Disabled:    pbv.Disabled,
			Push:        pbv.Push,
			BatchTime:   pbv.BatchTime,
			Stepback:    pbv.Stepback,
			RunOn:       pbv.RunOn,
			Tags:        pbv.Tags,
		}
		bv.Tasks, errs = evaluateBVTasks(tse, tgse, vse, pbv)

		// evaluate any rules passed in during matrix construction
		for _, r := range pbv.matrixRules {
			// remove_tasks removes all tasks with matching names
			if len(r.RemoveTasks) > 0 {
				prunedTasks := []BuildVariantTaskUnit{}
				toRemove := []string{}
				for _, t := range r.RemoveTasks {
					removed, err := tse.evalSelector(ParseSelector(t))
					if err != nil {
						evalErrs = append(evalErrs, errors.Wrap(err, "remove rule"))
						continue
					}
					toRemove = append(toRemove, removed...)
				}
				for _, t := range bv.Tasks {
					if !util.StringSliceContains(toRemove, t.Name) {
						prunedTasks = append(prunedTasks, t)
					}
				}
				bv.Tasks = prunedTasks
			}

			// add_tasks adds the given BuildVariantTasks, returning errors for any collisions
			if len(r.AddTasks) > 0 {
				// cache existing tasks so we can check for duplicates
				existing := map[string]*BuildVariantTaskUnit{}
				for i, t := range bv.Tasks {
					existing[t.Name] = &bv.Tasks[i]
				}

				var added []BuildVariantTaskUnit
				pbv.Tasks = r.AddTasks
				added, errs = evaluateBVTasks(tse, tgse, vse, pbv)
				evalErrs = append(evalErrs, errs...)
				// check for conflicting duplicates
				for _, t := range added {
					if old, ok := existing[t.Name]; ok {
						if !reflect.DeepEqual(t, *old) {
							evalErrs = append(evalErrs, errors.Errorf(
								"conflicting definitions of added tasks '%v': %v != %v", t.Name, t, old))
						}
					} else {
						bv.Tasks = append(bv.Tasks, t)
						existing[t.Name] = &t
					}
				}
			}
		}

		tgMap := map[string]TaskGroup{}
		for _, tg := range tgs {
			tgMap[tg.Name] = tg
		}
		dtse := newDisplayTaskSelectorEvaluator(bv, tasks, tgs, tgMap)

		// check that display tasks contain real tasks that are not duplicated
		bvTasks := make(map[string]struct{})        // map of all execution tasks
		displayTaskContents := make(map[string]int) // map of execution tasks in a display task
		for _, t := range bv.Tasks {
			if tg, exists := tgMap[t.Name]; exists {
				for _, tgTask := range tg.Tasks {
					bvTasks[tgTask] = struct{}{}
				}
			} else {
				bvTasks[t.Name] = struct{}{}
			}
		}

		// save display task if it contains valid execution tasks
		for _, dt := range pbv.DisplayTasks {
			projectDt := DisplayTask{Name: dt.Name}
			if _, exists := bvTasks[dt.Name]; exists {
				errs = append(errs, fmt.Errorf("display task %s cannot have the same name as an execution task", dt.Name))
				continue
			}

			//resolve tags for display tasks
			tasks := []string{}
			for _, et := range dt.ExecutionTasks {
				results, err := dtse.evalSelector(ParseSelector(et))
				if err != nil {
					errs = append(errs, err)
				}
				tasks = append(tasks, results...)
			}

			for _, et := range tasks {
				if _, exists := bvTasks[et]; !exists {
					errs = append(errs, fmt.Errorf("display task %s contains execution task %s which does not exist in build variant", dt.Name, et))
				} else {
					projectDt.ExecutionTasks = append(projectDt.ExecutionTasks, et)
					displayTaskContents[et]++
				}
			}
			if len(projectDt.ExecutionTasks) > 0 {
				bv.DisplayTasks = append(bv.DisplayTasks, projectDt)
			}
		}
		for taskId, count := range displayTaskContents {
			if count > 1 {
				errs = append(errs, fmt.Errorf("execution task %s is listed in more than 1 display task", taskId))
				bv.DisplayTasks = nil
			}
		}

		evalErrs = append(evalErrs, errs...)
		bvs = append(bvs, bv)
	}
	return bvs, evalErrs
}

// evaluateBVTasks translates intermediate tasks into true BuildVariantTaskUnit types,
// evaluating any selectors referencing tasks, and further evaluating any selectors
// in the DependsOn or Requires fields of those tasks.
func evaluateBVTasks(tse *taskSelectorEvaluator, tgse *tagSelectorEvaluator, vse *variantSelectorEvaluator,
	pbv parserBV) ([]BuildVariantTaskUnit, []error) {
	var evalErrs, errs []error
	ts := []BuildVariantTaskUnit{}
	taskUnitsByName := map[string]BuildVariantTaskUnit{}
	for _, pt := range pbv.Tasks {
		// evaluate each task against both the task and task group selectors
		// only error if both selectors error because each task should only be found
		// in one or the other
		var names, temp []string
		var err1, err2 error
		isGroup := false
		if tse != nil {
			temp, err1 = tse.evalSelector(ParseSelector(pt.Name))
			names = append(names, temp...)
		}
		if tgse != nil {
			temp, err2 = tgse.evalSelector(ParseSelector(pt.Name))
			if len(temp) > 0 {
				names = append(names, temp...)
				isGroup = true
			}
		}
		if err1 != nil && err2 != nil {
			evalErrs = append(evalErrs, err1, err2)
			continue
		}
		// create new task definitions--duplicates must have the same status requirements
		for _, name := range names {
			// create a new task by copying the task that selected it,
			// so we can preserve the "Variant" and "Status" field.
			t := BuildVariantTaskUnit{
				Name:             name,
				Patchable:        pt.Patchable,
				PatchOnly:        pt.PatchOnly,
				Priority:         pt.Priority,
				ExecTimeoutSecs:  pt.ExecTimeoutSecs,
				Stepback:         pt.Stepback,
				Distros:          pt.Distros,
				CommitQueueMerge: pt.CommitQueueMerge,
			}

			// Task-level dependencies in the variant override variant-level dependencies
			// in the variant. If neither is present, then the BuildVariantTaskUnit unit
			// will contain no dependencies, so dependencies will come from the task
			// spec at task creation time.
			var dependsOn parserDependencies
			var requires taskSelectors
			if len(pbv.DependsOn) > 0 {
				dependsOn = pbv.DependsOn
			}
			if len(pt.DependsOn) > 0 {
				dependsOn = pt.DependsOn
			}
			if len(pbv.Requires) > 0 {
				requires = pbv.Requires
			}
			if len(pt.Requires) > 0 {
				requires = pt.Requires
			}

			t.DependsOn, errs = evaluateDependsOn(tse.tagEval, tgse, vse, dependsOn)
			evalErrs = append(evalErrs, errs...)
			t.Requires, errs = evaluateRequires(tse.tagEval, tgse, vse, requires)
			evalErrs = append(evalErrs, errs...)
			t.IsGroup = isGroup

			// add the new task if it doesn't already exists (we must avoid conflicting status fields)
			if old, ok := taskUnitsByName[t.Name]; !ok {
				ts = append(ts, t)
				taskUnitsByName[t.Name] = t
			} else {
				// it's already in the new list, so we check to make sure the status definitions match.
				if !reflect.DeepEqual(t, old) {
					evalErrs = append(evalErrs, errors.Errorf(
						"conflicting definitions of build variant tasks '%v': %v != %v", name, t, old))
					continue
				}
			}
		}
	}
	return ts, evalErrs
}

// evaluateDependsOn expands any selectors in a dependency definition.
func evaluateDependsOn(tse *tagSelectorEvaluator, tgse *tagSelectorEvaluator, vse *variantSelectorEvaluator,
	deps []parserDependency) ([]TaskUnitDependency, []error) {
	var evalErrs []error
	var err error
	newDeps := []TaskUnitDependency{}
	newDepsByNameAndVariant := map[TVPair]TaskUnitDependency{}
	for _, d := range deps {
		var names []string

		if d.TaskSelector.Name == AllDependencies {
			// * is a special case for dependencies, so don't eval it
			names = []string{AllDependencies}
		} else {
			var temp []string
			var err1, err2 error
			if tse != nil {
				temp, err1 = tse.evalSelector(ParseSelector(d.TaskSelector.Name))
				names = append(names, temp...)
			}
			if tgse != nil {
				temp, err2 = tgse.evalSelector(ParseSelector(d.TaskSelector.Name))
				names = append(names, temp...)
			}
			if err1 != nil && err2 != nil {
				evalErrs = append(evalErrs, err1, err2)
				continue
			}
		}
		// we default to handle the empty variant, but expand the list of variants
		// if the variant field is set.
		variants := []string{""}
		if d.TaskSelector.Variant != nil {
			variants, err = vse.evalSelector(d.TaskSelector.Variant)
			if err != nil {
				evalErrs = append(evalErrs, err)
				continue
			}
		}
		// create new dependency definitions--duplicates must have the same status requirements
		for _, name := range names {
			for _, variant := range variants {
				// create a newDep by copying the dep that selected it,
				// so we can preserve the "Status" and "PatchOptional" field.
				newDep := TaskUnitDependency{
					Name:          name,
					Variant:       variant,
					Status:        d.Status,
					PatchOptional: d.PatchOptional,
				}
				// add the new dep if it doesn't already exists (we must avoid conflicting status fields)
				if oldDep, ok := newDepsByNameAndVariant[TVPair{newDep.Variant, newDep.Name}]; !ok {
					newDeps = append(newDeps, newDep)
					newDepsByNameAndVariant[TVPair{newDep.Variant, newDep.Name}] = newDep
				} else {
					// it's already in the new list, so we check to make sure the status definitions match.
					if !reflect.DeepEqual(newDep, oldDep) {
						evalErrs = append(evalErrs, errors.Errorf(
							"conflicting definitions of dependency '%v': %v != %v", name, newDep, oldDep))
						continue
					}
				}
			}
		}
	}
	return newDeps, evalErrs
}

// evaluateRequires expands any selectors in a requirement definition.
func evaluateRequires(tse *tagSelectorEvaluator, tgse *tagSelectorEvaluator, vse *variantSelectorEvaluator,
	reqs []taskSelector) ([]TaskUnitRequirement, []error) {
	var evalErrs []error
	newReqs := []TaskUnitRequirement{}
	newReqsByNameAndVariant := map[TVPair]struct{}{}
	for _, r := range reqs {
		var names, temp []string
		var err, err1, err2 error
		if tse != nil {
			temp, err1 = tse.evalSelector(ParseSelector(r.Name))
			names = append(names, temp...)
		}
		if tgse != nil {
			temp, err2 = tgse.evalSelector(ParseSelector(r.Name))
			names = append(names, temp...)
		}
		if err1 != nil && err2 != nil {
			evalErrs = append(evalErrs, err1, err2)
			continue
		}
		// we default to handle the empty variant, but expand the list of variants
		// if the variant field is set.
		variants := []string{""}
		if r.Variant != nil {
			variants, err = vse.evalSelector(r.Variant)
			if err != nil {
				evalErrs = append(evalErrs, err)
				continue
			}
		}
		for _, name := range names {
			for _, variant := range variants {
				newReq := TaskUnitRequirement{Name: name, Variant: variant}
				newReq.Name = name
				// add the new req if it doesn't already exists (we must avoid duplicates)
				if _, ok := newReqsByNameAndVariant[TVPair{newReq.Variant, newReq.Name}]; !ok {
					newReqs = append(newReqs, newReq)
					newReqsByNameAndVariant[TVPair{newReq.Variant, newReq.Name}] = struct{}{}
				}
			}
		}
	}
	return newReqs, evalErrs
}
