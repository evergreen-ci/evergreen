package main

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/repotracker"
	"os"
)

func main() {
	config := evergreen.MustConfig()
	if config.RepoTracker.LogFile != "" {
		evergreen.SetLogger(config.RepoTracker.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(config))
	r := &repotracker.Runner{}
	if err := r.Run(config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
