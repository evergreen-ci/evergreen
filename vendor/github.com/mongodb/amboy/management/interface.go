package management

import (
	"context"

	"github.com/pkg/errors"
)

// Manager is an interface that describes queue introspection tools and
// utility for queue management that make it possible to get more details about
// the running jobs in an amboy queue and gives users broader capabilities than
// the Queue interface itself.
type Manager interface {
	// JobStatus returns statistics of the number of jobs of each job type
	// matching the status.
	JobStatus(context.Context, StatusFilter) ([]JobTypeCount, error)
	// JobIDsByState returns a report of job IDs filtered by a job type and
	// status filter. Depending on the implementation, the returned job IDs can
	// be either logical job IDs or internally-stored job IDs.
	JobIDsByState(context.Context, string, StatusFilter) ([]GroupedID, error)

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
	Pending       StatusFilter = "pending"
	InProgress    StatusFilter = "in-progress"
	Stale         StatusFilter = "stale"
	Completed     StatusFilter = "completed"
	Retrying      StatusFilter = "retrying"
	StaleRetrying StatusFilter = "stale-retrying"
	All           StatusFilter = "all"
)

// Validate returns an error if a filter value is not valid.
func (t StatusFilter) Validate() error {
	switch t {
	case InProgress, Pending, Stale, Completed, Retrying, All:
		return nil
	default:
		return errors.Errorf("%s is not a valid status filter type", t)
	}
}

// ValidStatusFilters returns all valid status filters.
func ValidStatusFilters() []StatusFilter {
	return []StatusFilter{Pending, InProgress, Stale, Completed, Retrying, All}
}
