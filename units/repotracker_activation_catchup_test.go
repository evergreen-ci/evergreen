package units

import (
	"testing"

	"github.com/mongodb/amboy/registry"
	"github.com/stretchr/testify/assert"
)

func TestStepbackActivationJob(t *testing.T) {
	// the main thing we're worried about here, is jobs getting
	// different IDs, somehow. Internally this is just glue code
	// to get the job to execute in the right place.

	assert := assert.New(t) // nolint

	factory, err := registry.GetJobFactory(stepbackActivationCatchupJobName)
	assert.NoError(err)
	assert.NotNil(factory)

	j, ok := factory().(*stepbackActivationCatchup)
	assert.True(ok)
	assert.NotNil(j)

	jOne := NewStepbackActiationJob("foo", "id")
	jTwo := NewStepbackActiationJob("foo", "id")
	jThree := NewStepbackActiationJob("foo", "id0")
	assert.Equal(jOne.ID(), jTwo.ID())
	assert.Equal(jOne, jTwo)
	assert.NotEqual(jThree.ID(), jOne.ID())
}
