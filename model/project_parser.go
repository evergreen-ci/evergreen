package model

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

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
	Enabled         bool                       `yaml:"enabled"`
	Stepback        bool                       `yaml:"stepback"`
	BatchTime       int                        `yaml:"batchtime"`
	Owner           string                     `yaml:"owner"`
	Repo            string                     `yaml:"repo"`
	RemotePath      string                     `yaml:"remote_path"`
	RepoKind        string                     `yaml:"repokind"`
	Branch          string                     `yaml:"branch"`
	Identifier      string                     `yaml:"identifier"`
	DisplayName     string                     `yaml:"display_name"`
	CommandType     string                     `yaml:"command_type"`
	Ignore          parserStringSlice          `yaml:"ignore"`
	Pre             *YAMLCommandSet            `yaml:"pre"`
	Post            *YAMLCommandSet            `yaml:"post"`
	Timeout         *YAMLCommandSet            `yaml:"timeout"`
	CallbackTimeout int                        `yaml:"callback_timeout_secs"`
	Modules         []Module                   `yaml:"modules"`
	BuildVariants   []parserBV                 `yaml:"buildvariants"`
	Functions       map[string]*YAMLCommandSet `yaml:"functions"`
	Tasks           []parserTask               `yaml:"tasks"`
	ExecTimeoutSecs int                        `yaml:"exec_timeout_secs"`

	// Matrix code
	Axes []matrixAxis `yaml:"axes"`
}

// parserTask represents an intermediary state of task definitions.
type parserTask struct {
	Name            string              `yaml:"name"`
	Priority        int64               `yaml:"priority"`
	ExecTimeoutSecs int                 `yaml:"exec_timeout_secs"`
	DisableCleanup  bool                `yaml:"disable_cleanup"`
	DependsOn       parserDependencies  `yaml:"depends_on"`
	Requires        taskSelectors       `yaml:"requires"`
	Commands        []PluginCommandConf `yaml:"commands"`
	Tags            parserStringSlice   `yaml:"tags"`
	Patchable       *bool               `yaml:"patchable"`
	Stepback        *bool               `yaml:"stepback"`
}

type displayTask struct {
	Name           string   `yaml:"name"`
	ExecutionTasks []string `yaml:"execution_tasks"`
}

// helper methods for task tag evaluations
func (pt *parserTask) name() string   { return pt.Name }
func (pt *parserTask) tags() []string { return pt.Tags }

