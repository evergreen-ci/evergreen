package command

import (
	"os"
)

var (
	// IMPORTANT: this key must be attached to the user below for
	// passwordless ssh to localhost on the machine where the unit tests are
	// run
	TestRemoteKey = os.ExpandEnv("${HOME}/.ssh/mcitest")
)

const (
	// IMPORTANT: this user must be configured for passwordless ssh to
	// localhost on the machine where the unit tests are run, using the key
	// above
	TestRemoteUser = "mcitestuser"
	TestRemote     = "localhost"
)
