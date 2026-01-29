package operations

import (
	"context"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"go.opentelemetry.io/otel/trace"
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
		Usage: "deployment helpers for Evergreen site administration",
		Subcommands: []cli.Command{
			smokeStartEvergreen(),
			startLocalEvergreen(),
		},
	}
}

func parseDB(c *cli.Context) *evergreen.DBSettings {
	if c == nil {
		return nil
	}
	url := c.String(dbUrlFlagName)
	awsAuthEnabled := c.Bool(dbAWSAuthFlagName)
	envUrl := os.Getenv(evergreen.MongodbURL)
	if url == evergreen.DefaultDatabaseURL && envUrl != "" {
		url = envUrl
	}

	return &evergreen.DBSettings{
		Url:       url,
		SharedURL: util.CoalesceString(c.String(sharedDBUrlFlagName), os.Getenv(evergreen.SharedMongoURL)),
		DB:        c.String(dbNameFlagName),
		WriteConcernSettings: evergreen.WriteConcern{
			W:     c.Int(dbWriteNumFlagName),
			WMode: c.String(dbWmodeFlagName),
		},
		ReadConcernSettings: evergreen.ReadConcern{
			Level: c.String(dbRmodeFlagName),
		},
		AWSAuthEnabled: awsAuthEnabled,
	}
}

////////////////////////////////////////////////////////////////////////
//
// Common Initialization Code

func startSystemCronJobs(ctx context.Context, env evergreen.Environment, tracer trace.Tracer) error {
	ctx, span := tracer.Start(ctx, "StartSystemCronJobs")
	defer span.End()
	// Remove the parent span from the context.
	ctx = trace.ContextWithSpan(ctx, nil)

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

	amboy.IntervalQueueOperation(ctx, populateQueue, 15*time.Second, utility.RoundPartOfMinute(0), opts, func(ctx context.Context, queue amboy.Queue) error {
		return errors.WithStack(queue.Put(ctx, units.NewCronRemoteFifteenSecondJob()))
	})
	amboy.IntervalQueueOperation(ctx, populateQueue, time.Minute, utility.RoundPartOfMinute(0), opts, func(ctx context.Context, queue amboy.Queue) error {
		return errors.WithStack(queue.Put(ctx, units.NewCronRemoteMinuteJob()))
	})
	amboy.IntervalQueueOperation(ctx, populateQueue, 5*time.Minute, utility.RoundPartOfHour(5), opts, func(ctx context.Context, queue amboy.Queue) error {
		return errors.WithStack(queue.Put(ctx, units.NewCronRemoteFiveMinuteJob()))
	})
	amboy.IntervalQueueOperation(ctx, populateQueue, 15*time.Minute, utility.RoundPartOfHour(15), opts, func(ctx context.Context, queue amboy.Queue) error {
		return errors.WithStack(queue.Put(ctx, units.NewCronRemoteFifteenMinuteJob()))
	})
	amboy.IntervalQueueOperation(ctx, populateQueue, time.Hour, utility.RoundPartOfDay(1), opts, func(ctx context.Context, queue amboy.Queue) error {
		return errors.WithStack(queue.Put(ctx, units.NewCronRemoteHourJob()))
	})
	amboy.IntervalQueueOperation(ctx, populateQueue, 30*24*time.Hour, utility.RoundPartOfDay(0), opts, func(ctx context.Context, queue amboy.Queue) error {
		return errors.WithStack(queue.Put(ctx, units.NewCronRemoteMonthJob()))
	})

	////////////////////////////////////////////////////////////////////////
	//
	// Local Queue Jobs
	local := env.LocalQueue()
	amboy.IntervalQueueOperation(ctx, local, 30*time.Second, utility.RoundPartOfMinute(0), opts, units.PopulateLocalQueueJobs(env))

	return nil
}
