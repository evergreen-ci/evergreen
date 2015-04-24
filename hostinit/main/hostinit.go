package main

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/hostinit"
	"fmt"
	"os"
)

func main() {
	config := mci.MustConfig()
	if config.HostInit.LogFile != "" {
		mci.SetLogger(config.HostInit.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(config))
	r := &hostinit.Runner{}
	if err := r.Run(config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
