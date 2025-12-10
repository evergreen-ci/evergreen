package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	Version                 string
	CurrentWorkingDirectory string
	ExecutablePath          string
	Arguments               []string
	StartTime               time.Time
	OperatingSystem         string
	Architecture            string
	ConfigFilePath          string

	Panic any
}

var (
	programDetails *ProgramDetails

	args = os.Args
)

func main() {
	// this is where the main action of the program starts. The
	// command line interface is managed by the cli package and
	// its objects/structures. This, plus the basic configuration
	// in buildApp(), is all that's necessary for bootstrapping the
	// environment.
	app := buildApp()

	defer recoverFromPanic()

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
		fmt.Println("Before setting details?")
		setupProgramDetails(c)
		return loggingSetup(app.Name, c.String("level"))
	}

	return app
}

func recoverFromPanic() {
	if r := recover(); r != nil {
		programDetails.Panic = r

		// Use a context with timeout to avoid hanging forever.
		_, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if programDetails == nil || programDetails.ConfigFilePath == "" {
			fmt.Fprintln(os.Stderr, "unexpected error occured, could not reach out to Evergreen service:", programDetails.Panic)
			os.Exit(1)
		}

		fmt.Println("Details", programDetails)

		fmt.Fprintln(os.Stderr, "unexpected error occured, the Evergreen team has been sent a report:", programDetails.Panic)
		os.Exit(1)
	}
}

// setupProgramDetails populates the global programDetails variable
// used for telemetry.
func setupProgramDetails(c *cli.Context) {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "Not found"
	}
	execPath, err := os.Executable()
	if err != nil {
		execPath = "Not found"
	}
	programDetails = &ProgramDetails{
		Version:                 evergreen.ClientVersion,
		CurrentWorkingDirectory: cwd,
		ExecutablePath:          execPath,
		Arguments:               args,
		StartTime:               time.Now(),
		OperatingSystem:         runtime.GOOS,
		Architecture:            runtime.GOARCH,
		ConfigFilePath:          c.String(operations.ConfFlagName),
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
