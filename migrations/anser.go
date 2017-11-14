package migrations

import (
	"github.com/mongodb/anser"
	"github.com/pkg/errors"
)

// Application is where the migrations are registered and defined,
// before being handed off to another calling environment for
// execution. See the anser documentation and the
// anser/example_test.go for an example.
func Application(env anser.Environment, db string, limit int) (*anser.Application, error) {
	app := &anser.Application{
		Limit: limit,
	}

	if err := registerTestResultsMigrationOperations(env); err != nil {
		return nil, errors.Wrap(err, "error registering test results migration operations")
	}

	app.Generators = append(app.Generators, testResultsGeneratorFactory(env, db, limit)...)

	if err := app.Setup(env); err != nil {
		return nil, errors.WithStack(err)
	}

	return app, nil
}
