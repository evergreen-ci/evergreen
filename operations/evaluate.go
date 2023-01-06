package operations

import (
	"context"
	"fmt"
	"io/ioutil"
	"sort"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	yaml "gopkg.in/20210107192922/yaml.v3"
	upgradedYAML "gopkg.in/yaml.v3"
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
			}, cli.BoolFlag{
				Name:  useUpgradedYAMLFlagName,
				Usage: "use upgraded YAML version",
			}, cli.BoolFlag{
				Name:  diffableFlagName,
				Usage: "show the project configuration in an ordered, diff-friendly format",
			},
		),
		Before: mergeBeforeFuncs(requirePathFlag),
		Action: func(c *cli.Context) error {
			path := c.String(pathFlagName)
			showTasks := c.Bool(taskFlagName)
			showVariants := c.Bool(variantsFlagName)
			useUpgradedYAML := c.Bool(useUpgradedYAMLFlagName)
			diffable := c.Bool(diffableFlagName)

			configBytes, err := ioutil.ReadFile(path)
			if err != nil {
				return errors.Wrap(err, "reading project config")
			}

			p := &model.Project{}
			ctx := context.Background()
			opts := &model.GetProjectOpts{
				ReadFileFrom:    model.ReadFromLocal,
				UseUpgradedYAML: useUpgradedYAML,
			}
			_, err = model.LoadProjectInto(ctx, configBytes, opts, "", p)
			if err != nil {
				return errors.Wrap(err, "loading project")
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
					if diffable {
						sortTasksByName := model.ProjectTasksByName(p.Tasks)
						sort.Sort(sortTasksByName)
						tmp.Tasks = sortTasksByName
					} else {
						tmp.Tasks = p.Tasks
					}
				}
				if showVariants {
					if diffable {
						sortBVsByName := model.BuildVariantsByName(p.BuildVariants)
						sort.Sort(sortBVsByName)
						tmp.Variants = sortBVsByName
					} else {
						tmp.Variants = p.BuildVariants
					}
				}
				out = tmp
			} else if diffable {
				// TODO (EVG-17291): remove the diffable and useUpgradedYAML
				// flags once the YAML is upgraded.
				//
				// Include all fields in the project config that potentially
				// have nested maps because those are affected by the bug fixed
				// in the YAML upgrade.

				tmp := struct {
					Functions  interface{} `yaml:"functions,omitempty"`
					Tasks      interface{} `yaml:"tasks,omitempty"`
					Variants   interface{} `yaml:"buildvariants,omitempty"`
					TaskGroups interface{} `yaml:"task_groups,omitempty"`
					Parameters interface{} `yaml:"parameters,omitempty"`
					Pre        interface{} `yaml:"pre,omitempty"`
					Post       interface{} `yaml:"post,omitempty"`
					Timeout    interface{} `yaml:"timeout,omitempty"`
				}{}

				// In order to make the project config easily diffable,
				// evergreen evaluate has to always produce the same output for
				// the same input (i.e. if you pass in the same YAML file
				// multiple times, it should always produce the same output).
				// LoadProjectInto does not provide this kind of guarantee, and
				// may produce some out-of-order results for unordered lists.
				//
				// To consistently produce the same output, sort fields that are
				// lists where order may be shuffled by LoadProjectInto. For
				// example, build variants are a list where order is not
				// important, and LoadProjectInto shuffles the order. In
				// contrast, Pre commands are an ordered list, so they already
				// retain their ordering.

				if p.Functions != nil {
					tmp.Functions = p.Functions
				}

				if p.Tasks != nil {
					sortTasksByName := model.ProjectTasksByName(p.Tasks)
					sort.Sort(sortTasksByName)
					tmp.Tasks = sortTasksByName
				}

				if p.BuildVariants != nil {
					sortBVsByName := model.BuildVariantsByName(p.BuildVariants)
					sort.Sort(sortBVsByName)
					tmp.Variants = sortBVsByName
				}

				if p.TaskGroups != nil {
					sortTaskGroupsByName := model.TaskGroupsByName(p.TaskGroups)
					sort.Sort(sortTaskGroupsByName)
					tmp.TaskGroups = sortTaskGroupsByName
				}

				if p.Parameters != nil {
					tmp.Parameters = p.Parameters
				}

				if p.Pre != nil {
					tmp.Pre = p.Pre
				}
				if p.Post != nil {
					tmp.Post = p.Post
				}
				if p.Timeout != nil {
					tmp.Timeout = p.Timeout
				}

				out = tmp
			} else {
				out = p
			}

			var outYAML []byte
			if useUpgradedYAML {
				outYAML, err = upgradedYAML.Marshal(out)
				if err != nil {
					return errors.Wrap(err, "marshalling evaluated project YAML with upgraded YAML version")
				}
			} else {
				outYAML, err = yaml.Marshal(out)
				if err != nil {
					return errors.Wrap(err, "marshalling evaluated project YAML")
				}
			}

			fmt.Println(string(outYAML))
			return nil
		},
	}
}
