package operations

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/migrations"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/client"
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
		Before:  addPositionalMigrationIds,
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			db := parseDB(c)
			env, err := evergreen.NewEnvironment(ctx, c.String(confFlagName), "", db)
			grip.EmergencyFatal(errors.Wrap(err, "configuring application environment"))
			evergreen.SetEnvironment(env)
			settings := env.Settings()

			// avoid working on remote jobs during migrations
			env.RemoteQueue().Runner().Close(ctx)

			opts := migrations.Options{
				Period:   c.Duration(anserPeriodFlagName),
				Target:   c.Int(anserTargetFlagName),
				Limit:    c.Int(anserLimitFlagName),
				DryRun:   c.Bool(anserDryRunFlagName),
				Workers:  c.Int(anserWorkersFlagName),
				IDs:      c.StringSlice(anserMigrationIDFlagName),
				Session:  env.Session(),
				Client:   client.WrapClient(env.Client()),
				Database: settings.Database.DB,
			}

			anserEnv, err := opts.Setup(ctx)
			if err != nil {
				return errors.Wrap(err, "setting up migration environment")
			}
			defer anserEnv.Close()

			app, err := opts.Application(anserEnv, env)
			if err != nil {
				return errors.Wrap(err, "configuring migration application")
			}

			grip.Debug("completed migration setup running generator and then migrations")
			return errors.Wrap(app.Run(ctx), "running migration operation")
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
			env, err := evergreen.NewEnvironment(ctx, confPath, "", db)
			grip.EmergencyFatal(errors.Wrap(err, "configuring application environment"))
			evergreen.SetEnvironment(env)
			settings := env.Settings()

			anserConf := &model.ConfigurationManualMigration{}
			err = utility.ReadYAMLFile(migrationConfFn, anserConf)
			if err != nil {
				return errors.Wrapf(err, "reading YAML configuration file '%s'", migrationConfFn)
			}

			opts := migrations.Options{
				Period:   c.Duration(anserPeriodFlagName),
				Target:   c.Int(anserTargetFlagName),
				Limit:    c.Int(anserLimitFlagName),
				DryRun:   c.Bool(anserDryRunFlagName),
				Workers:  c.Int(anserWorkersFlagName),
				Session:  env.Session(),
				Client:   client.WrapClient(env.Client()),
				Database: settings.Database.DB,
			}

			anserEnv, err := opts.Setup(ctx)
			if err != nil {
				return errors.Wrap(err, "setting up migration environment")
			}
			defer anserEnv.Close()

			app, err := opts.CustomApplication(anserEnv, anserConf)
			if err != nil {
				return errors.Wrap(err, "creating migration application")
			}
			return errors.Wrap(app.Run(ctx), "running migration operation")
		},
	}
}
