package operations

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/cheynewallace/tabby"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func List() cli.Command {
	const (
		projectsFlagName             = "projects"
		variantsFlagName             = "variants"
		tasksFlagName                = "tasks"
		distrosFlagName              = "distros"
		spawnableFlagName            = "spawnable"
		includeConfigAliasesFlagName = "include-config-aliases"
		parametersFlagName           = "parameters"
		patchAliasesFlagName         = "patch-aliases"
		triggerAliasesFlagName       = "trigger-aliases"
	)

	return cli.Command{
		Name:  "list",
		Usage: "displays requested information about Evergreen",
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
				Name:  parametersFlagName,
				Usage: "list all parameters for a project",
			},
			cli.BoolFlag{
				Name:  patchAliasesFlagName,
				Usage: "list all patch aliases for a project",
			},
			cli.BoolFlag{
				Name:  includeConfigAliasesFlagName,
				Usage: "include YAML defined aliases when listing patch aliases",
			},
			cli.BoolFlag{
				Name:  triggerAliasesFlagName,
				Usage: "list all trigger aliases for a project",
			},
			cli.BoolFlag{
				Name:  spawnableFlagName,
				Usage: "list all spawnable distros for a project",
			})...),
		Before: requireOnlyOneBool(projectsFlagName, variantsFlagName, tasksFlagName, patchAliasesFlagName, triggerAliasesFlagName, distrosFlagName, spawnableFlagName, parametersFlagName),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(ConfFlagName)
			project := c.String(projectFlagName)
			filename := c.String(pathFlagName)
			onlyUserSpawnable := c.Bool(spawnableFlagName)
			includeProjectConfig := c.Bool(includeConfigAliasesFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			switch {
			case c.Bool(projectsFlagName):
				return listProjects(ctx, confPath)
			case c.Bool(variantsFlagName):
				return listVariants(ctx, confPath, project, filename)
			case c.Bool(tasksFlagName):
				return listTasks(ctx, confPath, project, filename)
			case c.Bool(parametersFlagName):
				return listParameters(ctx, confPath, project, filename)
			case c.Bool(patchAliasesFlagName):
				return listPatchAliases(ctx, confPath, project, includeProjectConfig)
			case c.Bool(triggerAliasesFlagName):
				return listTriggerAliases(ctx, confPath, project)
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
		return errors.Wrap(err, "loading configuration")
	}

	ac, _, err := conf.getLegacyClients()
	if err != nil {
		return errors.Wrap(err, "setting up legacy Evergreen client")
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
	fmt.Println(len(matching), "projects:")

	t := tabby.New()
	t.AddHeader("Identifier", "Id", "Description")
	for _, prj := range matching {
		if prj.Id != prj.Identifier {
			t.AddLine(prj.Identifier, prj.Id, prj.DisplayName)
		} else {
			t.AddLine(prj.Identifier, "", prj.DisplayName)
		}
	}
	t.Print()
	return nil
}

func listVariants(ctx context.Context, confPath, project, filename string) error {
	conf, err := NewClientSettings(confPath)
	if err != nil {
		return errors.Wrap(err, "loading configuration")
	}

	var variants []model.BuildVariant
	if project != "" {
		ac, _, err := conf.getLegacyClients()
		if err != nil {
			return errors.Wrap(err, "setting up legacy Evergreen client")
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
		return errors.Wrap(err, "loading configuration")
	}

	var tasks []model.ProjectTask
	if project != "" {
		ac, _, err := conf.getLegacyClients()
		if err != nil {
			return errors.Wrap(err, "setting up legacy Evergreen client")
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
func listParameters(ctx context.Context, confPath, project, filename string) error {
	conf, err := NewClientSettings(confPath)
	if err != nil {
		return errors.Wrap(err, "loading configuration")
	}
	comm, err := conf.SetupRestCommunicator(ctx, false)
	if err != nil {
		return errors.Wrap(err, "setting up REST communicator")
	}
	defer comm.Close()

	var params []model.ParameterInfo
	if project != "" {
		params, err = comm.GetParameters(ctx, project)
		if err != nil {
			return errors.Wrapf(err, "getting parameters")
		}
	} else if filename != "" {
		project, err := loadLocalConfig(filename)
		if err != nil {
			return err
		}
		params = project.Parameters
	} else {
		return errors.New("no project specified")
	}

	if len(params) == 0 {
		fmt.Println("No parameters to list for project")
		return nil
	}
	t := tabby.New()
	t.AddHeader("Name", "Default", "Description")

	for _, param := range params {
		t.AddLine(param.Key, param.Value, param.Description)
	}
	t.Print()
	return nil
}

func listTriggerAliases(ctx context.Context, confPath, project string) error {
	conf, err := NewClientSettings(confPath)
	if err != nil {
		return errors.Wrap(err, "loading configuration")
	}
	comm, err := conf.SetupRestCommunicator(ctx, false)
	if err != nil {
		return errors.Wrap(err, "setting up REST communicator")
	}
	defer comm.Close()

	if project == "" {
		return errors.New("no project specified")
	}

	aliases, err := comm.ListPatchTriggerAliases(ctx, project)
	if err != nil {
		return err
	}

	for _, alias := range aliases {
		fmt.Printf("%s\n", alias)
	}

	return nil
}

func listPatchAliases(ctx context.Context, confPath, project string, includeProjectConfig bool) error {
	conf, err := NewClientSettings(confPath)
	if err != nil {
		return errors.Wrap(err, "loading configuration")
	}
	comm, err := conf.SetupRestCommunicator(ctx, false)
	if err != nil {
		return errors.Wrap(err, "setting up REST communicator")
	}
	defer comm.Close()

	if project == "" {
		return errors.New("no project specified")
	}

	aliases, err := comm.ListAliases(ctx, project, includeProjectConfig)
	if err != nil {
		return err
	}

	for _, alias := range aliases {
		if !utility.StringSliceContains(evergreen.InternalAliases, alias.Alias) {
			fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\n", alias.Alias, alias.Description, alias.Variant, strings.Join(alias.VariantTags, ","),
				alias.Task, strings.Join(alias.TaskTags, ", "))
		}
	}

	return nil
}

func listDistros(ctx context.Context, confPath string, onlyUserSpawnable bool) error {
	conf, err := NewClientSettings(confPath)
	if err != nil {
		return errors.Wrap(err, "loading configuration")
	}
	client, err := conf.SetupRestCommunicator(ctx, false)
	if err != nil {
		return errors.Wrap(err, "setting up REST communicator")
	}
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
			fmt.Println(utility.FromStringPtr(distro.Name))
		}

	} else {
		fmt.Println(len(distros), "distros:")
		for _, distro := range distros {
			fmt.Println(utility.FromStringPtr(distro.Name))
		}

		aliases := map[string][]string{}
		for _, d := range distros {
			for _, a := range d.Aliases {
				aliases[a] = append(aliases[a], utility.FromStringPtr(d.Name))
			}
		}

		if len(aliases) > 0 {
			fmt.Printf("\n%d distro aliases:\n", len(aliases))
			for a, names := range aliases {
				fmt.Println(a, "=>", names)
			}
		}
	}

	return nil
}

// LoadLocalConfig loads the local project config into a project
func loadLocalConfig(filepath string) (*model.Project, error) {
	configBytes, err := os.ReadFile(filepath)
	if err != nil {
		return nil, errors.Wrapf(err, "reading project config from file '%s'", filepath)
	}

	project := &model.Project{}
	ctx := context.Background()
	if _, err = model.LoadProjectInto(ctx, configBytes, nil, "", project); err != nil {
		return nil, errors.Wrapf(err, "loading project from file '%s'", filepath)
	}

	return project, nil
}
