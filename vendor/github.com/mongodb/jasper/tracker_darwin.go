package jasper

// TODO

type darwinProcessTracker struct {
	*processTrackerBase
}

// NewProcessTracker is unimplemented.
func NewProcessTracker(name string) (ProcessTracker, error) {
	return &darwinProcessTracker{processTrackerBase: &processTrackerBase{Name: name}}, nil
}
