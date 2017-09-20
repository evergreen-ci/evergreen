package cli

import (
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

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

	client, settings, err := getAPIV2Client(c.GlobalOpts)
	if err != nil {
		return err
	}

	client.SetAPIUser(settings.User)
	client.SetAPIKey(settings.APIKey)
	ctx := context.Background()

	return errors.Wrap(client.SetBannerMessage(ctx, c.Message),
		"problem setting the site-wide banner message")
}

type AdminDisableServiceCommand struct {
	GlobalOpts *Options `no-flag:"true"`
}

func (c *AdminDisableServiceCommand) Execute(args []string) error {
	client, settings, err := getAPIV2Client(c.GlobalOpts)
	if err != nil {
		return err
	}

	client.SetAPIUser(settings.User)
	client.SetAPIKey(settings.APIKey)
	ctx := context.Background()
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
	client, settings, err := getAPIV2Client(c.GlobalOpts)
	if err != nil {
		return err
	}

	client.SetAPIUser(settings.User)
	client.SetAPIKey(settings.APIKey)
	ctx := context.Background()

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
		case "dispatch", "tasks", "taskdispatch", "task_dispatch":
			flags.TaskDispatchDisabled = target
		case "hostinit":
			flags.HostinitDisabled = target
		case "monitor":
			flags.MonitorDisabled = target
		case "notify", "notifications", "notification":
			flags.NotificationsDisabled = target
		case "alerts", "alert":
			flags.AlertsDisabled = target
		case "taskrunner", "new-agents", "agents":
			flags.TaskrunnerDisabled = target
		case "github", "repotracker", "gitter", "commits":
			flags.RepotrackerDisabled = target
		case "scheduler":
			flags.SchedulerDisabled = target
		default:
			catcher.Add(errors.Errorf("%s is not a recognized service flag", f))
		}
	}

	return catcher.Resolve()
}
