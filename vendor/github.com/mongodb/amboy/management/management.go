package management

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

// Management is an interface that describes queue introspection tools and
// utility for queue management that make it possible to get more details about
// the running jobs in an amboy queue and gives users broader capabilities than
// the Queue interface itself.
type Management interface {
	JobStatus(context.Context, StatusFilter) (*JobStatusReport, error)
	RecentTiming(context.Context, time.Duration, RuntimeFilter) (*JobRuntimeReport, error)
	JobIDsByState(context.Context, string, StatusFilter) (*JobReportIDs, error)
	RecentErrors(context.Context, time.Duration, ErrorFilter) (*JobErrorsReport, error)
	RecentJobErrors(context.Context, string, time.Duration, ErrorFilter) (*JobErrorsReport, error)
	CompleteJobsByType(context.Context, StatusFilter, string) error
	CompleteJob(context.Context, string) error
	CompleteJobs(context.Context, StatusFilter) error
}

// StatusFilter defines a number of dimensions with which to filter
// current jobs in a queue by status
type StatusFilter string

// nolint
const (
	InProgress StatusFilter = "in-progress"
	Pending                 = "pending"
	Stale                   = "stale"
	Completed               = "completed"
	All                     = "all"
)

// Validate returns an error if a filter value is not valid.
func (t StatusFilter) Validate() error {
	switch t {
	case InProgress, Pending, Stale, Completed, All:
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
