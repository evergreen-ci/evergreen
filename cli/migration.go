package cli

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/migrations"
	"github.com/pkg/errors"
)

type MigrationCommand struct {
	ConfigPath string `long:"conf" default:"/etc/mci_settings.yml" description:"path to the service configuration file"`
	MongoDBURI string `long:"mongodburi" default:"" description:"alternate mongodb uri, override config file"`
	DryRun     bool   `long:"dry-run" short:"n" description:"run migration in a dry-run mode"`
	Limit      int    `long:"limit" short:"l" description:"run migration with a limit"`
}

func (c *MigrationCommand) Execute(_ []string) error {
	settings, err := evergreen.NewSettings(c.ConfigPath)
	if err != nil {
		return errors.Wrap(err, "problem getting settings")
	}

	if err = settings.Validate(); err != nil {
		return errors.Wrap(err, "problem validating settings")
	}

	if c.MongoDBURI == "" {
		c.MongoDBURI = settings.Database.Url
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env, err := migrations.Setup(ctx, c.MongoDBURI)
	if err != nil {
		return errors.Wrap(err, "problem setting up migration environment")
	}
	defer env.Close()

	app, err := migrations.Application(env, settings.Database.DB)
	if err != nil {
		return errors.Wrap(err, "problem configuring migration application")
	}
	app.DryRun = c.DryRun
	app.Limit = c.Limit
	return errors.Wrap(app.Run(ctx), "problem running migration operation")
}
