package model

import "github.com/mongodb/grip"

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
