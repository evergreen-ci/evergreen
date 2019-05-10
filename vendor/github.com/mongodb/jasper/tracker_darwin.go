package jasper

// TODO

type darwinProcessTracker struct{}

func newProcessTracker(name string) (processTracker, error) {
	return &darwinProcessTracker{}, nil
}

func (_ *darwinProcessTracker) add(_ uint) error {
	return nil
}

func (_ *darwinProcessTracker) cleanup() error {
	return nil
}
