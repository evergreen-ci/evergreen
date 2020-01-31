package operations

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

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

			tags, err := host.MakeHostTags(tagSlice)
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

			grip.Infof("Spawn host created with ID '%s'. Visit the hosts page in Evergreen to check on its status.", model.FromStringPtr(host.Id))
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
		Flags: mergeFlagSlices(addHostFlag(), addSubscriptionTypeFlag(
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
		)),
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
			subscriptionType := c.String(subscriptionTypeFlag)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			addTags, err := host.MakeHostTags(addTagSlice)
			if err != nil {
				return errors.Wrap(err, "problem generating tags to add")
			}

			hostChanges := host.HostModifyOptions{
				AddInstanceTags:    addTags,
				DeleteInstanceTags: deleteTagSlice,
				InstanceType:       instanceType,
				AddHours:           time.Duration(extension) * time.Hour,
				SubscriptionType:   subscriptionType,
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
		Flags: mergeFlagSlices(addHostFlag(), addSubscriptionTypeFlag(
			cli.BoolFlag{
				Name:  joinFlagNames(waitFlagName, "w"),
				Usage: "command will block until host stopped",
			},
		)),
		Before: mergeBeforeFuncs(setPlainLogger, requireHostFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			hostID := c.String(hostFlagName)
			subscriptionType := c.String(subscriptionTypeFlag)
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

			err = client.StopSpawnHost(ctx, hostID, subscriptionType, wait)
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
		Flags: mergeFlagSlices(addHostFlag(), addSubscriptionTypeFlag(
			cli.BoolFlag{
				Name:  joinFlagNames(waitFlagName, "w"),
				Usage: "command will block until host started",
			})),
		Before: mergeBeforeFuncs(setPlainLogger, requireHostFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			hostID := c.String(hostFlagName)
			subscriptionType := c.String(subscriptionTypeFlag)
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

			err = client.StartSpawnHost(ctx, hostID, subscriptionType, wait)
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

func hostListVolume() cli.Command {
	return cli.Command{
		Name:   "list",
		Usage:  "list volumes for user",
		Before: setPlainLogger,
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.getRestCommunicator(ctx)
			defer client.Close()

			volumes, err := client.GetVolumesByUser(ctx)
			if err != nil {
				return err
			}
			printVolumes(volumes, conf.User)
			return nil
		},
	}
}

func printVolumes(volumes []model.APIVolume, userID string) {
	if len(volumes) == 0 {
		grip.Infof("no volumes started by user '%s'", userID)
		return
	}
	totalSize := 0
	for _, v := range volumes {
		totalSize += v.Size
	}
	grip.Infof("%d volumes started by %s (total size %d):", len(volumes), userID, totalSize)
	for _, v := range volumes {
		grip.Infof("\n%-18s: %s\n", "ID", model.FromStringPtr(v.ID))
		grip.Infof("%-18s: %d\n", "Size", v.Size)
		grip.Infof("%-18s: %s\n", "Type", model.FromStringPtr(v.Type))
		grip.Infof("%-18s: %s\n", "Availability Zone", model.FromStringPtr(v.AvailabilityZone))
		if model.FromStringPtr(v.HostID) != "" {
			grip.Infof("%-18s: %s\n", "Device Name", model.FromStringPtr(v.DeviceName))
			grip.Infof("%-18s: %s\n", "Attached to Host", model.FromStringPtr(v.HostID))

		}
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

			grip.Infof("Created volume '%s'.", model.FromStringPtr(volume.ID))

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
				Usage: "list hosts spawned by the current user",
			},
		},
		Before: setPlainLogger,
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			showMine := c.Bool(mineFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			params := model.APIHostParams{
				UserSpawned: true,
				Mine:        showMine,
			}
			hosts, err := client.GetHosts(ctx, params)
			if err != nil {
				return errors.Wrap(err, "problem getting hosts")
			}
			printHosts(hosts)

			return nil
		},
	}
}

func printHosts(hosts []*model.APIHost) {
	for _, h := range hosts {
		grip.Infof("ID: %s; Distro: %s; Status: %s; Host name: %s; User: %s, Availability Zone: %s",
			model.FromStringPtr(h.Id),
			model.FromStringPtr(h.Distro.Id),
			model.FromStringPtr(h.Status),
			model.FromStringPtr(h.HostURL),
			model.FromStringPtr(h.User),
			model.FromStringPtr(h.AvailabilityZone))
	}
}

