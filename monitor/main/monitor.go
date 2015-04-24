package main

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/monitor"
	"fmt"
	"os"
)

func main() {
	config := mci.MustConfig()
	if config.Monitor.LogFile != "" {
		mci.SetLogger(config.Monitor.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(config))
	r := &monitor.Runner{}
	if err := r.Run(config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
