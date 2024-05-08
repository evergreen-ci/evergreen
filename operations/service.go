package operations

import (
	"context"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
			deployMigration(),
			deployDataTransforms(),
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

	// TODO (DEVPROD-6951): remove static auth once IRSA auth is reliable again.
	username := os.Getenv(evergreen.MongoUsername)
	password := os.Getenv(evergreen.MongoPassword)

	envUrl := os.Getenv(evergreen.MongodbUrl)
	if url == evergreen.DefaultDatabaseURL && envUrl != "" {
		url = envUrl
	}

	return &evergreen.DBSettings{
		Url: url,
		DB:  c.String(dbNameFlagName),
		WriteConcernSettings: evergreen.WriteConcern{
			W:     c.Int(dbWriteNumFlagName),
			WMode: c.String(dbWmodeFlagName),
		},
		ReadConcernSettings: evergreen.ReadConcern{
			Level: c.String(dbRmodeFlagName),
		},
		AWSAuthEnabled: awsAuthEnabled,
		Username:       username,
		Password:       password,
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

	////////////////////////////////////////////////////////////////////////
	//
	// Local Queue Jobs
	local := env.LocalQueue()
	amboy.IntervalQueueOperation(ctx, local, 30*time.Second, utility.RoundPartOfMinute(0), opts, units.PopulateLocalQueueJobs(env))

	// Enqueue jobs to ensure each app server has the correct SSH key files.
	ts := utility.RoundPartOfHour(30).Format(units.TSFormat)
	grip.Error(message.WrapError(local.Put(ctx, units.NewLocalUpdateSSHKeysJob(ts)), message.Fields{
		"message": "enqueueing jobs to update app server's local SSH keys",
	}))

	return nil
}
