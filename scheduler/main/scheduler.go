package main

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/scheduler"
	"fmt"
	"os"
)

func main() {
	config := mci.MustConfig()
	if config.Scheduler.LogFile != "" {
		mci.SetLogger(config.Scheduler.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(config))
	r := &scheduler.Runner{}
	if err := r.Run(config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
