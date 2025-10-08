package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/client"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/google/shlex"
	"github.com/mitchellh/go-homedir"
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
		distroFlagName           = "distro"
		keyFlagName              = "key"
		scriptFlagName           = "script"
		tagFlagName              = "tag"
		instanceTypeFlagName     = "type"
		noExpireFlagName         = "no-expire"
		wholeWeekdaysOffFlagName = "weekdays-off"
		dailyStartTimeFlagName   = "daily-start"
		dailyStopTimeFlagName    = "daily-stop"
		timeZoneFlagName         = "timezone"
		fileFlagName             = "file"
		setupFlagName            = "setup"
	)

	return cli.Command{
		Name:  "create",
		Usage: "spawn a host",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  joinFlagNames(distroFlagName, "d"),
				Usage: "name of an Evergreen distro",
			},
			cli.StringFlag{
				Name:  joinFlagNames(keyFlagName, "k"),
				Usage: "provide either the value of a public key to use, or the Evergreen-managed name of a key (which can be viewed using 'evergreen keys list')",
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
			cli.StringSliceFlag{
				Name:  wholeWeekdaysOffFlagName,
				Usage: "for an unexpirable host, the days when the host should be turned off for its sleep schedule (allowed values: Sunday, Monday, Tuesday, Wednesday, Thursday, Friday, Saturday)",
			},
			cli.StringFlag{
				Name:  dailyStartTimeFlagName,
				Usage: "for an unexpirable host, the time when the host should start each day for its sleep schedule (format: HH:MM, e.g. 12:34)",
			},
			cli.StringFlag{
				Name:  dailyStopTimeFlagName,
				Usage: "for an unexpirable host, the time when the host should stop each day for its sleep schedule (format: HH:MM, e.g. 12:34)",
			},
			cli.StringFlag{
				Name:  timeZoneFlagName,
				Usage: "for an unexpirable host, the time zone of the sleep schedule (e.g. America/New_York)",
			},
			cli.StringFlag{
				Name:  joinFlagNames(fileFlagName, "f"),
				Usage: "name of a JSON or YAML file containing the spawn host params",
			},
		},
		Before: requireStringFlag(keyFlagName),
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
			weekdaysOffStrs := c.StringSlice(wholeWeekdaysOffFlagName)
			wholeWeekdaysOff, err := convertWeekdays(weekdaysOffStrs)
			if err != nil {
				return err
			}
			dailyStartTime := c.String(dailyStartTimeFlagName)
			dailyStopTime := c.String(dailyStopTimeFlagName)
			timeZone := c.String(timeZoneFlagName)
			file := c.String(fileFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			spawnRequest := &restModel.HostRequestOptions{}
			var tags []host.Tag
			if file != "" {
				if err = utility.ReadYAMLFile(file, &spawnRequest); err != nil {
					return errors.Wrapf(err, "reading from file '%s'", file)
				}
				if spawnRequest.Tag != "" {
					tags, err = host.MakeHostTags([]string{spawnRequest.Tag})
					if err != nil {
						return errors.Wrap(err, "generating host tags")
					}
					spawnRequest.InstanceTags = tags
				}

				userdataFile = spawnRequest.UserData
				setupFile = spawnRequest.SetupScript
			} else {
				tags, err = host.MakeHostTags(tagSlice)
				if err != nil {
					return errors.Wrap(err, "generating host tags")
				}
				spawnRequest = &restModel.HostRequestOptions{
					DistroID:     distro,
					KeyName:      key,
					InstanceTags: tags,
					InstanceType: instanceType,
					Region:       region,
					NoExpiration: noExpire,
					SleepScheduleOptions: host.SleepScheduleOptions{
						WholeWeekdaysOff: wholeWeekdaysOff,
						DailyStartTime:   dailyStartTime,
						DailyStopTime:    dailyStopTime,
						TimeZone:         timeZone,
					},
				}
			}

			if userdataFile != "" {
				var out []byte
				out, err = os.ReadFile(userdataFile)
				if err != nil {
					return errors.Wrapf(err, "reading userdata file '%s'", userdataFile)
				}
				spawnRequest.UserData = string(out)
			}
			if setupFile != "" {
				var out []byte
				out, err = os.ReadFile(setupFile)
				if err != nil {
					return errors.Wrapf(err, "reading setup file '%s'", setupFile)
				}
				spawnRequest.SetupScript = string(out)
			}

			host, err := client.CreateSpawnHost(ctx, spawnRequest)
			if err != nil {
				return errors.Wrap(err, "creating spawn host")
			}
			if host == nil {
				return errors.New("Unable to create a spawn host. Double check that the params and .evergreen.yml are correct")
			}

			grip.Infof("Spawn host created with ID '%s'. Visit the hosts page in Evergreen to check on its status, or check `evergreen host list --mine", utility.FromStringPtr(host.Id))
			return nil
		},
	}
}

