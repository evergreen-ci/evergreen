package anser

import (
	"context"
	"testing"

	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/anser/mock"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestStreamMigrationJob(t *testing.T) {
	assert := assert.New(t)
	const (
		jobTypeName       = "stream-migration"
		processorTypeName = "processor-name"
	)
	ctx := context.Background()
	env := mock.NewEnvironment()
	mh := &MigrationHelperMock{Environment: env}

	migration := NewStreamMigration(env, model.Stream{})
	assert.Equal(jobTypeName, migration.Type().Name)

	// first test the factory
	factory, err := registry.GetJobFactory(jobTypeName)
	assert.NoError(err)
	job, ok := factory().(*streamMigrationJob)
	assert.True(ok)
	assert.Equal(job.Type().Name, jobTypeName)

	// now we run the jobs a couple of times to verify expected behavior and outcomes

	// first we start off with a "processor not defined error"
	job.MigrationHelper = mh
	job.Definition.ProcessorName = processorTypeName
	job.Run(ctx)
	assert.True(job.Status().Completed)
	if assert.True(job.HasErrors()) {
		err = job.Error()
		assert.Error(err)
		assert.Contains(err.Error(), job.Definition.ProcessorName)
	}

	// set up the processor
	processor := &mock.Processor{}
	env.ProcessorRegistry[processorTypeName] = processor

	// reset for next case.
	// now we find a poorly configured/implemented processor
	job = factory().(*streamMigrationJob)
	job.Definition.ProcessorName = processorTypeName
	job.MigrationHelper = mh
	job.Run(ctx)
	assert.True(job.Status().Completed)
	if assert.True(job.HasErrors()) {
		err = job.Error()
		assert.Error(err)
		assert.Contains(err.Error(), "could not return iterator")
	}

	// reset for next case.
	// now we have a properly configured job iterator
	job = factory().(*streamMigrationJob)
	job.MigrationHelper = mh
	job.Definition.ProcessorName = processorTypeName
	processor.Iter = &mock.Iterator{}
	job.Run(ctx)
	assert.False(job.HasErrors())
	assert.True(job.Status().Completed)

	// reset for the next case:
	// we have an iterator that returns an error.
	job = factory().(*streamMigrationJob)
	job.MigrationHelper = mh
	job.Definition.ProcessorName = processorTypeName
	processor.MigrateError = errors.New("has error 123")
	job.Run(ctx)
	assert.True(job.Status().Completed)
	if assert.True(job.HasErrors()) {
		err = job.Error()
		assert.Error(err)
		assert.Contains(err.Error(), "has error 123")
	}
}
