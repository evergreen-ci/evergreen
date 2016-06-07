package cli

import (
	"fmt"
	"io/ioutil"

	"github.com/evergreen-ci/evergreen/model"
	"gopkg.in/yaml.v2"
)

// EvaluateCommand reads in a project config, expanding tags and matrix definitions,
// then prints the expanded definitions back out as yaml.
type EvaluateCommand struct {
	Tasks    bool `short:"t" long:"tasks" description:"only show task and function definitions"`
	Variants bool `short:"v" long:"variants" description:"only show variant definitions"`
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
	err = model.LoadProjectInto(configBytes, "", p)
	if err != nil {
		return fmt.Errorf("error loading project: %v", err)
	}

	var out interface{}
	if ec.Tasks || ec.Variants {
		tmp := struct {
			Functions interface{} `yaml:"functions,omitempty"`
			Tasks     interface{} `yaml:"tasks,omitempty"`
			Variants  interface{} `yaml:"buildvariants,omitempty`
		}{}
		if ec.Tasks {
			tmp.Functions = p.Functions
			tmp.Tasks = p.Tasks
		}
		if ec.Variants {
			tmp.Variants = p.BuildVariants
		}
		out = tmp
	} else {
		out = p
	}

	outYAML, err := yaml.Marshal(out)
	if err != nil {
		return fmt.Errorf("error marshalling evaluated project YAML: %v", err)
	}
	fmt.Println(string(outYAML))

	return nil
}
