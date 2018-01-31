package migrations

import (
	"context"
	"time"

	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type Options struct {
	Limit    int
	Target   int
	Workers  int
	DryRun   bool
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

type migrationGeneratorFactory func(anser.Environment, string, int) (anser.Generator, error)

// Application is where the migrations are registered and defined,
// before being handed off to another calling environment for
// execution. See the anser documentation and the
// anser/example_test.go for an example.
func (opts Options) Application(env anser.Environment) (*anser.Application, error) {
	app := &anser.Application{
		Options: model.ApplicationOptions{
			Limit:  opts.Limit,
			DryRun: opts.DryRun,
		},
	}

	generatorFactories := []migrationGeneratorFactory{
		// addExecutionToTasksGenerator,
		oldTestResultsGenerator,
		testResultsGenerator,
		projectAliasesToCollectionGenerator,
	}

	catcher := grip.NewBasicCatcher()
	for _, factory := range generatorFactories {
		generator, err := factory(env, opts.Database, opts.Limit)
		catcher.Add(err)
		if generator != nil {
			app.Generators = append(app.Generators, generator)
		}
	}

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	if err := app.Setup(env); err != nil {
		return nil, errors.WithStack(err)
	}

	return app, nil
}
