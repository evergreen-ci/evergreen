package anser

import (
	"context"
	"testing"

	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/anser/client"
	"github.com/mongodb/anser/mock"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleMigrationJob(t *testing.T) {
	env := mock.NewEnvironment()
	mh := &MigrationHelperMock{Environment: env}
	ctx := context.Background()

	const jobTypeName = "simple-migration"

	// first test the factory and registry
	factory, err := registry.GetJobFactory(jobTypeName)
	require.NoError(t, err)
	job, ok := factory().(*simpleMigrationJob)
	require.True(t, ok)
	require.Equal(t, jobTypeName, job.Type().Name)

	t.Run("Factory", func(t *testing.T) {
		// verify that the factory doesn't share state.
		jone, ok := factory().(*simpleMigrationJob)
		require.True(t, ok)
		jone.SetID("foo")
		jtwo, ok := factory().(*simpleMigrationJob)
		require.True(t, ok)
		jtwo.SetID("bar")
		assert.NotEqual(t, jone, jtwo)
	})
	t.Run("Constructor", func(t *testing.T) {
		// verify that the public constructor returns the correct type
		migration := NewSimpleMigration(env, model.Simple{})
		assert.NotNil(t, migration)
		assert.Equal(t, jobTypeName, migration.Type().Name)
	})

	t.Run("Legacy", func(t *testing.T) {
		env.ShouldPreferClient = false
		t.Run("Noop", func(t *testing.T) {
			// run a test were nothing happens so it's not an error
			job = factory().(*simpleMigrationJob)
			job.MigrationHelper = mh
			job.Run(ctx)
			assert.True(t, job.Status().Completed)
			assert.False(t, job.HasErrors())
		})
		t.Run("FailedOperation", func(t *testing.T) {
			// run a test where the writes fail and make sure the operation fails
			job = factory().(*simpleMigrationJob)
			job.Definition.Namespace = model.Namespace{DB: "foo", Collection: "bar"}
			env.Session = mock.NewSession()
			env.Session.DB("foo").C("bar").(*mock.LegacyCollection).FailWrites = true
			job.MigrationHelper = mh
			job.Run(ctx)

			assert.True(t, job.Status().Completed)
			require.True(t, job.HasErrors())
			err = job.Error()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "writes fail")
		})
		t.Run("NoSession", func(t *testing.T) {
			job = factory().(*simpleMigrationJob)
			// run a test where we can't get a db session
			job.MigrationHelper = mh
			env.SessionError = errors.New("no session, sorry")
			job.Run(ctx)
			assert.True(t, job.Status().Completed)
			require.True(t, job.HasErrors())
			err = job.Error()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "no session, sorry")
		})
	})
	t.Run("Client", func(t *testing.T) {
		env.ShouldPreferClient = true
		t.Run("SuccessfulOperation", func(t *testing.T) {
			env.Client = mock.NewClient()
			env.Client.Databases["foo"] = &mock.Database{DBName: "foo", Collections: map[string]*mock.Collection{"bar": &mock.Collection{UpdateResult: client.UpdateResult{ModifiedCount: 1}}}}
			// run a test were nothing happens so it's not an error
			job = factory().(*simpleMigrationJob)
			job.Definition.Namespace = model.Namespace{DB: "foo", Collection: "bar"}
			job.MigrationHelper = mh
			job.Run(ctx)
			assert.True(t, job.Status().Completed)
			assert.NoError(t, job.Error())
		})
		t.Run("FailedOperation", func(t *testing.T) {
			env.Client = mock.NewClient()
			env.Client.Databases["foo"] = &mock.Database{DBName: "foo", Collections: map[string]*mock.Collection{"bar": &mock.Collection{UpdateResult: client.UpdateResult{ModifiedCount: 0}}}}
			// run a test were nothing happens so it's not an error
			job = factory().(*simpleMigrationJob)
			job.Definition.Namespace = model.Namespace{DB: "foo", Collection: "bar"}
			job.MigrationHelper = mh
			job.Run(ctx)
			assert.True(t, job.Status().Completed)
			err = job.Error()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "could not update")
		})
		t.Run("NoClient", func(t *testing.T) {
			job = factory().(*simpleMigrationJob)
			// run a test where we can't get a db session
			job.MigrationHelper = mh
			env.ClientError = errors.New("no client")
			job.Run(ctx)
			assert.True(t, job.Status().Completed)
			require.True(t, job.HasErrors())
			err = job.Error()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no client")
		})
	})

}
