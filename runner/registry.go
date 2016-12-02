// Package runner provides a basic interface to run various processes within Evergreen.
// Its primary job is to expose the basic Run method of such process in an abstraction
// that allows them to be executed outside of their main functions.
package runner

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/hostinit"
	"github.com/evergreen-ci/evergreen/monitor"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/evergreen-ci/evergreen/scheduler"
	"github.com/evergreen-ci/evergreen/taskrunner"
)

// ProcessRunner wraps a basic Run method that allows various processes in Evergreen
// to be executed outside of their main methods.
type ProcessRunner interface {
	// Name returns the id of the process runner.
	Name() string
	// Description returns a description of the runner for use in -help text.
	Description() string
	// Run executes the process runner with the supplied configuration.
	Run(*evergreen.Settings) error
}

// Runners is a slice of all Evergreen processes that implement the ProcessRunner interface.
var (
	Runners = []ProcessRunner{
		&hostinit.Runner{},
		&monitor.Runner{},
		&notify.Runner{},
		&repotracker.Runner{},
		&taskrunner.Runner{},
		&alerts.QueueProcessor{},
		&scheduler.Runner{},
	}
)
