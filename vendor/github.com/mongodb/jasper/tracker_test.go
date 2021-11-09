package jasper

import "errors"

type mockProcessTracker struct {
	FailAdd     bool
	FailCleanup bool
	Infos       []ProcessInfo
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
