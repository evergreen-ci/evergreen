package model

import (
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// Make represents the configuration for Make-based projects.
type Make struct {
	TargetSequences []MakeTargetSequence `yaml:"sequences,omitempty"`
	Tasks           []MakeTask           `yaml:"tasks,omitempty"`
	Variants        []MakeVariant        `yaml:"variants"`
	DefaultTags     []string             `yaml:"default_tags,omitempty"`
	// Environment defines global environment variables. Definitions can be
	// overridden at the task or variant level.
	Environment map[string]string `yaml:"environment,omitempty"`

	// WorkingDirectory determines the project working directory where the
	// project will be cloned.
	WorkingDirectory string `yaml:"-"`
}

// NewMake creates a new evergreen config generator for Make from a single file
// that contains all the necessary generation information.
func NewMake(file, workingDir string) (*Make, error) {
	m := Make{
		WorkingDirectory: workingDir,
	}
	if err := utility.ReadYAMLFileStrict(file, &m); err != nil {
		return nil, errors.Wrap(err, "unmarshalling from YAML file")
	}

	m.ApplyDefaultTags()

	if err := m.Validate(); err != nil {
		return nil, errors.Wrap(err, "make generator configuration")
	}

	return &m, nil
}

// ApplyDefaultTags applies all the default tags to the existing tasks, subject
// to task-level exclusion rules.
func (m *Make) ApplyDefaultTags() {
	for _, tag := range m.DefaultTags {
		for i, mt := range m.Tasks {
			if !utility.StringSliceContains(mt.Tags, tag) && !utility.StringSliceContains(mt.ExcludeTags, tag) {
				m.Tasks[i].Tags = append(m.Tasks[i].Tags, tag)
			}
		}
	}
}

// MergeTargetSequences merges target sequence definitions with the existing
// ones by sequence name. For a given sequence name, existing targets are
// overwritten if they are already defined.
func (m *Make) MergeTargetSequences(mtss ...MakeTargetSequence) *Make {
	for _, mts := range mtss {
		if _, i, err := m.GetTargetSequenceIndexByName(mts.Name); err == nil {
			m.TargetSequences[i] = mts
		} else {
			m.TargetSequences = append(m.TargetSequences, mts)
		}
	}
	return m
}

// MergeTasks merges task definitions with the existing ones by task name. For a
// given task name, existing tasks are overwritten if they are already defined.
func (m *Make) MergeTasks(mts ...MakeTask) *Make {
	for _, mt := range mts {
		if _, i, err := m.GetTaskIndexByName(mt.Name); err == nil {
			m.Tasks[i] = mt
		} else {
			m.Tasks = append(m.Tasks, mt)
		}
	}
	return m
}

// MergeVariantDistros merges variant-distro mappings with the existing ones by
// variant name. For a given variant name, existing variant-distro mappings are
// overwritten if they are already defined.
func (m *Make) MergeVariantDistros(vds ...VariantDistro) *Make {
	for _, vd := range vds {
		if mv, i, err := m.GetVariantIndexByName(vd.Name); err == nil {
			mv.VariantDistro = vd
			m.Variants[i] = *mv
		} else {
			m.Variants = append(m.Variants, MakeVariant{
				VariantDistro: vd,
			})
		}
	}
	return m
}

// MergeVariantParameters merges variant parameters with the existing ones by
// name. For a given variant name, existing variant options are overwritten if
// they are already defined.
func (m *Make) MergeVariantParameters(nmvps ...NamedMakeVariantParameters) *Make {
	for _, nmvp := range nmvps {
		if mv, i, err := m.GetVariantIndexByName(nmvp.Name); err == nil {
			mv.MakeVariantParameters = nmvp.MakeVariantParameters
			m.Variants[i] = *mv
		} else {
			m.Variants = append(m.Variants, MakeVariant{
				VariantDistro:         VariantDistro{Name: nmvp.Name},
				MakeVariantParameters: nmvp.MakeVariantParameters,
			})
		}
	}
	return m
}

// MergeEnvironments merges the given environments with the existing environment
// variables. Duplicates are overwritten in the order in which environments are
// passed into the function.
func (m *Make) MergeEnvironments(envs ...map[string]string) *Make {
	m.Environment = MergeEnvironments(append([]map[string]string{m.Environment}, envs...)...)
	return m
}

// MergeDefaultTags merges the given tags with the existing default tags.
// Duplicates are ignored.
func (m *Make) MergeDefaultTags(tags ...string) *Make {
	for _, tag := range tags {
		if !utility.StringSliceContains(m.DefaultTags, tag) {
			m.DefaultTags = append(m.DefaultTags, tag)
		}
	}
	return m
}

// Validate checks that the entire Make build configuration is valid.
func (m *Make) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.Wrap(m.validateTargetSequences(), "invalid target sequence definitions")
	catcher.Wrap(m.validateTasks(), "invalid task definitions")
	catcher.Wrap(m.validateVariants(), "invalid variant definitions")
	return catcher.Resolve()
}

