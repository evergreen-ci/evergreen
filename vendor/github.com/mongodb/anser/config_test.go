package anser

import (
	"testing"

	"github.com/mongodb/anser/mock"
	"github.com/mongodb/anser/model"
	"github.com/stretchr/testify/require"
)

func TestApplicationConstructor(t *testing.T) {
	require := require.New(t) // nolint

	env := mock.NewEnvironment()
	require.NotNil(env)

	///////////////////////////////////
	//
	// the constructor should return errors without proper inputs

	app, err := NewApplication(nil, nil)
	require.Error(err)
	require.Nil(app)

	app, err = NewApplication(env, nil)
	require.Error(err)
	require.Nil(app)

	conf := &model.Configuration{}
	app, err = NewApplication(nil, conf)
	require.Error(err)
	require.Nil(app)

	///////////////////////////////////
	//
	// configure a valid, noop configuration without any generators defined

	app, err = NewApplication(env, conf)
	require.NoError(err)
	require.NotNil(app)
	require.Len(app.Generators, 0)

	///////////////////////////////////
	//
	// Configure a working and valid populated configuration, with all three types.

	conf.SimpleMigrations = []model.ConfigurationSimpleMigration{
		{
			Options: model.GeneratorOptions{
				JobID: "foo-0",
				NS:    model.Namespace{DB: "db", Collection: "coll"},
				Query: map[string]interface{}{"_id": "1"}},
			Update: map[string]interface{}{"$set": 1},
		},
	}

	app, err = NewApplication(env, conf)
	require.NoError(err)
	require.NotNil(app)
	require.Len(app.Generators, 1)

	///////////////////////////////////
	//
	// construct invalid migrations, and ensure that it errors

	conf.SimpleMigrations = []model.ConfigurationSimpleMigration{
		{
			Options: model.GeneratorOptions{
				JobID: "foo-3",
				NS:    model.Namespace{DB: "db", Collection: "coll"},
				Query: map[string]interface{}{},
			},
			Update: map[string]interface{}{},
		},
		{
			Options: model.GeneratorOptions{
				JobID: "foo-4",
				NS:    model.Namespace{DB: "db", Collection: "coll"},
				Query: map[string]interface{}{},
			},
		},
		{
			Options: model.GeneratorOptions{},
			Update:  map[string]interface{}{},
		},
		{
			Options: model.GeneratorOptions{},
		},
	}

	conf.ManualMigrations = []model.ConfigurationManualMigration{
		{
			Options: model.GeneratorOptions{
				JobID: "foo-5",
				NS:    model.Namespace{DB: "db", Collection: "coll"},
				Query: map[string]interface{}{"_id": "1"}},
			Name: "manualTwo",
		},
		{
			Options: model.GeneratorOptions{},
			Name:    "manualOne",
		},
	}

	conf.StreamMigrations = []model.ConfigurationManualMigration{
		{
			Options: model.GeneratorOptions{
				JobID: "foo-6",
				NS:    model.Namespace{DB: "db", Collection: "coll"},
				Query: map[string]interface{}{"_id": "1"}},
			Name: "streamTwo",
		},
		{
			Options: model.GeneratorOptions{},
			Name:    "streamOne",
		},
	}

	app, err = NewApplication(env, conf)
	require.Error(err)
	require.Nil(app)
}