func convertWeekdays(weekdayStrs []string) ([]time.Weekday, error) {
	if len(weekdayStrs) == 0 {
		return []time.Weekday{}, nil
	}

	var weekdays []time.Weekday
	weekdayStrToEnum := map[string]time.Weekday{}
	for _, weekday := range utility.Weekdays {
		weekdayStrToEnum[strings.ToLower(weekday.String())] = weekday
	}
	for _, weekdayStr := range weekdayStrs {
		weekday, ok := weekdayStrToEnum[strings.ToLower(weekdayStr)]
		if !ok {
			return nil, errors.Errorf("invalid weekday '%s'", weekdayStr)
		}
		weekdays = append(weekdays, weekday)
	}
	return weekdays, nil
}

func hostModify() cli.Command {
	const (
		addTagFlagName             = "tag"
		deleteTagFlagName          = "delete-tag"
		instanceTypeFlagName       = "type"
		displayNameFlagName        = "name"
		noExpireFlagName           = "no-expire"
		wholeWeekdaysOffFlagName   = "weekdays-off"
		dailyStartTimeFlagName     = "daily-start"
		dailyStopTimeFlagName      = "daily-stop"
		timeZoneFlagName           = "timezone"
		expireFlagName             = "expire"
		extendFlagName             = "extend"
		temporaryExemptionFlagName = "extend-temporary-exemption"
		addSSHKeyFlag              = "add-ssh-key"
		addSSHKeyNameFlag          = "add-ssh-key-name"
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
			cli.IntFlag{
				Name:  temporaryExemptionFlagName,
				Usage: "create or extend a temporary exemption from the host's sleep schedule by `HOURS`",
			},
			cli.BoolFlag{
				Name:  noExpireFlagName,
				Usage: "make host never expire",
			},
			cli.BoolFlag{
				Name:  expireFlagName,
				Usage: "make host expire like a normal spawn host, in 24 hours",
			},
			cli.StringSliceFlag{
				Name:  wholeWeekdaysOffFlagName,
				Usage: "for an unexpirable host, the days when the host should be turned off for its sleep schedule (allowed values: Sunday, Monday, Tuesday, Wednesday, Thursday, Friday, Saturday)",
			},
			cli.StringFlag{
				Name:  dailyStartTimeFlagName,
				Usage: "for an unexpirable host, the time when the host should start each day for its sleep schedule (format: HH:MM, e.g. 12:34)",
			},
			cli.StringFlag{
				Name:  dailyStopTimeFlagName,
				Usage: "for an unexpirable host, the time when the host should stop each day for its sleep schedule (format: HH:MM, e.g. 12:34)",
			},
			cli.StringFlag{
				Name:  timeZoneFlagName,
				Usage: "for an unexpirable host, the time zone of the sleep schedule (e.g. America/New_York)",
			},
			cli.StringFlag{
				Name:  addSSHKeyFlag,
				Usage: "add public key from local file `PATH` to the host's authorized_keys",
			},
			cli.StringFlag{
				Name:  addSSHKeyNameFlag,
				Usage: "add user defined public key named `KEY_NAME` to the host's authorized_keys",
			},
		)),
		Before: mergeBeforeFuncs(
			setPlainLogger,
			requireHostFlag,
			requireAtLeastOneFlag(addTagFlagName, deleteTagFlagName, instanceTypeFlagName, expireFlagName, noExpireFlagName, extendFlagName, temporaryExemptionFlagName, addSSHKeyFlag, addSSHKeyNameFlag, wholeWeekdaysOffFlagName, dailyStartTimeFlagName, dailyStopTimeFlagName, timeZoneFlagName),
			mutuallyExclusiveArgs(false, noExpireFlagName, extendFlagName),
			mutuallyExclusiveArgs(false, noExpireFlagName, expireFlagName),
			mutuallyExclusiveArgs(false, addSSHKeyFlag, addSSHKeyNameFlag),
		),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			hostID := c.String(hostFlagName)
			addTagSlice := c.StringSlice(addTagFlagName)
			deleteTagSlice := c.StringSlice(deleteTagFlagName)
			instanceType := c.String(instanceTypeFlagName)
			displayName := c.String(displayNameFlagName)
			noExpire := c.Bool(noExpireFlagName)
			expire := c.Bool(expireFlagName)
			weekdaysOffStrs := c.StringSlice(wholeWeekdaysOffFlagName)
			wholeWeekdaysOff, err := convertWeekdays(weekdaysOffStrs)
			if err != nil {
				return err
			}
			dailyStartTime := c.String(dailyStartTimeFlagName)
			dailyStopTime := c.String(dailyStopTimeFlagName)
			timeZone := c.String(timeZoneFlagName)
			temporaryExemptionHours := c.Int(temporaryExemptionFlagName)
			extension := c.Int(extendFlagName)
			subscriptionType := c.String(subscriptionTypeFlag)
			publicKeyFile := c.String(addSSHKeyFlag)
			publicKeyName := c.String(addSSHKeyNameFlag)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			addTags, err := host.MakeHostTags(addTagSlice)
			if err != nil {
				return errors.Wrap(err, "generating host tags")
			}

			publicKey, err := getPublicKey(ctx, client, publicKeyFile, publicKeyName)
			if err != nil {
				return errors.Wrap(err, "getting public key")
			}

			hostChanges := host.HostModifyOptions{
				AddInstanceTags:            addTags,
				DeleteInstanceTags:         deleteTagSlice,
				InstanceType:               instanceType,
				AddHours:                   time.Duration(extension) * time.Hour,
				AddTemporaryExemptionHours: temporaryExemptionHours,
				SubscriptionType:           subscriptionType,
				NewName:                    displayName,
				AddKey:                     publicKey,
				SleepScheduleOptions: host.SleepScheduleOptions{
					WholeWeekdaysOff: wholeWeekdaysOff,
					DailyStartTime:   dailyStartTime,
					DailyStopTime:    dailyStopTime,
					TimeZone:         timeZone,
				},
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

func getPublicKey(ctx context.Context, client client.Communicator, keyFile, keyName string) (string, error) {
	if keyFile != "" {
		return readKeyFromFile(keyFile)
	}

	if keyName != "" {
		return getUserKeyByName(ctx, client, keyName)
	}

	return "", nil
}

func readKeyFromFile(keyFile string) (string, error) {
	if !utility.FileExists(keyFile) {
		return "", errors.Errorf("key file '%s' does not exist", keyFile)
	}

	publicKeyBytes, err := os.ReadFile(keyFile)
	if err != nil {
		return "", errors.Wrapf(err, "reading public key from file")
	}

	return string(publicKeyBytes), nil
}

func getUserKeyByName(ctx context.Context, client client.Communicator, keyName string) (string, error) {
	userKeys, err := client.GetCurrentUsersKeys(ctx)
	if err != nil {
		return "", errors.New("getting user keys")
	}

	for _, pubKey := range userKeys {
		if utility.FromStringPtr(pubKey.Name) == keyName {
			return utility.FromStringPtr(pubKey.Key), nil
		}
	}

	return "", errors.Errorf("key name '%s' is not defined for the authenticated user", keyName)
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
				return errors.Wrap(err, "loading configuration")
			}

			client, err := conf.setupRestCommunicator(ctx, !quiet)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			projectRef, err := ac.GetProjectRef(project)
			if err != nil {
				return errors.Wrapf(err, "finding project '%s'", project)
			}

			if distroName != "" {
				var currentDistro *restModel.APIDistro
				currentDistro, err = client.GetDistroByName(ctx, distroName)
				if err != nil {
					return errors.Wrapf(err, "getting distro '%s'", distroName)
				}

				if currentDistro.IsVirtualWorkstation {
					if directory == cwd {
						grip.Warning("overriding directory flag for workstation setup")
						directory = ""
					}
					var userHome string
					userHome, err = homedir.Dir()
					if err != nil {
						return errors.Wrap(err, "finding home directory")
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
				return errors.Wrapf(err, "getting project setup commands")
			}

			grip.Info(message.Fields{
				"operation":          "setup project",
				"directory":          directory,
				"commands":           len(cmds),
				"project":            projectRef.Id,
				"project_identifier": projectRef.Identifier,
				"dry-run":            dryRun,
			})

			for idx, cmd := range cmds {
				if !dryRun {
					if err := makeWorkingDir(cmd); err != nil {
						return errors.Wrap(err, "making working directory")
					}
				}

				if err := cmd.Run(ctx); err != nil {
					grip.Infof("You may need to accept an email invite to receive access to repo '%s/%s'", projectRef.Owner, projectRef.Repo)
					return errors.Wrapf(err, "running command %d of %d to provision project '%s'", idx+1, len(cmds), projectRef.Id)
				}
			}
			return nil
		},
	}
}

