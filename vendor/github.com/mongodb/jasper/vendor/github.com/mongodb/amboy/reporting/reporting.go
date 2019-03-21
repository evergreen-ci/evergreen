package reporting

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

// Reporter is an interface that describes queue introspection tools
// that make it possible to get more details about the running jobs in
// an amboy queue.
type Reporter interface {
	JobStatus(context.Context, CounterFilter) (*JobStatusReport, error)
	RecentTiming(context.Context, time.Duration, RuntimeFilter) (*JobRuntimeReport, error)
	JobIDsByState(context.Context, string, CounterFilter) (*JobReportIDs, error)
	RecentErrors(context.Context, time.Duration, ErrorFilter) (*JobErrorsReport, error)
	RecentJobErrors(context.Context, string, time.Duration, ErrorFilter) (*JobErrorsReport, error)
}

// CounterFilter defines a number of dimensions with which to filter
// current jobs in a queue.
type CounterFilter string

// nolint
const (
	InProgress CounterFilter = "in-progress"
	Pending                  = "pending"
	Stale                    = "stale"
)

// Validate returns an error if a filter value is not valid.
func (t CounterFilter) Validate() error {
	switch t {
	case InProgress, Pending, Stale:
		return nil
	default:
		return errors.Errorf("%s is not a valid counter filter type", t)
	}
}

// RuntimeFilter provides ways to filter the timing data returned by
// the reporting interface.
type RuntimeFilter string

// nolint
const (
	Duration RuntimeFilter = "completed"
	Latency                = "latency"
	Running                = "running"
)

// Validate returns an error if a filter value is not valid.
func (t RuntimeFilter) Validate() error {
	switch t {
	case Duration, Latency, Running:
		return nil
	default:
		return errors.Errorf("%s is not a valid runtime filter type", t)
	}
}

// ErrorFilter defines post-processing on errors as returned to users
// in error queries.
type ErrorFilter string

// nolint
const (
	UniqueErrors ErrorFilter = "unique-errors"
	AllErrors                = "all-errors"
	StatsOnly                = "stats-only"
)

// Validate returns an error if a filter value is not valid.
func (t ErrorFilter) Validate() error {
	switch t {
	case UniqueErrors, AllErrors, StatsOnly:
		return nil
	default:
		return errors.Errorf("%s is not a valid error filter", t)
	}
}
