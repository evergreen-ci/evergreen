package main

import (
	"10gen.com/mci"
	"10gen.com/mci/apiserver"
	"10gen.com/mci/db"
	"10gen.com/mci/plugin"
	"10gen.com/mci/util"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"os"
)

func main() {
	var err error
	mciSettings := mci.MustConfig()
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(mciSettings))

	apis, err := apiserver.New(mciSettings, plugin.Published)
	if err != nil {
		fmt.Println("Failed to create API server:", err)
		os.Exit(1)
	}

	tlsConfig, err := util.MakeTlsConfig(mciSettings.Expansions["api_httpscert"], mciSettings.Api.HttpsKey)
	if err != nil {
		fmt.Println("Failed to make TLS config: ", err)
		os.Exit(1)
	}

	nonssl, err := apiserver.GetListener(mciSettings.Api.HttpListenAddr)
	if err != nil {
		fmt.Println("Failed to listen for HTTP: ", err)
		os.Exit(1)
	}
	ssl, err := apiserver.GetTLSListener(mciSettings.Api.HttpsListenAddr, tlsConfig)
	if err != nil {
		fmt.Println("Failed to listen for HTTPS: ", err)
		os.Exit(1)
	}
	// Start SSL and non-SSL servers in independent goroutines, but exit the process if either one fails
	errChan := make(chan error)

	handler, err := apis.Handler()
	if err != nil {
		fmt.Println("Failed to listen for HTTPS: ", err)
		os.Exit(1)
	}

	go func() {
		mci.Logger.Logf(slogger.INFO, "Starting nonssl API server")
		errChan <- apiserver.Serve(nonssl, handler)
	}()

	go func() {
		mci.Logger.Logf(slogger.INFO, "Starting ssl API server")
		errChan <- apiserver.Serve(ssl, handler)
	}()

	err = <-errChan
	if err != nil {
		fmt.Println("Error returned from API server: ", err)
		os.Exit(1)
	}
}
