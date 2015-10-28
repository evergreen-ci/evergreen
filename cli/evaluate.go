package cli

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/model"
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

type evalMode string

const (
	All      evalMode = "all"
	Tasks    evalMode = "tasks"
	Variants evalMode = "variants"
)

// EvaluateCommand reads in a project config, expanding tags and matrix definitions,
// then prints the expanded definitions back out as yaml.
type EvaluateCommand struct {
	// Mode defines which portions of the project to print
	Mode evalMode
}

func (ec *EvaluateCommand) Execute(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("the evaluate command takes one project config path as an argument")
	}
	configBytes, err := ioutil.ReadFile(args[0])
	if err != nil {
		return fmt.Errorf("error reading project config: %v", err)
	}

	p := &model.Project{}
	err = yaml.Unmarshal(configBytes, p)
	if err != nil {
		return fmt.Errorf("error parsing project YAML: %v", err)
	}

	err = p.EvaluateTags()
	if err != nil {
		return err
	}

	var out interface{}
	switch ec.Mode {
	case All:
		out = p
	case Tasks:
		out = struct {
			Functions interface{} `yaml:"functions,omitempty"`
			Tasks     interface{} `yaml:"tasks,omitempty"`
		}{p.Functions, p.Tasks}
	case Variants:
		out = p.BuildVariants
	}

	outYAML, err := yaml.Marshal(out)
	if err != nil {
		return fmt.Errorf("error marshalling evaluated project YAML: %v", err)
	}
	fmt.Println(string(outYAML))

	return nil
}
