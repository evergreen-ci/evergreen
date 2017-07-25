package cli

import (
	"github.com/evergreen-ci/evergreen/agent"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
)

type AgentCommand struct {
	HostID     string `long:"host_id" description:"id of machine agent is running on"`
	HostSecret string `long:"host_secret" description:"secret for the current host"`
	ServiceURL string `long:"api_server" description:"URL of API server"`
	LogPrefix  string `long:"log_prefix" default:"evg-agent" description:"prefix for the agent's log filename"`
	StatusPort int    `long:"status_part" default:"2285" description:"port to run the status server on"`
}

func (c *AgentCommand) Execute(_ []string) error {
	if c.ServiceURL == "" || c.HostID == "" || c.HostSecret == "" {
		return errors.New("cannot start agent without a service url and host ID")
	}

	if err := agent.SetupLogging("agent-startup", "init"); err != nil {
		return errors.Wrap(err, "problem configuring logging")
	}

	grip.SetDefaultLevel(level.Info)
	grip.SetThreshold(level.Debug)
	grip.SetName("evg-agent")

	// all we need is the host id and host secret
	initialOptions := agent.Options{
		APIURL:     c.ServiceURL,
		HostId:     c.HostID,
		HostSecret: c.HostSecret,
		StatusPort: c.StatusPort,
		LogPrefix:  c.LogPrefix,
	}

	agt, err := agent.New(initialOptions)
	if err != nil {
		return errors.Wrap(err, "could not create new agent")
	}

	return errors.WithStack(agt.Run())
}
