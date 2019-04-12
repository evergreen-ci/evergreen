package migrations

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
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
	IDs      []string
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

	q := queue.NewAdaptiveOrderedLocalQueue(1, 10*opts.Target*opts.Workers)
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

type migrationGeneratorFactoryOptions struct {
	id    string
	db    string
	limit int
}

type migrationGeneratorFactory func(anser.Environment, migrationGeneratorFactoryOptions) (anser.Generator, error)

// Application is where the migrations are registered and defined,
// before being handed off to another calling environment for
// execution. See the anser documentation and the
// anser/example_test.go for an example.
func (opts Options) Application(env anser.Environment, evgEnv evergreen.Environment) (*anser.Application, error) {

	app := &anser.Application{
		Options: model.ApplicationOptions{
			Limit:  opts.Limit,
			DryRun: opts.DryRun,
		},
	}

	githubToken, err := evgEnv.Settings().GetGithubOauthToken()
	if err != nil {
		return nil, err
	}

	generatorFactories := map[string]migrationGeneratorFactory{
		// Early Migrations, disabled because the generator queries are not properly indexed.
		//
		// migrationTestResultsLegacyExecution: addExecutionToTasksGenerator,
		// migrationTestResultsOldTasks: oldTestResultsGenerator,
		// migrationTestResultstasks: testResultsGenerator,

		// Migration disabled because it iterates over all existing tasks.
		// migrationTaskCreateTime:                     taskCreateTimeGenerator,

		migrationProjectAliasesToCollection:         projectAliasesToCollectionGenerator,
		migrationGithubHooksToCollection:            githubHooksToCollectionGenerator,
		migrationZeroDateFix:                        zeroDateFixGenerator(githubToken),
		migrationAdminEventRestructure:              adminEventRestructureGenerator,
		migrationEventRtypeRestructureAllLogs:       eventRTypeMigration,
		migrationSetDefaultBranch:                   setDefaultBranchMigrationGenerator,
		migrationAdminMapRestructure:                adminMapRestructureGenerator,
		migrationSpawnhostExpirationPreference:      setSpawnhostPreferenceGenerator,
		migrationDistroSecurityGroups:               distroSecurityGroupsGenerator,
		migrationLegacyNotificationsToSubscriptions: legacyNotificationsToSubscriptionsGenerator,
		migrationSubscriptionBSONObjectIDToString:   makeBSONObjectIDToStringGenerator("subscriptions"),
	}
	catcher := grip.NewBasicCatcher()

	for _, id := range opts.IDs {
		if _, ok := generatorFactories[id]; !ok {
			catcher.Add(errors.Errorf("no migration defined matching id '%s'", id))
		}
	}

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	for name, factory := range generatorFactories {
		if opts.shouldSkipMigration(name) {
			continue
		}

		args := migrationGeneratorFactoryOptions{
			id:    name,
			db:    opts.Database,
			limit: opts.Limit,
		}

		generator, err := factory(env, args)
		catcher.Add(err)
		if generator != nil {
			app.Generators = append(app.Generators, generator)
			grip.Debugf("adding generator named: %s", name)
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

func (opts Options) shouldSkipMigration(id string) bool {
	if len(opts.IDs) == 0 {
		return false
	}

	if util.StringSliceContains(opts.IDs, id) {
		return false
	}

	return true
}
