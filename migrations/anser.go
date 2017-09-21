package migrations

import (
	"github.com/mongodb/anser"
)

// Application is where the migrations are registered and defined,
// before being handed off to another calling environment for
// execution. See the anser documentation and the
// anser/example_test.go for an example.
func Application(env anser.Environment) *anser.Application {
	app := &anser.Application{}

	app.Setup(env)
	return app
}
