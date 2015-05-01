// Main package for the Evergreen runner.
package main

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	. "github.com/evergreen-ci/evergreen/runner"
)

func main() {
	config := evergreen.MustConfig()
	if config.Runner.LogFile != "" {
		evergreen.SetLogger(config.Runner.LogFile)
	}

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(config))

	// TODO: (MCI-2318) Put this on an internal scheduler using goroutines/waitgroups with timeouts.
	for _, runner := range Runners {
		if err := runner.Run(config); err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Error running %v: %v", runner.Name(), err)
		}
	}
	evergreen.Logger.Logf(slogger.INFO, "Done!")
}
