package subprocess

import "github.com/mongodb/grip"

func TrackProcess(key string, pid int, logger grip.Journaler) {
	// trackProcess is a noop on freebsd, because process handling
	// isn't implemented; however we expect that we can do process
	// tracking post-facto
}

func cleanup(_ string, logger grip.Journaler) error {
	logger.Alert("process cleanup is not implemented on freebsd")

	return nil
}