// validateTargetSequences checks that:
// - Each target name is unique.
// - Each target definition is valid.
func (m *Make) validateTargetSequences() error {
	catcher := grip.NewBasicCatcher()
	seqNames := map[string]struct{}{}
	for _, mts := range m.TargetSequences {
		if _, ok := seqNames[mts.Name]; ok {
			catcher.Errorf("duplicate target sequence '%s'", mts.Name)
		}
		catcher.Wrapf(mts.Validate(), "invalid target sequence '%s'", mts.Name)
	}
	return catcher.Resolve()
}

// validateTasks checks that:
// - Tasks are defined.
// - Each task name is unique.
// - Each task definition is valid.
// - Each target referencing a sequence is a defined sequence.
func (m *Make) validateTasks() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(len(m.Tasks) == 0, "must have at least one task")
	taskNames := map[string]struct{}{}
	for _, mt := range m.Tasks {
		if _, ok := taskNames[mt.Name]; ok {
			catcher.Errorf("duplicate task name '%s'", mt.Name)
		}
		taskNames[mt.Name] = struct{}{}
		catcher.Wrapf(mt.Validate(), "invalid task '%s'", mt.Name)

		for _, mtt := range mt.Targets {
			if mtt.Sequence != "" {
				_, _, err := m.GetTargetSequenceIndexByName(mtt.Sequence)
				catcher.Wrapf(err, "invalid target sequence reference '%s' in task '%s'", mtt.Sequence, mt.Name)
			}
		}
	}
	return catcher.Resolve()
}

// validateVariants checks that:
// - Variants are defined.
// - Each variant name is unique.
// - Each task referenced in a variant has a defined task.
// - Each variant does not specify a duplicate task.
func (m *Make) validateVariants() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(len(m.Variants) == 0, "must have at least one variant")
	varNames := map[string]struct{}{}
	for _, mv := range m.Variants {
		catcher.Wrapf(mv.Validate(), "invalid definitions for variant '%s'", mv.Name)

		if _, ok := varNames[mv.Name]; ok {
			catcher.Errorf("cannot have duplicate variant name '%s'", mv.Name)
		}
		varNames[mv.Name] = struct{}{}

		taskNames := map[string]struct{}{}
		for _, mvt := range mv.Tasks {
			tasks, err := m.GetTasksFromRef(mvt)
			if err != nil {
				catcher.Wrapf(err, "invalid task reference in variant '%s'", mv.Name)
				continue
			}
			for _, mt := range tasks {
				if _, ok := taskNames[mt.Name]; ok {
					catcher.Errorf("duplicate reference to task '%s' in variant '%s'", mt.Name, mv.Name)
				}
				taskNames[mt.Name] = struct{}{}
			}
		}
	}
	return catcher.Resolve()
}

// GetTargets returns the resolved targets from the reference specified in the
// given MakeTaskTarget.
func (m *Make) GetTargetsFromRef(mtt MakeTaskTarget) ([]string, error) {
	if mtt.Name != "" {
		return []string{mtt.Name}, nil
	}
	if mtt.Sequence != "" {
		mts, _, err := m.GetTargetSequenceIndexByName(mtt.Sequence)
		if err != nil {
			return nil, errors.Wrapf(err, "finding definition for target sequence '%s'", mtt.Sequence)
		}
		return mts.Targets, nil
	}
	return nil, errors.New("empty target reference")
}

// GetTargetSequenceIndexByName finds the target sequence by name and returns
// the target sequence definition and its index.
func (m *Make) GetTargetSequenceIndexByName(name string) (mts *MakeTargetSequence, index int, err error) {
	for i, ts := range m.TargetSequences {
		if ts.Name == name {
			return &ts, i, nil
		}
	}
	return nil, -1, errors.Errorf("target sequence with name '%s' not found", name)
}

