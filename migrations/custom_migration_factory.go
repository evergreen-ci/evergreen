package migrations

import (
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
)

const perfCopyVariantMigrationFactory = "perf-copy-variant-factory"

type migrationGeneratorFactoryFactory func(args map[string]string) migrationGeneratorFactory

var CustomMigrationRegistry = map[string]migrationGeneratorFactoryFactory{
	perfCopyVariantMigrationFactory: perfCopyVariantFactoryFactory,
}

// NewCustomApplication constructs and sets up an application instance from
// a configuration structure and a migration factory.
func NewCustomApplication(env anser.Environment, conf *CustomConfiguration, factory map[string]migrationGeneratorFactoryFactory) (*anser.Application, error) {
	if conf == nil {
		return nil, errors.New("cannot specify a nil configuration")
	}

	app := &anser.Application{
		Options: conf.Options,
	}

	g := conf.CustomMigration
	if !g.Options.IsValid() {
		return nil, errors.Errorf("custom migration generator '%s' is not valid", g.Options.JobID)
	}

	generatorFactory := factory[g.Name](g.Params)
	opts := migrationGeneratorFactoryOptions{
		id:    g.Options.JobID,
		db:    g.Options.NS.DB,
		limit: conf.Options.Limit,
	}
	generator, err := generatorFactory(env, opts)
	if err != nil {
		return nil, errors.Wrap(err, "error creating generator")
	}
	app.Generators = []anser.Generator{generator}

	if err := app.Setup(env); err != nil {
		return nil, errors.Wrap(err, "problem loading migration utility from config file")
	}

	return app, nil
}

type CustomConfiguration struct {
	Options         model.ApplicationOptions `bson:"options" json:"options" yaml:"options"`
	CustomMigration CustomManualMigration    `bson:"custom_migrations" json:"custom_migrations" yaml:"custom_migrations"`
}

type CustomManualMigration struct {
	Options model.GeneratorOptions `bson:"options" json:"options" yaml:"options"`
	Name    string                 `bson:"name" json:"name" yaml:"name"`
	Params  map[string]string      `bson:"params" json:"params" yaml:"params"`
}
