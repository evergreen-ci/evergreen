package cli

import (
	"errors"

	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"golang.org/x/net/context"
)

// HostCreateCommand is the subcommand to spawn a host
type HostCreateCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	Distro     string   `short:"d" long:"distro" description:"distro of the host to spawn" required:"true"`
	PubKey     string   `short:"k" long:"key" description:"name or value of the public key to use" required:"true"`
}

// Execute will run the evergreen host create command
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
	grip.Infof("Spawn host created with ID %s. Visit the hosts page in Evergreen to check on its status.", host.Id)

	return nil
}

// HostListCommand is the subcommand to list hosts
type HostListCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	Mine       bool     `long:"mine" description:"list hosts spawned by the current user"`
	All        bool     `long:"all" description:"list all hosts"`
}

func (cmd *HostListCommand) Execute(_ []string) error {
	if cmd.All == cmd.Mine {
		return errors.New("Must specify exactly one of --all or --mine")
	}

	client, settings, err := getAPIV2Client(cmd.GlobalOpts)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if cmd.Mine {
		var hosts []*model.APIHost
		client.SetAPIUser(settings.User)
		client.SetAPIKey(settings.APIKey)

		hosts, err = client.GetHostsByUser(ctx, settings.User)
		if err != nil {
			return err
		}

		grip.Infof("%d hosts started by '%s':", len(hosts), settings.User)
		printHosts(hosts)

	} else if cmd.All {
		err = client.GetHosts(ctx, printHosts)
		if err != nil {
			return err
		}
	}

	return nil
}

func printHosts(hosts []*model.APIHost) error {
	for _, h := range hosts {
		grip.Infof("ID: %s; Distro: %s; Status: %s; Host name: %s; User: %s", h.Id, h.Distro.Id, h.Status, h.HostURL, h.User)
	}
	return nil
}

// HostStatusCommand is the subcommand to return the status of a host
type HostStatusCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	HostID     string   `short:"h" long:"host" description:"terminates the specified host" required:"true"`
}

// Execute will...
func (cmd *HostStatusCommand) Execute(_ []string) error {
	return errors.New("not implemented")
}

// HostTerminateCommand is the subcommand to terminate a host
type HostTerminateCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	HostID     string   `short:"h" long:"host" description:"terminates the specified host" required:"true"`
}

// Execute will...
func (cmd *HostTerminateCommand) Execute(_ []string) error {
	return errors.New("not implemented")
}
