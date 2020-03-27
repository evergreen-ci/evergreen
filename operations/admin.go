package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/pail"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	amboyCLI "github.com/mongodb/amboy/cli"
	"github.com/mongodb/anser/backup"
	amodel "github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	yaml "gopkg.in/yaml.v2"
)

func Admin() cli.Command {
	return cli.Command{
		Name:  "admin",
		Usage: "site administration for an evergreen deployment",
		Subcommands: []cli.Command{
			adminSetBanner(),
			adminDisableService(),
			adminEnableService(),
			adminBackup(),
			viewSettings(),
			updateSettings(),
			listEvents(),
			revert(),
			fetchAllProjectConfigs(),
			amboyCmd(),
			fromMdbForLocal(),
			toMdbForLocal(),
			updateRoleCmd(),
		},
	}
}

func adminSetBanner() cli.Command {
	const (
		messageFlagName = "message"
		clearFlagName   = "clear"
		themeFlagName   = "theme"
	)

	return cli.Command{
		Name:    "banner",
		Aliases: []string{"set-banner"},
		Usage:   "modify the contents of the site-wide display banner",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  fmt.Sprintf("%s, m", messageFlagName),
				Usage: "content of new message",
			},
			cli.StringFlag{
				Name:  fmt.Sprintf("%s, t", themeFlagName),
				Usage: "color theme to use for banner",
			},
			cli.BoolFlag{
				Name:  clearFlagName,
				Usage: "clear the content of the banner",
			},
		},
		Before: mergeBeforeFuncs(
			func(c *cli.Context) error {
				if c.String(messageFlagName) != "" && c.Bool(clearFlagName) {
					return errors.New("must specify either a message or the  'clear' option, but not both")
				}
				return nil
			},
			func(c *cli.Context) error {
				if c.String(messageFlagName) == "" && !c.Bool(clearFlagName) {
					return errors.New("must specify either a message or the 'clear' option, but not both")
				}
				return nil
			},
		),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			themeName := c.String(themeFlagName)
			msgContent := c.String(messageFlagName)

			var theme evergreen.BannerTheme
			var ok bool
			if themeName != "" {
				if ok, theme = evergreen.IsValidBannerTheme(themeName); !ok {
					return errors.Errorf("%s is not a valid banner theme", themeName)
				}
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			return errors.Wrap(client.SetBannerMessage(ctx, msgContent, theme),
				"problem setting the site-wide banner message")
		},
	}
}

func adminDisableService() cli.Command {
	return cli.Command{
		Name:   "disable-service",
		Usage:  "disable a background service",
		Flags:  adminFlagFlag(),
		Action: adminServiceChange(true),
	}
}

func adminEnableService() cli.Command {
	return cli.Command{
		Name:   "enable-service",
		Usage:  "enable a background service",
		Flags:  adminFlagFlag(),
		Action: adminServiceChange(false),
	}

}

func adminServiceChange(disable bool) cli.ActionFunc {
	return func(c *cli.Context) error {
		confPath := c.Parent().Parent().String(confFlagName)
		flagsToSet := c.StringSlice(adminFlagsFlagName)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		conf, err := NewClientSettings(confPath)
		if err != nil {
			return errors.Wrap(err, "problem loading configuration")
		}
		client := conf.setupRestCommunicator(ctx)
		defer client.Close()

		flags, err := client.GetServiceFlags(ctx)
		if err != nil {
			return errors.Wrap(err, "problem getting current service flag state")
		}

		if err := setServiceFlagValues(flagsToSet, disable, flags); err != nil {
			return errors.Wrap(err, "invalid service flags")
		}

		return errors.Wrap(client.SetServiceFlags(ctx, flags),
			"problem changing service state")

	}

}

func setServiceFlagValues(args []string, target bool, flags *model.APIServiceFlags) error {
	catcher := grip.NewSimpleCatcher()

	for _, f := range args {
		switch f {
		case "dispatch", "tasks", "taskdispatch", "task-dispatch":
			flags.TaskDispatchDisabled = target
		case "hostinit", "host-init":
			flags.HostInitDisabled = target
		case "monitor":
			flags.MonitorDisabled = target
		case "alerts", "alert", "notify", "notifications", "notification":
			flags.AlertsDisabled = target
		case "taskrunner", "new-agents", "agents":
			flags.AgentStartDisabled = target
		case "github", "repotracker", "gitter", "commits", "repo-tracker":
			flags.RepotrackerDisabled = target
		case "scheduler":
			flags.SchedulerDisabled = target
		default:
			catcher.Add(errors.Errorf("%s is not a recognized service flag", f))
		}
	}

	return catcher.Resolve()
}

