package operations

import (
	"context"
	"io/ioutil"
	"strings"
	"time"

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
		distroFlagName       = "distro"
		keyFlagName          = "key"
		scriptFlagName       = "script"
		tagFlagName          = "tag"
		instanceTypeFlagName = "type"
		noExpireFlagName     = "no-expire"
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
				Name:  joinFlagNames(instanceTypeFlagName, "i"),
				Usage: "name of an instance type",
			},
			cli.StringSliceFlag{
				Name:  joinFlagNames(tagFlagName, "t"),
				Usage: "key=value pair representing an instance tag, with one pair per flag",
			},
			cli.BoolFlag{
				Name:  noExpireFlagName,
				Usage: "make host never expire",
			},
		},
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			distro := c.String(distroFlagName)
			key := c.String(keyFlagName)
			fn := c.String(scriptFlagName)
			tagSlice := c.StringSlice(tagFlagName)
			instanceType := c.String(instanceTypeFlagName)
			noExpire := c.Bool(noExpireFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.setupRestCommunicator(ctx)
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
				InstanceType: instanceType,
				NoExpiration: noExpire,
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
		addTagFlagName       = "tag"
		deleteTagFlagName    = "delete-tag"
		instanceTypeFlagName = "type"
		noExpireFlagName     = "no-expire"
		expireFlagName       = "expire"
		extendFlagName       = "extend"
	)

	return cli.Command{
		Name:  "modify",
		Usage: "modify an existing host",
		Flags: addHostFlag(
			cli.StringSliceFlag{
				Name:  joinFlagNames(addTagFlagName, "t"),
				Usage: "add instance tag `KEY=VALUE`, one tag per flag",
			},
			cli.StringSliceFlag{
				Name:  joinFlagNames(deleteTagFlagName, "d"),
				Usage: "delete instance tag `KEY`, one tag per flag",
			},
			cli.StringFlag{
				Name:  joinFlagNames(instanceTypeFlagName, "i"),
				Usage: "change instance type to `TYPE`",
			},
			cli.IntFlag{
				Name:  extendFlagName,
				Usage: "extend the expiration of a spawn host by `HOURS`",
			},
			cli.BoolFlag{
				Name:  noExpireFlagName,
				Usage: "make host never expire",
			},
			cli.BoolFlag{
				Name:  expireFlagName,
				Usage: "make host expire like a normal spawn host, in 24 hours",
			},
		),
		Before: mergeBeforeFuncs(setPlainLogger, requireHostFlag, requireAtLeastOneFlag(
			addTagFlagName, deleteTagFlagName, instanceTypeFlagName, expireFlagName, noExpireFlagName, extendFlagName)),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			hostID := c.String(hostFlagName)
			addTagSlice := c.StringSlice(addTagFlagName)
			deleteTagSlice := c.StringSlice(deleteTagFlagName)
			instanceType := c.String(instanceTypeFlagName)
			noExpire := c.Bool(noExpireFlagName)
			expire := c.Bool(expireFlagName)
			extension := c.Int(extendFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			addTags, err := makeAWSTags(addTagSlice)
			if err != nil {
				return errors.Wrap(err, "problem generating tags to add")
			}

			hostChanges := host.HostModifyOptions{
				AddInstanceTags:    addTags,
				DeleteInstanceTags: deleteTagSlice,
				InstanceType:       instanceType,
				AddHours:           time.Duration(extension) * time.Hour,
			}

			if noExpire {
				noExpirationValue := true
				hostChanges.NoExpiration = &noExpirationValue
			} else if expire {
				noExpirationValue := false
				hostChanges.NoExpiration = &noExpirationValue
			} else {
				hostChanges.NoExpiration = nil
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
			client := conf.setupRestCommunicator(ctx)
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
			client := conf.setupRestCommunicator(ctx)
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

func hostAttach() cli.Command {
	const (
		volumeFlagName = "volume"
		deviceFlagName = "device"
	)

	return cli.Command{
		Name:  "attach",
		Usage: "attach a volume to a spawn host",
		Flags: addHostFlag(
			cli.StringFlag{
				Name:  joinFlagNames(volumeFlagName, "v"),
				Usage: "`ID` of volume to attach",
			},
			cli.StringFlag{
				Name:  joinFlagNames(deviceFlagName, "n"),
				Usage: "device `NAME` for attached volume",
			},
		),
		Before: mergeBeforeFuncs(setPlainLogger, requireStringFlag(volumeFlagName)),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			hostID := c.String(hostFlagName)
			volumeID := c.String(volumeFlagName)
			deviceName := c.String(deviceFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.getRestCommunicator(ctx)
			defer client.Close()

			volume := &host.VolumeAttachment{
				VolumeID:   volumeID,
				DeviceName: deviceName,
			}

			err = client.AttachVolume(ctx, hostID, volume)
			if err != nil {
				return err
			}

			grip.Infof("Attached volume '%s'.", volumeID)

			return nil
		},
	}
}

func hostDetach() cli.Command {
	const (
		volumeFlagName = "volume"
	)

	return cli.Command{
		Name:  "detach",
		Usage: "detach a volume from a spawn host",
		Flags: addHostFlag(
			cli.StringFlag{
				Name:  joinFlagNames(volumeFlagName, "v"),
				Usage: "`ID` of volume to detach",
			},
		),
		Before: mergeBeforeFuncs(setPlainLogger, requireStringFlag(volumeFlagName)),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			hostID := c.String(hostFlagName)
			volumeID := c.String(volumeFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.getRestCommunicator(ctx)
			defer client.Close()

			err = client.DetachVolume(ctx, hostID, volumeID)
			if err != nil {
				return err
			}

			grip.Infof("Detached volume '%s'.", volumeID)

			return nil
		},
	}
}

func hostCreateVolume() cli.Command {
	const (
		sizeFlag = "size"
		typeFlag = "type"
		zoneFlag = "zone"
	)

	return cli.Command{
		Name:  "create",
		Usage: "create a volume for spawn hosts",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  joinFlagNames(sizeFlag, "s"),
				Usage: "set volume `SIZE` in GiB",
			},
			cli.StringFlag{
				Name:  joinFlagNames(typeFlag, "t"),
				Usage: "set volume `TYPE` (default gp2)",
			},
			cli.StringFlag{
				Name:  joinFlagNames(zoneFlag, "z"),
				Usage: "set volume `AVAILABILITY ZONE` (default us-east-1a)",
			},
		},
		Before: mergeBeforeFuncs(setPlainLogger, requireStringFlag(sizeFlag)),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			volumeType := c.String(typeFlag)
			volumeZone := c.String(zoneFlag)
			volumeSize := c.Int(sizeFlag)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.getRestCommunicator(ctx)
			defer client.Close()

			volumeRequest := &host.Volume{
				Type:             volumeType,
				Size:             volumeSize,
				AvailabilityZone: volumeZone,
			}

			volume, err := client.CreateVolume(ctx, volumeRequest)
			if err != nil {
				return err
			}

			grip.Infof("Created volume '%s'.", model.FromAPIString(volume.ID))

			return nil
		},
	}
}

func hostDeleteVolume() cli.Command {
	const (
		idFlagName = "id"
	)

	return cli.Command{
		Name:  "delete",
		Usage: "delete a volume for spawn hosts",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  idFlagName,
				Usage: "`ID` of volume to delete",
			},
		},
		Before: mergeBeforeFuncs(setPlainLogger, requireStringFlag(idFlagName)),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			volumeID := c.String(idFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.getRestCommunicator(ctx)
			defer client.Close()

			if err = client.DeleteVolume(ctx, volumeID); err != nil {
				return err
			}

			grip.Infof("Deleted volume '%s'", volumeID)

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
			client := conf.setupRestCommunicator(ctx)
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
		grip.Infof("ID: %s; Distro: %s; Status: %s; Host name: %s; User: %s, Availability Zone: %s",
			model.FromAPIString(h.Id),
			model.FromAPIString(h.Distro.Id),
			model.FromAPIString(h.Status),
			model.FromAPIString(h.HostURL),
			model.FromAPIString(h.User),
			model.FromAPIString(h.AvailabilityZone))
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
			client := conf.setupRestCommunicator(ctx)
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
