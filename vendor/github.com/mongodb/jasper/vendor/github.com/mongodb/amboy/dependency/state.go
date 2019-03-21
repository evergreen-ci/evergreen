package dependency

// State provides a consistent set of values for
// DependencyManager implementations to use to report their state, and
// provide Queues and Jobs with a common set of terms to describe the
// state of a task's dependencies
type State int

//go:generate stringer -type=State

const (
	// Ready indicates that a task is safe to execute from the
	// perspective of the Dependency Manager.
	Ready State = iota

	// Passed indicates that there is no work to be done for this
	// dependency.
	Passed

	// Blocked tasks are waiting for their dependencies to be
	// resolved.
	Blocked

	// Unresolved states are for cyclic dependencies or cases
	// where tasks depend on resources that cannot be built.
	Unresolved
)

// IsValidState checks states and ensures that a state is valid.
func IsValidState(s State) bool {
	if s >= 0 && s <= 3 {
		return true
	}

	return false
}
