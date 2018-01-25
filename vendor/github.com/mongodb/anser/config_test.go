package anser

import (
	"testing"

	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/mock"
	"github.com/mongodb/anser/model"
	"github.com/stretchr/testify/assert"
	"gopkg.in/mgo.v2/bson"
)

func TestApplicationConstructor(t *testing.T) {
	assert := assert.New(t) // nolint

	env := mock.NewEnvironment()
	assert.NotNil(env)

	///////////////////////////////////
	//
	// the constructor should return errors without proper inputs

	app, err := NewApplication(nil, nil)
	assert.Nil(app)
	assert.Error(err)

	app, err = NewApplication(env, nil)
	assert.Nil(app)
	assert.Error(err)

	conf := &model.Configuration{}
	app, err = NewApplication(nil, conf)
	assert.Nil(app)
	assert.Error(err)

	///////////////////////////////////
	//
	// configure a valid, noop configuration without any generators defined

	app, err = NewApplication(env, conf)
	assert.NotNil(app)
	assert.NoError(err)
	assert.Len(app.Generators, 0)

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

	env.MigrationRegistry["manualOne"] = func(s db.Session, d bson.RawD) error { return nil }
	conf.ManualMigrations = []model.ConfigurationManualMigration{
		{
			Options: model.GeneratorOptions{
				JobID: "foo-1",
				NS:    model.Namespace{DB: "db", Collection: "coll"},
				Query: map[string]interface{}{"_id": "1"}},
			Name: "manualOne",
		},
	}

	env.ProcessorRegistry["streamOne"] = &mock.Processor{}
	conf.StreamMigrations = []model.ConfigurationManualMigration{
		{
			Options: model.GeneratorOptions{
				JobID: "foo-2",
				NS:    model.Namespace{DB: "db", Collection: "coll"},
				Query: map[string]interface{}{"_id": "1"}},
			Name: "streamOne",
		},
	}

	app, err = NewApplication(env, conf)
	assert.NotNil(app)
	assert.NoError(err)
	assert.Len(app.Generators, 3)

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
	assert.Nil(app)
	assert.Error(err)
}
