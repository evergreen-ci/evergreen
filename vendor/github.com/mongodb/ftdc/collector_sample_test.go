package ftdc

import (
	"testing"
	"time"

	"github.com/mongodb/ftdc/testutil"
	"github.com/stretchr/testify/assert"
)

func TestSamplingCollector(t *testing.T) {
	collector := NewSamplingCollector(10*time.Millisecond, &betterCollector{maxDeltas: 20})
	assert.Equal(t, 0, collector.Info().SampleCount)
	for i := 0; i < 10; i++ {
		assert.NoError(t, collector.Add(testutil.RandFlatDocument(20)))
	}
	assert.Equal(t, 1, collector.Info().SampleCount)

	for i := 0; i < 4; i++ {
		time.Sleep(10 * time.Millisecond)
		assert.NoError(t, collector.Add(testutil.RandFlatDocument(20)))
	}

	assert.Equal(t, 5, collector.Info().SampleCount)
}
