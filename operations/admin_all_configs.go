package operations

import (
	"context"
	"io/ioutil"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func fetchAllProjectConfigs() cli.Command {
	const (
		includeDisabledFlagName = "include-disabled"
	)

	return cli.Command{
		Name:    "all-configs",
		Aliases: []string{"all-configs"},
		Flags: []cli.Flag{
			cli.BoolFlag{
				Name:  includeDisabledFlagName,
				Usage: "include disabled projects",
			},
		},
		Usage:  "download the configuration files of all evergreen projects to the current directory",
		Before: setPlainLogger,
		Action: func(c *cli.Context) error {
			includeDisabled := c.BoolT(includeDisabledFlagName)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			settings, err := NewClientSettings(c.GlobalString("config"))
			if err != nil {
				return err
			}

			client := settings.setupRestCommunicator(ctx)
			defer client.Close()

			ac, rc, err := settings.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			projects, err := ac.ListProjects()
			if err != nil {
				return errors.Wrap(err, "can't fetch projects from evergreen")
			}

			return fetchAndWriteConfigs(rc, projects, includeDisabled)
		},
	}
}

// fetchAndWriteConfig downloads the most recent config for a project
// and writes it to "project_name.yml" locally.
func fetchAndWriteConfigs(c *legacyClient, projects []model.ProjectRef, includeDisabled bool) error {
	catcher := grip.NewSimpleCatcher()
	type projectRepo struct {
		Owner      string
		Repo       string
		Branch     string
		ConfigFile string
	}
	configDownloaded := map[projectRepo]bool{}
	for _, p := range projects {
		if !p.IsEnabled() && !includeDisabled {
			continue
		}
		repo := projectRepo{
			Owner:      p.Owner,
			Repo:       p.Repo,
			Branch:     p.Branch,
			ConfigFile: p.RemotePath,
		}
		if exists := configDownloaded[repo]; exists {
			grip.Infof("Configuration for project '%s' already downloaded", p.Identifier)
			continue
		}
		grip.Infof("Downloading configuration for '%s'", p.Identifier)
		versions, err := c.GetRecentVersions(p.Id)
		if err != nil {
			catcher.Wrapf(err, "failed to fetch recent versions for '%s'", p.Identifier)
			continue
		}
		if len(versions) == 0 {
			catcher.Errorf("WARNING: project '%s' has no versions", p.Identifier)
			continue
		}

		config, err := c.GetConfig(versions[0])
		if err != nil {
			catcher.Wrapf(err, "failed to fetch config for project '%s', version '%s'", p.Identifier, versions[0])
			continue
		}
		configDownloaded[repo] = true

		err = ioutil.WriteFile(p.Identifier+".yml", config, 0644)
		if err != nil {
			catcher.Wrapf(err, "failed to write configuration for project '%s'", p.Identifier)
		}
	}

	return nil
}
