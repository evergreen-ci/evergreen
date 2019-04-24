package operations

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func Service() cli.Command {
	return cli.Command{
		Name:  "service",
		Usage: "run evergreen services",
		Subcommands: []cli.Command{
			deploy(),
			startWebService(),
		},
	}
}

func deploy() cli.Command {
	return cli.Command{
		Name:  "deploy",
		Usage: "deployment helpers for evergreen site administration",
		Subcommands: []cli.Command{
			deployMigration(),
			deployDataTransforms(),
			smokeStartEvergreen(),
			smokeTestEndpoints(),
		},
	}
}

func parseDB(c *cli.Context) *evergreen.DBSettings {
	if c == nil {
		return nil
	}
	url := c.String(dbUrlFlagName)
	envUrl := os.Getenv(evergreen.MongodbUrl)
	if url == evergreen.DefaultDatabaseUrl && envUrl != "" {
		url = envUrl
	}
	return &evergreen.DBSettings{
		Url: url,
		DB:  c.String(dbNameFlagName),
		WriteConcernSettings: evergreen.WriteConcern{
			W:     c.Int(dbWriteNumFlagName),
			WMode: c.String(dbWmodeFlagName),
		},
	}
}

////////////////////////////////////////////////////////////////////////
//
// Common Initialization Code

func startSystemCronJobs(ctx context.Context, env evergreen.Environment) error {
	// Add jobs to a remote queue at various intervals for
	// repotracker operations. Generally the intervals are half the
	// actual frequency of the job, which are controlled by the
	// population functions.

	populateQueue, err := env.RemoteQueueGroup().Get(ctx, "service.populate")
	if err != nil {
		return errors.WithStack(err)
	}

	opts := amboy.QueueOperationConfig{
		ContinueOnError: true,
		LogErrors:       false,
		DebugLogging:    false,
	}

	amboy.IntervalQueueOperation(ctx, populateQueue, 15*time.Second, util.RoundPartOfMinute(0), opts, func(queue amboy.Queue) error {
		return errors.WithStack(queue.Put(units.NewCronRemoteFifteenSecondsJob()))
	})
	amboy.IntervalQueueOperation(ctx, populateQueue, time.Minute, util.RoundPartOfMinute(0), opts, func(queue amboy.Queue) error {
		return errors.WithStack(queue.Put(units.NewCronRemoteMinuteJob()))
	})
	amboy.IntervalQueueOperation(ctx, populateQueue, 5*time.Minute, util.RoundPartOfHour(5), opts, func(queue amboy.Queue) error {
		return errors.WithStack(queue.Put(units.NewCronRemoteFiveMinuteJob()))
	})
	amboy.IntervalQueueOperation(ctx, env.RemoteQueue(), 15*time.Minute, util.RoundPartOfMinute(0), opts, amboy.GroupQueueOperationFactory(
		units.PopulateCatchupJobs(30),
		units.PopulateHostAlertJobs(20),
	))

	amboy.IntervalQueueOperation(ctx, env.RemoteQueue(), 3*time.Hour, util.RoundPartOfMinute(0), opts, amboy.GroupQueueOperationFactory(
		units.PopulateCacheHistoricalTestDataJob(6)))

	////////////////////////////////////////////////////////////////////////
	//
	// Local Queue Jobs
	amboy.IntervalQueueOperation(ctx, env.LocalQueue(), 30*time.Second, util.RoundPartOfMinute(0), opts, func(queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			grip.Alert(message.WrapError(err, message.Fields{
				"message":   "problem fetching service flags",
				"operation": "system stats",
			}))
			return err
		}

		if flags.BackgroundStatsDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "system stats ",
				"impact":  "memory, cpu, runtime stats",
				"mode":    "degraded",
			})
			return nil
		}

		return queue.Put(units.NewSysInfoStatsCollector(fmt.Sprintf("sys-info-stats-%d", time.Now().Unix())))
	})

	amboy.IntervalQueueOperation(ctx, env.LocalQueue(), time.Minute, util.RoundPartOfMinute(0), opts, amboy.GroupQueueOperationFactory(
		units.PopulateJasperCleanup(env),
		func(queue amboy.Queue) error {
			flags, err := evergreen.GetServiceFlags()
			if err != nil {
				grip.Alert(message.WrapError(err, message.Fields{
					"message":   "problem fetching service flags",
					"operation": "background stats",
				}))
				return err
			}

			if flags.BackgroundStatsDisabled {
				grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
					"message": "background stats collection disabled",
					"impact":  "amboy stats disabled",
					"mode":    "degraded",
				})
				return nil
			}

			return queue.Put(units.NewLocalAmboyStatsCollector(env, fmt.Sprintf("amboy-local-stats-%d", time.Now().Unix())))
		}))

	return nil
}
