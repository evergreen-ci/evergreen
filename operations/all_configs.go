package operations

import (
	"context"
	"io/ioutil"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	yaml "gopkg.in/yaml.v2"
)

func FetchAllConfigs() cli.Command {
	return cli.Command{
		Name:     "all-configs",
		Aliases:  []string{"all-configs"},
		Usage:    "download the configuration files of all evergreen projects",
		HideHelp: true,
		Before:   setPlainLogger,
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			settings, err := NewClientSetttings(c.GlobalString("config"))
			if err != nil {
				return err
			}

			_ = settings.GetRestCommunicator(ctx)

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
				catcher.Add(fetchAndWriteConfig(rc, p.Identifier))
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

	data, err := yaml.Marshal(config)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal configuration for project %s", project)
	}

	err = ioutil.WriteFile(project+".yml", []byte(data), 0666)
	if err != nil {
		return errors.Wrapf(err, "failed to write configuration for project %s", project)
	}

	return nil
}