// GetTasks returns the tasks that match the reference specified in the
// given MakeVariantTask.
func (m *Make) GetTasksFromRef(mvt MakeVariantTask) ([]MakeTask, error) {
	if mvt.Tag != "" {
		tasks := m.GetTasksByTag(mvt.Tag)
		if len(tasks) == 0 {
			return nil, errors.Errorf("no tasks matched tag '%s'", mvt.Tag)
		}
		return tasks, nil
	}

	if mvt.Name != "" {
		task, _, err := m.GetTaskIndexByName(mvt.Name)
		if err != nil {
			return nil, errors.Wrapf(err, "finding definition for task '%s'", mvt.Name)
		}
		return []MakeTask{*task}, nil
	}

	return nil, errors.New("empty task reference")
}

// GetTaskIndexByName finds the task by name and returns the task definition and
// its index.
func (m *Make) GetTaskIndexByName(name string) (mt *MakeTask, index int, err error) {
	for i, t := range m.Tasks {
		if t.Name == name {
			return &t, i, nil
		}
	}
	return nil, -1, errors.Errorf("task with name '%s' not found", name)
}

// GetTasksByTag finds the definition of tasks matching the given tag.
func (m *Make) GetTasksByTag(tag string) []MakeTask {
	var tasks []MakeTask
	for _, mt := range m.Tasks {
		if utility.StringSliceContains(mt.Tags, tag) {
			tasks = append(tasks, mt)
		}
	}
	return tasks
}

// GetVariantIndexByName finds the variant by name and returns the variant
// definition and its index.
func (m *Make) GetVariantIndexByName(name string) (mv *MakeVariant, index int, err error) {
	for i, v := range m.Variants {
		if v.Name == name {
			return &v, i, nil
		}
	}
	return nil, -1, errors.Errorf("variant with name '%s' not found", name)
}

// MakeTargetSequence represents an ordered sequence of targets to execute.
type MakeTargetSequence struct {
	Name    string   `yaml:"name"`
	Targets []string `yaml:"targets"`
}

func (mts *MakeTargetSequence) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(mts.Name == "", "missing target sequence name")
	catcher.NewWhen(len(mts.Targets) == 0, "must specify at least one target")
	return catcher.Resolve()
}

// MakeTask represents a task that runs a group of Make targets.
type MakeTask struct {
	// Name is the name of the task.
	Name string `yaml:"name"`
	// Targets are the names of targets or sequences of targets to run.
	Targets []MakeTaskTarget `yaml:"targets"`
	// Tags are labels that allow you to logically group related tasks.
	Tags []string `yaml:"tags,omitempty"`
	// ExcludeTags allows you to specify tags that should not be applied to the
	// task. This can be useful for excluding a task from the default tags.
	ExcludeTags []string `yaml:"exclude_tags,omitempty"`
	// Environment defines task-specific environment variables. This has higher
	// precedence than global environment variables but lower precedence than
	// variant-specific environment variables.
	Environment map[string]string `yaml:"environment,omitempty"`
	// Options are task-specific options that modify runtime execution. If
	// options are specified at the target level, these options will be appended.
	Options MakeRuntimeOptions `yaml:"options,omitempty"`
	// Report describe how test results are reported after the task is
	// complete.
	Reports []FileReport `yaml:"reports,omitempty"`
}

// MakeTarget represents a reference to a single Make target or a sequence of
// targets.
type MakeTaskTarget struct {
	// Name is the name of target to run.
	Name string `yaml:"name"`
	// Sequences is a reference to a defined target sequence.
	Sequence string `yaml:"sequence"`
	// Options are target-specific options that modify runtime execution.
	Options MakeRuntimeOptions `yaml:"options,omitempty"`
	// Reports describe how output files are reported after running the target.
	Reports []FileReport `yaml:"reports,omitempty"`
}

// Validate checks that exactly one kind of reference is specified in a target
// reference for a task.
func (mtt *MakeTaskTarget) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(mtt.Name == "" && mtt.Sequence == "", "must specify either a target name or sequence")
	catcher.NewWhen(mtt.Name != "" && mtt.Sequence != "", "cannot specify both a target name and sequence")
	for _, fr := range mtt.Reports {
		catcher.Wrap(fr.Validate(), "invalid file report specification")
	}
	return catcher.Resolve()
}

