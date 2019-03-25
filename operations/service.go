package operations

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
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
		SSL: c.Bool(dbSslFlagName),
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

func startSystemCronJobs(ctx context.Context, env evergreen.Environment) {
	// Add jobs to a remote queue at various intervals for
	// repotracker operations. Generally the intervals are half the
	// actual frequency of the job, which are controlled by the
	// population functions.
	opts := amboy.QueueOperationConfig{
		ContinueOnError: true,
		LogErrors:       false,
		DebugLogging:    false,
	}

	amboy.IntervalQueueOperation(ctx, env.RemoteQueue(), time.Minute, time.Now(), opts, amboy.GroupQueueOperationFactory(
		units.PopulateHostCreationJobs(env, 0),
		units.PopulateIdleHostJobs(env),
		units.PopulateHostTerminationJobs(env),
		units.PopulateHostMonitoring(env),
		units.PopulateTaskMonitoring(),
		units.PopulateEventAlertProcessing(1),
		units.PopulateBackgroundStatsJobs(env, 0),
		units.PopulateLastContainerFinishTimeJobs(),
		units.PopulateParentDecommissionJobs(),
		units.PopulatePeriodicNotificationJobs(1),
		units.PopulateContainerStateJobs(env),
		units.PopulateOldestImageRemovalJobs(),
		units.PopulateCommitQueueJobs(env)))

	amboy.IntervalQueueOperation(ctx, env.RemoteQueue(), 15*time.Second, time.Now(), opts, amboy.GroupQueueOperationFactory(
		units.PopulateHostSetupJobs(env),
		units.PopulateSchedulerJobs(env),
		units.PopulateHostAllocatorJobs(env),
		units.PopulateAgentDeployJobs(env)))

	amboy.IntervalQueueOperation(ctx, env.RemoteQueue(), 150*time.Second, time.Now(), opts, amboy.GroupQueueOperationFactory(
		units.PopulateActivationJobs(6),
		units.PopulateRepotrackerPollingJobs(5)))

	amboy.IntervalQueueOperation(ctx, env.RemoteQueue(), 15*time.Minute, time.Now(), opts, amboy.GroupQueueOperationFactory(
		units.PopulateCatchupJobs(30),
		units.PopulateHostAlertJobs(20)))

	amboy.IntervalQueueOperation(ctx, env.RemoteQueue(), 3*time.Hour, time.Now(), opts, amboy.GroupQueueOperationFactory(
		units.PopulateCacheHistoricalTestDataJob(6)))

	////////////////////////////////////////////////////////////////////////
	//
	// Local Queue Jobs
	amboy.IntervalQueueOperation(ctx, env.LocalQueue(), 15*time.Second, time.Now(), opts, func(queue amboy.Queue) error {
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

	amboy.IntervalQueueOperation(ctx, env.LocalQueue(), time.Minute, time.Now(), opts, func(queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		catcher.Add(queue.Put(units.PopulateJasperCleanup(env)))
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
	})
}