func hostTerminate() cli.Command {
	return cli.Command{
		Name:   "terminate",
		Usage:  "terminate active spawn hosts",
		Flags:  addHostFlag(),
		Before: mergeBeforeFuncs(setPlainLogger, requireHostFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
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

func hostRunCommand() cli.Command {
	const (
		scriptFlagName        = "script"
		pathFlagName          = "path"
		createdBeforeFlagName = "created-before"
		createdAfterFlagName  = "created-after"
		distroFlagName        = "distro"
		userHostFlagName      = "user-host"
		mineFlagName          = "mine"
		batchSizeFlagName     = "batch-size"
	)

	return cli.Command{
		Name:  "exec",
		Usage: "run a bash shell script on host(s) and print the output",
		Flags: mergeFlagSlices(addHostFlag(), addYesFlag(
			cli.StringFlag{
				Name:  createdBeforeFlagName,
				Usage: "only run on hosts created before `TIME` in RFC3339 format",
			},
			cli.StringFlag{
				Name:  createdAfterFlagName,
				Usage: "only run on hosts created after `TIME` in RFC3339 format",
			},
			cli.StringFlag{
				Name:  distroFlagName,
				Usage: "only run on hosts of `DISTRO`",
			},
			cli.BoolFlag{
				Name:  userHostFlagName,
				Usage: "only run on user hosts",
			},
			cli.BoolFlag{
				Name:  mineFlagName,
				Usage: "only run on my hosts",
			},
			cli.StringFlag{
				Name:  scriptFlagName,
				Usage: "script to pass to bash",
			},
			cli.StringFlag{
				Name:  pathFlagName,
				Usage: "path to a file containing a script",
			},
			cli.IntFlag{
				Name:  batchSizeFlagName,
				Usage: "limit requests to batches of `BATCH_SIZE`",
				Value: 10,
			},
		)),
		Before: mergeBeforeFuncs(setPlainLogger, mutuallyExclusiveArgs(true, scriptFlagName, pathFlagName)),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			hostID := c.String(hostFlagName)
			createdBefore := c.String(createdBeforeFlagName)
			createdAfter := c.String(createdAfterFlagName)
			distro := c.String(distroFlagName)
			userSpawned := c.Bool(userHostFlagName)
			mine := c.Bool(mineFlagName)
			script := c.String(scriptFlagName)
			path := c.String(pathFlagName)
			skipConfirm := c.Bool(yesFlagName)
			batchSize := c.Int(batchSizeFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			var hostIDs []string
			if hostID != "" {
				hostIDs = []string{hostID}
			} else {
				var createdBeforeTime, createdAfterTime time.Time
				if createdBefore != "" {
					createdBeforeTime, err = time.Parse(time.RFC3339, createdBefore)
					if err != nil {
						return errors.Wrap(err, "can't parse created before time")
					}
				}
				if createdAfter != "" {
					createdAfterTime, err = time.Parse(time.RFC3339, createdAfter)
					if err != nil {
						return errors.Wrap(err, "can't parse create after time")
					}
				}

				hosts, err := client.GetHosts(ctx, model.APIHostParams{
					CreatedBefore: createdBeforeTime,
					CreatedAfter:  createdAfterTime,
					Distro:        distro,
					UserSpawned:   userSpawned,
					Mine:          mine,
					Status:        evergreen.HostRunning,
				})
				if err != nil {
					return errors.Wrapf(err, "can't get matching hosts")
				}
				if len(hosts) == 0 {
					grip.Info("no matching hosts")
					return nil
				}
				for _, host := range hosts {
					hostIDs = append(hostIDs, model.FromStringPtr(host.Id))
				}

				if !skipConfirm {
					if !confirm(fmt.Sprintf("The script will run on %d host(s), \n%s\nContinue? (y/n): ", len(hostIDs), strings.Join(hostIDs, "\n")), true) {
						return nil
					}
				}
			}

			if path != "" {
				scriptBytes, err := ioutil.ReadFile(path)
				if err != nil {
					return errors.Wrapf(err, "can't read script from '%s'", path)
				}
				script = string(scriptBytes)
				if script == "" {
					return errors.New("script is empty")
				}
			}

			hostsOutput, err := client.StartHostProcesses(ctx, hostIDs, script, batchSize)
			if err != nil {
				return errors.Wrap(err, "problem running command")
			}

			// poll for process output
			for len(hostsOutput) > 0 {
				time.Sleep(time.Second * 5)

				runningProcesses := 0
				for _, hostOutput := range hostsOutput {
					if hostOutput.Complete {
						grip.Infof("'%s' output: ", hostOutput.HostID)
						grip.Info(hostOutput.Output)
					} else {
						hostsOutput[runningProcesses] = hostOutput
						runningProcesses++
					}
				}
				hostsOutput, err = client.GetHostProcessOutput(ctx, hostsOutput[:runningProcesses], batchSize)
				if err != nil {
					return errors.Wrap(err, "can't get process output")
				}
			}

			return nil
		},
	}
}
