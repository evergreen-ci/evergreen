package main

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/taskrunner"
	"fmt"
	"os"
)

func main() {
	config := mci.MustConfig()
	if config.TaskRunner.LogFile != "" {
		mci.SetLogger(config.TaskRunner.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(config))
	r := &taskrunner.Runner{}
	if err := r.Run(config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
