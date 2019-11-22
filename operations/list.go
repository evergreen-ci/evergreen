package operations

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/cheynewallace/tabby"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func List() cli.Command {
	const (
		projectsFlagName    = "projects"
		variantsFlagName    = "variants"
		tasksFlagName       = "tasks"
		distrosFlagName     = "distros"
		spawnableFlagName   = "spawnable"
		aliasesFlagName     = "aliases"
		projectTagsFlagName = "taggedProjects"
	)

	return cli.Command{
		Name:  "list",
		Usage: "displays requested information about evergreen",
		Flags: addPathFlag(addProjectFlag(
			cli.BoolFlag{
				Name:  projectsFlagName,
				Usage: "list all configured projects",
			},
			cli.BoolFlag{
				Name:  distrosFlagName,
				Usage: "list all available distros",
			},
			cli.BoolFlag{
				Name:  variantsFlagName,
				Usage: "list all variants defined in the specified file",
			},
			cli.BoolFlag{
				Name:  tasksFlagName,
				Usage: "list all tasks for the specified file",
			},
			cli.BoolFlag{
				Name:  aliasesFlagName,
				Usage: "list all patch aliases for a project",
			},
			cli.BoolFlag{
				Name:  spawnableFlagName,
				Usage: "list all spawnable distros for a project",
			},
			cli.StringFlag{
				Name:  projectTagsFlagName,
				Usage: "list all projects with this tag",
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
			case c.IsSet(projectTagsFlagName):
				return listTaggedProjects(ctx, confPath, c.String(projectTagsFlagName))
			}
			return errors.Errorf("this code should not be reachable")
		},
	}
}

func listTaggedProjects(ctx context.Context, confPath string, tag string) error {
	conf, err := NewClientSettings(confPath)
	if err != nil {
		return errors.Wrap(err, "problem loading configuration")
	}
	client := conf.setupRestCommunicator(ctx)
	defer client.Close()

	ac, _, err := conf.getLegacyClients()
	if err != nil {
		return errors.Wrap(err, "problem accessing evergreen service")
	}

	projs, err := ac.ListProjects()
	if err != nil {
		return err
	}

	matching := []model.ProjectRef{}
	for _, prj := range projs {
		if prj.Enabled && util.StringSliceContains(prj.Tags, tag) {
			matching = append(matching, prj)
		}
	}

	sort.Slice(matching, func(i, j int) bool { return matching[i].Identifier < matching[j].Identifier })

	t := tabby.New()
	t.AddHeader("Name", "Description", "Tags")
	for _, prj := range matching {
		t.AddLine(prj.Identifier, prj.DisplayName, strings.Join(prj.Tags, ", "))
	}
	fmt.Printf("%d matching projects:\n", len(matching))
	t.Print()
	return nil
}

func listProjects(ctx context.Context, confPath string) error {
	conf, err := NewClientSettings(confPath)
	if err != nil {
		return errors.Wrap(err, "problem loading configuration")
	}
	client := conf.setupRestCommunicator(ctx)
	defer client.Close()

	ac, _, err := conf.getLegacyClients()
	if err != nil {
		return errors.Wrap(err, "problem accessing evergreen service")
	}

	projs, err := ac.ListProjects()
	if err != nil {
		return err
	}
	matching := []model.ProjectRef{}
	for _, proj := range projs {
		if proj.Enabled {
			matching = append(matching, proj)
		}
	}

	sort.Slice(matching, func(i, j int) bool { return matching[i].Identifier < matching[j].Identifier })
	fmt.Println(len(ids), "projects:")

	t := tabby.New()
	t.AddHeader("Name", "Description")
	for _, prj := range matching {
		t.AddLine(prj.Identifier, prj.DisplayName)
	}
	t.Print()
	return nil
}

func listVariants(ctx context.Context, confPath, project, filename string) error {
	conf, err := NewClientSettings(confPath)
	if err != nil {
		return errors.Wrap(err, "problem loading configuration")
	}
	client := conf.setupRestCommunicator(ctx)
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
	client := conf.setupRestCommunicator(ctx)
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
	comm := conf.setupRestCommunicator(ctx)
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
	client := conf.setupRestCommunicator(ctx)
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
