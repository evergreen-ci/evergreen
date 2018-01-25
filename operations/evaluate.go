package operations

import (
	"fmt"
	"io/ioutil"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	yaml "gopkg.in/yaml.v2"
)

func Evaluate() cli.Command {
	const (
		taskFlagName     = "tasks"
		variantsFlagName = "variants"
	)

	return cli.Command{
		Name:  "evaluate",
		Usage: "reads a project configuration and expands tags and matrix definitions, printing the expanded definitions",
		Flags: addPathFlag(
			cli.BoolFlag{
				Name:  taskFlagName,
				Usage: "only show task and function definitions",
			},
			cli.BoolFlag{
				Name:  variantsFlagName,
				Usage: "only show variant definitions",
			}),
		Before: requirePathFlag,
		Action: func(c *cli.Context) error {
			path := c.String(pathFlagName)
			showTasks := c.Bool(taskFlagName)
			showVariants := c.Bool(variantsFlagName)

			configBytes, err := ioutil.ReadFile(path)
			if err != nil {
				return errors.Wrap(err, "error reading project config")
			}

			p := &model.Project{}
			err = model.LoadProjectInto(configBytes, "", p)
			if err != nil {
				return errors.Wrap(err, "error loading project")
			}

			var out interface{}
			if showTasks || showVariants {
				tmp := struct {
					Functions interface{} `yaml:"functions,omitempty"`
					Tasks     interface{} `yaml:"tasks,omitempty"`
					Variants  interface{} `yaml:"buildvariants,omitempty"`
				}{}
				if showTasks {
					tmp.Functions = p.Functions
					tmp.Tasks = p.Tasks
				}
				if showVariants {
					tmp.Variants = p.BuildVariants
				}
				out = tmp
			} else {
				out = p
			}

			outYAML, err := yaml.Marshal(out)
			if err != nil {
				return errors.Wrap(err, "error marshaling evaluated project YAML")
			}

			fmt.Println(string(outYAML))
			return nil
		},
	}
}
