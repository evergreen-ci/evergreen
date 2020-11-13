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
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/host"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/google/shlex"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
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
		fileFlagName         = "file"
		setupFlagName        = "setup"
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
				Usage: "provide either the value of a public key to use, or the Evergreen-managed name of a key",
			},
			cli.StringFlag{
				Name:  joinFlagNames(scriptFlagName, "s"),
				Usage: "path to userdata script to run",
			},
			cli.StringFlag{
				Name:  setupFlagName,
				Usage: "path to a setup script to run",
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
			cli.StringFlag{
				Name:  joinFlagNames(fileFlagName, "f"),
				Usage: "name of a json or yaml file containing the spawn host params",
			},
		},
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			distro := c.String(distroFlagName)
			key := c.String(keyFlagName)
			userdataFile := c.String(scriptFlagName)
			setupFile := c.String(setupFlagName)
			tagSlice := c.StringSlice(tagFlagName)
			instanceType := c.String(instanceTypeFlagName)
			region := c.String(regionFlagName)
			noExpire := c.Bool(noExpireFlagName)
			file := c.String(fileFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			spawnRequest := &restModel.HostRequestOptions{}
			var tags []host.Tag
			if file != "" {
				if err = utility.ReadYAMLFile(file, &spawnRequest); err != nil {
					return errors.Wrapf(err, "problem reading from file '%s'", file)
				}
				if spawnRequest.Tag != "" {
					tags, err = host.MakeHostTags([]string{spawnRequest.Tag})
					if err != nil {
						return errors.Wrap(err, "a problem generating tags")
					}
					spawnRequest.InstanceTags = tags
				}

				userdataFile = spawnRequest.UserData
				setupFile = spawnRequest.SetupScript
			} else {
				tags, err = host.MakeHostTags(tagSlice)
				if err != nil {
					return errors.Wrap(err, "problem generating tags")
				}
				spawnRequest = &restModel.HostRequestOptions{
					DistroID:     distro,
					KeyName:      key,
					InstanceTags: tags,
					InstanceType: instanceType,
					Region:       region,
					NoExpiration: noExpire,
				}
			}

			if userdataFile != "" {
				var out []byte
				out, err = ioutil.ReadFile(userdataFile)
				if err != nil {
					return errors.Wrapf(err, "problem reading userdata file '%s'", userdataFile)
				}
				spawnRequest.UserData = string(out)
			}
			if setupFile != "" {
				var out []byte
				out, err = ioutil.ReadFile(setupFile)
				if err != nil {
					return errors.Wrapf(err, "problem reading setup file '%s'", setupFile)
				}
				spawnRequest.SetupScript = string(out)
			}

			host, err := client.CreateSpawnHost(ctx, spawnRequest)
			if err != nil {
				return errors.Wrap(err, "problem contacting evergreen service")
			}
			if host == nil {
				return errors.New("Unable to create a spawn host. Double check that the params and .evergreen.yml are correct")
			}

			grip.Infof("Spawn host created with ID '%s'. Visit the hosts page in Evergreen to check on its status, or check `evergreen host list --mine", restModel.FromStringPtr(host.Id))
			return nil
		},
	}
}

