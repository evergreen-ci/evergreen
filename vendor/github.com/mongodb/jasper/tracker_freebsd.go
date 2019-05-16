package jasper

// TODO: implement

type freebsdProcessTracker struct {
	*processTrackerBase
}

func NewProcessTracker(name string) (ProcessTracker, error) {
	return &freebsdProcessTracker{processTrackerBase: &processTrackerBase{Name: name}}, nil
}