func makeWorkingDir(cmd *jasper.Command) error {
	opts, err := cmd.Export()
	if err != nil {
		return errors.Wrap(err, "exporting command options")
	}
	if len(opts) == 0 {
		return errors.New("export returned empty options")
	}

	workingDir := opts[0].WorkingDirectory
	return errors.Wrapf(os.MkdirAll(workingDir, 0755), "creating directory '%s'", workingDir)
}

func hostStop() cli.Command {
	const (
		shouldKeepOffFlagName = "keep-off"
		waitFlagName          = "wait"
	)
	return cli.Command{
		Name:  "stop",
		Usage: "stop a running spawn host",
		Flags: mergeFlagSlices(addHostFlag(), addSubscriptionTypeFlag(
			cli.BoolFlag{
				Name:  joinFlagNames(waitFlagName, "w"),
				Usage: "command will block until host stopped",
			},
			cli.BoolTFlag{
				Name:  joinFlagNames(shouldKeepOffFlagName, "k"),
				Usage: "if stopping an unexpirable host with a sleep schedule, keep the host off indefinitely (and ignore its sleep schedule) until the host is manually started again (default: true)",
			},
		)),
		Before: mergeBeforeFuncs(setPlainLogger, requireHostFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			hostID := c.String(hostFlagName)
			subscriptionType := c.String(subscriptionTypeFlag)
			shouldKeepOff := c.Bool(shouldKeepOffFlagName)
			wait := c.Bool(waitFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			if wait {
				grip.Infof("Stopping host '%s'. This may take a few minutes...", hostID)
			}

			err = client.StopSpawnHost(ctx, hostID, subscriptionType, shouldKeepOff, wait)
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
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
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
		dryRunFlagName   = "dry-run"
	)

	return cli.Command{
		Name:  "ssh",
		Usage: "ssh into a spawn host",
		UsageText: `
SSH into a host by its ID and optionally pass arbitrary additional parameters to the SSH binary.

Examples:
* SSH into a host (i-abcdef12345) by ID:
	evergreen host ssh --host i-abcdef12345

* As a special case, if you pass a single argument, it will be interpreted as the host ID:
	evergreen host ssh i-abcdef12345

* SSH into a host (i-abcdef12345) and pass additional arguments to the SSH binary to use verbose mode (-vvv) and run a command (echo hello world):
	evergreen host ssh --host i-abcdef12345 -- -vvv "echo hello world"
`,
		Flags: addHostFlag(
			cli.StringFlag{
				Name:  joinFlagNames(identityFlagName, "i"),
				Usage: "Path to a specific identity (private key), for ssh -i",
			},
			cli.BoolFlag{
				Name:  dryRunFlagName,
				Usage: "show the SSH command that would run if executed, but do not actually execute it",
			},
		),
		Before: mergeBeforeFuncs(setPlainLogger, requireHostFlag),
		Action: func(c *cli.Context) error {
			grip.Info("Note: User must be on the VPN to gain access to the host.")
			confPath := c.Parent().Parent().String(confFlagName)
			hostID := c.String(hostFlagName)
			key := c.String(identityFlagName)
			dryRun := c.Bool(dryRunFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			h, err := client.GetSpawnHost(ctx, hostID)
			if err != nil {
				return errors.Wrapf(err, "getting spawn host '%s'", hostID)
			}
			if utility.FromStringPtr(h.Status) != evergreen.HostRunning {
				return errors.New("host is not running")
			}
			user := utility.FromStringPtr(h.User)
			url := getHostname(h)
			if user == "" || url == "" {
				return errors.New("unable to ssh into host without user or DNS name")
			}
			args := []string{"ssh", "-tt", fmt.Sprintf("%s@%s", user, url)}
			if key != "" {
				args = append(args, "-i", key)
			}

			// Append the arbitrary additional SSH args unless it's the special
			// case where it's a single arg and it's the host ID.
			if c.NArg() > 1 || c.NArg() == 1 && c.Args().Get(0) != hostID {
				args = append(args, c.Args()...)
			}

			if dryRun {
				fmt.Printf("Going to execute the following command:\n%s\n", strings.Join(args, " "))
				return nil
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
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "getting REST communicator")
			}
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
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "getting REST communicator")
			}
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
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "getting REST communicator")
			}
			defer client.Close()

			opts := restModel.VolumeModifyOptions{
				NewName:       name,
				Size:          int32(size),
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
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, false)
			if err != nil {
				return errors.Wrap(err, "getting REST communicator")
			}
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
		grip.Infof("\n%-18s: %s\n", "ID", utility.FromStringPtr(v.ID))
		if utility.FromStringPtr(v.DisplayName) != "" {
			grip.Infof("%-18s: %s\n", "Name", utility.FromStringPtr(v.DisplayName))
		}
		grip.Infof("%-18s: %d\n", "Size", v.Size)
		grip.Infof("%-18s: %s\n", "Type", utility.FromStringPtr(v.Type))
		grip.Infof("%-18s: %s\n", "Availability Zone", utility.FromStringPtr(v.AvailabilityZone))
		if utility.FromStringPtr(v.HostID) != "" {
			grip.Infof("%-18s: %s\n", "Device Name", utility.FromStringPtr(v.DeviceName))
			grip.Infof("%-18s: %s\n", "Attached to Host", utility.FromStringPtr(v.HostID))
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
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "getting REST communicator")
			}
			defer client.Close()

			volumeRequest := &host.Volume{
				Type:             volumeType,
				Size:             int32(volumeSize),
				AvailabilityZone: volumeZone,
				DisplayName:      volumeName,
			}

			volume, err := client.CreateVolume(ctx, volumeRequest)
			if err != nil {
				return err
			}

			grip.Infof("Created volume '%s'.", utility.FromStringPtr(volume.ID))

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
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "getting REST communicator")
			}
			defer client.Close()
			v, err := client.GetVolume(ctx, volumeID)
			if err != nil {
				return errors.Wrapf(err, "getting volume '%s'", volumeID)
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
			cli.BoolFlag{
				Name:  jsonFlagName,
				Usage: "list hosts in JSON format",
			},
		},
		Before: setPlainLogger,
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			showMine := c.Bool(mineFlagName)
			region := c.String(regionFlagName)
			showJSON := c.Bool(jsonFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, !showJSON)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			params := restModel.APIHostParams{
				UserSpawned: true,
				Mine:        showMine,
				Region:      region,
			}
			hosts, err := client.GetHosts(ctx, params)
			if err != nil {
				return errors.Wrap(err, "getting hosts")
			}

			if showJSON {
				printHostsJSON(hosts)
			} else {
				printHosts(hosts)
			}

			return nil
		},
	}
}

