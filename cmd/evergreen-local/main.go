package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/operations"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "evergreen-local"
	app.Usage = "Run Evergreen tasks locally from YAML configuration files"
	app.Version = fmt.Sprintf("%s (%s)", evergreen.AgentVersion, evergreen.BuildRevision)

	setupLogging()

	app.Commands = append(
		operations.DaemonCommands(),
	)

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "log-level",
			Usage: "Set the logging level (debug, info, warning, error)",
			Value: "info",
		},
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		grip.Infof("Received signal %s, shutting down", sig)
		os.Exit(0)
	}()

	if err := app.Run(os.Args); err != nil {
		grip.Emergency(err)
		os.Exit(1)
	}
}

func setupLogging() {
	sender := grip.GetSender()

	_ = sender.SetLevel(send.LevelInfo{
		Default:   level.Info,
		Threshold: level.Debug,
	})

	_ = grip.SetSender(sender)
}
