package jasper

// TODO: implement

type freebsdProcessTracker struct {
	*processTrackerBase
}

// NewProcessTracker is unimplemented.
func NewProcessTracker(name string) (ProcessTracker, error) {
	return &freebsdProcessTracker{processTrackerBase: &processTrackerBase{Name: name}}, nil
}
