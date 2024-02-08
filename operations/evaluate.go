package operations

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"gopkg.in/yaml.v3"
)

func Evaluate() cli.Command {
	const (
		taskFlagName     = "tasks"
		variantsFlagName = "variants"
		diffableFlagName = "diffable"
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
			},
			cli.BoolFlag{
				Name:  diffableFlagName,
				Usage: "show the project configuration in an ordered, diff-friendly format",
			},
		),
		Before: mergeBeforeFuncs(requirePathFlag),
		Action: func(c *cli.Context) error {
			path := c.String(pathFlagName)
			showTasks := c.Bool(taskFlagName)
			showVariants := c.Bool(variantsFlagName)
			diffable := c.Bool(diffableFlagName)

			configBytes, err := os.ReadFile(path)
			if err != nil {
				return errors.Wrap(err, "reading project config")
			}

			p := &model.Project{}
			ctx := context.Background()
			opts := &model.GetProjectOpts{
				ReadFileFrom: model.ReadFromLocal,
			}
			_, err = model.LoadProjectInto(ctx, configBytes, opts, "", p)
			if err != nil {
				return errors.Wrap(err, "loading project")
			}
			if diffable {
				sortTasksByName := model.ProjectTasksByName(p.Tasks)
				sort.Sort(sortTasksByName)
				p.Tasks = sortTasksByName

				sortBuildVariantsByName := model.BuildVariantsByName(p.BuildVariants)
				sort.Sort(sortBuildVariantsByName)
				p.BuildVariants = model.BuildVariants(sortBuildVariantsByName)
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
				return errors.Wrap(err, "marshalling evaluated project YAML")
			}

			fmt.Println(string(outYAML))
			return nil
		},
	}
}
