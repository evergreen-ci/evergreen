// Main package for the Evergreen runner.
package main

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	. "10gen.com/mci/runner"
	"github.com/10gen-labs/slogger/v1"
)

func main() {
	config := mci.MustConfig()
	if config.Runner.LogFile != "" {
		mci.SetLogger(config.Runner.LogFile)
	}

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(config))

	// TODO: (MCI-2318) Put this on an internal scheduler using goroutines/waitgroups with timeouts.
	for _, runner := range Runners {
		if err := runner.Run(config); err != nil {
			mci.Logger.Logf(slogger.ERROR, "Error running %v: %v", runner.Name(), err)
		}
	}
	mci.Logger.Logf(slogger.INFO, "Done!")
}
