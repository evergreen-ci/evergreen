package operations

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/migrations"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func deployMigration() cli.Command {
	return cli.Command{
		Name:    "anser",
		Aliases: []string{"migrations", "migrate", "migration"},
		Usage:   "database migration tool",
		Flags:   mergeFlagSlices(serviceConfigFlags(), addMigrationRuntimeFlags(), addDbSettingsFlags()),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			db := parseDB(c)
			env := evergreen.GetEnvironment()
			err := env.Configure(ctx, c.String(confFlagName), db)

			grip.CatchEmergencyFatal(errors.Wrap(err, "problem configuring application environment"))
			settings := env.Settings()

			opts := migrations.Options{
				Period:   c.Duration(anserPeriodFlagName),
				Target:   c.Int(anserTargetFlagName),
				Limit:    c.Int(anserLimitFlagName),
				DryRun:   c.Bool(anserDryRunFlagName),
				Workers:  c.Int(anserWorkersFlagName),
				Session:  env.Session(),
				Database: settings.Database.DB,
			}

			anserEnv, err := opts.Setup(ctx)
			if err != nil {
				return errors.Wrap(err, "problem setting up migration environment")
			}
			defer anserEnv.Close()

			app, err := opts.Application(anserEnv, env)
			if err != nil {
				return errors.Wrap(err, "problem configuring migration application")
			}

			return errors.Wrap(app.Run(ctx), "problem running migration operation")
		},
	}
}

func deployDataTransforms() cli.Command {
	return cli.Command{
		Name:    "transform",
		Aliases: []string{"modify-data"},
		Usage:   "run database migrations defined in a configuration file",
		Flags:   mergeFlagSlices(serviceConfigFlags(), addPathFlag(), addMigrationRuntimeFlags(), addDbSettingsFlags()),
		Before:  mergeBeforeFuncs(requirePathFlag, requireFileExists(confFlagName)),
		Action: func(c *cli.Context) error {
			migrationConfFn := c.String(pathFlagName)
			confPath := c.String(confFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			db := parseDB(c)
			env := evergreen.GetEnvironment()
			err := env.Configure(ctx, confPath, db)
			if err != nil {
				return errors.Wrap(err, "problem configuring application environment")
			}
			settings := env.Settings()

			anserConf := &model.Configuration{}
			err = util.ReadFromYAMLFile(migrationConfFn, anserConf)
			if err != nil {
				return errors.Wrap(err, "problem parsing configuration file")
			}

			opts := migrations.Options{
				Period:   c.Duration(anserPeriodFlagName),
				Target:   c.Int(anserTargetFlagName),
				Limit:    c.Int(anserLimitFlagName),
				DryRun:   c.Bool(anserDryRunFlagName),
				Workers:  c.Int(anserWorkersFlagName),
				Session:  env.Session(),
				Database: settings.Database.DB,
			}

			anserEnv, err := opts.Setup(ctx)
			if err != nil {
				return errors.Wrap(err, "problem setting up migration environment")
			}
			defer anserEnv.Close()

			app, err := anser.NewApplication(anserEnv, anserConf)
			if err != nil {
				return errors.Wrap(err, "problem creating migration application")
			}
			return errors.Wrap(app.Run(ctx), "problem running migration operation")
		},
	}
}
