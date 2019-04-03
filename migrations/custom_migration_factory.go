package migrations

import (
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
)

const perfCopyVariantMigrationFactory = "perf-copy-variant-factory"

type customMigrationGeneratorFactory func(args map[string]string) migrationGeneratorFactory

func (opts Options) CustomApplication(env anser.Environment, conf *model.ConfigurationManualMigration) (*anser.Application, error) {
	if conf == nil {
		return nil, errors.New("cannot specify a nil configuration")
	}

	customMigrationRegistry := map[string]customMigrationGeneratorFactory{
		// add custom migrations here
	}

	app := &anser.Application{
		Options: model.ApplicationOptions{
			Limit:  opts.Limit,
			DryRun: opts.DryRun,
		},
	}

	if !conf.Options.IsValid() {
		return nil, errors.Errorf("custom migration generator '%s' is not valid", conf.Options.JobID)
	}

	mopts := migrationGeneratorFactoryOptions{
		id:    conf.Options.JobID,
		db:    conf.Options.NS.DB,
		limit: opts.Limit,
	}

	generator, err := customMigrationRegistry[conf.Name](conf.Params)(env, mopts)
	if err != nil {
		return nil, errors.Wrap(err, "error creating generator")
	}
	app.Generators = []anser.Generator{generator}

	if err := app.Setup(env); err != nil {
		return nil, errors.Wrap(err, "problem loading migration utility from config file")
	}

	return app, nil
}
