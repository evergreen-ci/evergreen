package operations

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func List() cli.Command {
	const (
		projectsFlagName  = "projects"
		variantsFlagName  = "variants"
		tasksFlagName     = "tasks"
		distrosFlagName   = "distros"
		spawnableFlagName = "spawnable"
		aliasesFlagName   = "aliases"
	)

	return cli.Command{
		Name:  "list",
		Usage: "displays requested information about evergreen",
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
				Name:  aliasesFlagName,
				Usage: "list all patch aliases for a project",
			},
			cli.BoolFlag{
				Name:  distrosFlagName,
				Usage: "list all distros for a project",
			},
			cli.BoolFlag{
				Name:  spawnableFlagName,
				Usage: "list all spawnable distros for a project",
			})...),
		Before: requireOnlyOneBool(projectsFlagName, variantsFlagName, tasksFlagName, aliasesFlagName, distrosFlagName, spawnableFlagName),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			project := c.String(projectFlagName)
			filename := c.String(pathFlagName)
			onlyUserSpawnable := c.Bool(spawnableFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			switch {
			case c.Bool(projectsFlagName):
				return listProjects(ctx, confPath)
			case c.Bool(variantsFlagName):
				return listVariants(ctx, confPath, project, filename)
			case c.Bool(tasksFlagName):
				return listTasks(ctx, confPath, project, filename)
			case c.Bool(aliasesFlagName):
				return listAliases(ctx, confPath, project, filename)
			case c.Bool(distrosFlagName), onlyUserSpawnable:
				return listDistros(ctx, confPath, onlyUserSpawnable)
			}
			return errors.Errorf("this code should not be reachable")
		},
	}
}

func listProjects(ctx context.Context, confPath string) error {
	conf, err := NewClientSettings(confPath)
	if err != nil {
		return errors.Wrap(err, "problem loading configuration")
	}
	client := conf.GetRestCommunicator(ctx)
	defer client.Close()

	ac, _, err := conf.getLegacyClients()
	if err != nil {
		return errors.Wrap(err, "problem accessing evergreen service")
	}

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

func listVariants(ctx context.Context, confPath, project, filename string) error {
	conf, err := NewClientSettings(confPath)
	if err != nil {
		return errors.Wrap(err, "problem loading configuration")
	}
	client := conf.GetRestCommunicator(ctx)
	defer client.Close()

	var variants []model.BuildVariant
	if project != "" {
		ac, _, err := conf.getLegacyClients()
		if err != nil {
			return errors.Wrap(err, "problem accessing evergreen service")
		}

		variants, err = ac.ListVariants(project)
		if err != nil {
			return err
		}
	} else if filename != "" {
		project, err := loadLocalConfig(filename)
		if err != nil {
			return err
		}
		variants = project.BuildVariants
	} else {
		return errors.New("could not resolve project")
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

func listTasks(ctx context.Context, confPath, project, filename string) error {
	conf, err := NewClientSettings(confPath)
	if err != nil {
		return errors.Wrap(err, "problem loading configuration")
	}
	client := conf.GetRestCommunicator(ctx)
	defer client.Close()

	var tasks []model.ProjectTask
	if project != "" {
		ac, _, err := conf.getLegacyClients()
		if err != nil {
			return errors.Wrap(err, "problem accessing evergreen service")
		}

		tasks, err = ac.ListTasks(project)
		if err != nil {
			return err
		}
	} else if filename != "" {
		project, err := loadLocalConfig(filename)
		if err != nil {
			return err
		}
		tasks = project.Tasks
	} else {
		return errors.New("could not resolve project")
	}
	fmt.Println(len(tasks), "tasks:")
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 8, 0, '\t', 0)
	for _, t := range tasks {
		line := fmt.Sprintf("\t%v\t", t.Name)
		fmt.Fprintln(w, line)
	}

	return w.Flush()
}

func listAliases(ctx context.Context, confPath, project, filename string) error {
	conf, err := NewClientSettings(confPath)
	if err != nil {
		return errors.Wrap(err, "problem loading configuration")
	}
	comm := conf.GetRestCommunicator(ctx)
	defer comm.Close()

	var aliases []model.ProjectAlias

	if project != "" {
		aliases, err = comm.ListAliases(ctx, project)
		if err != nil {
			return err
		}
	} else if filename != "" {
		project, err := loadLocalConfig(filename)
		if err != nil {
			return err
		}
		aliases, err = comm.ListAliases(ctx, project.Identifier)
		if err != nil {
			return errors.Wrap(err, "error returned from API")
		}
	} else {
		return errors.New("no project specified")
	}

	for _, alias := range aliases {
		if alias.Alias != patch.GithubAlias {
			fmt.Printf("%s\t%s\t%s\t%s\t%s\n", alias.Alias, alias.Variant, strings.Join(alias.VariantTags, ","),
				alias.Task, strings.Join(alias.TaskTags, ", "))
		}
	}

	return nil
}

func listDistros(ctx context.Context, confPath string, onlyUserSpawnable bool) error {
	conf, err := NewClientSettings(confPath)
	if err != nil {
		return errors.Wrap(err, "problem loading configuration")
	}
	client := conf.GetRestCommunicator(ctx)
	defer client.Close()

	distros, err := client.GetDistrosList(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	if onlyUserSpawnable {
		spawnableDistros := []restmodel.APIDistro{}
		for _, distro := range distros {
			if distro.UserSpawnAllowed {
				spawnableDistros = append(spawnableDistros, distro)
			}
		}

		fmt.Println(len(spawnableDistros), "spawnable distros:")
		for _, distro := range spawnableDistros {
			fmt.Println(restmodel.FromAPIString(distro.Name))
		}

	} else {
		fmt.Println(len(distros), "distros:")
		for _, distro := range distros {
			fmt.Println(restmodel.FromAPIString(distro.Name))
		}

		aliases := map[string][]string{}
		for _, d := range distros {
			for _, a := range d.Aliases {
				aliases[a] = append(aliases[a], restmodel.FromAPIString(d.Name))
			}
		}

		if len(aliases) > 0 {
			fmt.Printf("\n%d distro aliases:", len(aliases))
			for a, names := range aliases {
				fmt.Println(a, "=>", names)
			}
		}
	}

	return nil
}

// LoadLocalConfig loads the local project config into a project
func loadLocalConfig(filepath string) (*model.Project, error) {
	configBytes, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, errors.Wrap(err, "error reading project config")
	}

	project := &model.Project{}
	err = model.LoadProjectInto(configBytes, "", project)
	if err != nil {
		return nil, errors.Wrap(err, "error loading project")
	}

	return project, nil
}
