package jasper

import "github.com/pkg/errors"

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

type mockProcessTracker struct {
	FailAdd     bool
	FailCleanup bool
	Infos       []ProcessInfo
}

func newMockProcessTracker() ProcessTracker {
	return &mockProcessTracker{
		Infos: []ProcessInfo{},
	}
}

func (t *mockProcessTracker) Add(info ProcessInfo) error {
	if t.FailAdd {
		return errors.New("failed in Add")
	}
	t.Infos = append(t.Infos, info)
	return nil
}

func (t *mockProcessTracker) Cleanup() error {
	if t.FailCleanup {
		return errors.New("failed in Cleanup")
	}
	t.Infos = []ProcessInfo{}
	return nil
}
