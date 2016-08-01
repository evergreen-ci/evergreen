package model

import (
	"bytes"
	"fmt"
	"reflect"

	"gopkg.in/yaml.v2"
)

// This file contains all of the infrastructure for turning a YAML project configuration
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
// and (TODO) matrix definitions. This step recursively crawls variants, tasks, their
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
	DisableCleanup  bool                       `yaml:"disable_cleanup"`
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
}

// parserTask represents an intermediary state of task definitions.
type parserTask struct {
	Name            string              `yaml:"name"`
	Priority        int64               `yaml:"priority"`
	ExecTimeoutSecs int                 `yaml:"exec_timeout_secs"`
	DisableCleanup  bool                `yaml:"disable_cleanup"`
	DependsOn       parserDependencies  `yaml:"depends_on"`
	Requires        TaskSelectors       `yaml:"requires"`
	Commands        []PluginCommandConf `yaml:"commands"`
	Tags            parserStringSlice   `yaml:"tags"`
	Patchable       *bool               `yaml:"patchable"`
	Stepback        *bool               `yaml:"stepback"`
}

// parserDependency represents the intermediary state for referencing dependencies.
type parserDependency struct {
	TaskSelector
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
	if err := unmarshal(&pd.TaskSelector); err != nil {
		return err
	}
	otherFields := struct {
		Status        string `yaml:"status"`
		PatchOptional bool   `yaml:"patch_optional"`
	}{}
	// ignore any errors here; if we're using a single-string selector, this is expected to fail
	unmarshal(&otherFields)
	// TODO validate status
	pd.Status = otherFields.Status
	pd.PatchOptional = otherFields.PatchOptional
	return nil
}

// TaskSelector handles the selection of specific task/variant combinations
// in the context of dependencies and requirements fields.
type TaskSelector struct {
	Name    string `yaml:"name"`
	Variant string `yaml:"variant"`
}

// TaskSelectors is a helper type for parsing arrays of TaskSelector.
type TaskSelectors []TaskSelector

// UnmarshalYAML reads YAML into an array of TaskSelector. It will
// successfully unmarshal arrays of dependency selectors or a single selector.
func (tss *TaskSelectors) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal a single selector
	var single TaskSelector
	if err := unmarshal(&single); err == nil {
		*tss = TaskSelectors([]TaskSelector{single})
		return nil
	}
	var slice []TaskSelector
	if err := unmarshal(&slice); err != nil {
		return err
	}
	*tss = TaskSelectors(slice)
	return nil
}

// UnmarshalYAML allows tasks to be referenced as single selector strings.
// This works by first attempting to unmarshal the YAML into a string
// and then falling back to the TaskSelector struct.
func (ts *TaskSelector) UnmarshalYAML(unmarshal func(interface{}) error) error {
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
	type copyType TaskSelector
	var tsc copyType
	if err := unmarshal(&tsc); err != nil {
		return err
	}
	if tsc.Name == "" {
		return fmt.Errorf("task selector must have a name")
	}
	*ts = TaskSelector(tsc)
	return nil
}

// parserBV is a helper type storing intermediary variant definitions.
type parserBV struct {
	Name        string            `yaml:"name"`
	DisplayName string            `yaml:"display_name"`
	Expansions  map[string]string `yaml:"expansions"`
	Modules     parserStringSlice `yaml:"modules"`
	Disabled    bool              `yaml:"disabled"`
	Push        bool              `yaml:"push"`
	BatchTime   *int              `yaml:"batchtime"`
	Stepback    *bool             `yaml:"stepback"`
	RunOn       parserStringSlice `yaml:"run_on"`
	Tasks       parserBVTasks     `yaml:"tasks"`
}

// parserBVTask is a helper type storing intermediary variant task configurations.
type parserBVTask struct {
	Name            string             `yaml:"name"`
	Patchable       *bool              `yaml:"patchable"`
	Priority        int64              `yaml:"priority"`
	DependsOn       parserDependencies `yaml:"depends_on"`
	Requires        TaskSelectors      `yaml:"requires"`
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
		return fmt.Errorf("task selector must have a name")
	}
	// logic for aliasing the "run_on" field to "distros"
	if len(copy.RunOn) > 0 {
		if len(copy.Distros) > 0 {
			return fmt.Errorf("cannot use both 'run_on' and 'distros' fields")
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
		return fmt.Errorf("error loading project yaml: %v", buf.String())
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
		DisableCleanup:  pp.DisableCleanup,
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
	var evalErrs, errs []error
	proj.Tasks, errs = evaluateTasks(tse, pp.Tasks)
	evalErrs = append(evalErrs, errs...)
	proj.BuildVariants, errs = evaluateBuildVariants(tse, pp.BuildVariants)
	evalErrs = append(evalErrs, errs...)
	return proj, evalErrs
}

// evaluateTasks translates intermediate tasks into true ProjectTask types,
// evaluating any selectors in the DependsOn or Requires fields.
func evaluateTasks(tse *taskSelectorEvaluator, pts []parserTask) ([]ProjectTask, []error) {
	tasks := []ProjectTask{}
	var evalErrs, errs []error
	for _, pt := range pts {
		t := ProjectTask{
			Name:            pt.Name,
			Priority:        pt.Priority,
			ExecTimeoutSecs: pt.ExecTimeoutSecs,
			DisableCleanup:  pt.DisableCleanup,
			Commands:        pt.Commands,
			Tags:            pt.Tags,
			Patchable:       pt.Patchable,
			Stepback:        pt.Stepback,
		}
		t.DependsOn, errs = evaluateDependsOn(tse, pt.DependsOn)
		evalErrs = append(evalErrs, errs...)
		t.Requires, errs = evaluateRequires(tse, pt.Requires)
		evalErrs = append(evalErrs, errs...)
		tasks = append(tasks, t)
	}
	return tasks, evalErrs
}

// evaluateBuildsVariants translates intermediate tasks into true BuildVariant types,
// evaluating any selectors in the Tasks fields.
func evaluateBuildVariants(tse *taskSelectorEvaluator, pbvs []parserBV) ([]BuildVariant, []error) {
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
		}
		bv.Tasks, errs = evaluateBVTasks(tse, pbv.Tasks)
		evalErrs = append(evalErrs, errs...)
		bvs = append(bvs, bv)
	}
	return bvs, errs
}

