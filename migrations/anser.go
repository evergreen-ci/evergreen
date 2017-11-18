package migrations

import (
	"context"
	"time"

	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/pkg/errors"
)

type Options struct {
	Limit    int
	Target   int
	Workers  int
	Period   time.Duration
	Database string
	Session  db.Session
}

// Setup configures the migration environment, configuring the backing
// queue and a database session.
func (opts Options) Setup(ctx context.Context) (anser.Environment, error) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)

	env := anser.GetEnvironment()
	env.RegisterCloser(func() error { cancel(); return nil })

	q := queue.NewAdaptiveOrderedLocalQueue(1)
	runner, err := pool.NewMovingAverageRateLimitedWorkers(opts.Workers, opts.Target, opts.Period, q)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if err = q.SetRunner(runner); err != nil {
		return nil, errors.WithStack(err)
	}

	if err = q.Start(ctx); err != nil {
		return nil, errors.WithStack(err)
	}

	if err = env.Setup(q, opts.Session); err != nil {
		return nil, errors.WithStack(err)
	}

	return env, nil
}

// Application is where the migrations are registered and defined,
// before being handed off to another calling environment for
// execution. See the anser documentation and the
// anser/example_test.go for an example.
func (opts Options) Application(env anser.Environment) (*anser.Application, error) {
	app := &anser.Application{
		Limit: opts.Limit,
	}

	if err := registerTestResultsMigrationOperations(env, opts.Database); err != nil {
		return nil, errors.Wrap(err, "error registering test results migration operations")
	}

	app.Generators = append(app.Generators, testResultsGeneratorFactory(env, opts.Database, opts.Limit)...)

	if err := app.Setup(env); err != nil {
		return nil, errors.WithStack(err)
	}

	return app, nil
}
