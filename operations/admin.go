package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	amboyCLI "github.com/mongodb/amboy/cli"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"gopkg.in/yaml.v3"
)

func Admin() cli.Command {
	return cli.Command{
		Name:  "admin",
		Usage: "site administration for an Evergreen deployment",
		Subcommands: []cli.Command{
			adminSetBanner(),
			viewSettings(),
			updateSettings(),
			listEvents(),
			revert(),
			fetchAllProjectConfigs(),
			amboyCmd(),
			fromMdbForLocal(),
			toMdbForLocal(),
			updateRoleCmd(),
			updateServiceUser(),
			getServiceUsers(),
			deleteServiceUser(),
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
					return errors.New("must specify either a message or the 'clear' option, but not both")
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
			confPath := c.Parent().Parent().String(ConfFlagName)
			themeName := strings.ToUpper(c.String(themeFlagName))
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
				return errors.Wrap(err, "loading configuration")
			}

			client, err := conf.setupRestCommunicator(ctx, false)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			return errors.Wrap(client.SetBannerMessage(ctx, msgContent, theme), "setting the site-wide banner message")
		},
	}
}

func viewSettings() cli.Command {
	return cli.Command{
		Name:   "get-settings",
		Usage:  "view the Evergreen configuration settings",
		Action: doViewSettings(),
		Before: setPlainLogger,
	}
}

func doViewSettings() cli.ActionFunc {
	return func(c *cli.Context) error {
		confPath := c.Parent().Parent().String(ConfFlagName)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		conf, err := NewClientSettings(confPath)
		if err != nil {
			return errors.Wrap(err, "loading configuration")
		}
		client, err := conf.setupRestCommunicator(ctx, false)
		if err != nil {
			return errors.Wrap(err, "setting up REST communicator")
		}
		defer client.Close()

		settings, err := client.GetSettings(ctx)
		if err != nil {
			return errors.Wrap(err, "getting Evergreen settings")
		}

		settingsYaml, err := json.MarshalIndent(settings, " ", " ")
		if err != nil {
			return errors.Wrap(err, "marshalling Evergreen settings")
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
		Usage:  "update the Evergreen configuration settings",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  joinFlagNames(updateFlagName, "u"),
				Usage: "update to make (JSON or YAML)",
			},
		},
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(ConfFlagName)
			updateString := c.String(updateFlagName)

			updateSettings := &model.APIAdminSettings{}
			err := yaml.Unmarshal([]byte(updateString), updateSettings)
			if err != nil {
				return errors.Wrap(err, "parsing update string as a settings document")
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, false)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			settings, err := client.UpdateSettings(ctx, updateSettings)
			if err != nil {
				return errors.Wrap(err, "updating Evergreen settings")
			}

			settingsYaml, err := json.MarshalIndent(settings, " ", " ")
			if err != nil {
				return errors.Wrap(err, "marshalling Evergreen settings")
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
			confPath := c.Parent().Parent().String(ConfFlagName)
			timeString := c.String(startTimeFlagName)
			var ts time.Time
			var err error
			if timeString != "" {
				ts, err = time.Parse(time.RFC3339, timeString)
				if err != nil {
					return errors.Wrap(err, "parsing start time")
				}
			} else {
				ts = time.Now()
			}
			limit := c.Int(limitFlagName)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, false)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			events, err := client.GetEvents(ctx, ts, limit)
			if err != nil {
				return errors.Wrap(err, "retrieving events")
			}

			eventsPretty, err := json.MarshalIndent(events, " ", " ")
			if err != nil {
				return errors.Wrap(err, "marshalling events")
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
			confPath := c.Parent().Parent().String(ConfFlagName)
			guid := c.Args().Get(0)
			if guid == "" {
				return errors.New("must specify a GUID to revert")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, false)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			err = client.RevertSettings(ctx, guid)
			if err != nil {
				return err
			}
			grip.Infof("Successfully reverted GUID '%s'", guid)

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
		BaseURL:                       "http://localhost:2285",
		QueueManagementPrefix:         "/amboy/remote/management",
		AbortablePoolManagementPrefix: "/amboy/remote/pool",
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
		opts.Client = utility.GetHTTPClient()
		if c.Bool(useLocalFlagName) {
			opts.QueueManagementPrefix = "/amboy/local/management"
			opts.AbortablePoolManagementPrefix = "/amboy/local/pool"
		} else if c.Bool(useGroupFlagName) {
			opts.QueueManagementPrefix = "/amboy/group/management"
			opts.AbortablePoolManagementPrefix = "/amboy/group/pool"
		}
		return nil
	}

	cmd.After = func(c *cli.Context) error {
		utility.PutHTTPClient(opts.Client)
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

			confPath := c.Parent().Parent().String(ConfFlagName)
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			return ac.UpdateRole(&role)
		},
	}
}

func getServiceUsers() cli.Command {
	return cli.Command{
		Name:   "get-service-users",
		Usage:  "prints all service users",
		Before: setPlainLogger,

		Action: func(c *cli.Context) error {

			confPath := c.Parent().Parent().String(ConfFlagName)
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client, err := conf.setupRestCommunicator(ctx, false)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			users, err := client.GetServiceUsers(ctx)
			if err != nil {
				return err
			}
			for _, u := range users {
				grip.Info(u)
			}
			return nil
		},
	}
}

func updateServiceUser() cli.Command {
	const (
		idFlag          = "id"
		displayNameFlag = "name"
		rolesFlag       = "roles"
	)
	return cli.Command{
		Name:  "update-service-user",
		Usage: "adds or updates a service user",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  idFlag,
				Usage: "the username of the service user",
			},
			cli.StringFlag{
				Name:  displayNameFlag,
				Usage: "the display name of the service user",
			},
			cli.StringSliceFlag{
				Name:  rolesFlag,
				Usage: "The roles for the user. Must be the entire list of roles to set",
			},
		},
		Before: requireStringFlag(idFlag),
		Action: func(c *cli.Context) error {
			id := c.String(idFlag)
			displayName := c.String(displayNameFlag)
			roles := c.StringSlice(rolesFlag)

			confPath := c.Parent().Parent().String(ConfFlagName)
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client, err := conf.setupRestCommunicator(ctx, false)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			return client.UpdateServiceUser(ctx, id, displayName, roles)
		},
	}
}

func deleteServiceUser() cli.Command {
	const (
		idFlag = "id"
	)
	return cli.Command{
		Name:  "delete-service-user",
		Usage: "deletes a service user",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  idFlag,
				Usage: "the username of the service user",
			},
		},
		Before: requireStringFlag(idFlag),
		Action: func(c *cli.Context) error {
			id := c.String(idFlag)

			confPath := c.Parent().Parent().String(ConfFlagName)
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client, err := conf.setupRestCommunicator(ctx, false)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			return client.DeleteServiceUser(ctx, id)
		},
	}
}
