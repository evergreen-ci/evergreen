package operations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/urfave/cli"
)

func List() cli.Command {
	const (
		projectsFlagName  = "projects"
		variantsFlagName  = "variants"
		tasksFlagName     = "tasks"
		distrosFlagName   = "distros"
		spawnableFlagName = "spawnable"
	)

	return cli.Command{
		Name:  "list",
		Usage: "",
		Flags: addPathFlag(addProjectFlag(
			cli.BoolFlag{
				Name:  projectsFlagName,
				Usage: "project whose variants or tasks should be listed (use with --variants/--tasks)",
			},
			cli.BoolFlag{
				Name:  variantsFlagName,
				Usage: "path to config file whose variants or tasks should be listed (use with --variants/--tasks)",
			},
			cli.BoolFlag{
				Name:  tasksFlagName,
				Usage: "list all tasks for a project",
			},
			cli.BoolFlag{
				Name:  distrosFlagName,
				Usage: "list all distros for a project",
			},
			cli.BoolFlag{
				Name:  spawnableFlagName,
				Usage: "list all spawnable distros for a project",
			})...),
		Before: requireOnlyOneBool(projectsFlagName, variantsFlagName, tasksFlagName, distrosFlagName, spawnableFlagName),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)

			// do setup

			switch {
			case c.Bool(projectsFlagName):
				return listProjects(confPath)
			case c.Bool(variantsFlagName):
			case c.Bool(tasksFlagName):
			case c.Bool(distrosFlagName):
			case c.Bool(spawnableFlagName), c.Bool(spawnableFlagName):
			}
			return errors.Errorf("this code should not be reachable")
		},
	}
}

func listProjects(confPath string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conf, err := NewClientSetttings(confPath)
	if err != nil {
		return errors.Wrap(err, "problem loading configuration")
	}

	_ = conf.GetRestCommunicator(ctx)

	ac, _, err := conf.getLegacyClients()
	if err != nil {
		return errors.Wrap(err, "problem accessing evergreen service")
	}

	notifyUserUpdate(ac)

	projs, err := ac.ListProjects()
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(projs))
	names := make(map[string]string)
	for _, proj := range projs {
		// Only list projects that are enabled
		if proj.Enabled {
			ids = append(ids, proj.Identifier)
			names[proj.Identifier] = proj.DisplayName
		}
	}
	sort.Strings(ids)
	fmt.Println(len(ids), "projects:")
	w := new(tabwriter.Writer)
	// Format in tab-separated columns with a tab stop of 8.
	w.Init(os.Stdout, 0, 8, 0, '\t', 0)
	for _, id := range ids {
		line := fmt.Sprintf("\t%v\t", id)
		if len(names[id]) > 0 && names[id] != id {
			line = line + fmt.Sprintf("%v", names[id])
		}
		fmt.Fprintln(w, line)
	}
	return errors.WithStack(w.Flush())
}

func listVariants(confPath, project, filename string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conf, err := NewClientSetttings(confPath)
	if err != nil {
		return errors.Wrap(err, "problem loading configuration")
	}
	_ = conf.GetRestCommunicator(ctx)

	var variants []model.BuildVariant
	if lc.Project != "" {
		ac, _, err := conf.getLegacyClients()
		if err != nil {
			return errors.Wrap(err, "problem accessing evergreen service")
		}

		notifyUserUpdate(ac)
		variants, err = ac.ListVariants(project)
		if err != nil {
			return err
		}
	} else if lc.File != "" {
		project, err := loadLocalConfig(filename)
		if err != nil {
			return err
		}
		variants = project.BuildVariants
	} else {
		return noProjectError
	}

	names := make([]string, 0, len(variants))
	displayNames := make(map[string]string)
	for _, variant := range variants {
		names = append(names, variant.Name)
		displayNames[variant.Name] = variant.DisplayName
	}
	sort.Strings(names)
	fmt.Println(len(names), "variants:")
	w := new(tabwriter.Writer)
	// Format in tab-separated columns with a tab stop of 8.
	w.Init(os.Stdout, 0, 8, 0, '\t', 0)
	for _, name := range names {
		line := fmt.Sprintf("\t%v\t", name)
		if len(displayNames[name]) > 0 && displayNames[name] != name {
			line = line + fmt.Sprintf("%v", displayNames[name])
		}
		fmt.Fprintln(w, line)
	}

	return w.Flush()

}
