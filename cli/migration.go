package cli

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/migrations"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type MigrationCommand struct {
	ConfigPath string `long:"conf" default:"/etc/mci_settings.yml" description:"path to the service configuration file"`
	MongoDBURI string `long:"mongodburi" default:"" description:"alternate mongodb uri, override config file"`
	DryRun     bool   `long:"dry-run" short:"n" description:"run migration in a dry-run mode"`
	Limit      int    `long:"limit" short:"l" description:"run migration with a limit"`
}

func (c *MigrationCommand) Execute(_ []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := evergreen.GetEnvironment()
	grip.CatchEmergencyFatal(errors.Wrap(env.Configure(ctx, c.ConfigPath), "problem configuring application environment"))
	settings := env.Settings()

	if c.MongoDBURI == "" {
		c.MongoDBURI = settings.Database.Url
	}

	anserEnv, err := migrations.Setup(ctx, c.MongoDBURI)
	if err != nil {
		return errors.Wrap(err, "problem setting up migration environment")
	}
	defer anserEnv.Close()

	app, err := migrations.Application(anserEnv, settings.Database.DB)
	if err != nil {
		return errors.Wrap(err, "problem configuring migration application")
	}
	app.DryRun = c.DryRun
	app.Limit = c.Limit
	return errors.Wrap(app.Run(ctx), "problem running migration operation")
}
