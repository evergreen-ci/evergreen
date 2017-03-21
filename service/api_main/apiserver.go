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
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
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
	settings := evergreen.GetSettingsOrExit()

	// setup the logging
	if settings.Api.LogFile != "" {
		sender, err := send.MakeFileLogger(settings.Api.LogFile)
		grip.CatchEmergencyFatal(err)
		defer sender.Close()
		grip.CatchEmergencyFatal(grip.SetSender(sender))
	} else {
		sender := send.MakeNative()
		defer sender.Close()
		grip.CatchEmergencyFatal(grip.SetSender(sender))
	}
	evergreen.SetLegacyLogger()
	grip.SetName("evg-api-server")
	grip.SetDefaultLevel(level.Info)
	grip.SetThreshold(level.Debug)
	grip.Notice(message.Fields{"build": evergreen.BuildRevision, "process": grip.Name()})

	defer util.RecoverAndLogStackTrace()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(settings))

	tlsConfig, err := util.MakeTlsConfig(settings.Api.HttpsCert, settings.Api.HttpsKey)
	if err != nil {
		grip.EmergencyFatalf("Failed to make TLS config: %+v", err)
	}

	nonSSL, err := service.GetListener(settings.Api.HttpListenAddr)
	if err != nil {
		grip.EmergencyFatalf("Failed to get HTTP listener: %+v", err)
	}

	ssl, err := service.GetTLSListener(settings.Api.HttpsListenAddr, tlsConfig)
	if err != nil {
		grip.EmergencyFatalf("Failed to get HTTPS listener: %+v", err)
	}

	// Start SSL and non-SSL servers in independent goroutines, but exit
	// the process if either one fails
	as, err := service.NewAPIServer(settings, plugin.APIPlugins)
	if err != nil {
		grip.EmergencyFatalf("Failed to create API server: %+v", err)
	}

	handler, err := as.Handler()
	if err != nil {
		grip.EmergencyFatalf("Failed to get API route handlers: %+v", err)
	}

	server := &http.Server{Handler: handler}

	errChan := make(chan error, 2)

	go func() {
		grip.Info("Starting non-SSL API server")
		err := graceful.Serve(server, nonSSL, requestTimeout)
		if err != nil {
			if opErr, ok := err.(*net.OpError); !ok || (ok && opErr.Op != "accept") {
				grip.Warningf("non-SSL API server error: %+v", err)
			} else {
				err = nil
			}
		}
		grip.Info("non-SSL API server cleanly terminated")
		errChan <- err
	}()

	go func() {
		grip.Info("Starting SSL API server")
		err := graceful.Serve(server, ssl, requestTimeout)
		if err != nil {
			if opErr, ok := err.(*net.OpError); !ok || (ok && opErr.Op != "accept") {
				grip.Warningf("SSL API server error: %+v", err)
			} else {
				err = nil
			}
		}
		grip.Info("SSL API server cleanly terminated")
		errChan <- err
	}()

	exitCode := 0

	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			grip.Errorf("Error returned from API server: %+v", err)
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}
