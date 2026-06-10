package operations

import (
	"context"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func fetchAllProjectConfigs() cli.Command {
	const (
		includeDisabledFlagName = "include-disabled"
		directoryFlagName       = "directory"
	)

	return cli.Command{
		Name: "all-configs",
		Flags: []cli.Flag{
			cli.BoolFlag{
				Name:  includeDisabledFlagName,
				Usage: "include disabled projects",
			},
			cli.StringFlag{
				Name:  joinFlagNames(directoryFlagName, "d"),
				Usage: "directory to write config files to (created if needed; defaults to current directory)",
			},
		},
		Usage:  "download the configuration files of all Evergreen projects",
		Before: setPlainLogger,
		Action: func(c *cli.Context) error {
			includeDisabled := c.Bool(includeDisabledFlagName)
			directory := c.String(directoryFlagName)
			settings, err := NewClientSettings(c.GlobalString("config"))
			if err != nil {
				return err
			}

			ac, rc, err := settings.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			projects, err := ac.ListProjects()
			if err != nil {
				return errors.Wrap(err, "fetching projects from Evergreen")
			}

			return fetchAndWriteConfigs(context.Background(), rc, projects, includeDisabled, directory)
		},
	}
}

// fetchAndWriteConfigs downloads the most recent config for a project and writes
// it to "<directory>/project_name.yml". An empty directory writes to the current
// directory.
func fetchAndWriteConfigs(ctx context.Context, c *legacyClient, projects []model.ProjectRef, includeDisabled bool, directory string) error {
	catcher := grip.NewSimpleCatcher()
	if directory != "" {
		if err := os.MkdirAll(directory, 0755); err != nil {
			return errors.Wrapf(err, "creating directory '%s'", directory)
		}
	}
	type projectRepo struct {
		Owner      string
		Repo       string
		Branch     string
		ConfigFile string
	}
	configDownloaded := map[projectRepo]bool{}
	for _, p := range projects {
		if !p.Enabled && !includeDisabled {
			continue
		}
		repo := projectRepo{
			Owner:      p.Owner,
			Repo:       p.Repo,
			Branch:     p.Branch,
			ConfigFile: p.RemotePath,
		}
		if exists := configDownloaded[repo]; exists {
			grip.Infof(ctx, "Configuration for project '%s' already downloaded", p.Identifier)
			continue
		}
		grip.Infof(ctx, "Downloading configuration for '%s'", p.Identifier)
		versions, err := c.GetRecentVersions(p.Id)
		if err != nil {
			catcher.Wrapf(err, "fetching recent versions for '%s'", p.Identifier)
			continue
		}
		if len(versions) == 0 {
			grip.Warningf(ctx, "project '%s' has no versions", p.Identifier)
			continue
		}

		config, err := c.GetConfig(versions[0])
		if err != nil {
			catcher.Wrapf(err, "fetching config for project '%s', version '%s'", p.Identifier, versions[0])
			continue
		}
		configDownloaded[repo] = true

		err = os.WriteFile(filepath.Join(directory, p.Identifier+".yml"), config, 0644)
		if err != nil {
			catcher.Wrapf(err, "writing configuration for project '%s'", p.Identifier)
		}
	}

	return catcher.Resolve()
}
