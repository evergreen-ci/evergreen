package model

import (
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

// GeneratedTasks is a subset of the Project type, and is generated from the
// JSON from a `generate.tasks` command.
type GeneratedTasks struct {
	BuildVariants []BuildVariant             `yaml:"buildvariants"`
	Tasks         []ProjectTask              `yaml:"tasks"`
	Functions     map[string]*YAMLCommandSet `yaml:"functions"`
}

// GeneratedTasksFromJSON returns a GeneratedTasks type from JSON. We use the
// YAML parser instead of the JSON parser because the JSON parser will not
// properly unmarshal into a struct with multiple fields as options, like the YAMLCommandSet.
func GeneratedTasksFromJSON(data []byte) (*GeneratedTasks, error) {
	g := &GeneratedTasks{}
	if err := yaml.Unmarshal(data, g); err != nil {
		return nil, errors.Wrap(err, "error unmarshaling into GeneratedTasks")
	}
	return g, nil
}
