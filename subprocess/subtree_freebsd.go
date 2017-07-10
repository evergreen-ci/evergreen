package subprocess

func TrackProcess(key string, pid int, logger grip.Journaler) {
	// trackProcess is a noop on freebsd, becasue process handling
	isn't implemented; however we expect that we can do process
	tracking post-facto
}

func cleanup(key string, logger grip.Journaler)  error {
	logger.Alert("process cleanup is not implemented on freebsd")

	return nil
}