// parserDependency represents the intermediary state for referencing dependencies.
type parserDependency struct {
	taskSelector
	Status        string `yaml:"status"`
	PatchOptional bool   `yaml:"patch_optional"`
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
	if err := unmarshal(&pd.taskSelector); err != nil {
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
	Name    string           `yaml:"name"`
	Variant *variantSelector `yaml:"variant"`
}

// TaskSelectors is a helper type for parsing arrays of TaskSelector.
type taskSelectors []taskSelector

// VariantSelector handles the selection of a variant, either by a id/tag selector
// or by matching against matrix axis values.
type variantSelector struct {
	stringSelector string
	matrixSelector matrixDefinition
}

// UnmarshalYAML allows variants to be referenced as single selector strings or
// as a matrix definition. This works by first attempting to unmarshal the YAML
// into a string and then falling back to the matrix.
func (vs *variantSelector) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal just a selector string
	var onlySelector string
	if err := unmarshal(&onlySelector); err == nil {
		if onlySelector != "" {
			vs.stringSelector = onlySelector
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
	vs.matrixSelector = md
	return nil
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
		return errors.New("task selector must have a name")
	}
	*ts = taskSelector(tsc)
	return nil
}

// parserBV is a helper type storing intermediary variant definitions.
type parserBV struct {
	Name         string            `yaml:"name"`
	DisplayName  string            `yaml:"display_name"`
	Expansions   util.Expansions   `yaml:"expansions"`
	Tags         parserStringSlice `yaml:"tags"`
	Modules      parserStringSlice `yaml:"modules"`
	Disabled     bool              `yaml:"disabled"`
	Push         bool              `yaml:"push"`
	BatchTime    *int              `yaml:"batchtime"`
	Stepback     *bool             `yaml:"stepback"`
	RunOn        parserStringSlice `yaml:"run_on"`
	Tasks        parserBVTasks     `yaml:"tasks"`
	DisplayTasks []displayTask     `yaml:"display_tasks"`

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

// parserBVTask is a helper type storing intermediary variant task configurations.
type parserBVTask struct {
	Name            string             `yaml:"name"`
	Patchable       *bool              `yaml:"patchable"`
	Priority        int64              `yaml:"priority"`
	DependsOn       parserDependencies `yaml:"depends_on"`
	Requires        taskSelectors      `yaml:"requires"`
	ExecTimeoutSecs int                `yaml:"exec_timeout_secs"`
	Stepback        *bool              `yaml:"stepback"`
	Distros         parserStringSlice  `yaml:"distros"`
	RunOn           parserStringSlice  `yaml:"run_on"` // Alias for "Distros" TODO: deprecate Distros
}

// UnmarshalYAML allows the YAML parser to read both a single selector string or
// a fully defined parserBVTask.
func (pbvt *parserBVTask) UnmarshalYAML(unmarshal func(interface{}) error) error {
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
	type copyType parserBVTask
	var copy copyType
	if err := unmarshal(&copy); err != nil {
		return err
	}
	if copy.Name == "" {
		return errors.New("task selector must have a name")
	}
	// logic for aliasing the "run_on" field to "distros"
	if len(copy.RunOn) > 0 {
		if len(copy.Distros) > 0 {
			return errors.New("cannot use both 'run_on' and 'distros' fields")
		}
		copy.Distros, copy.RunOn = copy.RunOn, nil
	}
	*pbvt = parserBVTask(copy)
	return nil
}

// parserBVTasks is a helper type for handling arrays of parserBVTask.
type parserBVTasks []parserBVTask

// UnmarshalYAML allows the YAML parser to read both a single parserBVTask or
// an array of them into a slice.
func (pbvts *parserBVTasks) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal just a selector string
	var single parserBVTask
	if err := unmarshal(&single); err == nil {
		*pbvts = parserBVTasks([]parserBVTask{single})
		return nil
	}
	var slice []parserBVTask
	if err := unmarshal(&slice); err != nil {
		return err
	}
	*pbvts = parserBVTasks(slice)
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
// and sets the project's identifier field to identifier. Tags are evaluateed.
func LoadProjectInto(data []byte, identifier string, project *Project) error {
	p, errs := projectFromYAML(data) // ignore warnings, for now (TODO)
	if len(errs) > 0 {
		// create a human-readable error list
		buf := bytes.Buffer{}
		for _, e := range errs {
			if len(errs) > 1 {
				buf.WriteString("\n\t") //only newline if we have multiple errs
			}
			buf.WriteString(e.Error())
		}
		if len(errs) > 1 {
			return errors.Errorf("project errors: %v", buf.String())
		}
		return errors.Errorf("project error: %v", buf.String())
	}
	*project = *p
	project.Identifier = identifier
	return nil
}

// projectFromYAML reads and evaluates project YAML, returning a project and warnings and
// errors encountered during parsing or evaluation.
func projectFromYAML(yml []byte) (*Project, []error) {
	pp, errs := createIntermediateProject(yml)
	if len(errs) > 0 {
		return nil, errs
	}
	p, errs := translateProject(pp)
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

	return p, nil
}

// translateProject converts our intermediate project representation into
// the Project type that Evergreen actually uses. Errors are added to
// pp.errors and pp.warnings and must be checked separately.
func translateProject(pp *parserProject) (*Project, []error) {
	// Transfer top level fields
	proj := &Project{
		Enabled:         pp.Enabled,
		Stepback:        pp.Stepback,
		BatchTime:       pp.BatchTime,
		Owner:           pp.Owner,
		Repo:            pp.Repo,
		RemotePath:      pp.RemotePath,
		RepoKind:        pp.RepoKind,
		Branch:          pp.Branch,
		Identifier:      pp.Identifier,
		DisplayName:     pp.DisplayName,
		CommandType:     pp.CommandType,
		Ignore:          pp.Ignore,
		Pre:             pp.Pre,
		Post:            pp.Post,
		Timeout:         pp.Timeout,
		CallbackTimeout: pp.CallbackTimeout,
		Modules:         pp.Modules,
		Functions:       pp.Functions,
		ExecTimeoutSecs: pp.ExecTimeoutSecs,
	}
	tse := NewParserTaskSelectorEvaluator(pp.Tasks)
	ase := NewAxisSelectorEvaluator(pp.Axes)
	regularBVs, matrices := sieveMatrixVariants(pp.BuildVariants)
	var evalErrs, errs []error
	matrixVariants, errs := buildMatrixVariants(pp.Axes, ase, matrices)
	evalErrs = append(evalErrs, errs...)
	pp.BuildVariants = append(regularBVs, matrixVariants...)
	vse := NewVariantSelectorEvaluator(pp.BuildVariants, ase)
	proj.Tasks, errs = evaluateTasks(tse, vse, pp.Tasks)
	evalErrs = append(evalErrs, errs...)
	proj.BuildVariants, errs = evaluateBuildVariants(tse, vse, pp.BuildVariants, pp.Tasks)
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

// evaluateTasks translates intermediate tasks into true ProjectTask types,
// evaluating any selectors in the DependsOn or Requires fields.
func evaluateTasks(tse *taskSelectorEvaluator, vse *variantSelectorEvaluator,
	pts []parserTask) ([]ProjectTask, []error) {
	tasks := []ProjectTask{}
	var evalErrs, errs []error
	for _, pt := range pts {
		t := ProjectTask{
			Name:            pt.Name,
			Priority:        pt.Priority,
			ExecTimeoutSecs: pt.ExecTimeoutSecs,
			Commands:        pt.Commands,
			Tags:            pt.Tags,
			Patchable:       pt.Patchable,
			Stepback:        pt.Stepback,
		}
		t.DependsOn, errs = evaluateDependsOn(tse, vse, pt.DependsOn)
		evalErrs = append(evalErrs, errs...)
		t.Requires, errs = evaluateRequires(tse, vse, pt.Requires)
		evalErrs = append(evalErrs, errs...)
		tasks = append(tasks, t)
	}
	return tasks, evalErrs
}

// evaluateBuildsVariants translates intermediate tasks into true BuildVariant types,
// evaluating any selectors in the Tasks fields.
func evaluateBuildVariants(tse *taskSelectorEvaluator, vse *variantSelectorEvaluator,
	pbvs []parserBV, tasks []parserTask) ([]BuildVariant, []error) {
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
		bv.Tasks, errs = evaluateBVTasks(tse, vse, pbv.Tasks)
		// evaluate any rules passed in during matrix construction
		for _, r := range pbv.matrixRules {
			// remove_tasks removes all tasks with matching names
			if len(r.RemoveTasks) > 0 {
				prunedTasks := []BuildVariantTask{}
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
				existing := map[string]*BuildVariantTask{}
				for i, t := range bv.Tasks {
					existing[t.Name] = &bv.Tasks[i]
				}

				var added []BuildVariantTask
				added, errs = evaluateBVTasks(tse, vse, r.AddTasks)
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

		//resolve tags for display tasks
		dtse := newDisplayTaskSelectorEvaluator(bv, tasks)
		for i, dt := range pbv.DisplayTasks {
			tasks := []string{}
			for _, et := range dt.ExecutionTasks {
				results, err := dtse.evalSelector(ParseSelector(et))
				if err != nil {
					errs = append(errs, err)
				}
				tasks = append(tasks, results...)
			}
			pbv.DisplayTasks[i].ExecutionTasks = tasks
		}

		// check that display tasks contain real tasks that are not duplicated
		bvTasks := make(map[string]string)          // map of all execution tasks
		displayTaskContents := make(map[string]int) // map of execution tasks in a display task
		for _, t := range bv.Tasks {
			bvTasks[t.Name] = ""
		}
		for _, dt := range pbv.DisplayTasks {
			projectDt := DisplayTask{Name: dt.Name}
			for _, et := range dt.ExecutionTasks {
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

// evaluateBVTasks translates intermediate tasks into true BuildVariantTask types,
// evaluating any selectors referencing tasks, and further evaluating any selectors
// in the DependsOn or Requires fields of those tasks.
func evaluateBVTasks(tse *taskSelectorEvaluator, vse *variantSelectorEvaluator,
	pbvts []parserBVTask) ([]BuildVariantTask, []error) {
	var evalErrs, errs []error
	ts := []BuildVariantTask{}
	tasksByName := map[string]BuildVariantTask{}
	for _, pt := range pbvts {
		names, err := tse.evalSelector(ParseSelector(pt.Name))
		if err != nil {
			evalErrs = append(evalErrs, err)
			continue
		}
		// create new task definitions--duplicates must have the same status requirements
		for _, name := range names {
			// create a new task by copying the task that selected it,
			// so we can preserve the "Variant" and "Status" field.
			t := BuildVariantTask{
				Name:            name,
				Patchable:       pt.Patchable,
				Priority:        pt.Priority,
				ExecTimeoutSecs: pt.ExecTimeoutSecs,
				Stepback:        pt.Stepback,
				Distros:         pt.Distros,
			}
			t.DependsOn, errs = evaluateDependsOn(tse, vse, pt.DependsOn)
			evalErrs = append(evalErrs, errs...)
			t.Requires, errs = evaluateRequires(tse, vse, pt.Requires)
			evalErrs = append(evalErrs, errs...)

			// add the new task if it doesn't already exists (we must avoid conflicting status fields)
			if old, ok := tasksByName[t.Name]; !ok {
				ts = append(ts, t)
				tasksByName[t.Name] = t
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
func evaluateDependsOn(tse *taskSelectorEvaluator, vse *variantSelectorEvaluator,
	deps []parserDependency) ([]TaskDependency, []error) {
	var evalErrs []error
	var err error
	newDeps := []TaskDependency{}
	newDepsByNameAndVariant := map[TVPair]TaskDependency{}
	for _, d := range deps {
		var names []string

		if d.Name == AllDependencies {
			// * is a special case for dependencies, so don't eval it
			names = []string{AllDependencies}
		} else {
			names, err = tse.evalSelector(ParseSelector(d.Name))
			if err != nil {
				evalErrs = append(evalErrs, err)
				continue
			}
		}
		// we default to handle the empty variant, but expand the list of variants
		// if the variant field is set.
		variants := []string{""}
		if d.Variant != nil {
			variants, err = vse.evalSelector(d.Variant)
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
				newDep := TaskDependency{
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
func evaluateRequires(tse *taskSelectorEvaluator, vse *variantSelectorEvaluator,
	reqs []taskSelector) ([]TaskRequirement, []error) {
	var evalErrs []error
	newReqs := []TaskRequirement{}
	newReqsByNameAndVariant := map[TVPair]struct{}{}
	for _, r := range reqs {
		names, err := tse.evalSelector(ParseSelector(r.Name))
		if err != nil {
			evalErrs = append(evalErrs, err)
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
				newReq := TaskRequirement{Name: name, Variant: variant}
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
