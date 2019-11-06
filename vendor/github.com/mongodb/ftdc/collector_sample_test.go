package ftdc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSamplingCollector(t *testing.T) {
	collector := NewSamplingCollector(time.Millisecond, &betterCollector{maxDeltas: 20})
	for i := 0; i < 10; i++ {
		assert.NoError(t, collector.Add(randFlatDocument(20)))
	}
	info := collector.Info()
	assert.Equal(t, 1, info.SampleCount)

	for i := 0; i < 4; i++ {
		time.Sleep(time.Millisecond)
		assert.NoError(t, collector.Add(randFlatDocument(20)))
	}

	info = collector.Info()
	assert.Equal(t, 5, info.SampleCount)
}
