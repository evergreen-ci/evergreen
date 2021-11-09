// Performance Points
//
// The Performance type represents a unified event to track an operation in a
// performance test. These events record three types of metrics: counters,
// timers, and gauges. Counters record the number of operations in different
// ways, including test iterations, logical operation counts, operation size
// (bytes), and error rate. Timers include both the latency of the core
// operation, for use in calculating latencies as well as the total taken which
// may be useful in calculating throughput. Finally gauges, capture changes in
// state or other information about the environment including the number of
// threads used in the test, or a failed Boolean when a test is aware of its
// own failure.
package events

import (
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/pkg/errors"
)

// Performance represents a single raw event in a metrics collection system for
// performance metric collection system.
//
// Each point must report the timestamp of its collection.
type Performance struct {
	Timestamp time.Time           `bson:"ts" json:"ts" yaml:"ts"`
	ID        int64               `bson:"id" json:"id" yaml:"id"`
	Counters  PerformanceCounters `bson:"counters" json:"counters" yaml:"counters"`
	Timers    PerformanceTimers   `bson:"timers" json:"timers" yaml:"timers"`
	Gauges    PerformanceGauges   `bson:"gauges" json:"gauges" yaml:"gauges"`
}

// PerformanceCounters refer to the number of operations/events or total of
// things since the last collection point. These values are used in computing
// various kinds of throughput measurements.
type PerformanceCounters struct {
	Number     int64 `bson:"n" json:"n" yaml:"n"`
	Operations int64 `bson:"ops" json:"ops" yaml:"ops"`
	Size       int64 `bson:"size" json:"size" yaml:"size"`
	Errors     int64 `bson:"errors" json:"errors" yaml:"errors"`
}

// PerformanceTimers refers to all of the timing data for this event. In
// general Total should equal the time since the last data point.
type PerformanceTimers struct {
	Duration time.Duration `bson:"dur" json:"dur" yaml:"dur"`
	Total    time.Duration `bson:"total" json:"total" yaml:"total"`
}

// PerformanceGauges holds simple counters that aren't expected to change
// between points, but are useful as annotations of the experiment or
// or descriptions of events in the system configuration.
type PerformanceGauges struct {
	State   int64 `bson:"state" json:"state" yaml:"state"`
	Workers int64 `bson:"workers" json:"workers" yaml:"workers"`
	Failed  bool  `bson:"failed" json:"failed" yaml:"failed"`
}

// MarshalBSON implements the bson marshaler interface to support
// converting this type into BSON without relying on a
// reflection-based BSON library.
func (p *Performance) MarshalBSON() ([]byte, error) { return birch.MarshalDocumentBSON(p) }

// MarshalDocument exports the Performance type as a birch.Document to
// support more efficient operations.
func (p *Performance) MarshalDocument() (*birch.Document, error) {
	return birch.DC.Elements(
		birch.EC.Time("ts", p.Timestamp),
		birch.EC.Int64("id", p.ID),
		birch.EC.SubDocument("counters", birch.DC.Elements(
			birch.EC.Int64("n", p.Counters.Number),
			birch.EC.Int64("ops", p.Counters.Operations),
			birch.EC.Int64("size", p.Counters.Size),
			birch.EC.Int64("errors", p.Counters.Errors),
		)),
		birch.EC.SubDocument("timers", birch.DC.Elements(
			birch.EC.Duration("dur", p.Timers.Duration),
			birch.EC.Duration("total", p.Timers.Total),
		)),
		birch.EC.SubDocument("gauges", birch.DC.Elements(
			birch.EC.Int64("state", p.Gauges.State),
			birch.EC.Int64("workers", p.Gauges.Workers),
			birch.EC.Boolean("failed", p.Gauges.Failed),
		)),
	), nil
}

func (p *Performance) UnmarshalDocument(doc *birch.Document) error {
	iter := doc.Iterator()
	for iter.Next() {
		elem := iter.Element()
		switch elem.Key() {
		case "ts":
			p.Timestamp = elem.Value().Time()
		case "id":
		case "counters":
			if err := p.Counters.UnmarshalDocument(elem.Value().MutableDocument()); err != nil {
				return errors.WithStack(err)
			}
		case "timers":
			if err := p.Timers.UnmarshalDocument(elem.Value().MutableDocument()); err != nil {
				return errors.WithStack(err)
			}
		case "gauges":
			if err := p.Gauges.UnmarshalDocument(elem.Value().MutableDocument()); err != nil {
				return errors.WithStack(err)
			}
		}
	}

	return errors.WithStack(iter.Err())
}

func (p *PerformanceCounters) UnmarshalDocument(doc *birch.Document) error {
	iter := doc.Iterator()
	for iter.Next() {
		elem := iter.Element()
		switch elem.Key() {
		case "n":
			p.Number = elem.Value().Int64()
		case "opts":
			p.Operations = elem.Value().Int64()
		case "size":
			p.Size = elem.Value().Int64()
		case "errors":
			p.Errors = elem.Value().Int64()
		}
	}

	return errors.WithStack(iter.Err())
}

func (p *PerformanceTimers) UnmarshalDocument(doc *birch.Document) error {
	iter := doc.Iterator()
	for iter.Next() {
		elem := iter.Element()
		switch elem.Key() {
		case "dur":
			p.Duration = time.Duration(elem.Value().Int64())
		case "total":
			p.Total = time.Duration(elem.Value().Int64())
		}
	}

	return errors.WithStack(iter.Err())
}

func (p *PerformanceGauges) UnmarshalDocument(doc *birch.Document) error {
	iter := doc.Iterator()
	for iter.Next() {
		elem := iter.Element()
		switch elem.Key() {
		case "state":
			p.State = elem.Value().Int64()
		case "workers":
			p.Workers = elem.Value().Int64()
		case "failed":
			p.Failed = elem.Value().Boolean()
		}
	}

	return errors.WithStack(iter.Err())
}

// Add combines the values of the input Performance struct into this struct,
// logically, overriding the Gauges values as well as the timestamp and ID
// ID values, while summing the Counters and Timers values.
func (p *Performance) Add(in *Performance) {
	if in.ID == 0 {
		in.ID = p.ID + 1
	}

	p.Timestamp = in.Timestamp
	p.ID = in.ID
	p.Counters.Number += in.Counters.Number
	p.Counters.Errors += in.Counters.Errors
	p.Counters.Operations += in.Counters.Operations
	p.Counters.Size += in.Counters.Size

	p.Timers.Duration += in.Timers.Duration
	p.Timers.Total += in.Timers.Total

	p.Gauges.Failed = in.Gauges.Failed
	p.Gauges.Workers = in.Gauges.Workers
	p.Gauges.State = in.Gauges.State
}

func (p *Performance) setTimestamp(started time.Time) {
	if p.Timestamp.IsZero() {
		if !started.IsZero() {
			p.Timestamp = started
		} else {
			p.Timestamp = time.Now()
		}
	}
}
