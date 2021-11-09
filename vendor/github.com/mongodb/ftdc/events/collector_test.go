package events

import (
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/ftdc"
	"github.com/stretchr/testify/assert"
)

func TestCollector(t *testing.T) {
	for _, fcTest := range []struct {
		name        string
		constructor func() ftdc.Collector
	}{
		{
			name: "Basic",
			constructor: func() ftdc.Collector {
				return ftdc.NewBaseCollector(100)
			},
		},
		{
			name: "Uncompressed",
			constructor: func() ftdc.Collector {
				return ftdc.NewUncompressedCollectorBSON(100)
			},
		},
		{
			name: "Dynamic",
			constructor: func() ftdc.Collector {
				return ftdc.NewDynamicCollector(100)
			},
		},
	} {
		t.Run(fcTest.name, func(t *testing.T) {
			for _, collectorTest := range []struct {
				name        string
				constructor func(ftdc.Collector) Collector
			}{
				{
					name: "Basic",
					constructor: func(fc ftdc.Collector) Collector {
						return NewBasicCollector(fc)
					},
				},
				{
					name: "Passthrough",
					constructor: func(fc ftdc.Collector) Collector {
						return NewPassthroughCollector(fc)
					},
				},
				{
					name: "SamplingAll",
					constructor: func(fc ftdc.Collector) Collector {
						return NewSamplingCollector(fc, 1)
					},
				},
				{
					name: "RandomSamplingAll",
					constructor: func(fc ftdc.Collector) Collector {
						return NewRandomSamplingCollector(fc, true, 1000)
					},
				},
				{
					name: "IntervalAll",
					constructor: func(fc ftdc.Collector) Collector {
						return NewIntervalCollector(fc, 0)
					},
				},
			} {
				t.Run(collectorTest.name, func(t *testing.T) {
					t.Run("Fixture", func(t *testing.T) {
						collector := collectorTest.constructor(fcTest.constructor())
						assert.NotNil(t, collector)
					})
					t.Run("AddMethod", func(t *testing.T) {
						collector := collectorTest.constructor(fcTest.constructor())
						assert.NoError(t, collector.Add(nil))
						assert.NoError(t, collector.Add(map[string]string{"foo": "bar"}))
					})
					t.Run("AddEvent", func(t *testing.T) {
						collector := collectorTest.constructor(fcTest.constructor())
						assert.Error(t, collector.AddEvent(nil))
						assert.Equal(t, 0, collector.Info().SampleCount)

						for idx, e := range []*Performance{
							{},
							{
								Timestamp: time.Now(),
								ID:        12,
							},
						} {
							assert.NoError(t, collector.AddEvent(e))
							assert.Equal(t, idx+1, collector.Info().SampleCount)
						}
					})
				})
			}
		})
	}
}

func TestFastMarshaling(t *testing.T) {
	assert.Implements(t, (*birch.DocumentMarshaler)(nil), &Performance{})
	assert.Implements(t, (*birch.DocumentMarshaler)(nil), &PerformanceHDR{})
	assert.Implements(t, (*birch.DocumentMarshaler)(nil), Custom{})
}
