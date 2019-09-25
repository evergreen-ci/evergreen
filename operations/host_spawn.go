package operations

import (
	"context"
	"io/ioutil"
	"strings"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	MaxTagKeyLength   = 128
	MaxTagValueLength = 256
)

// makeAWSTags creates and validates a map of supplied instance tags
func makeAWSTags(tagSlice []string) ([]host.Tag, error) {
	catcher := grip.NewBasicCatcher()
	tagsMap := make(map[string]string)
	for _, tagString := range tagSlice {
		pair := strings.Split(tagString, "=")
		if len(pair) != 2 {
			catcher.Add(errors.Errorf("problem parsing tag '%s'", tagString))
			continue
		}

		key := pair[0]
		value := pair[1]

		// AWS tag key must contain no more than 128 characters
		if len(key) > MaxTagKeyLength {
			catcher.Add(errors.Errorf("key '%s' is longer than 128 characters", key))
		}
		// AWS tag value must contain no more than 256 characters
		if len(value) > MaxTagValueLength {
			catcher.Add(errors.Errorf("value '%s' is longer than 256 characters", value))
		}
		// tag prefix aws: is reserved
		if strings.HasPrefix(key, "aws:") || strings.HasPrefix(value, "aws:") {
			catcher.Add(errors.Errorf("illegal tag prefix 'aws:'"))
		}

		tagsMap[key] = value
	}

	// Make slice of host.Tag structs from map
	tags := []host.Tag{}
	for key, value := range tagsMap {
		tags = append(tags, host.Tag{Key: key, Value: value, CanBeModified: true})
	}

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	return tags, nil
}

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
			cli.StringSliceFlag{
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

			tags, err := makeAWSTags(tagSlice)
			if err != nil {
				return errors.Wrap(err, "problem generating tags")
			}

			spawnRequest := &model.HostRequestOptions{
				DistroID:     distro,
				KeyName:      key,
				UserData:     script,
				InstanceTags: tags,
			}

			host, err := client.CreateSpawnHost(ctx, spawnRequest)
			if err != nil {
				return errors.Wrap(err, "problem contacting evergreen service")
			}
			if host == nil {
				return errors.New("Unable to create a spawn host. Double check that the params and .evergreen.yml are correct")
			}

			grip.Infof("Spawn host created with ID '%s'. Visit the hosts page in Evergreen to check on its status.", model.FromAPIString(host.Id))
			return nil
		},
	}

}

func hostModify() cli.Command {
	const (
		addTagFlagName    = "tag"
		deleteTagFlagName = "delete-tag"
	)

	return cli.Command{
		Name:  "modify",
		Usage: "modify an existing host",
		Flags: addHostFlag(
			cli.StringSliceFlag{
				Name:  joinFlagNames(addTagFlagName, "t"),
				Usage: "key=value pair representing an instance tag, with one pair per flag",
			},
			cli.StringSliceFlag{
				Name:  joinFlagNames(deleteTagFlagName, "d"),
				Usage: "key of a single tag to be deleted",
			},
		),
		Before: mergeBeforeFuncs(setPlainLogger, requireHostFlag, requireAtLeastOneStringSlice(addTagFlagName, deleteTagFlagName)),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			hostID := c.String(hostFlagName)
			addTagSlice := c.StringSlice(addTagFlagName)
			deleteTagSlice := c.StringSlice(deleteTagFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.GetRestCommunicator(ctx)
			defer client.Close()

			addTags, err := makeAWSTags(addTagSlice)
			if err != nil {
				return errors.Wrap(err, "problem generating tags to add")
			}

			hostChanges := host.HostModifyOptions{
				AddInstanceTags:    addTags,
				DeleteInstanceTags: deleteTagSlice,
			}
			err = client.ModifySpawnHost(ctx, hostID, hostChanges)
			if err != nil {
				return err
			}

			grip.Infof("Successfully queued changes to spawn host with ID '%s'.", hostID)
			return nil
		},
	}
}

func hostStop() cli.Command {
	const waitFlagName = "wait"
	return cli.Command{
		Name:  "stop",
		Usage: "stop a running spawn host",
		Flags: addHostFlag(
			cli.BoolFlag{
				Name:  joinFlagNames(waitFlagName, "w"),
				Usage: "command will block until host stopped",
			},
		),
		Before: mergeBeforeFuncs(setPlainLogger, requireHostFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			hostID := c.String(hostFlagName)
			wait := c.Bool(waitFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.GetRestCommunicator(ctx)
			defer client.Close()

			if wait {
				grip.Infof("Stopping host '%s'. This may take a few minutes...", hostID)
			}

			err = client.StopSpawnHost(ctx, hostID, wait)
			if err != nil {
				return err
			}

			if wait {
				grip.Infof("Stopped host '%s'", hostID)
			} else {
				grip.Infof("Stopping host '%s'. Visit the hosts page in Evergreen to check on its status.", hostID)
			}
			return nil
		},
	}
}

func hostStart() cli.Command {
	const waitFlagName = "wait"
	return cli.Command{
		Name:  "start",
		Usage: "start a stopped spawn host",
		Flags: addHostFlag(
			cli.BoolFlag{
				Name:  joinFlagNames(waitFlagName, "w"),
				Usage: "command will block until host started",
			}),
		Before: mergeBeforeFuncs(setPlainLogger, requireHostFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			hostID := c.String(hostFlagName)
			wait := c.Bool(waitFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.GetRestCommunicator(ctx)
			defer client.Close()

			if wait {
				grip.Infof("Starting host '%s'. This may take a few minutes...", hostID)
			}

			err = client.StartSpawnHost(ctx, hostID, wait)
			if err != nil {
				return err
			}

			if wait {
				grip.Infof("Started host '%s'", hostID)
			} else {
				grip.Infof("Starting host '%s'. Visit the hosts page in Evergreen to check on its status.", hostID)
			}

			return nil
		},
	}
}

func hostList() cli.Command {
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
