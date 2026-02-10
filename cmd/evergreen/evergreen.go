package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"

	"github.com/evergreen-ci/evergreen"
	evergreenlocal "github.com/evergreen-ci/evergreen/cmd/evergreen-local"
	"github.com/evergreen-ci/evergreen/operations"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/urfave/cli"
)

const (
	notFound = "Not found"
)

var (
	panicReport *model.PanicReport

	args = os.Args
)

func main() {
	// this is where the main action of the program starts. The
	// command line interface is managed by the cli package and
	// its objects/structures. This, plus the basic configuration
	// in buildApp(), is all that's necessary for bootstrapping the
	// environment.

	// Detect which CLI to use based on the executable name.
	// This allows a single binary to serve both CLIs via symlinks.
	// If invoked as "evergreen-local" (or with that in the name),
	// use the debugger CLI. Otherwise, use the main evergreen CLI.
	execName := filepath.Base(os.Args[0])
	var app *cli.App

	if strings.Contains(execName, "evergreen-local") {
		// Running as evergreen-local - use debugger CLI
		app = evergreenlocal.BuildDebugger()
	} else {
		// Running as evergreen - use main CLI
		app = buildApp()
		defer recoverFromPanic()
	}

	grip.EmergencyFatal(app.Run(args))
}

func buildApp() *cli.App {
	app := cli.NewApp()
	app.Name = "evergreen"
	app.Usage = "MongoDB Continuous Integration Platform"
	app.Version = evergreen.ClientVersion

	// Register sub-commands here.
	app.Commands = []cli.Command{
		// Version and auto-update
		operations.Version(),
		operations.Update(),

		// Sub-Commands
		operations.Service(),
		operations.Agent(),
		operations.Admin(),
		operations.Host(),
		operations.Volume(),
		operations.Notification(),
		operations.Task(),

		// Top-level commands.
		operations.Keys(),
		operations.Fetch(),
		operations.Evaluate(),
		operations.Validate(),
		operations.List(),
		operations.LastGreen(),
		operations.LastRevision(),
		operations.Subscriptions(),
		operations.Client(),
		operations.Login(),

		// Patch creation and management commands (top-level)
		operations.Patch(),
		operations.PatchFile(),
		operations.PatchList(),
		operations.PatchSetModule(),
		operations.PatchRemoveModule(),
		operations.PatchFinalize(),
		operations.PatchCancel(),
	}

	userHome, _ := util.GetUserHome()
	confPath := filepath.Join(userHome, evergreen.DefaultEvergreenConfig)

	// These are global options. Use this to configure logging or
	// other options independent from specific sub commands.
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "level",
			Value: "info",
			Usage: "Specify lowest visible log level as string: 'emergency|alert|critical|error|warning|notice|info|debug|trace'",
		},
		cli.StringFlag{
			Name:  "conf, config, c",
			Usage: "specify the path for the evergreen CLI config",
			Value: confPath,
		},
	}

	app.Before = func(c *cli.Context) error {
		setupPanicReport(c)
		return loggingSetup(app.Name, c.String("level"))
	}

	return app
}

func recoverFromPanic() {
	if r := recover(); r != nil {
		panicReport.Panic = r
		panicReport.EndTime = time.Now()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := operations.SendPanicReport(ctx, panicReport); err != nil {
			fmt.Fprintf(os.Stderr, "error: could not send panic report to Evergreen service, please reach out to the Evergreen team: %v\n", err)
			os.Exit(1)
		}

		fmt.Fprintln(os.Stderr, "unexpected error: the Evergreen team has been sent a report:", panicReport.Panic)
		os.Exit(1)
	}
}

// setupPanicReport populates the global programDetails variable
// used for telemetry.
func setupPanicReport(c *cli.Context) {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = notFound
	}
	execPath, err := os.Executable()
	if err != nil {
		execPath = notFound
	}
	conf, err := operations.NewClientSettings(c.String(operations.ConfFlagName))
	if err != nil {
		conf = &operations.ClientSettings{
			User:       notFound,
			LoadedFrom: notFound,
		}
	}
	panicReport = &model.PanicReport{
		Version:                 evergreen.ClientVersion,
		AgentVersion:            evergreen.AgentVersion,
		BuildRevision:           evergreen.BuildRevision,
		CurrentWorkingDirectory: cwd,
		ExecutablePath:          execPath,
		Arguments:               args,
		StartTime:               time.Now(),
		OperatingSystem:         runtime.GOOS,
		Architecture:            runtime.GOARCH,
		ConfigFilePath:          c.String(operations.ConfFlagName),
		ConfigAbsFilePath:       conf.LoadedFrom,
		User:                    conf.User,
		Project:                 conf.FindDefaultProject(cwd, true),
	}
}

func loggingSetup(name, l string) error {
	if err := grip.SetSender(send.MakeErrorLogger()); err != nil {
		return err
	}
	grip.SetName(name)

	sender := grip.GetSender()
	info := sender.Level()
	info.Threshold = level.FromString(l)

	return sender.SetLevel(info)
}