func printHosts(hosts []*restModel.APIHost) {
	for _, h := range hosts {
		hostname := getHostname(h)
		grip.Infof("ID: %s; Name: %s; Distro: %s; Status: %s; Host name: %s; User: %s; Availability Zone: %s",
			utility.FromStringPtr(h.Id),
			utility.FromStringPtr(h.DisplayName),
			utility.FromStringPtr(h.Distro.Id),
			utility.FromStringPtr(h.Status),
			hostname,
			utility.FromStringPtr(h.User),
			utility.FromStringPtr(h.AvailabilityZone))
	}
}

func printHostsJSON(hosts []*restModel.APIHost) {
	type hostResult struct {
		Id               string `json:"id"`
		Name             string `json:"name"`
		Distro           string `json:"distro"`
		Status           string `json:"status"`
		HostName         string `json:"host_name"`
		User             string `json:"user"`
		AvailabilityZone string `json:"availability_zone"`
	}
	hostResults := []hostResult{}
	for _, h := range hosts {
		hostname := getHostname(h)
		hostResults = append(hostResults, hostResult{
			Id:               utility.FromStringPtr(h.Id),
			Name:             utility.FromStringPtr(h.DisplayName),
			Distro:           utility.FromStringPtr(h.Distro.Id),
			Status:           utility.FromStringPtr(h.Status),
			HostName:         hostname,
			User:             utility.FromStringPtr(h.User),
			AvailabilityZone: utility.FromStringPtr(h.AvailabilityZone),
		})
	}
	h, err := json.MarshalIndent(hostResults, "", "\t")
	if err != nil {
		return
	}
	grip.Infof(string(h))
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
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			h, err := client.GetSpawnHost(ctx, hostID)
			if err != nil {
				return errors.Wrapf(err, "getting spawn host '%s'", hostID)
			}
			if h.NoExpiration {
				msg := fmt.Sprintf("This host is non-expirable. Please type '%s' if you are sure you want to terminate", deleteConfirmation)
				if !confirmWithMatchingString(msg, deleteConfirmation) {
					return nil
				}
			}
			err = client.TerminateSpawnHost(ctx, hostID)
			if err != nil {
				return errors.Wrap(err, "terminating spawn host")
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
		Flags: mergeFlagSlices(addHostFlag(), addSkipConfirmFlag(
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
			skipConfirm := c.Bool(skipConfirmFlagName)
			batchSize := c.Int(batchSizeFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			var hostIDs []string
			if hostID != "" {
				hostIDs = []string{hostID}
			} else {
				var createdBeforeTime, createdAfterTime time.Time
				if createdBefore != "" {
					createdBeforeTime, err = time.Parse(time.RFC3339, createdBefore)
					if err != nil {
						return errors.Wrap(err, "parsing created before time")
					}
				}
				if createdAfter != "" {
					createdAfterTime, err = time.Parse(time.RFC3339, createdAfter)
					if err != nil {
						return errors.Wrap(err, "parsing created after time")
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
					return errors.Wrapf(err, "getting matching hosts")
				}
				if len(hosts) == 0 {
					grip.Info("no matching hosts")
					return nil
				}
				for _, host := range hosts {
					hostIDs = append(hostIDs, utility.FromStringPtr(host.Id))
				}

				if !skipConfirm {
					if !confirm(fmt.Sprintf("The script will run on %d host(s), \n%s\nContinue?", len(hostIDs), strings.Join(hostIDs, "\n")), true) {
						return nil
					}
				}
			}

			if path != "" {
				var scriptBytes []byte
				scriptBytes, err = os.ReadFile(path)
				if err != nil {
					return errors.Wrapf(err, "read script from file '%s'", path)
				}
				script = string(scriptBytes)
				if script == "" {
					return errors.New("script is empty")
				}
			}

			hostsOutput, err := client.StartHostProcesses(ctx, hostIDs, script, batchSize)
			if err != nil {
				return errors.Wrap(err, "starting host processes")
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
					return errors.Wrap(err, "getting process output")
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
				Usage: "perform basic checks for common sources of error (ignored on dry runs) (default: true)",
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
				ok := verifyRsync(localPath, remotePath, pull)
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
					return errors.Wrap(err, "creating directory structure")
				}
			}
			cmd, err := buildRsyncCommand(opts)
			if err != nil {
				return errors.Wrap(err, "building rsync command")
			}
			return cmd.Run(ctx)
		},
	}
}

func hostFindBy() cli.Command {
	const (
		ipAddressFlagName = "ip-address"
	)
	return cli.Command{
		Name:    "get",
		Aliases: []string{"get-host"},
		Usage:   "get link to existing hosts",
		Flags: addHostFlag(
			cli.StringFlag{
				Name:  joinFlagNames(ipAddressFlagName, "ip"),
				Usage: "ip address used to find host",
			},
		),
		Before: requireAtLeastOneFlag(ipAddressFlagName),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			ipAddress := c.String(ipAddressFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "getting REST communicator")
			}
			defer client.Close()

			host, err := client.FindHostByIpAddress(ctx, ipAddress)
			if err != nil {
				return errors.Wrapf(err, "fetching host with IP address '%s'", ipAddress)
			}
			if host == nil {
				return errors.Errorf("host with IP address '%s' not found", ipAddress)
			}

			hostId := utility.FromStringPtr(host.Id)
			hostUser := utility.FromStringPtr(host.StartedBy)
			link := fmt.Sprintf("%s/host/%s", conf.UIServerHost, hostId)
			fmt.Printf(`
	     ID : %s
	   Link : %s
	   User : %s
`, hostId, link, hostUser)
			return nil
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

	return errors.Wrapf(os.MkdirAll(dirs, 0755), "creating local parent directories '%s'", dirs)
}

// getUserAndHostname gets the user's spawn hosts and if the hostID matches one
// of the user's spawn hosts, it returns the username and hostname for that
// host.
func getUserAndHostname(ctx context.Context, hostID, confPath string) (user, hostname string, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conf, err := NewClientSettings(confPath)
	if err != nil {
		return "", "", errors.Wrap(err, "loading configuration")
	}
	client, err := conf.setupRestCommunicator(ctx, true)
	if err != nil {
		return "", "", errors.Wrap(err, "setting up REST communicator")
	}
	defer client.Close()

	params := restModel.APIHostParams{
		UserSpawned: true,
		Mine:        true,
		Status:      evergreen.HostRunning,
	}
	hosts, err := client.GetHosts(ctx, params)
	if err != nil {
		return "", "", errors.Wrap(err, "getting user spawn hosts")
	}

	for _, h := range hosts {
		if utility.FromStringPtr(h.Id) == hostID {
			catcher := grip.NewBasicCatcher()
			user = utility.FromStringPtr(h.User)
			catcher.ErrorfWhen(user == "", "could not find login user for host '%s'", hostID)
			hostname = getHostname(h)
			catcher.ErrorfWhen(hostname == "", "could not find hostname for host '%s'", hostID)
			return user, hostname, catcher.Resolve()
		}
	}
	return "", "", errors.Errorf("could not find host '%s' in user's spawn hosts", hostID)
}

// getHostname returns the primary hostname for the host. If it has a persistent
// DNS name, that one is preferred.
func getHostname(h *restModel.APIHost) string {
	if persistentDNSName := utility.FromStringPtr(h.PersistentDNSName); persistentDNSName != "" {
		return persistentDNSName
	}
	return utility.FromStringPtr(h.HostURL)
}

// verifyRsync performs some basic validation for the common case in
// which you want to mirror two directories. The trailing slash means to
// overwrite all of the contents of the destination directory, so we check that
// they really want to do this.
func verifyRsync(localPath, remotePath string, pull bool) bool {
	localPathIsDir := strings.HasSuffix(localPath, "/")
	remotePathIsDir := strings.HasSuffix(remotePath, "/")

	if localPathIsDir && !pull {
		ok := confirm(fmt.Sprintf("The local directory '%s' will overwrite any existing contents in the remote directory '%s'. Continue?", localPath, remotePath), false)
		if !ok {
			return false
		}
	}
	if remotePathIsDir && pull {
		ok := confirm(fmt.Sprintf("The remote directory '%s' will overwrite any existing contents in the local directory '%s'. Continue?", remotePath, localPath), false)
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
		fmt.Printf("Going to execute the following command:\n%s\n", strings.Join(cmdWithoutDryRun, " "))
	}
	logLevel := level.Info
	stdout, err := send.NewPlainLogger("stdout", send.LevelInfo{Threshold: logLevel, Default: logLevel})
	if err != nil {
		return nil, errors.Wrap(err, "setting up standard output logger")
	}
	stderr, err := send.NewPlainErrorLogger("stderr", send.LevelInfo{Threshold: logLevel, Default: logLevel})
	if err != nil {
		return nil, errors.Wrap(err, "setting up standard error logger")
	}
	return jasper.NewCommand().Add(args).SetOutputSender(logLevel, stdout).SetOutputSender(logLevel, stderr), nil
}
