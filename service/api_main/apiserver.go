package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"golang.org/x/net/context"
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
	sender, err := settings.GetSender(settings.Api.LogFile)
	grip.CatchEmergencyFatal(err)
	defer sender.Close()
	grip.CatchEmergencyFatal(grip.SetSender(sender))
	evergreen.SetLegacyLogger()
	grip.SetName("evg-api-server")
	grip.SetDefaultLevel(level.Info)
	grip.SetThreshold(level.Debug)

	grip.Notice(message.Fields{"build": evergreen.BuildRevision, "process": grip.Name()})

	defer util.RecoverAndLogStackTrace()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go evergreen.SystemInfoCollector(ctx)

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(settings))

	as, err := service.NewAPIServer(settings)
	if err != nil {
		grip.EmergencyFatalf("Failed to create API server: %+v", err)
	}

	handler, err := as.Handler()
	if err != nil {
		grip.EmergencyFatalf("Failed to get API route handlers: %+v", err)
	}

	grip.CatchEmergencyFatal(service.RunGracefully(settings.Api.HttpListenAddr, requestTimeout, handler))
}
