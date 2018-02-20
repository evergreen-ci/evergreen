package operations

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
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

		sender, err := send.NewPlainLogger("", send.LevelInfo{Default: level.Info, Threshold: level.Debug})
		if err != nil {
			return errors.Wrap(err, "problem creating a sender")
		}

		if err = grip.SetSender(sender); err != nil {
			return errors.Wrap(err, "problem setting up logger")
		}

		grip.Info(settingsYaml)

		return nil
	}
}
