package reliability

import (
        "github.com/mongodb/grip"
)

// ValidateForTaskReliability validates that the StartAt struct is valid for use with test stats.
func (f *TaskReliabilityFilter) ValidateForTaskReliability() error {
        catcher := grip.NewBasicCatcher()
        return catcher.Resolve()
}

// TaskReliabilityFilter represents search and aggregation parameters when querying the test or task statistics.
type TaskReliabilityFilter struct {
        Project string
}