func viewSettings() cli.Command {
	return cli.Command{
		Name:   "get-settings",
		Usage:  "view the evergreen configuration settings",
		Action: doViewSettings(),
		Before: setPlainLogger,
	}
}

func doViewSettings() cli.ActionFunc {
	return func(c *cli.Context) error {
		confPath := c.Parent().Parent().String(confFlagName)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		conf, err := NewClientSettings(confPath)
		if err != nil {
			return errors.Wrap(err, "problem loading configuration")
		}
		client := conf.setupRestCommunicator(ctx)
		defer client.Close()

		settings, err := client.GetSettings(ctx)
		if err != nil {
			return errors.Wrap(err, "problem getting evergreen settings")
		}

		settingsYaml, err := json.MarshalIndent(settings, " ", " ")
		if err != nil {
			return errors.Wrap(err, "problem marshalling evergreen settings")
		}

		grip.Info(settingsYaml)

		return nil
	}
}

func updateSettings() cli.Command {
	const updateFlagName = "update"

	return cli.Command{
		Name:   "update-settings",
		Before: mergeBeforeFuncs(setPlainLogger, requireStringFlag(updateFlagName)),
		Usage:  "update the evergreen configuration settings",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  joinFlagNames(updateFlagName, "u"),
				Usage: "update to make (JSON or YAML)",
			},
		},
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			updateString := c.String(updateFlagName)

			updateSettings := &model.APIAdminSettings{}
			err := yaml.Unmarshal([]byte(updateString), updateSettings)
			if err != nil {
				return errors.Wrap(err, "problem parsing update string as a settings document")
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			settings, err := client.UpdateSettings(ctx, updateSettings)
			if err != nil {
				return errors.Wrap(err, "problem updating evergreen settings")
			}

			settingsYaml, err := json.MarshalIndent(settings, " ", " ")
			if err != nil {
				return errors.Wrap(err, "problem marshalling evergreen settings")
			}

			grip.Info(settingsYaml)

			return nil
		},
	}
}

func listEvents() cli.Command {
	return cli.Command{
		Name:   "list-events",
		Before: setPlainLogger,
		Usage:  "print the admin event log",
		Flags:  mergeFlagSlices(addStartTimeFlag(), addLimitFlag()),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			timeString := c.String(startTimeFlagName)
			var ts time.Time
			var err error
			if timeString != "" {
				ts, err = time.Parse(time.RFC3339, timeString)
				if err != nil {
					return errors.Wrap(err, "error parsing start time")
				}
			} else {
				ts = time.Now()
			}
			limit := c.Int(limitFlagName)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			events, err := client.GetEvents(ctx, ts, limit)
			if err != nil {
				return errors.Wrap(err, "error retrieving events")
			}

			eventsPretty, err := json.MarshalIndent(events, " ", " ")
			if err != nil {
				return errors.Wrap(err, "problem marshalling events")
			}
			grip.Info(eventsPretty)

			return nil
		},
	}
}

func revert() cli.Command {
	return cli.Command{
		Name:   "revert-event",
		Before: mergeBeforeFuncs(setPlainLogger),
		Usage:  "revert a specific change to the admin settings",
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			guid := c.Args().Get(0)
			if guid == "" {
				return errors.New("must specify a guid to revert")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			err = client.RevertSettings(ctx, guid)
			if err != nil {
				return err
			}
			grip.Infof("Successfully reverted %s", guid)

			return nil
		},
	}
}

func amboyCmd() cli.Command {
	const (
		useLocalFlagName = "local"
		useGroupFlagName = "group"
	)

	opts := &amboyCLI.ServiceOptions{
		BaseURL:          "http://localhost:2285",
		ReportingPrefix:  "/amboy/remote/reporting",
		ManagementPrefix: "/amboy/remote/pool",
	}

	cmd := amboyCLI.Amboy(opts)
	cmd.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  useLocalFlagName,
			Usage: "set the default to use the local queue.",
		},
		cli.BoolFlag{
			Name:  useGroupFlagName,
			Usage: "set the default to use the group queue",
		},
	}

	cmd.Before = func(c *cli.Context) error {
		opts.Client = util.GetHTTPClient()
		if c.Bool(useLocalFlagName) {
			opts.ReportingPrefix = "/amboy/local/reporting"
			opts.ManagementPrefix = "/amboy/local/pool"
		} else if c.Bool(useGroupFlagName) {
			opts.ReportingPrefix = "/amboy/group/reporting"
			opts.ManagementPrefix = "/amboy/group/pool"
		}
		return nil
	}

	cmd.After = func(c *cli.Context) error {
		util.PutHTTPClient(opts.Client)
		return nil
	}

	return cmd
}

