package operations

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/migrations"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func deployMigration() cli.Command {
	const (
		dryRunFlagName  = "dry-run"
		limitFlagName   = "limit"
		targetFlagName  = "target"
		workersFlagName = "workers"
		periodFlagName  = "period"
	)

	return cli.Command{
		Name:    "anser",
		Aliases: []string{"migrations", "migrate", "migration"},
		Usage:   "database migration tool",
		Flags: serviceConfigFlags(
			cli.BoolFlag{
				Name:  joinFlagNames(dryRunFlagName, "n"),
				Usage: "run migration in a dry-run mode",
			},
			cli.IntFlag{
				Name:  joinFlagNames(limitFlagName, "l"),
				Usage: "limit the number of migration jobs to process",
			},
			cli.IntFlag{
				Name:  joinFlagNames(targetFlagName, "t"),
				Usage: "target number of migrations",
				Value: 60,
			},
			cli.IntFlag{
				Name:  joinFlagNames(workersFlagName, "j"),
				Usage: "total number of parallel migration workers",
				Value: 4,
			},
			cli.DurationFlag{
				Name:  joinFlagNames(periodFlagName, "p"),
				Usage: "length of scheduling window",
				Value: time.Minute,
			},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := evergreen.GetEnvironment()
			err := env.Configure(ctx, c.String(confFlagName))

			grip.CatchEmergencyFatal(errors.Wrap(err, "problem configuring application environment"))
			settings := env.Settings()

			opts := migrations.Options{
				Period:   c.Duration(periodFlagName),
				Target:   c.Int(targetFlagName),
				Limit:    c.Int(limitFlagName),
				DryRun:   c.Bool(dryRunFlagName),
				Session:  env.Session(),
				Workers:  c.Int(workersFlagName),
				Database: settings.Database.DB,
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

			return errors.Wrap(app.Run(ctx), "problem running migration operation")
		},
	}
}
