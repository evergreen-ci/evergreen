package model

import "github.com/mongodb/grip"

// VariablesSection defines an optional variables section that can be used
// within YAML files to define additional structures (e.g. anchors).
type VariablesSection struct {
	Variables interface{} `yaml:"variables,omitempty"`
}

// GeneralConfig defines common top-level configuration between different
// generator types.
type GeneralConfig struct {
	// Environment defines global environment variables.
	Environment map[string]string `yaml:"env,omitempty"`
	// DefaultTags are tags that apply to all units of work.
	DefaultTags      []string `yaml:"default_tags,omitempty"`
	WorkingDirectory string   `yaml:"working_dir,omitempty"`
}

// VariantDistro represents a mapping between a variant name and the distros
// that it runs on.
type VariantDistro struct {
	Name    string   `yaml:"name"`
	Distros []string `yaml:"distros"`
}

// Validate checks that the variant name is set and distros are specified.
func (vd *VariantDistro) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(vd.Name == "", "missing variant name")
	catcher.NewWhen(len(vd.Distros) == 0, "need to specify at least one distro")
	return catcher.Resolve()
}

// MergeEnvironments returns the merged environment variable mappings where the
// given input environments are ordered in increasing priority. If there are
// duplicate environment variables, in the environments, the variable definition
// of the higher priority environment takes precedence.
func MergeEnvironments(envsByPriority ...map[string]string) map[string]string {
	merged := map[string]string{}
	for _, env := range envsByPriority {
		for k, v := range env {
			merged[k] = v
		}
	}
	return merged
}

// FileReport defines options to report output files.
type FileReport struct {
	// ReportFiles are the files used to create the report.
	Files []string `yaml:"files"`
	// Format is the kind of format of the files to report.
	Format ReportFormat `yaml:"format"`
}

// Validate checks that the file report contains files and the format is valid.
func (fr *FileReport) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(len(fr.Files) == 0, "must specify at least one file to report")
	catcher.Wrap(fr.Format.Validate(), "invalid report format")
	catcher.NewWhen(fr.Format == EvergreenJSON && len(fr.Files) != 1, "evergreen JSON results format requires exactly one file to be reported")
	return catcher.Resolve()
}
