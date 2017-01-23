package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/plugin"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/tychoish/grip"
	"github.com/tychoish/grip/slogger"
	"gopkg.in/tylerb/graceful.v1"
)

var (
	// requestTimeout is the duration to wait until killing
	// active requests and stopping the server.
	requestTimeout = 10 * time.Second
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s handles communication with running tasks and command line tools.\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Usage:\n  %s [flags]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Supported flags are:\n")
		flag.PrintDefaults()
	}
}

func main() {
	go util.DumpStackOnSIGQUIT(os.Stdout)
	settings := evergreen.GetSettingsOrExit()
	if settings.Api.LogFile != "" {
		evergreen.SetLogger(settings.Api.LogFile)
	}
	grip.SetName("api-server")

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(settings))

	tlsConfig, err := util.MakeTlsConfig(settings.Api.HttpsCert, settings.Api.HttpsKey)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Failed to make TLS config: %v", err)
		os.Exit(1)
	}

	nonSSL, err := service.GetListener(settings.Api.HttpListenAddr)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Failed to get HTTP listener: %v", err)
		os.Exit(1)
	}

	ssl, err := service.GetTLSListener(settings.Api.HttpsListenAddr, tlsConfig)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Failed to get HTTPS listener: %v", err)
		os.Exit(1)
	}

	// Start SSL and non-SSL servers in independent goroutines, but exit
	// the process if either one fails
	as, err := service.NewAPIServer(settings, plugin.APIPlugins)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Failed to create API server: %v", err)
		os.Exit(1)
	}

	handler, err := as.Handler()
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Failed to get API route handlers: %v", err)
		os.Exit(1)
	}

	server := &http.Server{Handler: handler}

	errChan := make(chan error, 2)

	go func() {
		evergreen.Logger.Logf(slogger.INFO, "Starting non-SSL API server")
		err := graceful.Serve(server, nonSSL, requestTimeout)
		if err != nil {
			if opErr, ok := err.(*net.OpError); !ok || (ok && opErr.Op != "accept") {
				evergreen.Logger.Logf(slogger.WARN, "non-SSL API server error: %v", err)
			} else {
				err = nil
			}
		}
		evergreen.Logger.Logf(slogger.INFO, "non-SSL API server cleanly terminated")
		errChan <- err
	}()

	go func() {
		evergreen.Logger.Logf(slogger.INFO, "Starting SSL API server")
		err := graceful.Serve(server, ssl, requestTimeout)
		if err != nil {
			if opErr, ok := err.(*net.OpError); !ok || (ok && opErr.Op != "accept") {
				evergreen.Logger.Logf(slogger.WARN, "SSL API server error: %v", err)
			} else {
				err = nil
			}
		}
		evergreen.Logger.Logf(slogger.INFO, "SSL API server cleanly terminated")
		errChan <- err
	}()

	exitCode := 0

	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Error returned from API server: %v", err)
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}
