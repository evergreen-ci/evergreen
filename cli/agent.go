package cli

import (
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type AgentCommand struct {
	HostID     string `long:"host_id" description:"id of machine agent is running on"`
	HostSecret string `long:"host_secret" description:"secret for the current host"`
	ServiceURL string `long:"api_server" description:"URL of API server"`
	LogPrefix  string `long:"log_prefix" default:"evg-agent" description:"prefix for the agent's log filename"`
	StatusPort int    `long:"status_part" default:"2285" description:"port to run the status server on"`
	NewAgent   bool   `long:"new_agent" description:"run new agent (defaults to legacy agent)"`
}

func (c *AgentCommand) Execute(_ []string) error {
	if c.ServiceURL == "" || c.HostID == "" || c.HostSecret == "" {
		return errors.New("cannot start agent without a service url and host ID")
	}

	opts := agent.Options{
		HostID:     c.HostID,
		HostSecret: c.HostSecret,
		StatusPort: c.StatusPort,
		LogPrefix:  c.LogPrefix,
	}

	agt, err := agent.New(opts, client.NewCommunicator(c.ServiceURL))
	if err != nil {
		return errors.Wrap(err, "problem configuring logging")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return errors.WithStack(agt.Start(ctx))
}
