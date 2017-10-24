package anser

import (
	"testing"

	"gopkg.in/mgo.v2/bson"

	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/mock"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func passingManualMigrationOp(s db.Session, d bson.RawD) error { return nil }
func failingManualMigrationOp(s db.Session, d bson.RawD) error { return errors.New("manual fail") }

func TestManualMigration(t *testing.T) {
	assert := assert.New(t)
	env := mock.NewEnvironment()
	mh := &MigrationHelperMock{Environment: env}
	const jobTypeName = "manual-migration"

	factory, err := registry.GetJobFactory(jobTypeName)
	assert.NoError(err)
	job, ok := factory().(*manualMigrationJob)
	assert.True(ok)
	assert.Equal(jobTypeName, job.Type().Name)

	// verify that the factory doesn't share state.
	jone := factory().(*manualMigrationJob)
	jone.SetID("foo")
	jtwo := factory().(*manualMigrationJob)
	jtwo.SetID("bar")
	assert.NotEqual(jone, jtwo)

	// verify that the public constructor returns the correct type
	migration := NewManualMigration(env, model.Manual{})
	assert.NotNil(migration)
	assert.Equal(jobTypeName, migration.Type().Name)

	job.MigrationHelper = mh
	job.Run()
	assert.True(job.Status().Completed)
	if assert.True(job.HasErrors()) {
		err = job.Error()
		assert.Error(err)
		assert.Contains(err.Error(), "could not find migration operation")
	}

	// set a migration job name
	assert.NoError(env.RegisterManualMigrationOperation("passing", passingManualMigrationOp))
	assert.NoError(env.RegisterManualMigrationOperation("failing", failingManualMigrationOp))

	// reset, define a job but have the session acquisition fail
	job = factory().(*manualMigrationJob)
	job.MigrationHelper = mh
	job.Definition.OperationName = "passing"
	env.SessionError = errors.New("no session, sorry")
	job.Run()
	assert.True(job.Status().Completed)
	if assert.True(job.HasErrors()) {
		err = job.Error()
		assert.Error(err)
		assert.Contains(err.Error(), "no session, sorry")
	}

	// reset, define a job but have the session acquisition fail
	job = factory().(*manualMigrationJob)
	job.MigrationHelper = mh
	job.Definition.OperationName = "passing"
	env.SessionError = nil
	job.Run()
	assert.True(job.Status().Completed)
	assert.False(job.HasErrors())

	// reset and have a job that always fails and make sure the error propagates
	job = factory().(*manualMigrationJob)
	job.MigrationHelper = mh
	job.Definition.OperationName = "failing"
	job.Run()
	assert.True(job.Status().Completed)
	if assert.True(job.HasErrors()) {
		err = job.Error()
		assert.Error(err)
		assert.Contains(err.Error(), "manual fail")
	}

	// reset and have the db operation fail
	job = factory().(*manualMigrationJob)
	job.Definition.OperationName = "passing"
	job.Definition.Namespace = model.Namespace{DB: "foo", Collection: "bar"}
	job.MigrationHelper = mh
	env.Session = mock.NewSession()
	env.Session.DB("foo").C("bar").(*mock.Collection).QueryError = errors.New("query error")
	job.Run()
	assert.True(job.Status().Completed)
	if assert.True(job.HasErrors()) {
		err = job.Error()
		assert.Error(err)
		assert.Contains(err.Error(), "query error")
	}
}
