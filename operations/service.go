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
			startRunnerService(),
			startWebService(),
			handcrankRunner(),
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
			fetchAllProjectConfigs(),
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

func startSysInfoCollectors(ctx context.Context, env evergreen.Environment, interval time.Duration, opts amboy.QueueOperationConfig) {
	amboy.IntervalQueueOperation(ctx, env.LocalQueue(), interval, time.Now(), opts, func(queue amboy.Queue) error {
		flags := env.Settings().ServiceFlags
		if err := flags.Get(); err != nil {
			grip.Alert(message.WrapError(err, message.Fields{
				"message":       "problem fetching service flags",
				"operation":     "system stats",
				"interval_secs": interval.Seconds(),
			}))
			return err
		}

		if flags.BackgroundStatsDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "system stats ",
				"impact":  "memory, cpu, runtime stats",
				"mode":    "degraded",
			})
		}

		return queue.Put(units.NewSysInfoStatsCollector(fmt.Sprintf("sys-info-stats-%d", time.Now().Unix())))
	})
}
