package events

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/mongodb/ftdc"
)

func BenchmarkEventCollection(b *testing.B) {
	for _, collect := range []struct {
		Name    string
		Factory func() ftdc.Collector
	}{
		{
			Name:    "Base",
			Factory: func() ftdc.Collector { return ftdc.NewBaseCollector(1000) },
		},
		{
			Name:    "Dynamic",
			Factory: func() ftdc.Collector { return ftdc.NewDynamicCollector(1000) },
		},
		{
			Name:    "Streaming",
			Factory: func() ftdc.Collector { return ftdc.NewStreamingCollector(1000, &bytes.Buffer{}) },
		},
		{
			Name:    "StreamingDynamic",
			Factory: func() ftdc.Collector { return ftdc.NewStreamingDynamicCollector(1000, &bytes.Buffer{}) },
		},
	} {

		b.Run(collect.Name, func(b *testing.B) {
			for _, test := range []struct {
				Name      string
				Generator func() interface{}
			}{
				{
					Name: "Performance",
					Generator: func() interface{} {
						return &Performance{
							Timestamp: time.Now(),
							Counters: PerformanceCounters{
								Number:     rand.Int63n(128),
								Operations: rand.Int63n(512),
								Size:       rand.Int63n(1024),
								Errors:     rand.Int63n(64),
							},
							Timers: PerformanceTimers{
								Duration: time.Duration(rand.Int63n(int64(time.Minute))),
								Total:    time.Duration(rand.Int63n(int64(2 * time.Minute))),
							},
							Gauges: PerformanceGauges{
								State:   rand.Int63n(2),
								Workers: rand.Int63n(4),
							},
						}
					},
				},
				{
					Name: "HistogramSecondZeroed",
					Generator: func() interface{} {
						return NewHistogramSecond(PerformanceGauges{})
					},
				},
				{
					Name: "HistogramMillisecondZeroed",
					Generator: func() interface{} {
						return NewHistogramMillisecond(PerformanceGauges{})
					},
				},
				{
					Name: "CustomMidsized",
					Generator: func() interface{} {
						point := MakeCustom(20)
						for i := int64(1); i <= 10; i++ {
							point.Add(fmt.Sprintln("stat", i), rand.Int63n(i))            // nolint
							point.Add(fmt.Sprintln("stat", i), float64(i)+rand.Float64()) // nolint
						}
						return point
					},
				},
				{
					Name: "CustomSmall",
					Generator: func() interface{} {
						point := MakeCustom(4)
						for i := int64(1); i <= 2; i++ {
							point.Add(fmt.Sprintln("stat", i), rand.Int63n(i))            // nolint
							point.Add(fmt.Sprintln("stat", i), float64(i)+rand.Float64()) // nolint
						}
						return point
					},
				},
			} {
				collector := collect.Factory()
				b.Run(test.Name, func(b *testing.B) {
					b.Run("Add", func(b *testing.B) {
						for n := 0; n < b.N; n++ {
							collector.Add(test.Generator()) // nolint
						}
					})
					b.Run("Resolve", func(b *testing.B) {
						for n := 0; n < b.N; n++ {
							collector.Resolve() // nolint
						}
					})
				})
			}
		})
	}
}
