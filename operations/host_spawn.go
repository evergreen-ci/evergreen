package operations

import (
	"context"
	"io/ioutil"
	"strings"

	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func hostCreate() cli.Command {
	const (
		distroFlagName = "distro"
		keyFlagName    = "key"
		scriptFlagName = "script"
		tagFlagName    = "tag"
	)

	return cli.Command{
		Name:  "create",
		Usage: "spawn a host",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  joinFlagNames(distroFlagName, "d"),
				Usage: "name of an evergreen distro",
			},
			cli.StringFlag{
				Name:  joinFlagNames(keyFlagName, "k"),
				Usage: "name or value of an public key to use",
			},
			cli.StringFlag{
				Name:  joinFlagNames(scriptFlagName, "s"),
				Usage: "path to userdata script to run",
			},
			cli.StringFlag{
				Name:  joinFlagNames(tagFlagName, "t"),
				Usage: "key=value pair representing an instance tag, with one pair per flag",
			},
		},
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			distro := c.String(distroFlagName)
			key := c.String(keyFlagName)
			fn := c.String(scriptFlagName)
			tagSlice := c.StringSlice(tagFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.GetRestCommunicator(ctx)
			defer client.Close()

			var script string
			if fn != "" {
				var out []byte
				out, err = ioutil.ReadFile(fn)
				if err != nil {
					return errors.Wrapf(err, "problem reading userdata file '%s'", fn)
				}
				script = string(out)
			}

			// create map of tags
			var tags = make(map[string]string)
			for _, tagString := range tagSlice {
				pair := strings.Split(tagString, "=")
				if len(pair) != 2 {
					return errors.Errorf("problem parsing tag %s", tagString)
				}
				tags[pair[0]] = pair[1]
			}

			host, err := client.CreateSpawnHost(ctx, distro, key, script, tags)
			if host == nil {
				return errors.New("Unable to create a spawn host. Double check that the params and .evergreen.yml are correct")
			}
			if err != nil {
				return errors.Wrap(err, "problem contacting evergreen service")
			}

			grip.Infof("Spawn host created with ID '%s'. Visit the hosts page in Evergreen to check on its status.", model.FromAPIString(host.Id))
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
		Before: mergeBeforeFuncs(setPlainLogger, requireOnlyOneBool(mineFlagName, allFlagName)),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			showMine := c.Bool(mineFlagName)
			showAll := c.Bool(allFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
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
		grip.Infof("ID: %s; Distro: %s; Status: %s; Host name: %s; User: %s",
			model.FromAPIString(h.Id),
			model.FromAPIString(h.Distro.Id),
			model.FromAPIString(h.Status),
			model.FromAPIString(h.HostURL),
			model.FromAPIString(h.User))
	}
	return nil
}

func hostTerminate() cli.Command {
	return cli.Command{
		Name:   "terminate",
		Usage:  "terminate active spawn hosts",
		Flags:  addHostFlag(),
		Before: mergeBeforeFuncs(setPlainLogger, requireHostFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			hostID := c.String(hostFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
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
