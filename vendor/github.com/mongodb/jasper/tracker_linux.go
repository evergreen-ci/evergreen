package jasper

// TODO: implement

type linuxProcessTracker struct{}

func newProcessTracker(name string) (processTracker, error) {
	return &linuxProcessTracker{}, nil
}

func (_ *linuxProcessTracker) add(pid uint) error {
	return nil
}

func (_ *linuxProcessTracker) cleanup() error {
	return nil
}