func hostModify() cli.Command {
	const (
		addTagFlagName       = "tag"
		deleteTagFlagName    = "delete-tag"
		instanceTypeFlagName = "type"
		displayNameFlagName  = "name"
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
			cli.StringFlag{
				Name:  displayNameFlagName,
				Usage: "set a user-friendly name for host",
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
		Before: mergeBeforeFuncs(
			setPlainLogger,
			requireHostFlag,
			requireAtLeastOneFlag(addTagFlagName, deleteTagFlagName, instanceTypeFlagName, expireFlagName, noExpireFlagName, extendFlagName),
			mutuallyExclusiveArgs(false, noExpireFlagName, extendFlagName),
			mutuallyExclusiveArgs(false, noExpireFlagName, expireFlagName),
		),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			hostID := c.String(hostFlagName)
			addTagSlice := c.StringSlice(addTagFlagName)
			deleteTagSlice := c.StringSlice(deleteTagFlagName)
			instanceType := c.String(instanceTypeFlagName)
			noExpire := c.Bool(noExpireFlagName)
			displayName := c.String(displayNameFlagName)
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
				NewName:            displayName,
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

func hostConfigure() cli.Command {
	const (
		distroNameFlagName = "distro"
		dryRunFlagName     = "dry-run"
	)
	cwd, _ := os.Getwd()

	return cli.Command{
		Name:  "configure",
		Usage: "run setup commands for a virtual workstation",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  joinFlagNames(projectFlagName, "p"),
				Usage: "name of an evergreen project to run commands from",
			},
			cli.StringFlag{
				Name:  joinFlagNames(dirFlagName, "d"),
				Usage: "directory to run commands from (will override project configuration)",
				Value: cwd,
			},
			cli.BoolFlag{
				Name:  joinFlagNames(quietFlagName, "q"),
				Usage: "suppress output",
			},
			cli.StringFlag{
				Name:  distroNameFlagName,
				Usage: "specify the name of the current distro for spawn hosts (optional)",
			},
			cli.BoolFlag{
				Name:  joinFlagNames(dryRunFlagName, "n"),
				Usage: "commands will print but not execute",
			},
		},
		Before: mergeBeforeFuncs(setPlainLogger, requireProjectFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			project := c.String(projectFlagName)
			directory := c.String(dirFlagName)
			distroName := c.String(distroNameFlagName)
			quiet := c.Bool(quietFlagName)
			dryRun := c.Bool(dryRunFlagName)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			projectRef, err := ac.GetProjectRef(project)
			if err != nil {
				return errors.Wrapf(err, "can't find project for queue id '%s'", project)
			}

			if distroName != "" {
				var currentDistro *restModel.APIDistro
				currentDistro, err = client.GetDistroByName(ctx, distroName)
				if err != nil {
					return errors.Wrap(err, "problem getting distro")
				}

				if currentDistro.IsVirtualWorkstation {
					if directory == cwd {
						grip.Warning("overriding directory flag for workstation setup")
						directory = ""
					}
					var userHome string
					userHome, err = homedir.Dir()
					if err != nil {
						return errors.Wrap(err, "problem finding home directory")
					}

					directory = filepath.Join(userHome, directory)
				}
			}

			cmds, err := projectRef.GetProjectSetupCommands(apimodels.WorkstationSetupCommandOptions{
				Directory: directory,
				Quiet:     quiet,
				DryRun:    dryRun,
			})
			if err != nil {
				return errors.Wrapf(err, "error getting commands")
			}

			grip.Info(message.Fields{
				"operation": "setup project",
				"directory": directory,
				"commands":  len(cmds),
				"project":   projectRef.Id,
				"dry-run":   dryRun,
			})

			for idx, cmd := range cmds {
				if !dryRun {
					if err := makeWorkingDir(cmd); err != nil {
						return errors.Wrap(err, "problem making working directory")
					}
				}

				if err := cmd.Run(ctx); err != nil {
					return errors.Wrapf(err, "problem running cmd %d of %d to provision %s", idx+1, len(cmds), projectRef.Id)
				}
			}
			return nil
		},
	}
}

func makeWorkingDir(cmd *jasper.Command) error {
	opts, err := cmd.Export()
	if err != nil {
		return errors.Wrap(err, "can't export command options")
	}
	if len(opts) == 0 {
		return errors.New("export returned empty options")
	}

	workingDir := opts[0].WorkingDirectory
	return errors.Wrapf(os.MkdirAll(workingDir, 0755), "can't make directory '%s'", workingDir)
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

func hostSSH() cli.Command {
	const (
		identityFlagName = "identity_file"
	)

	return cli.Command{
		Name:  "ssh",
		Usage: "ssh into a spawn host",
		Flags: addHostFlag(
			cli.StringFlag{
				Name:  joinFlagNames(identityFlagName, "i"),
				Usage: "Path to a specific identity (private key), for ssh -i",
			},
		),
		Before: mergeBeforeFuncs(setPlainLogger, requireHostFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			hostID := c.String(hostFlagName)
			key := c.String(identityFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			h, err := client.GetSpawnHost(ctx, hostID)
			if err != nil {
				return errors.Wrap(err, "problem getting host")
			}
			if restModel.FromStringPtr(h.Status) != evergreen.HostRunning {
				return errors.New("host is not running")
			}
			user := restModel.FromStringPtr(h.User)
			url := restModel.FromStringPtr(h.HostURL)
			if user == "" || url == "" {
				return errors.New("unable to ssh into host without user or DNS name")
			}
			args := []string{"ssh", "-tt", fmt.Sprintf("%s@%s", user, url)}
			if key != "" {
				args = append(args, "-i", key)
			}
			return jasper.NewCommand().Add(args).SetErrorWriter(os.Stderr).SetOutputWriter(os.Stdout).SetInput(os.Stdin).Run(ctx)
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

func hostModifyVolume() cli.Command {
	const (
		idFlagName       = "id"
		sizeFlag         = "size"
		extendFlag       = "extend"
		noExpireFlagName = "no-expire"
		expireFlagName   = "expire"
	)
	return cli.Command{
		Name:  "modify",
		Usage: "modify a volume",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  idFlagName,
				Usage: "`ID` of volume to modify",
			},
			cli.StringFlag{
				Name:  displayNameFlagName,
				Usage: "new user-friendly name for volume",
			},
			cli.IntFlag{
				Name:  joinFlagNames(sizeFlag, "s"),
				Usage: "set new volume `SIZE` in GiB",
			},
			cli.IntFlag{
				Name:  extendFlag,
				Usage: "extend the expiration by `HOURS`",
			},
			cli.BoolFlag{
				Name:  noExpireFlagName,
				Usage: "make volume never expire",
			},
			cli.BoolFlag{
				Name:  expireFlagName,
				Usage: "reinstate volume expiration",
			},
		},
		Before: mergeBeforeFuncs(
			setPlainLogger,
			requireStringFlag(idFlagName),
			requireAtLeastOneFlag(displayNameFlagName, sizeFlag, extendFlag, noExpireFlagName),
			mutuallyExclusiveArgs(false, extendFlag, noExpireFlagName),
			mutuallyExclusiveArgs(false, noExpireFlagName, expireFlagName),
		),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			volumeID := c.String(idFlagName)
			name := c.String(displayNameFlagName)
			size := c.Int(sizeFlag)
			extendDuration := c.Int(extendFlag)
			noExpiration := c.Bool(noExpireFlagName)
			hasExpiration := c.Bool(expireFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.getRestCommunicator(ctx)
			defer client.Close()

			opts := restModel.VolumeModifyOptions{
				NewName:       name,
				Size:          size,
				NoExpiration:  noExpiration,
				HasExpiration: hasExpiration,
			}
			if extendDuration > 0 {
				opts.Expiration = time.Now().Add(time.Duration(extendDuration) * time.Hour)
			}
			return client.ModifyVolume(ctx, volumeID, &opts)
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

func printVolumes(volumes []restModel.APIVolume, userID string) {
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
		grip.Infof("\n%-18s: %s\n", "ID", restModel.FromStringPtr(v.ID))
		if restModel.FromStringPtr(v.DisplayName) != "" {
			grip.Infof("%-18s: %s\n", "Name", restModel.FromStringPtr(v.DisplayName))
		}
		grip.Infof("%-18s: %d\n", "Size", v.Size)
		grip.Infof("%-18s: %s\n", "Type", restModel.FromStringPtr(v.Type))
		grip.Infof("%-18s: %s\n", "Availability Zone", restModel.FromStringPtr(v.AvailabilityZone))
		if restModel.FromStringPtr(v.HostID) != "" {
			grip.Infof("%-18s: %s\n", "Device Name", restModel.FromStringPtr(v.DeviceName))
			grip.Infof("%-18s: %s\n", "Attached to Host", restModel.FromStringPtr(v.HostID))
		} else {
			t, err := restModel.FromTimePtr(v.Expiration)
			if err == nil && !utility.IsZeroTime(t) {
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
			cli.IntFlag{
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
			cli.StringFlag{
				Name:  displayNameFlagName,
				Usage: "set a user-friendly name for volume",
			},
		},
		Before: mergeBeforeFuncs(setPlainLogger, requireStringFlag(sizeFlag)),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			volumeType := c.String(typeFlag)
			volumeZone := c.String(zoneFlag)
			volumeName := c.String(displayNameFlagName)
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
				DisplayName:      volumeName,
			}

			volume, err := client.CreateVolume(ctx, volumeRequest)
			if err != nil {
				return err
			}

			grip.Infof("Created volume '%s'.", restModel.FromStringPtr(volume.ID))

			return nil
		},
	}
}

func hostDeleteVolume() cli.Command {
	const (
		idFlagName         = "id"
		deleteConfirmation = "delete"
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
			v, err := client.GetVolume(ctx, volumeID)
			if err != nil {
				return errors.Wrap(err, "problem getting volume")
			}
			if v.NoExpiration {
				msg := fmt.Sprintf("This volume is non-expirable. Please type '%s' if you are sure you want to terminate", deleteConfirmation)
				if !confirmWithMatchingString(msg, deleteConfirmation) {
					return nil
				}
			}
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

			params := restModel.APIHostParams{
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

func printHosts(hosts []*restModel.APIHost) {
	for _, h := range hosts {
		grip.Infof("ID: %s; Name: %s; Distro: %s; Status: %s; Host name: %s; User: %s, Availability Zone: %s",
			restModel.FromStringPtr(h.Id),
			restModel.FromStringPtr(h.DisplayName),
			restModel.FromStringPtr(h.Distro.Id),
			restModel.FromStringPtr(h.Status),
			restModel.FromStringPtr(h.HostURL),
			restModel.FromStringPtr(h.User),
			restModel.FromStringPtr(h.AvailabilityZone))
	}
}

func hostTerminate() cli.Command {
	const (
		deleteConfirmation = "delete"
	)
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

			h, err := client.GetSpawnHost(ctx, hostID)
			if err != nil {
				return errors.Wrap(err, "problem getting spawn host")
			}
			if h.NoExpiration {
				msg := fmt.Sprintf("This host is non-expirable. Please type '%s' if you are sure you want to terminate", deleteConfirmation)
				if !confirmWithMatchingString(msg, deleteConfirmation) {
					return nil
				}
			}
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

				var hosts []*restModel.APIHost
				hosts, err = client.GetHosts(ctx, restModel.APIHostParams{
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
					hostIDs = append(hostIDs, restModel.FromStringPtr(host.Id))
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
		binaryParamsFlagName          = "params"
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

If you already know how to use rsync and this command doesn't have an explicit
option for something that rsync provides, you can pass parameters directly to
the rsync binary using --params.

Examples:
* Create or overwrite a file between the local filesystem and remote spawn host:

	evergreen host rsync -l /path/to/local/file1 -r /path/to/remote/file2 --host <host_id>

	If file2 does not exist on the remote host, it will be created (including
	any parent directories) and its contents will exactly match those of file1
	on the local filesystem.  Otherwrise, file2 will be overwritten.

* A more practical example to upload your .bashrc to the host:

	evergreen host rsync -l ~/.bashrc -r .bashrc --host <host_id>

* Push (mirror) the directory contents from the local filesystem to the directory on the remote spawn host:

	evergreen host rsync -l /path/to/local/dir1/ -r /path/to/remote/dir2/ --host <host_id>

	NOTE: the trailing slash in the file paths are required here.
	This will replace all the contents of the remote directory dir2 to match the
	contents of the local directory dir1.

* Pull (mirror) a spawn host's remote directory onto the local filesystem:

	evergreen host rsync -l /path/to/local/dir1/ -r /path/to/remote/dir2/ --pull --host <host_id>

	NOTE: the trailing slash in the file paths are required here.
	This will replace all the contents of the local directory dir1 to match the
	contents of the remote directory dir2.

* Push a local directory to become a subdirectory at the remote path on the remote spawn host:

	evergreen host rsync -l /path/to/local/dir1 -r /path/to/remote/dir2 --host <host_id>

	NOTE: this should not have a trailing slash at the end of the file paths.
	This will create a subdirectory dir1 containing the local dir1's contents below
	dir2 (i.e. it will create /path/to/remote/dir2/dir1).

* Mirror two directories on the same host:

	evergreen host rsync -l /path/to/first/local/dir1/ -r /path/to/second/local/dir2/ --remote-is-local

	This will make the contents of dir2 match the contents of dir1, both of
	which are on your local machine.

* Exclude files/directories from being synced:

	evergreen host rsync -l /path/to/local/dir1/ -r /path/to/remote/dir2/ --host <host_id> -x excluded_dir/ -x excluded_file

	NOTE: paths to excluded files/directory are relative to the source directory.
	This will mirror all the contents of the local dir1 in the remote dir2
	except for dir1/excluded_dir and dir1/excluded_file.

* Disable sanity checking prompt when mirroring directories:

	evergreen host rsync -l /path/to/local/dir1/ -r /path/to/remote/dir2/ --host <host_id> --sanity-checks=false

* Dry run the command to see what will be changed without actually changing anything:

	evergreen host rsync -l /path/to/local/dir1/ -r /path/to/remote/dir2/ --host <host_id> --dry-run
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
			cli.StringSliceFlag{
				Name:  binaryParamsFlagName,
				Usage: "pass arbitrary additional parameters to the rsync binary",
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
			binaryParams, err := splitRsyncBinaryParams(c.StringSlice(binaryParamsFlagName)...)
			if err != nil {
				return errors.Wrap(err, "splitting rsync parameters")
			}

			if strings.HasSuffix(localPath, "/") && !strings.HasSuffix(remotePath, "/") {
				remotePath = remotePath + "/"
			}
			if strings.HasSuffix(remotePath, "/") && !strings.HasSuffix(localPath, "/") {
				localPath = localPath + "/"
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var user, host string
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
				binaryParams:         binaryParams,
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

// splitRsyncBinaryParams splits parameters to the rsync binary using shell
// parsing rules.
func splitRsyncBinaryParams(params ...string) ([]string, error) {
	var rsyncParams []string
	for _, param := range params {
		splitParams, err := shlex.Split(param)
		if err != nil {
			return nil, errors.Wrap(err, "shell parsing")
		}
		rsyncParams = append(rsyncParams, splitParams...)
	}
	return rsyncParams, nil
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

	params := restModel.APIHostParams{
		UserSpawned: true,
		Mine:        true,
		Status:      evergreen.HostRunning,
	}
	hosts, err := client.GetHosts(ctx, params)
	if err != nil {
		return "", "", errors.Wrap(err, "problem getting your spawn hosts")
	}

	for _, h := range hosts {
		if restModel.FromStringPtr(h.Id) == hostID {
			catcher := grip.NewBasicCatcher()
			user = restModel.FromStringPtr(h.User)
			catcher.ErrorfWhen(user == "", "could not find login user for host '%s'", hostID)
			hostname = restModel.FromStringPtr(h.HostURL)
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
	binaryParams         []string
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

	args = append(args, opts.binaryParams...)

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
