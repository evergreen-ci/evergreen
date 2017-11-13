package cli

import (
	"context"
	"os"

	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type AgentCommand struct {
	HostID           string `long:"host_id" description:"id of machine agent is running on"`
	HostSecret       string `long:"host_secret" description:"secret for the current host"`
	ServiceURL       string `long:"api_server" description:"URL of API server"`
	LogPrefix        string `long:"log_prefix" default:"evg-agent" description:"prefix for the agent's log filename"`
	StatusPort       int    `long:"status_part" default:"2285" description:"port to run the status server on"`
	WorkingDirectory string `long:"working_directory" default:"" description:"working directory"`
}

func (c *AgentCommand) Execute(_ []string) error {
	if c.ServiceURL == "" || c.HostID == "" || c.HostSecret == "" {
		return errors.New("cannot start agent without a service url and host ID")
	}

	opts := agent.Options{
		HostID:           c.HostID,
		HostSecret:       c.HostSecret,
		StatusPort:       c.StatusPort,
		LogPrefix:        c.LogPrefix,
		WorkingDirectory: c.WorkingDirectory,
	}

	if err := os.MkdirAll(c.WorkingDirectory, 0777); err != nil {
		return errors.Wrap(err, "problem creating working directory")
	}

	grip.Info(message.Fields{
		"message":  "starting agent",
		"commands": command.RegisteredCommandNames(),
		"dir":      c.WorkingDirectory,
		"host":     c.HostID,
	})

	agt := agent.New(opts, client.NewCommunicator(c.ServiceURL))

	sender, err := agent.GetSender(opts.LogPrefix, "init")
	if err != nil {
		return errors.Wrap(err, "problem configuring logger")
	}

	l := sender.Level()
	l.Threshold = level.Debug
	if err = sender.SetLevel(l); err != nil {
		return errors.Wrap(err, "problem configuring logger")
	}

	if err = grip.SetSender(sender); err != nil {
		return errors.Wrap(err, "problem setting up logger")
	}

	grip.SetName("evergreen.agent")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = agt.Start(ctx)
	grip.Emergency(err)

	return err
}
