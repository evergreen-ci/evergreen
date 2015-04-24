package main

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/notify"
	"fmt"
	"os"
)

func main() {
	config := mci.MustConfig()
	if config.Notify.LogFile != "" {
		mci.SetLogger(config.Notify.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(config))
	r := &notify.Runner{}
	if err := r.Run(config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