func updateRoleCmd() cli.Command {
	const (
		idFlagName          = "id"
		nameFlagName        = "name"
		scopeFlagName       = "scope"
		permissionsFlagName = "permissions"
		ownersFlagName      = "owners"
	)

	return cli.Command{
		Name:   "update-role",
		Before: mergeBeforeFuncs(setPlainLogger, requireStringFlag(idFlagName), requireStringFlag(scopeFlagName)),
		Usage:  "create or update a role",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  idFlagName,
				Usage: "ID of role. Specifying an existing ID will overwrite that role, and a new ID will create a new role",
			},
			cli.StringFlag{
				Name:  nameFlagName,
				Usage: "Name of role, used to identify it to users",
			},
			cli.StringFlag{
				Name:  scopeFlagName,
				Usage: "Scope of the role, used to identify what resources it has permissions for",
			},
			cli.StringSliceFlag{
				Name:  permissionsFlagName,
				Usage: "Permissions to grant the role, in the format of permission:level",
			},
			cli.StringSliceFlag{
				Name:  ownersFlagName,
				Usage: "Owners of the role, who will approve requests from users who wish to have the role",
			},
		},
		Action: func(c *cli.Context) error {
			id := c.String(idFlagName)
			name := c.String(nameFlagName)
			scope := c.String(scopeFlagName)
			owners := c.StringSlice(ownersFlagName)
			tempPermissions := c.StringSlice(permissionsFlagName)

			permissions := map[string]int{}
			for _, permission := range tempPermissions {
				parts := strings.Split(permission, ":")
				if len(parts) != 2 {
					return errors.Errorf("permission '%s' must be in the form of 'permission:level'", permission)
				}
				val, err := strconv.Atoi(parts[1])
				if err != nil {
					return errors.Errorf("level for permission '%s' must be an integer", parts[1])
				}
				permissions[parts[0]] = val
			}
			role := gimlet.Role{
				ID:          id,
				Name:        name,
				Scope:       scope,
				Owners:      owners,
				Permissions: permissions,
			}

			confPath := c.Parent().Parent().String(confFlagName)
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.setupRestCommunicator(context.Background())
			defer client.Close()
			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			return ac.UpdateRole(&role)
		},
	}
}

func adminBackup() cli.Command {
	const (
		collectionNameFlag = "collection"
		prefixFlagName     = "prefix"
	)

	return cli.Command{
		Name:  "backup",
		Usage: "create a backup (to s3) of a collection",
		Flags: mergeFlagSlices(
			serviceConfigFlags(),
			addDbSettingsFlags(
				cli.StringFlag{
					Name:  joinFlagNames(collectionNameFlag, "c"),
					Usage: "specify the name of the collection to backup",
				},
				cli.StringFlag{
					Name:  prefixFlagName,
					Value: "archive",
					Usage: "specify prefix within the bucket to store this archive",
				},
			),
		),
		Before: requireStringFlag(collectionNameFlag),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			collectionName := c.String(collectionNameFlag)

			env, err := evergreen.NewEnvironment(ctx, c.String(confFlagName), parseDB(c))
			grip.EmergencyFatal(errors.Wrap(err, "problem configuring application environment"))
			evergreen.SetEnvironment(env)

			// avoid working on remote jobs during the backup
			env.RemoteQueue().Runner().Close(ctx)
			env.RemoteQueueGroup().Close(ctx)

			client := util.GetHTTPClient()
			client.Timeout = 30 * 24 * time.Hour
			defer util.PutHTTPClient(client)

			conf := env.Settings().Backup
			conf.Prefix = c.String(prefixFlagName)
			bucket, err := pail.NewS3MultiPartBucketWithHTTPClient(client, pail.S3Options{
				Credentials: pail.CreateAWSCredentials(conf.Key, conf.Secret, ""),
				Permissions: pail.S3PermissionsPrivate,
				Name:        conf.BucketName,
				Compress:    conf.Compress,
				Prefix:      strings.Join([]string{conf.Prefix, time.Now().Format(units.TSFormat), "dump"}, "/"),
				MaxRetries:  10,
			})
			grip.EmergencyFatal(errors.Wrap(err, "problem constructing bucket"))
			opts := backup.Options{
				NS: amodel.Namespace{
					DB:         env.Settings().Database.DB,
					Collection: collectionName,
				},
				EnableLogging: true,
				Target:        bucket.Writer,
			}
			return backup.Collection(ctx, env.Client(), opts)
		},
	}

}
