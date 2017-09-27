package migrations

import (
	"github.com/mongodb/anser"
	"github.com/pkg/errors"
)

// Application is where the migrations are registered and defined,
// before being handed off to another calling environment for
// execution. See the anser documentation and the
// anser/example_test.go for an example.
func Application(env anser.Environment) (*anser.Application, error) {
	app := &anser.Application{}

	if err := app.Setup(env); err != nil {
		return nil, errors.WithStack(err)
	}

	return app, nil
}
