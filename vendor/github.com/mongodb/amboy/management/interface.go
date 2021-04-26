package management

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

// Manager is an interface that describes queue introspection tools and
// utility for queue management that make it possible to get more details about
// the running jobs in an amboy queue and gives users broader capabilities than
// the Queue interface itself.
type Manager interface {
	// JobStatus returns a report of job statistics filtered by status.
	JobStatus(context.Context, StatusFilter) (*JobStatusReport, error)
	// RecentTiming returns a report of job runtime statistics filtered by
	// runtime status.
	RecentTiming(context.Context, time.Duration, RuntimeFilter) (*JobRuntimeReport, error)
	// JobIDsByState returns a report of job IDs filtered by a job type and
	// status filter. The returned job IDs can be either logical job IDs or
	// internally-stored job IDs.
	JobIDsByState(context.Context, string, StatusFilter) (*JobReportIDs, error)
	// RecentErrors returns a report of job errors within the given duration
	// window matching the error filter.
	RecentErrors(context.Context, time.Duration, ErrorFilter) (*JobErrorsReport, error)
	// RecentJobErrors is the same as RecentErrors but additionally filters by
	// job type.
	RecentJobErrors(context.Context, string, time.Duration, ErrorFilter) (*JobErrorsReport, error)

	// For all CompleteJob* methods, implementations should mark jobs as
	// completed. Furthermore, for implementations managing retryable queues,
	// they should complete the job's retrying phase (i.e. the job will not
	// retry).

	// CompleteJob marks a job complete by ID. Implementations may differ on
	// whether it matches the logical job ID (i.e. (amboy.Job).ID) or
	// internally-stored job IDs (which may differ from the user-visible job
	// ID).
	CompleteJob(context.Context, string) error
	// CompleteJobs marks all jobs complete that match the given status filter.
	CompleteJobs(context.Context, StatusFilter) error
	// CompleteJobsByType marks all jobs complete that match the given status
	// filter and job type.
	CompleteJobsByType(context.Context, StatusFilter, string) error
	// CompleteJobsByPattern marks all jobs complete that match the given status
	// filter and whose job ID matches the given regular expression.
	// Implementations may differ on whether the pattern matches the logical job
	// ID (i.e. (amboy.Job).ID) or internally-stored job IDs (which may differ
	// from the user-visible job ID). Furthermore, implementations may differ on
	// the accepted pattern-matching language.
	CompleteJobsByPattern(context.Context, StatusFilter, string) error
}

// StatusFilter defines a number of dimensions with which to filter
// current jobs in a queue by status
type StatusFilter string

// Constants representing valid StatusFilters.
const (
	InProgress StatusFilter = "in-progress"
	Pending    StatusFilter = "pending"
	Stale      StatusFilter = "stale"
	Completed  StatusFilter = "completed"
	Retrying   StatusFilter = "retrying"
	All        StatusFilter = "all"
)

// Validate returns an error if a filter value is not valid.
func (t StatusFilter) Validate() error {
	switch t {
	case InProgress, Pending, Stale, Completed, Retrying, All:
		return nil
	default:
		return errors.Errorf("%s is not a valid counter filter type", t)
	}
}

// ValidStatusFilters returns all valid status filters.
func ValidStatusFilters() []StatusFilter {
	return []StatusFilter{Pending, InProgress, Stale, Completed, Retrying, All}
}

// RuntimeFilter provides ways to filter the timing data returned by
// the reporting interface.
type RuntimeFilter string

// Constants representing valid RuntimeFilters.
const (
	Duration RuntimeFilter = "completed"
	Latency  RuntimeFilter = "latency"
	Running  RuntimeFilter = "running"
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

// ValidRuntimeFilters returns all valid runtime filters.
func ValidRuntimeFilters() []RuntimeFilter {
	return []RuntimeFilter{Duration, Latency, Running}
}

// ErrorFilter defines post-processing on errors as returned to users
// in error queries.
type ErrorFilter string

// Constants representing valid ErrorFilters.
const (
	UniqueErrors ErrorFilter = "unique-errors"
	AllErrors    ErrorFilter = "all-errors"
	StatsOnly    ErrorFilter = "stats-only"
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

// ValidErrorFilters returns all valid error filters.
func ValidErrorFilters() []ErrorFilter {
	return []ErrorFilter{UniqueErrors, AllErrors, StatsOnly}
}