// Validate checks that targets are valid and all tags are unique.
func (mt *MakeTask) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(mt.Name == "", "missing task name")
	catcher.NewWhen(len(mt.Targets) == 0, "must specify at least one target")
	for _, mtt := range mt.Targets {
		catcher.Wrap(mtt.Validate(), "invalid target")
	}
	tags := map[string]struct{}{}
	for _, tag := range mt.Tags {
		if _, ok := tags[tag]; ok {
			catcher.Errorf("duplicate tag '%s'", tag)
		}
		tags[tag] = struct{}{}
	}
	for _, fr := range mt.Reports {
		catcher.Wrap(fr.Validate(), "invalid file report specification")
	}
	return catcher.Resolve()
}

// MakeRuntimeOptions specify additional optional to the make binary to modify
// the behavior of runtime execution.
type MakeRuntimeOptions []string

// Merge appends the new runtime options to the existing ones. Duplicates and
// conflicting flags are appended.
func (mro MakeRuntimeOptions) Merge(toAdd ...MakeRuntimeOptions) MakeRuntimeOptions {
	merged := mro
	for _, opts := range toAdd {
		merged = append(merged, opts...)
	}
	return merged
}

// MakeVariant defines a variant that runs Make tasks.
type MakeVariant struct {
	VariantDistro         `yaml:",inline"`
	MakeVariantParameters `yaml:",inline"`
	// Options are variant-specific options that modify runtime execution. If
	// options are specified at the task or target level, these options will be
	// appended.
	Options MakeRuntimeOptions `yaml:"options,omitempty"`
}

// Validate checks that the variant-distro mapping and the Make-specific
// parameters are valid.
func (mv *MakeVariant) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(mv.VariantDistro.Validate())
	catcher.Add(mv.MakeVariantParameters.Validate())
	return catcher.Resolve()
}

// MakeVariantParameters describes Make-specific variant configuration.
type MakeVariantParameters struct {
	Tasks []MakeVariantTask `yaml:"tasks"`
	// Environment defines variant-specific environment variables. This has
	// higher precedence than global or task-specific environment variables.
	Environment map[string]string `yaml:"environment,omitempty"`
}

// Validate checks that task references are specified and each reference is
// valid.
func (mvp *MakeVariantParameters) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(len(mvp.Tasks) == 0, "must specify at least one task")
	taskNames := map[string]struct{}{}
	taskTags := map[string]struct{}{}
	for _, mvt := range mvp.Tasks {
		catcher.Wrap(mvt.Validate(), "invalid task reference")
		if mvt.Name != "" {
			if _, ok := taskNames[mvt.Name]; ok {
				catcher.Errorf("duplicate reference to task name '%s'", mvt.Name)
			}
			taskNames[mvt.Name] = struct{}{}
		}
		if mvt.Tag != "" {
			if _, ok := taskTags[mvt.Tag]; ok {
				catcher.Errorf("duplicate reference to task tag '%s'", mvt.Tag)
			}
			taskTags[mvt.Tag] = struct{}{}
		}
	}
	return catcher.Resolve()
}

// NamedMakeVariantParameters describes Make-specific variant configuration
// associated with a particular variant name.
type NamedMakeVariantParameters struct {
	// Name is the variant name.
	Name                  string `yaml:"name"`
	MakeVariantParameters `yaml:",inline"`
}

// Validate checks that there is a variant name and valid parameters.
func (nmvp *NamedMakeVariantParameters) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(nmvp.Name == "", "must specify variant name")
	catcher.Add(nmvp.MakeVariantParameters.Validate())
	return catcher.Resolve()
}

// MakeVariantTask represents either a reference to a task by task name, or a
// group of tasks containing a particular tag.
type MakeVariantTask struct {
	Name string `yaml:"name,omitempty"`
	Tag  string `yaml:"tag,omitempty"`
}

// Validate checks that exactly one kind of reference is specified in a task
// reference for a variant.
func (mvt *MakeVariantTask) Validate() error {
	if mvt.Name == "" && mvt.Tag == "" {
		return errors.New("must specify either a task name or tag")
	}
	if mvt.Name != "" && mvt.Tag != "" {
		return errors.New("cannot specify both task name and tag")
	}
	return nil
}
