package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/amboy/registry"
	"github.com/stretchr/testify/assert"
)

func TestVersionActivationJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testutil.TestSpan(ctx, t)

	// the main thing we're worried about here, is jobs getting
	// different IDs, somehow. Internally this is just glue code
	// to get the job to execute in the right place.

	assert := assert.New(t)

	factory, err := registry.GetJobFactory(versionActivationCatchupJobName)
	assert.NoError(err)
	assert.NotNil(factory)

	j, ok := factory().(*versionActivationCatchup)
	assert.True(ok)
	assert.NotNil(j)

	jOne := NewVersionActivationJob("id")
	jTwo := NewVersionActivationJob("id")
	jThree := NewVersionActivationJob("id0")
	assert.Equal(jOne.ID(), jTwo.ID())
	assert.Equal(jOne, jTwo)
	assert.NotEqual(jThree.ID(), jOne.ID())
}