// evaluateBVTasks translates intermediate tasks into true BuildVariantTask types,
// evaluating any selectors referencing tasks, and further evaluating any selectors
// in the DependsOn or Requires fields of those tasks.
func evaluateBVTasks(tse *taskSelectorEvaluator, pbvts []parserBVTask) ([]BuildVariantTask, []error) {
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
			t.DependsOn, errs = evaluateDependsOn(tse, pt.DependsOn)
			evalErrs = append(evalErrs, errs...)
			t.Requires, errs = evaluateRequires(tse, pt.Requires)
			evalErrs = append(evalErrs, errs...)

			// add the new task if it doesn't already exists (we must avoid conflicting status fields)
			if old, ok := tasksByName[t.Name]; !ok {
				ts = append(ts, t)
				tasksByName[t.Name] = t
			} else {
				// it's already in the new list, so we check to make sure the status definitions match.
				if !reflect.DeepEqual(t, old) {
					evalErrs = append(evalErrs, fmt.Errorf(
						"conflicting definitions of build variant tasks '%v': %v != %v", name, t, old))
					continue
				}
			}
		}
	}
	return ts, evalErrs
}

// evaluateDependsOn expands any selectors in a dependency definition.
func evaluateDependsOn(tse *taskSelectorEvaluator, deps []parserDependency) ([]TaskDependency, []error) {
	var evalErrs []error
	newDeps := []TaskDependency{}
	newDepsByNameAndVariant := map[TVPair]TaskDependency{}
	for _, d := range deps {
		if d.Name == AllDependencies {
			// * is a special case for dependencies
			allDep := TaskDependency{
				Name:          AllDependencies,
				Variant:       d.Variant,
				Status:        d.Status,
				PatchOptional: d.PatchOptional,
			}
			newDeps = append(newDeps, allDep)
			newDepsByNameAndVariant[TVPair{d.Variant, d.Name}] = allDep
			continue
		}
		names, err := tse.evalSelector(ParseSelector(d.Name))
		if err != nil {
			evalErrs = append(evalErrs, err)
			continue
		}
		// create new dependency definitions--duplicates must have the same status requirements
		for _, name := range names {
			// create a newDep by copying the dep that selected it,
			// so we can preserve the "Variant" and "Status" field.
			newDep := TaskDependency{
				Name:          name,
				Variant:       d.Variant,
				Status:        d.Status,
				PatchOptional: d.PatchOptional,
			}
			newDep.Name = name
			// add the new dep if it doesn't already exists (we must avoid conflicting status fields)
			if oldDep, ok := newDepsByNameAndVariant[TVPair{newDep.Variant, newDep.Name}]; !ok {
				newDeps = append(newDeps, newDep)
				newDepsByNameAndVariant[TVPair{newDep.Variant, newDep.Name}] = newDep
			} else {
				// it's already in the new list, so we check to make sure the status definitions match.
				if !reflect.DeepEqual(newDep, oldDep) {
					evalErrs = append(evalErrs, fmt.Errorf(
						"conflicting definitions of dependency '%v': %v != %v", name, newDep, oldDep))
					continue
				}
			}
		}
	}
	return newDeps, evalErrs
}

// evaluateRequires expands any selectors in a requirement definition.
func evaluateRequires(tse *taskSelectorEvaluator, reqs []TaskSelector) ([]TaskRequirement, []error) {
	var evalErrs []error
	newReqs := []TaskRequirement{}
	newReqsByNameAndVariant := map[TVPair]struct{}{}
	for _, r := range reqs {
		names, err := tse.evalSelector(ParseSelector(r.Name))
		if err != nil {
			evalErrs = append(evalErrs, err)
			continue
		}
		for _, name := range names {
			newReq := TaskRequirement{Name: name, Variant: r.Variant}
			newReq.Name = name
			// add the new req if it doesn't already exists (we must avoid duplicates)
			if _, ok := newReqsByNameAndVariant[TVPair{newReq.Variant, newReq.Name}]; !ok {
				newReqs = append(newReqs, newReq)
				newReqsByNameAndVariant[TVPair{newReq.Variant, newReq.Name}] = struct{}{}
			}
		}
	}
	return newReqs, evalErrs
}
