package model

import (
	"path/filepath"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/util"
	"github.com/pkg/errors"
)

// MakeControl represents a control file which can be used to build a Make
// generator from multiple files containing the necessary build configuration.
type MakeControl struct {
	TargetSequenceFiles   []string `yaml:"target_sequence_files"`
	TaskFiles             []string `yaml:"task_files"`
	VariantDistroFiles    []string `yaml:"variant_distro_files"`
	VariantParameterFiles []string `yaml:"variant_parameter_files"`
	EnvironmentFiles      []string `yaml:"environment_files,omitempty"`
	DefaultTagFiles       []string `yaml:"default_tag_files,omitempty"`
	TestResultsFiles      []string `yaml:"test_results_files,omitempty"`

	WorkingDirectory string `yaml:"-"`
	ControlDirectory string `yaml:"-"`
}

// NewMakeControl creates a new representation of a Make control file from the
// given file.
func NewMakeControl(file, workingDir string) (*MakeControl, error) {
	mc := MakeControl{
		ControlDirectory: util.ConsistentFilepath(filepath.Dir(file)),
		WorkingDirectory: workingDir,
	}
	if err := utility.ReadYAMLFileStrict(file, &mc); err != nil {
		return nil, errors.Wrap(err, "unmarshalling from YAML file")
	}
	return &mc, nil
}

// Build creates a Make model from the files referenced in the MakeControl.
func (mc *MakeControl) Build() (*Make, error) {
	var m Make

	mtss, err := mc.buildTargetSequences()
	if err != nil {
		return nil, errors.Wrap(err, "building target sequence definitions")
	}
	_ = m.MergeTargetSequences(mtss...)

	mts, err := mc.buildTasks()
	if err != nil {
		return nil, errors.Wrap(err, "building target definitions")
	}
	_ = m.MergeTasks(mts...)

	vds, err := mc.buildVariantDistros()
	if err != nil {
		return nil, errors.Wrap(err, "building variant-distro mappings")
	}
	_ = m.MergeVariantDistros(vds...)

	nmvps, err := mc.buildVariantParameters()
	if err != nil {
		return nil, errors.Wrap(err, "building variant parameters")
	}
	_ = m.MergeVariantParameters(nmvps...)

	envs, err := mc.buildEnvironments()
	if err != nil {
		return nil, errors.Wrap(err, "building environment variables")
	}
	_ = m.MergeEnvironments(envs...)

	tags, err := mc.buildDefaultTags()
	if err != nil {
		return nil, errors.Wrap(err, "building default tags")
	}
	_ = m.MergeDefaultTags(tags...)

	m.WorkingDirectory = mc.WorkingDirectory

	m.ApplyDefaultTags()

	if err := m.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid build configuration")
	}

	return &m, nil
}

func (mc *MakeControl) buildTargetSequences() ([]MakeTargetSequence, error) {
	var all []MakeTargetSequence

	if err := withMatchingFiles(mc.ControlDirectory, mc.TargetSequenceFiles, func(file string) error {
		mtss := []MakeTargetSequence{}
		if err := utility.ReadYAMLFileStrict(file, &mtss); err != nil {
			return errors.Wrap(err, "unmarshalling from YAML file")
		}

		catcher := grip.NewBasicCatcher()
		for _, mts := range mtss {
			catcher.Wrapf(mts.Validate(), "target sequence '%s'", mts.Name)
		}
		if catcher.HasErrors() {
			return catcher.Resolve()
		}

		all = append(all, mtss...)

		return nil
	}); err != nil {
		return nil, errors.WithStack(err)
	}

	return all, nil
}

func (mc *MakeControl) buildTasks() ([]MakeTask, error) {
	var all []MakeTask

	if err := withMatchingFiles(mc.ControlDirectory, mc.TaskFiles, func(file string) error {
		mts := []MakeTask{}
		if err := utility.ReadYAMLFileStrict(file, &mts); err != nil {
			return errors.Wrap(err, "unmarshalling from YAML file")
		}

		catcher := grip.NewBasicCatcher()
		for _, mt := range mts {
			catcher.Wrapf(mt.Validate(), "task '%s'", mt.Name)
		}
		if catcher.HasErrors() {
			return errors.Wrap(catcher.Resolve(), "invalid task definitions")
		}

		all = append(all, mts...)

		return nil
	}); err != nil {
		return nil, errors.WithStack(err)
	}

	return all, nil
}

func (mc *MakeControl) buildVariantDistros() ([]VariantDistro, error) {
	var all []VariantDistro

	if err := withMatchingFiles(mc.ControlDirectory, mc.VariantDistroFiles, func(file string) error {
		vds := []VariantDistro{}
		if err := utility.ReadYAMLFileStrict(file, &vds); err != nil {
			return errors.Wrap(err, "unmarshalling from YAML file")
		}

		catcher := grip.NewBasicCatcher()
		for _, vd := range vds {
			catcher.Wrapf(vd.Validate(), "variant '%s'", vd.Name)
		}
		if catcher.HasErrors() {
			return errors.Wrap(catcher.Resolve(), "invalid variant-distro mappings")
		}

		all = append(all, vds...)

		return nil
	}); err != nil {
		return nil, errors.WithStack(err)
	}

	return all, nil
}

func (mc *MakeControl) buildVariantParameters() ([]NamedMakeVariantParameters, error) {
	var all []NamedMakeVariantParameters

	if err := withMatchingFiles(mc.ControlDirectory, mc.VariantParameterFiles, func(file string) error {
		nmvps := []NamedMakeVariantParameters{}
		if err := utility.ReadYAMLFileStrict(file, &nmvps); err != nil {
			return errors.Wrap(err, "unmarshalling from YAML file")
		}

		catcher := grip.NewBasicCatcher()
		for _, nmvp := range nmvps {
			catcher.Wrapf(nmvp.Validate(), "variant '%s'", nmvp.Name)
		}
		if catcher.HasErrors() {
			return errors.Wrap(catcher.Resolve(), "invalid variant parameters")
		}

		all = append(all, nmvps...)

		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "building variant parameters")
	}

	return all, nil
}

func (mc *MakeControl) buildEnvironments() ([]map[string]string, error) {
	var all []map[string]string

	if err := withMatchingFiles(mc.ControlDirectory, mc.EnvironmentFiles, func(file string) error {
		env := map[string]string{}
		if err := utility.ReadYAMLFileStrict(file, &env); err != nil {
			return errors.Wrap(err, "unmarshalling from YAML file")
		}

		all = append(all, env)

		return nil
	}); err != nil {
		return nil, errors.WithStack(err)
	}

	return all, nil
}

func (mc *MakeControl) buildDefaultTags() ([]string, error) {
	var all []string
	if err := withMatchingFiles(mc.ControlDirectory, mc.DefaultTagFiles, func(file string) error {
		tags := []string{}
		if err := utility.ReadYAMLFileStrict(file, &tags); err != nil {
			return errors.Wrap(err, "unmarshalling from YAML file")
		}

		all = append(all, tags...)

		return nil
	}); err != nil {
		return nil, errors.WithStack(err)
	}

	return all, nil
}
