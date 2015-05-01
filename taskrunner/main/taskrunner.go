package main

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/taskrunner"
	"os"
)

func main() {
	config := evergreen.MustConfig()
	if config.TaskRunner.LogFile != "" {
		evergreen.SetLogger(config.TaskRunner.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(config))
	r := &taskrunner.Runner{}
	if err := r.Run(config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
