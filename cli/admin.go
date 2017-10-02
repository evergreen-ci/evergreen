package cli

import (
	"time"

	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type AdminRestartTasks struct {
	GlobalOpts *Options `no-flag:"true"`
	StartTime  string   `long:"startAt" short:"s" description:"RFC3339 formated date of the start time for the period of failed tasks to restart"`
	EndTime    string   `long:"startAt" short:"s" description:"RFC3339 formated date of the end time for the period of tasks to restart. Defaults to current time."`
	Period     int      `long:"period" short:"p" description:"number of minutes. Specify either period or start time."`
}

func (c *AdminRestartTasks) Execute(_ []string) error {
	if c.StartTime != "" && c.Period == 0 {
		return errors.New("you must specify either a period or a start time")
	}

	var (
		err     error
		startAt time.Time
		endAt   time.Time
	)

	if c.EndTime == "" {
		endAt = time.Now()
	} else {
		endAt, err = time.Parse(time.RFC3339, c.EndTime)
		if err != nil {
			return errors.Wrap(err, "problem formatting the startAt Time")
		}
	}

	if c.StartTime == "" {
		startAt = endAt.Add(-time.Duration(c.Period) * time.Minute)
	} else {
		startAt, err = time.Parse(time.RFC3339, c.StartTime)
		if err != nil {
			return errors.Wrap(err, "problem formatting the startAt Time")
		}
	}

	ctx := context.Background()

	client, settings, err := getAPIV2Client(ctx, c.GlobalOpts)
	if err != nil {
		return errors.Wrap(err, "problem configuring api client")
	}

	client.SetAPIUser(settings.User)
	client.SetAPIKey(settings.APIKey)

	if err = client.RestartRecentTasks(ctx, startAt, endAt); err != nil {
		return errors.Wrapf(err, "problem restarting tasks for %s period starting at",
			startAt.Sub(endAt), startAt)
	}

	grip.Infof("restarted failed tasks for %s period starting at %s",
		startAt.Sub(endAt), startAt)

	return nil
}

type AdminBannerCommand struct {
	GlobalOpts            *Options `no-flag:"true"`
	Message               string   `long:"message" short:"m" description:"content of new message"`
	Clear                 bool     `long:"clear" description:"cleat the banner"`
	disableNetworkForTest bool
}

func (c *AdminBannerCommand) Execute(_ []string) error {
	if c.Message != "" && c.Clear {
		return errors.New("cannot specify a message and the 'clear' option at the same time")
	}

	if c.Message == "" && !c.Clear {
		return errors.New("cannot set the message to the empty string. Use --clear to unset the message")
	}

	if c.disableNetworkForTest {
		return nil
	}

	ctx := context.Background()
	client, settings, err := getAPIV2Client(ctx, c.GlobalOpts)
	if err != nil {
		return err
	}

	client.SetAPIUser(settings.User)
	client.SetAPIKey(settings.APIKey)

	return errors.Wrap(client.SetBannerMessage(ctx, c.Message),
		"problem setting the site-wide banner message")
}

type AdminDisableServiceCommand struct {
	GlobalOpts *Options `no-flag:"true"`
}

func (c *AdminDisableServiceCommand) Execute(args []string) error {
	ctx := context.Background()
	client, settings, err := getAPIV2Client(ctx, c.GlobalOpts)
	if err != nil {
		return err
	}

	client.SetAPIUser(settings.User)
	client.SetAPIKey(settings.APIKey)
	flags, err := client.GetServiceFlags(ctx)
	if err != nil {
		return errors.Wrap(err, "problem getting current service flag state")
	}

	if err := setServiceFlagValues(args, true, flags); err != nil {
		return errors.Wrap(err, "invalid service flags")
	}

	return errors.Wrap(client.SetServiceFlags(ctx, flags),
		"problem disabling services")
}

type AdminEnableServiceCommand struct {
	GlobalOpts *Options `no-flag:"true"`
}

func (c *AdminEnableServiceCommand) Execute(args []string) error {
	ctx := context.Background()
	client, settings, err := getAPIV2Client(ctx, c.GlobalOpts)
	if err != nil {
		return err
	}

	client.SetAPIUser(settings.User)
	client.SetAPIKey(settings.APIKey)

	flags, err := client.GetServiceFlags(ctx)
	if err != nil {
		return errors.Wrap(err, "problem getting current service flag state")
	}

	if err := setServiceFlagValues(args, false, flags); err != nil {
		return errors.Wrap(err, "invalid service flags")
	}

	return errors.Wrap(client.SetServiceFlags(ctx, flags),
		"problem enabling services")

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
