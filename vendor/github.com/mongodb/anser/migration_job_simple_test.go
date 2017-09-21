package anser

import (
	"testing"

	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/mongodb/anser/mock"
	"github.com/mongodb/anser/model"
)

func TestSimpleMigrationJob(t *testing.T) {
	assert := assert.New(t)
	env := mock.NewEnvironment()
	mh := &MigrationHelperMock{Environment: env}

	const jobTypeName = "simple-migration"

	// first test the factory and registry
	factory, err := registry.GetJobFactory(jobTypeName)
	assert.NoError(err)
	job, ok := factory().(*simpleMigrationJob)
	assert.True(ok)
	assert.Equal(jobTypeName, job.Type().Name)

	// verify that the factory doesn't share state.
	jone := factory().(*simpleMigrationJob)
	jone.SetID("foo")
	jtwo := factory().(*simpleMigrationJob)
	jtwo.SetID("bar")
	assert.NotEqual(jone, jtwo)

	// verify that the public constructor returns the correct type
	migration := NewSimpleMigration(env, model.Simple{})
	assert.NotNil(migration)
	assert.Equal(jobTypeName, migration.Type().Name)

	// run a test where we can't get a db session
	job.MigrationHelper = mh
	env.SessionError = errors.New("no session, sorry")
	job.Run()
	assert.True(job.Status().Completed)
	if assert.True(job.HasErrors()) {
		err = job.Error()
		assert.Error(err)
		assert.Contains(err.Error(), "no session, sorry")
	}

	// run a test were nothing happens so it's not an error
	job = factory().(*simpleMigrationJob)
	env.SessionError = nil
	job.MigrationHelper = mh
	job.Run()
	assert.True(job.Status().Completed)
	assert.False(job.HasErrors())

	// run a test where the writes fail and make sure the operation fails
	job = factory().(*simpleMigrationJob)
	job.Definition.Namespace = model.Namespace{DB: "foo", Collection: "bar"}
	env.SessionError = nil
	env.Session = mock.NewSession()
	env.Session.DB("foo").C("bar").(*mock.Collection).FailWrites = true
	job.MigrationHelper = mh
	job.Run()
	assert.True(job.Status().Completed)
	if assert.True(job.HasErrors()) {
		err = job.Error()
		assert.Error(err)
		assert.Contains(err.Error(), "writes fail")
	}
}
