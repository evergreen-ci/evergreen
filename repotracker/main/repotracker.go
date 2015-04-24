package main

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/repotracker"
	"fmt"
	"os"
)

func main() {
	config := mci.MustConfig()
	if config.RepoTracker.LogFile != "" {
		mci.SetLogger(config.RepoTracker.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(config))
	r := &repotracker.Runner{}
	if err := r.Run(config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
