package anser

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/anser/mock"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type doc struct {
	ID interface{} `bson:"_id"`
}

func TestStreamMigrationGenerator(t *testing.T) {
	ctx := context.Background()
	env := mock.NewEnvironment()
	mh := &MigrationHelperMock{Environment: env}
	const jobTypeName = "stream-migration-generator"

	factory, err := registry.GetJobFactory(jobTypeName)
	require.NoError(t, err)
	job, ok := factory().(*streamMigrationGenerator)
	require.True(t, ok)
	require.Equal(t, job.Type().Name, jobTypeName)

	ns := model.Namespace{DB: "foo", Collection: "bar"}
	opts := model.GeneratorOptions{}

	t.Run("Interface", func(t *testing.T) {
		assert.Implements(t, (*Generator)(nil), &streamMigrationGenerator{})
	})
	t.Run("Constructor", func(t *testing.T) {
		// check that the public method produces a reasonable object
		// of the correct type, without shared state
		generator := NewStreamMigrationGenerator(env, opts, "").(*streamMigrationGenerator)
		require.NotNil(t, generator)
		assert.Equal(t, generator.Type().Name, jobTypeName)
		assert.NotEqual(t, generator, job)
	})
	t.Run("DependencyCheck", func(t *testing.T) {
		// check that the run method returns an error if it can't get a dependency error
		env.NetworkError = errors.New("injected network error")
		job.MigrationHelper = mh
		job.Run(ctx)
		assert.True(t, job.Status().Completed)
		require.True(t, job.HasErrors())
		err = job.Error()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "injected network error")
		env.NetworkError = nil
	})
	t.Run("Legacy", func(t *testing.T) {
		require.False(t, env.PreferClient())
		t.Run("SessionError", func(t *testing.T) {
			// make sure that session acquisition errors propagate
			job = factory().(*streamMigrationGenerator)
			env.SessionError = errors.New("injected session error")
			job.MigrationHelper = mh
			job.Run(ctx)
			assert.True(t, job.Status().Completed)
			if assert.True(t, job.HasErrors()) {
				err = job.Error()
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "injected session error")
			}
			env.SessionError = nil
		})
		t.Run("IteratorError", func(t *testing.T) {
			// make sure errors closing the iterator propagate
			job = factory().(*streamMigrationGenerator)
			job.NS = ns
			job.MigrationHelper = mh
			env.Session = mock.NewSession()
			env.Session.DB("foo").C("bar").(*mock.LegacyCollection).QueryError = errors.New("query error")
			job.Run(ctx)
			assert.True(t, job.Status().Completed)
			if assert.True(t, job.HasErrors()) {
				err = job.Error()
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "query error")
			}
			env.Session.DB("foo").C("bar").(*mock.LegacyCollection).QueryError = nil // reset
		})
		t.Run("JobProduction", func(t *testing.T) {
			// check job production
			job.Migrations = []*streamMigrationJob{
				NewStreamMigration(env, model.Stream{}).(*streamMigrationJob),
				NewStreamMigration(env, model.Stream{}).(*streamMigrationJob),
				NewStreamMigration(env, model.Stream{}).(*streamMigrationJob),
			}
			counter := 0
			for migration := range job.Jobs() {
				require.NotNil(t, migration)
				counter++
			}
			require.Equal(t, 3, counter)
		})
		t.Run("Generation", func(t *testing.T) {
			// make sure that we generate the jobs we would expect to:
			job = factory().(*streamMigrationGenerator)
			job.NS = ns
			job.MigrationHelper = mh
			job.Limit = 3
			job.SetID("stream")
			iter := &mock.Iterator{
				ShouldIter: true,
				Results: []interface{}{
					&doc{"one"}, &doc{"two"}, &doc{"three"}, &doc{"four"},
				},
			}

			ids := job.generateLegacyJobs(env, iter)
			for idx, id := range ids {
				require.True(t, strings.HasPrefix(id, "stream."))
				require.True(t, strings.HasSuffix(id, fmt.Sprintf(".%d", idx)))
				switch idx {
				case 0:
					assert.Contains(t, id, ".one.")
				case 1:
					assert.Contains(t, id, ".two.")
				case 2:
					assert.Contains(t, id, ".three.")
				}
			}

			assert.Len(t, ids, 3)
			assert.Len(t, job.Migrations, 3)

			network, err := env.GetDependencyNetwork()
			require.NoError(t, err)
			network.AddGroup(job.ID(), ids)
			networkMap := network.Network()
			assert.Len(t, networkMap[job.ID()], 3)
		})
	})
	t.Run("Client", func(t *testing.T) {
		env.Client = mock.NewClient()
		env.ShouldPreferClient = true
		defer func() {
			env.ShouldPreferClient = false
			env.Client = nil
		}()
		require.True(t, env.PreferClient())
		t.Run("ClientError", func(t *testing.T) {
			// make sure that client acquisition errors propagate
			job = factory().(*streamMigrationGenerator)
			env.ClientError = errors.New("injected client error")
			job.MigrationHelper = mh
			job.Run(ctx)
			assert.True(t, job.Status().Completed)
			if assert.True(t, job.HasErrors()) {
				err = job.Error()
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "injected client error")
			}
			env.ClientError = nil

		})
		t.Run("FindError", func(t *testing.T) {
			// make sure that client acquisition errors propagate
			job = factory().(*streamMigrationGenerator)
			job.NS.DB = "foo"
			job.NS.Collection = "bar"
			assert.NotNil(t, env.Client.Database("foo").Collection("bar"))
			env.Client.Databases["foo"].Collections["bar"].FindError = errors.New("injected query error")
			job.MigrationHelper = mh
			job.Run(ctx)
			assert.True(t, job.Status().Completed)
			if assert.True(t, job.HasErrors()) {
				err = job.Error()
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "injected query error")
			}
			env.ClientError = nil
		})
		t.Run("Generation", func(t *testing.T) {
			defer func() { env.Network = mock.NewDependencyNetwork() }()
			env.Network = mock.NewDependencyNetwork()
			// make sure that we generate the jobs we would expect to:
			job = factory().(*streamMigrationGenerator)
			job.NS = ns
			job.MigrationHelper = mh
			job.Limit = 3
			job.SetID("stream")

			cursor := &mock.Cursor{
				Results: []interface{}{
					&doc{"one"}, &doc{"two"}, &doc{"three"}, &doc{"four"},
				},
				ShouldIter:   true,
				MaxNextCalls: 4,
			}

			ids := job.generateJobs(ctx, env, cursor)
			for idx, id := range ids {
				require.True(t, strings.HasPrefix(id, "stream."))
				require.True(t, strings.HasSuffix(id, fmt.Sprintf(".%d", idx)))
				switch idx {
				case 0:
					assert.Contains(t, id, ".one.")
				case 1:
					assert.Contains(t, id, ".two.")
				case 2:
					assert.Contains(t, id, ".three.")
				case 3:
					assert.Contains(t, id, ".four.")
				}
			}

			assert.Len(t, ids, 3)
			assert.Len(t, job.Migrations, 3)

			network, err := env.GetDependencyNetwork()
			require.NoError(t, err)
			network.AddGroup(job.ID(), ids)
			networkMap := network.Network()
			assert.Len(t, networkMap[job.ID()], 3)
		})
	})
}
