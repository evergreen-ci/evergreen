package cli

import (
	"context"
	"time"

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
	Target     int    `long:"target" short:"t" default:"60" description:"target number of migrations to run in the specified period"`
	Workers    int    `long:"workers" short:"j" default:"4" description:"number of migration worker goroutines"`
	Period     string `long:"period" short:"p" default:"1m" description:"length of scheduling window"`
}

func (c *MigrationCommand) Execute(_ []string) error {
	period, err := time.ParseDuration(c.Period)
	if err != nil {
		return errors.Wrap(err, "problem parsing arguments")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := evergreen.GetEnvironment()
	grip.CatchEmergencyFatal(errors.Wrap(env.Configure(ctx, c.ConfigPath), "problem configuring application environment"))
	settings := env.Settings()

	opts := migrations.Options{
		Period: period,
		Target: c.Target,
		Limit:  c.Limit,
		URI:    c.MongoDBURI,
	}
	if opts.URI == "" {
		opts.URI = settings.Database.DB
	}

	anserEnv, err := opts.Setup(ctx)
	if err != nil {
		return errors.Wrap(err, "problem setting up migration environment")
	}
	defer anserEnv.Close()

	app, err := opts.Application(anserEnv)
	if err != nil {
		return errors.Wrap(err, "problem configuring migration application")
	}
	app.DryRun = c.DryRun
	return errors.Wrap(app.Run(ctx), "problem running migration operation")
}
