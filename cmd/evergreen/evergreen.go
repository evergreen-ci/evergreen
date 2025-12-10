package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen/util"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/operations"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/urfave/cli"
)

type ProgramDetails struct {
	Version string
}

var programDetails ProgramDetails

func main() {
	// this is where the main action of the program starts. The
	// command line interface is managed by the cli package and
	// its objects/structures. This, plus the basic configuration
	// in buildApp(), is all that's necessary for bootstrapping the
	// environment.
	app := buildApp()

	defer func() {
		if r := recover(); r != nil {
			_, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			// sendPanicTelemetry(ctx, PanicReport{
			// 	Message: fmt.Sprintf("%v", r),
			// 	Stack:   string(debug.Stack()),
			// 	// add fields like version, command, args, GOOS/GOARCH, git SHA, etc.
			// })

			// print a friendly message or log if you want
			fmt.Fprintln(os.Stderr, "unexpected error occurred; details have been reported")
			os.Exit(1)
		}
	}()

	grip.EmergencyFatal(app.Run(os.Args))
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

		programDetails := ProgramDetails{}

		return loggingSetup(app.Name, c.String("level"))
	}

	return app
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
