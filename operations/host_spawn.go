package operations

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/model"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
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
			cli.StringFlag{
				Name:  joinFlagNames(regionFlagName, "r"),
				Usage: fmt.Sprintf("AWS region to spawn host in (defaults to user-defined region, or %s)", evergreen.DefaultEC2Region),
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
			region := c.String(regionFlagName)
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
				Region:       region,
				NoExpiration: noExpire,
			}

			host, err := client.CreateSpawnHost(ctx, spawnRequest)
			if err != nil {
				return errors.Wrap(err, "problem contacting evergreen service")
			}
			if host == nil {
				return errors.New("Unable to create a spawn host. Double check that the params and .evergreen.yml are correct")
			}

			grip.Infof("Spawn host created with ID '%s'. Visit the hosts page in Evergreen to check on its status, or check `evergreen host list --mine", model.FromStringPtr(host.Id))
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
		} else {
			t, err := model.FromTimePtr(v.Expiration)
			if err == nil && !util.IsZeroTime(t) {
				grip.Infof("%-18s: %s\n", "Expiration", t.Format(time.RFC3339))
			}
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
	)

	return cli.Command{
		Name:  "list",
		Usage: "list active spawn hosts",
		Flags: []cli.Flag{
			cli.BoolFlag{
				Name:  mineFlagName,
				Usage: "list hosts spawned by the current user",
			},
			cli.StringFlag{
				Name:  regionFlagName,
				Usage: "list hosts in specified region",
			},
		},
		Before: setPlainLogger,
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			showMine := c.Bool(mineFlagName)
			region := c.String(regionFlagName)

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
				Region:      region,
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

				var hosts []*restmodel.APIHost
				hosts, err = client.GetHosts(ctx, model.APIHostParams{
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
				var scriptBytes []byte
				scriptBytes, err = ioutil.ReadFile(path)
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

func hostRsync() cli.Command {
	const (
		localPathFlagName             = "local"
		remotePathFlagName            = "remote"
		remoteIsLocalFlagName         = "remote-is-local"
		makeParentDirectoriesFlagName = "make-parent-dirs"
		excludeFlagName               = "exclude"
		deleteFlagName                = "delete"
		pullFlagName                  = "pull"
		timeoutFlagName               = "timeout"
		sanityChecksFlagName          = "sanity-checks"
		dryRunFlagName                = "dry-run"
	)
	return cli.Command{
		Name:  "rsync",
		Usage: "synchronize files between local and remote hosts",
		Description: `
Rsync is a utility for mirroring files and directories between two
hosts.

Paths can be specified as absolute or relative paths. For the local path, a
relative path is relative to the current working directory. For the remote path,
a relative path is relative to the home directory.

Examples:
* Create or overwrite a file between the local filesystem and remote spawn host:

	evergreen host rsync -l /path/to/local/file1 -r /path/to/remote/file2 -h <host_id>

	If file2 does not exist on the remote host, it will be created (including
	any parent directories) and its contents will exactly match those of file1
	on the local filesystem.  Otherwrise, file2 will be overwritten.

* A more practical example to upload your .bashrc to the host:

	evergreen host rsync -l ~/.bashrc -r .bashrc -h <host_id>

* Push (mirror) the directory contents from the local filesystem to the directory on the remote spawn host:

	evergreen host rsync -l /path/to/local/dir1/ -r /path/to/remote/dir2/ -h <host_id>

	NOTE: the trailing slash in the file paths are required here.
	This will replace all the contents of the remote directory dir2 to match the
	contents of the local directory dir1.

* Pull (mirror) a spawn host's remote directory onto the local filesystem:

	evergreen host rsync -l /path/to/local/dir1/ -r /path/to/remote/dir2/ --pull -h <host_id>

	NOTE: the trailing slash in the file paths are required here.
	This will replace all the contents of the local directory dir1 to match the
	contents of the remote directory dir2.

* Push a local directory to become a subdirectory at the remote path on the remote spawn host:

	evergreen host rsync -l /path/to/local/dir1 -r /path/to/remote/dir2 -h <host_id>

	NOTE: this should not have a trailing slash at the end of the file paths.
	This will create a subdirectory dir1 containing the local dir1's contents below
	dir2 (i.e. it will create /path/to/remote/dir2/dir1).

* Mirror two directories on the same host:

	evergreen host rsync -l /path/to/first/local/dir1/ -r /path/to/second/local/dir2/ --remote-is-local

	This will make the contents of dir2 match the contents of dir1, both of
	which are on your local machine.

* Exclude files/directories from being synced:

	evergreen host rsync -l /path/to/local/dir1/ -r /path/to/remote/dir2/ -h <host_id> -x excluded_dir/ -x excluded_file

	NOTE: paths to excluded files/directory are relative to the source directory.
	This will mirror all the contents of the local dir1 in the remote dir2
	except for dir1/excluded_dir and dir1/excluded_file.

* Disable sanity checking prompt when mirroring directories:

	evergreen host rsync -l /path/to/local/dir1/ -r /path/to/remote/dir2/ -h <host_id> --sanity-checks=false

* Dry run the command to see what will be changed without actually changing anything:

	evergreen host rsync -l /path/to/local/dir1/ -r /path/to/remote/dir2/ -h <host_id> --dry-run
`,
		Flags: addHostFlag(
			cli.StringFlag{
				Name:  joinFlagNames(localPathFlagName, "l"),
				Usage: "the local directory/file (required)",
			},
			cli.StringFlag{
				Name:  joinFlagNames(remotePathFlagName, "r"),
				Usage: "the remote directory/file (required)",
			},
			cli.BoolFlag{
				Name:  remoteIsLocalFlagName,
				Usage: "if set, both the source and destination filepaths are on the local machine (host does not need to be specified)",
			},
			cli.StringSliceFlag{
				Name:  joinFlagNames(excludeFlagName, "x"),
				Usage: "ignore syncing any files matched by the given pattern",
			},
			cli.BoolTFlag{
				Name:  joinFlagNames(deleteFlagName, "d"),
				Usage: "delete any files in the destination directory that are not present on the source directory (default: true)",
			},
			cli.BoolTFlag{
				Name:  joinFlagNames(makeParentDirectoriesFlagName, "p"),
				Usage: "create parent directories for the destination if they do not already exist (default: true)",
			},
			cli.DurationFlag{
				Name:  joinFlagNames(timeoutFlagName, "t"),
				Usage: "timeout for rsync command, e.g. '5m' = 5 minutes, '30s' = 30 seconds (if unset, there is no timeout)",
			},
			cli.BoolFlag{
				Name:  joinFlagNames(pullFlagName),
				Usage: "pull files from the remote host to the local host (default is to push files from the local host to the remote host)",
			},
			cli.BoolTFlag{
				Name:  sanityChecksFlagName,
				Usage: "perform basic sanity checks for common sources of error (ignored on dry runs) (default: true)",
			},
			cli.BoolFlag{
				Name:  joinFlagNames(dryRunFlagName, "n"),
				Usage: "show what would occur if executed, but do not actually execute",
			},
		),
		Before: mergeBeforeFuncs(
			mutuallyExclusiveArgs(true, hostFlagName, remoteIsLocalFlagName),
			requireStringFlag(localPathFlagName),
			requireStringFlag(remotePathFlagName),
		),
		Action: func(c *cli.Context) error {
			doSanityCheck := c.BoolT(sanityChecksFlagName)
			localPath := c.String(localPathFlagName)
			remotePath := c.String(remotePathFlagName)
			pull := c.Bool(pullFlagName)
			dryRun := c.Bool(dryRunFlagName)
			remoteIsLocal := c.Bool(remoteIsLocalFlagName)

			if strings.HasSuffix(localPath, "/") && !strings.HasSuffix(remotePath, "/") {
				remotePath = remotePath + "/"
			}
			if strings.HasSuffix(remotePath, "/") && !strings.HasSuffix(localPath, "/") {
				localPath = localPath + "/"
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var user, host string
			var err error
			if !remoteIsLocal {
				hostID := c.String(hostFlagName)
				user, host, err = getUserAndHostname(ctx, c.String(hostFlagName), c.Parent().Parent().String(confFlagName))
				if err != nil {
					return errors.Wrapf(err, "could not get username and host for host ID '%s'", hostID)
				}
			}

			if doSanityCheck && !dryRun {
				ok := sanityCheckRsync(localPath, remotePath, pull)
				if !ok {
					fmt.Println("Refusing to perform rsync, exiting")
					return nil
				}
			}

			if timeout := c.Duration(timeoutFlagName); timeout != time.Duration(0) {
				ctx, cancel = context.WithTimeout(context.Background(), timeout)
				defer cancel()
			}
			makeParentDirs := c.BoolT(makeParentDirectoriesFlagName)
			makeParentDirsOnRemote := makeParentDirs && !pull && !remoteIsLocal
			opts := rsyncOpts{
				local:                localPath,
				remote:               remotePath,
				user:                 user,
				host:                 host,
				makeRemoteParentDirs: makeParentDirsOnRemote,
				excludePatterns:      c.StringSlice(excludeFlagName),
				shouldDelete:         c.BoolT(deleteFlagName),
				pull:                 pull,
				dryRun:               dryRun,
			}
			if makeParentDirs && !makeParentDirsOnRemote {
				if err = makeLocalParentDirs(localPath, remotePath, pull); err != nil {
					return errors.Wrap(err, "could not create directory structure")
				}
			}
			cmd, err := buildRsyncCommand(opts)
			if err != nil {
				return errors.Wrap(err, "could not build rsync command")
			}
			return cmd.Run(ctx)
		},
	}
}

// makeLocalParentDirs creates the parent directories for the rsync destination
// depending on whether or not we are doing a push or pull operation.
func makeLocalParentDirs(local, remote string, pull bool) error {
	var dirs string
	if pull {
		dirs = filepath.Dir(local)
	} else {
		dirs = filepath.Dir(remote)
	}

	return errors.Wrapf(os.MkdirAll(dirs, 0755), "could not create local parent directories '%s'", dirs)
}

// getUserAndHostname gets the user's spawn hosts and if the hostID matches one
// of the user's spawn hosts, it returns the username and hostname for that
// host.
func getUserAndHostname(ctx context.Context, hostID, confPath string) (user, hostname string, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conf, err := NewClientSettings(confPath)
	if err != nil {
		return "", "", errors.Wrap(err, "problem loading configuration")
	}
	client := conf.setupRestCommunicator(ctx)
	defer client.Close()

	params := model.APIHostParams{
		UserSpawned: true,
		Mine:        true,
		Status:      evergreen.HostRunning,
	}
	hosts, err := client.GetHosts(ctx, params)
	if err != nil {
		return "", "", errors.Wrap(err, "problem getting your spawn hosts")
	}

	for _, h := range hosts {
		if model.FromStringPtr(h.Id) == hostID {
			catcher := grip.NewBasicCatcher()
			user = model.FromStringPtr(h.User)
			catcher.ErrorfWhen(user == "", "could not find login user for host '%s'", hostID)
			hostname = model.FromStringPtr(h.HostURL)
			catcher.ErrorfWhen(hostname == "", "could not find hostname for host '%s'", hostID)
			return user, hostname, catcher.Resolve()
		}
	}
	return "", "", errors.Errorf("could not find host '%s' in user's spawn hosts", hostID)
}

// sanityCheckRsync performs some basic sanity checks for the common case in
// which you want to mirror two directories. The trailing slash means to
// overwrite all of the contents of the destination directory, so we check that
// they really want to do this.
func sanityCheckRsync(localPath, remotePath string, pull bool) bool {
	localPathIsDir := strings.HasSuffix(localPath, "/")
	remotePathIsDir := strings.HasSuffix(remotePath, "/")

	if localPathIsDir && !pull {
		ok := confirm(fmt.Sprintf("The local directory '%s' will overwrite any existing contents in the remote directory '%s'. Continue? (y/n)", localPath, remotePath), false)
		if !ok {
			return false
		}
	}
	if remotePathIsDir && pull {
		ok := confirm(fmt.Sprintf("The remote directory '%s' will overwrite any existing contents in the local directory '%s'. Continue? (y/n)", remotePath, localPath), false)
		if !ok {
			return false
		}
	}
	return true
}

type rsyncOpts struct {
	local                string
	remote               string
	user                 string
	host                 string
	makeRemoteParentDirs bool
	excludePatterns      []string
	shouldDelete         bool
	pull                 bool
	dryRun               bool
}

// buildRsyncCommand takes the given options and constructs an rsync command.
func buildRsyncCommand(opts rsyncOpts) (*jasper.Command, error) {
	rsync, err := exec.LookPath("rsync")
	if err != nil {
		return nil, errors.Wrap(err, "could not find rsync binary in the PATH")
	}

	args := []string{rsync, "-a", "--no-links", "--no-devices", "--no-specials", "-hh", "-I", "-z", "--progress", "-e", "ssh"}
	if opts.shouldDelete {
		args = append(args, "--delete")
	}
	for _, pattern := range opts.excludePatterns {
		args = append(args, "--exclude", pattern)
	}
	if opts.makeRemoteParentDirs {
		var parentDir string
		if runtime.GOOS == "windows" {
			// If we're using cygwin rsync, we have to use the POSIX path and
			// not the native one.
			baseIndex := strings.LastIndex(opts.remote, "/")
			if baseIndex == -1 {
				parentDir = opts.remote
			} else if baseIndex == 0 {
				parentDir = "/"
			} else {
				parentDir = opts.remote[:baseIndex]
			}
		} else {
			parentDir = filepath.Dir(opts.remote)
		}
		args = append(args, fmt.Sprintf(`--rsync-path=mkdir -p "%s" && rsync`, parentDir))
	}

	var dryRunIndex int
	if opts.dryRun {
		dryRunIndex = len(args)
		args = append(args, "-n")
	}

	var remote string
	if opts.user != "" && opts.host != "" {
		remote = fmt.Sprintf("%s@%s:%s", opts.user, opts.host, opts.remote)
	} else {
		remote = opts.remote
	}
	if opts.pull {
		args = append(args, remote, opts.local)
	} else {
		args = append(args, opts.local, remote)
	}

	if opts.dryRun {
		cmdWithoutDryRun := make([]string, len(args[:dryRunIndex]), len(args)-1)
		_ = copy(cmdWithoutDryRun, args[:dryRunIndex])
		cmdWithoutDryRun = append(cmdWithoutDryRun, args[dryRunIndex+1:]...)
		fmt.Printf("Going to execute the following command: %s\n", strings.Join(cmdWithoutDryRun, " "))
	}
	logLevel := level.Info
	stdout, err := send.NewPlainLogger("stdout", send.LevelInfo{Threshold: logLevel, Default: logLevel})
	if err != nil {
		return nil, errors.Wrap(err, "problem setting up standard output")
	}
	stderr, err := send.NewPlainErrorLogger("stderr", send.LevelInfo{Threshold: logLevel, Default: logLevel})
	if err != nil {
		return nil, errors.Wrap(err, "problem setting up standard error")
	}
	return jasper.NewCommand().Add(args).SetOutputSender(logLevel, stdout).SetOutputSender(logLevel, stderr), nil
}
