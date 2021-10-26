package anser

import (
	"context"
	"testing"

	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/anser/mock"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamMigrationJob(t *testing.T) {
	const (
		jobTypeName       = "stream-migration"
		processorTypeName = "processor-name"
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := mock.NewEnvironment()
	mh := &MigrationHelperMock{Environment: env}

	migration := NewStreamMigration(env, model.Stream{})
	require.Equal(t, jobTypeName, migration.Type().Name)

	factory, err := registry.GetJobFactory(jobTypeName)
	require.NoError(t, err)
	t.Run("Factory", func(t *testing.T) {
		// first test the factory
		job, ok := factory().(*streamMigrationJob)
		require.True(t, ok)
		assert.Equal(t, job.Type().Name, jobTypeName)
	})
	t.Run("ProcessorNotDefined", func(t *testing.T) {
		job := factory().(*streamMigrationJob)
		// first we start off with a "processor not defined error"
		job.MigrationHelper = mh
		job.Definition.ProcessorName = processorTypeName
		job.Run(ctx)
		assert.True(t, job.Status().Completed)
		require.True(t, job.HasErrors())
		err := job.Error()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), job.Definition.ProcessorName)
	})

	t.Run("Processor", func(t *testing.T) {
		processor := &mock.Processor{}
		env.ProcessorRegistry[processorTypeName] = processor
		defer func() { delete(env.ProcessorRegistry, processorTypeName) }()

		t.Run("BadIterator", func(t *testing.T) {
			// now we find a poorly configured/implemented processor
			job := factory().(*streamMigrationJob)
			job.Definition.ProcessorName = processorTypeName
			job.MigrationHelper = mh
			job.Run(ctx)
			assert.True(t, job.Status().Completed)
			require.True(t, job.HasErrors())
			err := job.Error()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "could not return iterator")
		})
		t.Run("GoodIterator", func(t *testing.T) {
			// reset for next case.
			// now we have a properly configured job iterator
			job := factory().(*streamMigrationJob)
			job.MigrationHelper = mh
			job.Definition.ProcessorName = processorTypeName
			processor.Cursor = &mock.Cursor{}
			// TODO write mock cursor
			job.Run(ctx)
			assert.False(t, job.HasErrors())
			assert.True(t, job.Status().Completed)
		})
		t.Run("IteratorError", func(t *testing.T) {
			// reset for the next case:
			// we have an iterator that returns an error.
			job := factory().(*streamMigrationJob)
			job.MigrationHelper = mh
			job.Definition.ProcessorName = processorTypeName
			processor.MigrateError = errors.New("has error 123")
			job.Run(ctx)
			assert.True(t, job.Status().Completed)
			require.True(t, job.HasErrors())
			err := job.Error()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "has error 123")
		})
		t.Run("NoClient", func(t *testing.T) {
			env.ClientError = errors.New("no client")

			job := factory().(*streamMigrationJob)
			job.MigrationHelper = mh
			job.Definition.ProcessorName = processorTypeName
			job.Run(ctx)
			assert.True(t, job.Status().Completed)
			require.True(t, job.HasErrors())
			err := job.Error()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "no client")
		})
	})

}
