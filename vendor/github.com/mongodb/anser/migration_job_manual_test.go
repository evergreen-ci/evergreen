package anser

import (
	"context"
	"testing"

	"gopkg.in/mgo.v2/bson"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/anser/client"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/mock"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManualMigration(t *testing.T) {
	env := mock.NewEnvironment()
	mh := &MigrationHelperMock{Environment: env}
	ctx := context.Background()
	const jobTypeName = "manual-migration"

	factory, err := registry.GetJobFactory(jobTypeName)
	require.NoError(t, err)
	job, ok := factory().(*manualMigrationJob)
	require.True(t, ok)
	require.Equal(t, jobTypeName, job.Type().Name)

	t.Run("Factory", func(t *testing.T) {
		// verify that the factory doesn't share state.
		jone := factory().(*manualMigrationJob)
		jone.SetID("foo")
		jtwo := factory().(*manualMigrationJob)
		jtwo.SetID("bar")
		assert.NotEqual(t, jone, jtwo)
	})
	t.Run("Constructor", func(t *testing.T) {
		// verify that the public constructor returns the correct type
		migration := NewManualMigration(env, model.Manual{})
		assert.NotNil(t, migration)
		assert.Equal(t, jobTypeName, migration.Type().Name)
	})

	t.Run("UnregisteredJob", func(t *testing.T) {
		job := factory().(*manualMigrationJob)
		job.MigrationHelper = mh
		job.Run(ctx)
		assert.True(t, job.Status().Completed)

		require.True(t, job.HasErrors())
		err = job.Error()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not find migration named")
	})

	t.Run("Legacy", func(t *testing.T) {
		// set a migration job name
		assert.NoError(t, env.RegisterLegacyManualMigrationOperation("passing", func(s db.Session, d bson.RawD) error { return nil }))
		assert.NoError(t, env.RegisterLegacyManualMigrationOperation("failing", func(s db.Session, d bson.RawD) error { return errors.New("manual fail") }))
		defer func() { env.LegacyMigrationRegistry = make(map[string]db.MigrationOperation) }()

		t.Run("NoSession", func(t *testing.T) {
			// reset, define a job but have the session acquisition fail
			job = factory().(*manualMigrationJob)
			job.MigrationHelper = mh
			job.Definition.OperationName = "passing"
			env.SessionError = errors.New("no session, sorry")
			defer func() { env.SessionError = nil }()
			job.Run(ctx)
			assert.True(t, job.Status().Completed)

			require.True(t, job.HasErrors())
			err = job.Error()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "no session, sorry")
		})
		t.Run("Passing", func(t *testing.T) {
			job = factory().(*manualMigrationJob)
			job.MigrationHelper = mh
			job.Definition.OperationName = "passing"
			job.Run(ctx)
			assert.True(t, job.Status().Completed)
			assert.False(t, job.HasErrors())
		})
		t.Run("Failing", func(t *testing.T) {
			// reset and have a job that always fails and make sure the error propagates
			job = factory().(*manualMigrationJob)
			job.MigrationHelper = mh
			job.Definition.OperationName = "failing"
			job.Run(ctx)
			assert.True(t, job.Status().Completed)

			require.True(t, job.HasErrors())
			err = job.Error()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "manual fail")
		})
		t.Run("DBError", func(t *testing.T) {
			// reset and have the db operation fail
			job = factory().(*manualMigrationJob)
			job.Definition.OperationName = "passing"
			job.Definition.Namespace = model.Namespace{DB: "foo", Collection: "bar"}
			job.MigrationHelper = mh
			env.Session = mock.NewSession()
			env.Session.DB("foo").C("bar").(*mock.LegacyCollection).QueryError = errors.New("query error")
			job.Run(ctx)
			assert.True(t, job.Status().Completed)

			require.True(t, job.HasErrors())
			err = job.Error()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "query error")
		})
	})
	t.Run("Client", func(t *testing.T) {
		assert.NoError(t, env.RegisterManualMigrationOperation("passing", func(c client.Client, d *birch.Document) error { return nil }))
		assert.NoError(t, env.RegisterManualMigrationOperation("failing", func(c client.Client, d *birch.Document) error { return errors.New("manual fail") }))
		defer func() { env.MigrationRegistry = make(map[string]client.MigrationOperation) }()

		t.Run("NoClient", func(t *testing.T) {
			// reset, define a job but have the session acquisition fail
			job = factory().(*manualMigrationJob)
			job.MigrationHelper = mh
			job.Definition.OperationName = "passing"
			env.ClientError = errors.New("no client, sorry")
			defer func() { env.ClientError = nil }()
			job.Run(ctx)
			assert.True(t, job.Status().Completed)

			require.True(t, job.HasErrors())
			err = job.Error()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "no client, sorry")
		})
		t.Run("Passing", func(t *testing.T) {
			env.Client = mock.NewClient()
			job = factory().(*manualMigrationJob)
			job.MigrationHelper = mh
			job.Definition.OperationName = "passing"
			job.Run(ctx)
			assert.True(t, job.Status().Completed)
			assert.False(t, job.HasErrors())
		})
		t.Run("Failing", func(t *testing.T) {
			// reset and have a job that always fails and make sure the error propagates
			job = factory().(*manualMigrationJob)
			job.MigrationHelper = mh
			job.Definition.OperationName = "failing"
			job.Run(ctx)
			assert.True(t, job.Status().Completed)

			require.True(t, job.HasErrors())
			err = job.Error()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "manual fail")
		})
		t.Run("DBErrors", func(t *testing.T) {
			t.Run("FindOne", func(t *testing.T) {
				env.Client = mock.NewClient()
				res := mock.NewSingleResult()
				res.ErrorValue = errors.New("not found")
				env.Client.Databases["foo"] = &mock.Database{DBName: "foo", Collections: map[string]*mock.Collection{"bar": &mock.Collection{SingleResult: res}}}

				job = factory().(*manualMigrationJob)
				job.Definition.Namespace = model.Namespace{DB: "foo", Collection: "bar"}
				job.MigrationHelper = mh
				job.Definition.OperationName = "passing"
				job.Run(ctx)
				assert.True(t, job.Status().Completed)
				require.True(t, job.HasErrors())
				err = job.Error()
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not found")
			})
			t.Run("DecodeError", func(t *testing.T) {
				env.Client = mock.NewClient()
				res := mock.NewSingleResult()
				res.DecodeBytesError = errors.New("could not decode")
				env.Client.Databases["foo"] = &mock.Database{DBName: "foo", Collections: map[string]*mock.Collection{"bar": &mock.Collection{SingleResult: res}}}

				job = factory().(*manualMigrationJob)
				job.Definition.Namespace = model.Namespace{DB: "foo", Collection: "bar"}
				job.MigrationHelper = mh
				job.Definition.OperationName = "passing"
				job.Run(ctx)
				assert.True(t, job.Status().Completed)
				require.True(t, job.HasErrors())
				err = job.Error()
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "could not decode")
			})
			t.Run("InvalidBSON", func(t *testing.T) {
				env.Client = mock.NewClient()
				res := mock.NewSingleResult()
				res.DecodeBytesValue = []byte{}
				env.Client.Databases["foo"] = &mock.Database{DBName: "foo", Collections: map[string]*mock.Collection{"bar": &mock.Collection{SingleResult: res}}}

				job = factory().(*manualMigrationJob)
				job.Definition.Namespace = model.Namespace{DB: "foo", Collection: "bar"}
				job.MigrationHelper = mh
				job.Definition.OperationName = "passing"
				job.Run(ctx)
				assert.True(t, job.Status().Completed)
				require.True(t, job.HasErrors())
				err = job.Error()
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "error: too small")

			})
		})

	})

}
