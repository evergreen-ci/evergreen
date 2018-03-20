package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
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
			viewSettings(),
			updateSettings(),
			listEvents(),
			revert(),
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
			requireClientConfig,
			func(c *cli.Context) error {
				if c.String(messageFlagName) != "" && c.Bool(clearFlagName) {
					return errors.New("cannot specify a message and the 'clear' option at the same time")
				}
				return nil
			},
			func(c *cli.Context) error {
				if c.String(messageFlagName) == "" && !c.Bool(clearFlagName) {
					return errors.New("cannot specify a message and the 'clear' option at the same time")
				}
				return nil
			},
		),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
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

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			client := conf.GetRestCommunicator(ctx)
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
		confPath := c.Parent().String(confFlagName)
		flagsToSet := c.StringSlice(adminFlagsFlagName)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		conf, err := NewClientSetttings(confPath)
		if err != nil {
			return errors.Wrap(err, "problem loading configuration")
		}
		client := conf.GetRestCommunicator(ctx)
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
			flags.HostinitDisabled = target
		case "monitor":
			flags.MonitorDisabled = target
		case "notify", "notifications", "notification":
			flags.NotificationsDisabled = target
		case "alerts", "alert":
			flags.AlertsDisabled = target
		case "taskrunner", "new-agents", "agents":
			flags.TaskrunnerDisabled = target
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
		confPath := c.Parent().String(confFlagName)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		conf, err := NewClientSetttings(confPath)
		if err != nil {
			return errors.Wrap(err, "problem loading configuration")
		}
		client := conf.GetRestCommunicator(ctx)
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
			confPath := c.Parent().String(confFlagName)
			updateString := c.String(updateFlagName)

			updateSettings := &model.APIAdminSettings{}
			err := yaml.Unmarshal([]byte(updateString), updateSettings)
			if err != nil {
				return errors.Wrap(err, "problem parsing update string as a settings document")
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.GetRestCommunicator(ctx)
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
			confPath := c.Parent().String(confFlagName)
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

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.GetRestCommunicator(ctx)
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
			confPath := c.Parent().String(confFlagName)
			guid := c.Args().Get(0)
			if guid == "" {
				return errors.New("must specify a guid to revert")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.GetRestCommunicator(ctx)
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
