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
	GeneralFile  string   `yaml:"general,omitempty"`
	VariantFiles []string `yaml:"variants"`
	TaskFiles    []string `yaml:"tasks"`

	ControlDirectory string `yaml:"-"`
}

// NewMakeControl creates a new representation of a Make control file from the
// given file.
func NewMakeControl(file string) (*MakeControl, error) {
	mc := MakeControl{
		ControlDirectory: util.ConsistentFilepath(filepath.Dir(file)),
	}
	if err := utility.ReadYAMLFileStrict(file, &mc); err != nil {
		return nil, errors.Wrap(err, "unmarshalling from YAML file")
	}
	return &mc, nil
}

// Build creates a Make model from the files referenced in the MakeControl.
func (mc *MakeControl) Build() (*Make, error) {
	var m Make

	mts, mtss, err := mc.buildTasksAndTargetSequences()
	if err != nil {
		return nil, errors.Wrap(err, "building target definitions")
	}
	m.MergeTasks(mts...)
	m.MergeTargetSequences(mtss...)

	mvs, err := mc.buildVariants()
	if err != nil {
		return nil, errors.Wrap(err, "building variant definitions")
	}
	m.MergeVariants(mvs...)

	gc, err := mc.buildGeneral()
	if err != nil {
		return nil, errors.Wrap(err, "building top-level configuration")
	}
	m.GeneralConfig = gc

	m.ApplyDefaultTags()

	if err := m.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid build configuration")
	}

	return &m, nil
}

func (mc *MakeControl) buildTasksAndTargetSequences() ([]MakeTask, []MakeTargetSequence, error) {
	var allTasks []MakeTask
	var allTargetSequences []MakeTargetSequence

	if err := withMatchingFiles(mc.ControlDirectory, mc.TaskFiles, func(file string) error {
		mtsv := struct {
			Tasks            []MakeTask           `yaml:"tasks"`
			TargetSequences  []MakeTargetSequence `yaml:"target_sequences,omitempty"`
			VariablesSection `yaml:",inline"`
		}{}
		if err := utility.ReadYAMLFileStrict(file, &mtsv); err != nil {
			return errors.Wrap(err, "unmarshalling from YAML file")
		}
		mts := mtsv.Tasks
		mtss := mtsv.TargetSequences

		catcher := grip.NewBasicCatcher()
		for _, mt := range mts {
			catcher.Wrapf(mt.Validate(), "task '%s'", mt.Name)
		}
		for _, mts := range mtss {
			catcher.Wrapf(mts.Validate(), "target '%s'", mts.Name)
		}
		if catcher.HasErrors() {
			return errors.Wrap(catcher.Resolve(), "invalid task/target definition(s)")
		}

		allTasks = append(allTasks, mts...)
		allTargetSequences = append(allTargetSequences, mtss...)

		return nil
	}); err != nil {
		return nil, nil, errors.WithStack(err)
	}

	return allTasks, allTargetSequences, nil
}

func (mc *MakeControl) buildVariants() ([]MakeVariant, error) {
	var all []MakeVariant

	if err := withMatchingFiles(mc.ControlDirectory, mc.VariantFiles, func(file string) error {
		mvsv := struct {
			Variants         []MakeVariant `yaml:",inline"`
			VariablesSection `yaml:",inline"`
		}{}
		if err := utility.ReadYAMLFileStrict(file, &mvsv); err != nil {
			return errors.Wrap(err, "unmarshalling from YAML file")
		}
		mvs := mvsv.Variants

		catcher := grip.NewBasicCatcher()
		for _, mv := range mvs {
			catcher.Wrapf(mv.Validate(), "variant '%s'", mv.Name)
		}
		if catcher.HasErrors() {
			return errors.Wrap(catcher.Resolve(), "invalid variant definition(s)")
		}

		all = append(all, mvs...)

		return nil
	}); err != nil {
		return nil, errors.WithStack(err)
	}

	return all, nil
}

func (mc *MakeControl) buildGeneral() (GeneralConfig, error) {
	gcv := struct {
		GeneralConfig    `yaml:"general"`
		VariablesSection `yaml:",inline"`
	}{}
	if err := utility.ReadYAMLFileStrict(mc.GeneralFile, &gcv); err != nil {
		return GeneralConfig{}, nil
	}
	gc := gcv.GeneralConfig

	return gc, nil
}
