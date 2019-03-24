package jasper

import "github.com/pkg/errors"

// Filter is type for classifying and grouping types of processes in filter
// operations, such as that found in List() on Managers.
type Filter string

const (
	// Running is a filter for processes that have not yet completed and are
	// still running.
	Running Filter = "running"
	// Terminated is the opposite of the Running filter.
	Terminated = "terminated"
	// All is a filter that is satisfied by any process.
	All = "all"
	// Failed refers to processes that have terminated unsuccessfully.
	Failed = "failed"
	// Successful refers to processes that have terminated successfully.
	Successful = "successful"
)

// Validate ensures that Filter is valid.
func (f Filter) Validate() error {
	switch f {
	case Running, Terminated, All, Failed, Successful:
		return nil
	default:
		return errors.Errorf("%s is not a valid filter", f)
	}
}
