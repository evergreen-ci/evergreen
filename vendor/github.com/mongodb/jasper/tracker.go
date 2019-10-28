package jasper

// ProcessTracker provides a way to logically group processes that
// should be managed collectively. Implementation details are
// platform-specific since each one has its own means of managing
// groups of processes.
type ProcessTracker interface {
	// Add begins tracking a process identified by its ProcessInfo.
	Add(ProcessInfo) error
	// Cleanup terminates this group of processes.
	Cleanup() error
}

// processTrackerBase provides convenience no-op implementations of the
// ProcessTracker interface.
type processTrackerBase struct {
	Name string
}

func (*processTrackerBase) Add(ProcessInfo) error {
	return nil
}

func (*processTrackerBase) Cleanup() error { return nil }
