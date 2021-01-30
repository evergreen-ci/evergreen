package operations

import (
	"context"
	"io/ioutil"

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

			catcher := grip.NewSimpleCatcher()
			for _, p := range projects {
				if p.IsEnabled() || includeDisabled {
					catcher.Add(fetchAndWriteConfig(rc, p.Id))
				}
			}

			return catcher.Resolve()
		},
	}
}

// fetchAndWriteConfig downloads the most recent config for a project
// and writes it to "project_name.yml" locally.
func fetchAndWriteConfig(c *legacyClient, project string) error {
	grip.Infof("Downloading configuration for %s", project)
	versions, err := c.GetRecentVersions(project)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch recent versions for %s", project)
	}
	if len(versions) == 0 {
		return errors.Errorf("WARNING: project %s has no versions", project)
	}

	config, err := c.GetConfig(versions[0])
	if err != nil {
		return errors.Wrapf(err, "failed to fetch config for project %s, version %s", project, versions[0])
	}

	err = ioutil.WriteFile(project+".yml", config, 0644)
	if err != nil {
		return errors.Wrapf(err, "failed to write configuration for project %s", project)
	}

	return nil
}
