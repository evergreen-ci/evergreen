package cli

import (
	"errors"

	"github.com/mongodb/grip"
	"golang.org/x/net/context"
)

// HostCommand is the parent command for the host subcommands
type HostCommand struct {
}

// GetSubCommand returns the correct subcommand struct for a given input
func (hc *HostCommand) GetSubCommand(subcommand string, opts *Options) interface{} {
	switch subcommand {
	case "create":
		return &HostCreateCommand{GlobalOpts: opts}
	case "list":
		return &HostListCommand{GlobalOpts: opts}
	case "status":
		return &HostStatusCommand{GlobalOpts: opts}
	case "terminate":
		return &HostTerminateCommand{GlobalOpts: opts}
	}
	return nil
}

// HostCreateCommand is the subcommand to spawn a host
type HostCreateCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	Distro     string   `short:"d" long:"distro" description:"distro of the host to spawn" required:"true"`
	PubKey     string   `short:"k" long:"key" description:"name or value of the public key to use" required:"true"`
}

func (cmd *HostCreateCommand) Execute(_ []string) error {

	client, settings, err := getAPIV2Client(cmd.GlobalOpts)
	if err != nil {
		return err
	}

	client.SetAPIUser(settings.User)
	client.SetAPIKey(settings.APIKey)
	ctx := context.Background()

	host, err := client.CreateSpawnHost(ctx, cmd.Distro, cmd.PubKey)
	if host == nil {
		return errors.New("Unable to create a spawn host. Double check that the params and .evergreen.yml are correct")
	}
	if err != nil {
		return err
	}
	grip.Infof("Spawn host created with ID %s. Visit the hosts page in Evergreen to check on its status.", host.HostID)

	return nil
}

// HostListCommand is the subcommand to list hosts
type HostListCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	Mine       bool     `long:"mine" description:"list hosts spawned by the current user"`
	All        bool     `long:"all" description:"list all hosts"`
}

func (cmd *HostListCommand) Execute(_ []string) error {
	return errors.New("not implemented")
}

// HostStatusCommand is the subcommand to return the status of a host
type HostStatusCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	HostID     string   `short:"h" long:"host" description:"terminates the specified host" required:"true"`
}

func (cmd *HostStatusCommand) Execute(_ []string) error {
	return errors.New("not implemented")
}

// HostTerminateCommand is the subcommand to terminate a host
type HostTerminateCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	HostID     string   `short:"h" long:"host" description:"terminates the specified host" required:"true"`
}

func (cmd *HostTerminateCommand) Execute(_ []string) error {
	return errors.New("not implemented")
}
