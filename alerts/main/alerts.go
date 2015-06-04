package main

import (
	"10gen.com/mci"
	"10gen.com/mci/alerts"
	"10gen.com/mci/db"
	"fmt"
	"os"
)

func main() {
	config := mci.MustConfig()
	if config.Monitor.LogFile != "" {
		mci.SetLogger(config.Monitor.LogFile)
	}

	home, err := mci.FindMCIHome()
	if err != nil {
		fmt.Println("Can't find home", err)
		os.Exit(1)
	}

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(config))
	qp := &alerts.QueueProcessor{Home: home}
	if err := qp.Run(config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
