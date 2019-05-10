package jasper

// processTracker provides a way to logically group processes that
// should be managed collectively. Implementation details are
// platform-specific since each one has its own means of managing
// groups of processes.
type processTracker interface {
	// add begins tracking a process identified by its PID.
	add(pid uint) error
	// cleanup terminates this group of processes.
	cleanup() error
}
