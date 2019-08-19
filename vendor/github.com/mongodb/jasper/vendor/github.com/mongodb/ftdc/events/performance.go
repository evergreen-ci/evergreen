// Performance Points
//
// The Performance type represents a unified event to track an
// operation in a performance test. These events record three types of
// metrics: counters, timers, and gauges. Counters record the number
// of operations in different ways, including test iterations, logical
// operations counts, operation size (bytes,) and error rate. Timers
// include both the latency of the core operation, for use in
// calculating latencies as well as the total taken which may be
// useful in calculating throughput. Finally gauges, capture changes
// in state or other information about the environment including the
// number threads used in the test, or a failed Boolean when a test is
// aware of its own failure.
package events

import "time"

// Performance represents a single raw event in a metrics collection
// system for performance metric collection system.
//
// Each point must report the timestamp of its collection.
type Performance struct {
	Timestamp time.Time           `bson:"ts" json:"ts" yaml:"ts"`
	ID        int64               `bson:"id" json:"id" yaml:"id"`
	Counters  PerformanceCounters `bson:"counters" json:"counters" yaml:"counters"`
	Timers    PerformanceTimers   `bson:"timers" json:"timers" yaml:"timers"`
	Gauges    PerformanceGauges   `bson:"gauges" json:"gauges" yaml:"gauges"`
}

// PerformanceCounters refer to the number of operations/events or total
// of things since the last collection point. These values are
// used in computing various kinds of throughput measurements.
type PerformanceCounters struct {
	Number     int64 `bson:"n" json:"n" yaml:"n"`
	Operations int64 `bson:"ops" json:"ops" yaml:"ops"`
	Size       int64 `bson:"size" json:"size" yaml:"size"`
	Errors     int64 `bson:"errors" json:"errors" yaml:"errors"`
}

// PerformanceTimers refers to all of the timing data for this event. In
// general Duration+Waiting should equal the time since the
// last data point.
type PerformanceTimers struct {
	Duration time.Duration `bson:"dur" json:"dur" yaml:"dur"`
	Total    time.Duration `bson:"total" json:"total" yaml:"total"`
}

// PerformanceGauges holds simple counters that aren't
// expected to change between points, but are useful as
// annotations of the experiment or descriptions of events in
// the system configuration.
type PerformanceGauges struct {
	State   int64 `bson:"state" json:"state" yaml:"state"`
	Workers int64 `bson:"workers" json:"workers" yaml:"workers"`
	Failed  bool  `bson:"failed" json:"failed" yaml:"failed"`
}
