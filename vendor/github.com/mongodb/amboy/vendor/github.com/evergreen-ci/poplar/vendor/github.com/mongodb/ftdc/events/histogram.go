// Histogram
//
// The histogram representation is broadly similar to the Performance structure
// but stores data in a histogram format, which offers a high fidelity
// representation of a very large number of raw events without the storage
// overhead. In general, use histograms to collect data for operation with
// throughput in the thousands or more operations per second.
package events

import (
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/ftdc/hdrhist"
)

// PerformanceHDR the same as the Performance structure, but with all time
// duration and counter values stored as histograms.
type PerformanceHDR struct {
	Timestamp time.Time              `bson:"ts" json:"ts" yaml:"ts"`
	ID        int64                  `bson:"id" json:"id" yaml:"id"`
	Counters  PerformanceCountersHDR `bson:"counters" json:"counters" yaml:"counters"`
	Timers    PerformanceTimersHDR   `bson:"timers" json:"timers" yaml:"timers"`
	Gauges    PerformanceGauges      `bson:"guages" json:"guages" yaml:"guages"`
}

type PerformanceCountersHDR struct {
	Number     *hdrhist.Histogram `bson:"n" json:"n" yaml:"n"`
	Operations *hdrhist.Histogram `bson:"ops" json:"ops" yaml:"ops"`
	Size       *hdrhist.Histogram `bson:"size" json:"size" yaml:"size"`
	Errors     *hdrhist.Histogram `bson:"errors" json:"errors" yaml:"errors"`
}

type PerformanceTimersHDR struct {
	Duration *hdrhist.Histogram `bson:"dur" json:"dur" yaml:"dur"`
	Total    *hdrhist.Histogram `bson:"total" json:"total" yaml:"total"`
}

func NewHistogramSecond(g PerformanceGauges) *PerformanceHDR {
	return &PerformanceHDR{
		Gauges: g,
		Counters: PerformanceCountersHDR{
			Number:     newSecondCounterHistogram(),
			Operations: newSecondCounterHistogram(),
			Size:       newSecondCounterHistogram(),
			Errors:     newSecondCounterHistogram(),
		},
		Timers: PerformanceTimersHDR{
			Duration: newSecondDurationHistogram(),
			Total:    newSecondDurationHistogram(),
		},
	}
}

func newSecondDurationHistogram() *hdrhist.Histogram {
	return hdrhist.New(int64(time.Microsecond), int64(20*time.Minute), 5)
}

func newSecondCounterHistogram() *hdrhist.Histogram {
	return hdrhist.New(0, 10*100*1000, 5)
}

func NewHistogramMillisecond(g PerformanceGauges) *PerformanceHDR {
	return &PerformanceHDR{
		Gauges: g,
		Counters: PerformanceCountersHDR{
			Number:     newMillisecondCounterHistogram(),
			Operations: newMillisecondCounterHistogram(),
			Size:       newMillisecondCounterHistogram(),
			Errors:     newMillisecondCounterHistogram(),
		},
		Timers: PerformanceTimersHDR{
			Duration: newMillisecondDurationHistogram(),
			Total:    newMillisecondDurationHistogram(),
		},
	}
}

func newMillisecondDurationHistogram() *hdrhist.Histogram {
	return hdrhist.New(int64(time.Microsecond), int64(time.Minute), 5)
}

func newMillisecondCounterHistogram() *hdrhist.Histogram {
	return hdrhist.New(0, 10*1000, 5)
}

func (p *PerformanceHDR) MarshalDocument() (*birch.Document, error) {
	return birch.DC.Elements(
		birch.EC.Time("ts", p.Timestamp),
		birch.EC.Int64("id", p.ID),
		birch.EC.SubDocumentFromElements("counters",
			birch.EC.DocumentMarshaler("n", p.Counters.Number),
			birch.EC.DocumentMarshaler("ops", p.Counters.Operations),
			birch.EC.DocumentMarshaler("size", p.Counters.Size),
			birch.EC.DocumentMarshaler("errors", p.Counters.Errors),
		),
		birch.EC.SubDocumentFromElements("timers",
			birch.EC.DocumentMarshaler("dur", p.Timers.Duration),
			birch.EC.DocumentMarshaler("total", p.Timers.Total),
		),
		birch.EC.SubDocumentFromElements("gauges",
			birch.EC.Int64("state", p.Gauges.State),
			birch.EC.Int64("workers", p.Gauges.Workers),
			birch.EC.Boolean("failed", p.Gauges.Failed),
		),
	), nil
}

func (p *PerformanceHDR) setTimestamp(started time.Time) {
	if p.Timestamp.IsZero() {
		if !started.IsZero() {
			p.Timestamp = started
		} else {
			p.Timestamp = time.Now()
		}
	}
}
