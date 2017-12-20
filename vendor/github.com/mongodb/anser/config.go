package anser

import (
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// NewApplication constructs and sets up an application instance from
// a configuration structure, presumably loaded from a configuration
// file.
//
// You can construct an application instance using default
// initializers, if you do not want to define the migrations using the
// configuration structures.
func NewApplication(env Environment, conf *model.Configuration) (*Application, error) {
	if conf == nil {
		return nil, errors.New("cannot specify a nil configuration")
	}

	app := &Application{
		Options: conf.Options,
	}

	catcher := grip.NewBasicCatcher()
	for _, g := range conf.SimpleMigrations {
		if !g.Options.IsValid() {
			catcher.Add(errors.Errorf("simple migration generator '%s' is not valid", g.Options.JobID))
			continue
		}

		if g.Update == nil || len(g.Update) == 0 {
			catcher.Add(errors.Errorf("simple migration generator '%s' does not contain an update", g.Options.JobID))
			continue
		}

		grip.Infof("registered simple migration '%s'", g.Options.JobID)
		app.Generators = append(app.Generators, NewSimpleMigrationGenerator(env, g.Options, g.Update))
	}

	for _, g := range conf.ManualMigrations {
		if !g.Options.IsValid() {
			catcher.Add(errors.Errorf("manual migration generator '%s' is not valid", g.Options.JobID))
			continue
		}

		if _, ok := env.GetManualMigrationOperation(g.Name); !ok {
			catcher.Add(errors.Errorf("manual migration operation '%s' is not defined", g.Name))
			continue
		}

		grip.Infof("registered manual migration '%s' (%s)", g.Options.JobID, g.Name)
		app.Generators = append(app.Generators, NewManualMigrationGenerator(env, g.Options, g.Name))
	}

	for _, g := range conf.StreamMigrations {
		if !g.Options.IsValid() {
			catcher.Add(errors.Errorf("stream migration generator '%s' is not valid", g.Options.JobID))
			continue
		}

		if _, ok := env.GetDocumentProcessor(g.Name); !ok {
			catcher.Add(errors.Errorf("stream migration operation '%s' is not defined", g.Name))
			continue
		}

		grip.Infof("registered stream migration '%s' (%s)", g.Options.JobID, g.Name)
		app.Generators = append(app.Generators, NewStreamMigrationGenerator(env, g.Options, g.Name))
	}

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	if err := app.Setup(env); err != nil {
		return nil, errors.Wrap(err, "problem loading migration utility from config file")
	}

	return app, nil
}
