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
		Usage: "prints the given project configuration with tags and included files expanded (excluding files included from a separate module)",
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
			cli.StringSliceFlag{
				Name:  joinFlagNames(localModulesFlagName, "lm"),
				Usage: "specify local modules for included files as MODULE_NAME=PATH pairs",
			},
		),
		Before: mergeBeforeFuncs(requirePathFlag),
		Action: func(c *cli.Context) error {
			path := c.String(pathFlagName)
			showTasks := c.Bool(taskFlagName)
			showVariants := c.Bool(variantsFlagName)
			diffable := c.Bool(diffableFlagName)
			localModulePaths := c.StringSlice(localModulesFlagName)
			localModuleMap, err := getLocalModulesFromInput(localModulePaths)
			if err != nil {
				return err
			}

			configBytes, err := os.ReadFile(path)
			if err != nil {
				return errors.Wrap(err, "reading project config")
			}

			p := &model.Project{}
			ctx := context.Background()
			opts := &model.GetProjectOpts{
				LocalModules: localModuleMap,
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

			var out any
			if showTasks || showVariants {
				tmp := struct {
					Functions any `yaml:"functions,omitempty"`
					Tasks     any `yaml:"tasks,omitempty"`
					Variants  any `yaml:"buildvariants,omitempty"`
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
