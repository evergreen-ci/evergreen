package cli

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"

	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// HostCreateCommand is the subcommand to spawn a host
type HostCreateCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	Distro     string   `short:"d" long:"distro" description:"distro of the host to spawn" required:"true"`
	PubKey     string   `short:"k" long:"key" description:"name or value of the public key to use" required:"true"`
}

// Execute will run the evergreen host create command
func (cmd *HostCreateCommand) Execute(_ []string) error {
	ctx := context.Background()

	client, settings, err := getAPIV2Client(ctx, cmd.GlobalOpts)
	if err != nil {
		return err
	}

	client.SetAPIUser(settings.User)
	client.SetAPIKey(settings.APIKey)

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

	ctx := context.Background()
	client, settings, err := getAPIV2Client(ctx, cmd.GlobalOpts)
	if err != nil {
		return err
	}

	if cmd.Mine {
		var hosts []*model.APIHost
		client.SetAPIUser(settings.User)
		client.SetAPIKey(settings.APIKey)

		hosts, err = client.GetHostsByUser(ctx, settings.User)
		if err != nil {
			return err
		}

		grip.Infof("%d hosts started by '%s':", len(hosts), settings.User)
		err = printHosts(hosts)
		if err != nil {
			return errors.Wrap(err, "problem printing hosts")
		}

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
	HostID     string   `short:"h" long:"host" description:"gets the status of the specified host" required:"true"`
}

// Execute will...
func (cmd *HostStatusCommand) Execute(_ []string) error {
	return errors.New("not implemented")
}

// HostTerminateCommand is the subcommand to terminate a host
type HostTerminateCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	HostID     string   `long:"host" short:"h" description:"terminates the specified host" required:"true"`
}

// Execute terminates a given host
func (cmd *HostTerminateCommand) Execute(_ []string) error {
	if cmd.HostID == "" {
		return errors.New("host ID cannot be blank")
	}

	ctx := context.Background()
	client, _, _, err := getAPIClients(ctx, cmd.GlobalOpts)
	if err != nil {
		return err
	}

	data := struct {
		HostID string `json:"host_id"`
		Action string `json:"action"`
	}{cmd.HostID, "terminate"}

	rPipe, wPipe := io.Pipe()
	encoder := json.NewEncoder(wPipe)
	go func() {
		grip.Warning(encoder.Encode(data))
		grip.Warning(wPipe.Close())
	}()
	defer rPipe.Close()

	resp, err := client.doReq("POST", "spawn", -1, rPipe)
	if err != nil {
		return err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	grip.Info(body)

	return nil
}
