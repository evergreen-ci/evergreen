package operations

import (
	"context"
	"errors"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/urfave/cli"
)

func hostStatus() cli.Command {
	return cli.Command{
		Name:  "status",
		Usage: "print the status of spawn hosts",
		Action: func(c *cli.Context) error {
			return errors.New("not implemented")
		},
	}
}

func hostCreate() cli.Command {
	const (
		distroFlagName = "distro"
		keyFlagName    = "key"
	)

	return cli.Command{
		Name:  "create",
		Usage: "spawn a host",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:    distroFlagName,
				Aliases: []string{"d"},
				Usage:   "name of an evergreen distro",
			},
			cli.StringFlag{
				Name:    keyFlagName,
				Aliases: []string{"k"},
				Usage:   "name or value of an public key to use",
			},
		},
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			distro := c.String(distroFlagName)
			key := c.string(keyFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.GetRestCommunicator(ctx)
			defer client.Close()

			host, err := client.CreateSpawnHost(ctx, distro, key)
			if host == nil {
				return errors.New("Unable to create a spawn host. Double check that the params and .evergreen.yml are correct")
			}
			if err != nil {
				return errors.Wrap(err, "problem contacting evergreen service")
			}

			grip.Infof("Spawn host created with ID '%s'. Visit the hosts page in Evergreen to check on its status.", host.Id)
			return nil
		},
	}

}
func hostlist() cli.Command {
	const (
		mineFlagName = "mine"
		allFlagName  = "all"
	)

	return cli.Command{
		Name:  "list",
		Usage: "list active spawn hosts",
		Flags: []cli.Flag{
			cli.BoolFlag{
				Name:  mineFlagName,
				Usage: "list hosts spawned but the current user",
			},
			cli.BoolFlag{
				Name:  allFlagName,
				Usage: "list all hosts",
			},
		},
		Befor: func(c *cli.Context) error {
			if c.Bool(mineFlagName) == c.Bool(allFlagName) {
				return errors.New("Must specify exactly one of --all or --mine")
			}

			grip.CatchWarning(grip.SetSender(send.MakePlainLogger()))
		},
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			showMine := c.Bool(mineFlagName)
			showAll := c.Bool(allFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.GetRestCommunicator(ctx)
			defer client.Close()

			switch {
			case showMine:
				var hosts []*model.APIHost

				hosts, err = client.GetHostsByUser(ctx, conf.User)
				if err != nil {
					return err
				}

				grip.Infof("%d hosts started by '%s':", len(hosts), conf.User)

				if err = printHosts(hosts); err != nil {
					return errors.Wrap(err, "problem printing hosts")
				}
			case showAll:
				if err = client.GetHosts(ctx, printHosts); err != nil {
					return errors.Wrap(err, "problem printing hosts")
				}
			}

			return nil
		},
	}
}

func printHosts(hosts []*model.APIHost) error {
	for _, h := range hosts {
		grip.Infof("ID: %s; Distro: %s; Status: %s; Host name: %s; User: %s", h.Id, h.Distro.Id, h.Status, h.HostURL, h.User)
	}
	return nil
}

func hostTerminate() cli.Command {
	const hostFlagName = "host"

	return cli.Command{
		Name:  "terminate",
		Usage: "terminate active spawn hosts",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:    hostFlagName,
				Aliases: []string{"h"},
				Usage:   "terminate the specified host",
			},
		},
		Before: func(c *cli.Context) error {
			grip.CatchWarning(grip.SetSender(send.MakePlainLogger()))

			if c.String(hostFlagName) == "" {
				return errors.New("must specify host to terminate")
			}

			return nil
		},
		Action: func(c *cli.Context) error {
			hostID := c.String(hostFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.GetRestCommunicator(ctx)
			defer client.Close()

			err = client.TerminateSpawnHost(ctx, hostID)
			if err != nil {
				return errors.Wrap(err, "problem terminating host")
			}

			grip.Infof("Terminated host '%s'", hostID)

			return nil
		},
	}
}
