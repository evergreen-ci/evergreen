// Package runner provides a basic interface to run various processes within Evergreen.
// Its primary job is to expose the basic Run method of such process in an abstraction
// that allows them to be executed outside of their main functions.
package runner

import (
	"10gen.com/mci"
	"10gen.com/mci/hostinit"
	"10gen.com/mci/monitor"
	"10gen.com/mci/notify"
	"10gen.com/mci/repotracker"
	"10gen.com/mci/scheduler"
	"10gen.com/mci/taskrunner"
)

// ProcessRunner wraps a basic Run method that allows various processes in Evergreen
// to be executed outside of their main methods.
type ProcessRunner interface {
	// Name returns the id of the process runner.
	Name() string
	// Run executes the process runner with the supplied configuration.
	Run(*mci.MCISettings) error
}

// Runners is a slice of all Evergreen processes that implement the ProcessRunner interface.
var (
	Runners = []ProcessRunner{
		&hostinit.Runner{},
		&monitor.Runner{},
		&notify.Runner{},
		&repotracker.Runner{},
		&scheduler.Runner{},
		&taskrunner.Runner{},
	}
)
