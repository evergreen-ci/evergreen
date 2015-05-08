package main

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apiserver"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"os"
)

func main() {
	var err error
	settings := evergreen.MustConfig()
	if settings.Api.LogFile != "" {
		evergreen.SetLogger(settings.Api.LogFile)
	}

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(settings))

	apis, err := apiserver.New(settings, plugin.Published)
	if err != nil {
		fmt.Println("Failed to create API server:", err)
		os.Exit(1)
	}

	tlsConfig, err := util.MakeTlsConfig(settings.Expansions["api_httpscert"], settings.Api.HttpsKey)
	if err != nil {
		fmt.Println("Failed to make TLS config: ", err)
		os.Exit(1)
	}

	nonssl, err := apiserver.GetListener(settings.Api.HttpListenAddr)
	if err != nil {
		fmt.Println("Failed to listen for HTTP: ", err)
		os.Exit(1)
	}
	ssl, err := apiserver.GetTLSListener(settings.Api.HttpsListenAddr, tlsConfig)
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
		evergreen.Logger.Logf(slogger.INFO, "Starting nonssl API server")
		errChan <- apiserver.Serve(nonssl, handler)
	}()

	go func() {
		evergreen.Logger.Logf(slogger.INFO, "Starting ssl API server")
		errChan <- apiserver.Serve(ssl, handler)
	}()

	err = <-errChan
	if err != nil {
		fmt.Println("Error returned from API server: ", err)
		os.Exit(1)
	}
}
