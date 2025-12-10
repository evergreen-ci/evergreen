package operations

import (
	"context"

	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func Notification() cli.Command {
	return cli.Command{
		Name:  "notify",
		Usage: "send notifications",
		Subcommands: []cli.Command{
			notificationSlack(),
			notificationEmail(),
		},
	}
}

func notificationSlack() cli.Command {
	const (
		targetFlagName = "target"
		msgFlagName    = "msg"
	)

	return cli.Command{
		Name:  "slack",
		Usage: "send a Slack message",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  joinFlagNames(targetFlagName, "t"),
				Usage: "target of the message",
			},
			cli.StringFlag{
				Name:  joinFlagNames(msgFlagName, "m"),
				Usage: "message to send",
			},
		},
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(ConfFlagName)
			target := c.String(targetFlagName)
			msg := c.String(msgFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			apiSlack := model.APISlack{
				Target: utility.ToStringPtr(target),
				Msg:    utility.ToStringPtr(msg),
			}

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			if err := client.SendNotification(ctx, "slack", apiSlack); err != nil {
				return errors.Wrap(err, "sending Slack notification")
			}

			return nil
		},
	}
}

func notificationEmail() cli.Command {
	const (
		bodyFlagName       = "body"
		fromFlagName       = "from"
		recipientsFlagName = "recipients"
		subjectFlagName    = "subject"
	)
	return cli.Command{
		Name:  "email",
		Usage: "send an email message",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  joinFlagNames(fromFlagName, "f"),
				Usage: "message sender. DEPRECATED: all emails are sent from the configured notifications address",
			},
			cli.StringSliceFlag{
				Name:  joinFlagNames(recipientsFlagName, "r"),
				Usage: "recipient(s) of the message",
			},
			cli.StringFlag{
				Name:  joinFlagNames(subjectFlagName, "s"),
				Usage: "subject of the message",
				Value: "",
			},
			cli.StringFlag{
				Name:  joinFlagNames(bodyFlagName, "b"),
				Usage: "body of message",
				Value: "",
			},
		},
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(ConfFlagName)
			recipients := c.StringSlice(recipientsFlagName)
			body := c.String(bodyFlagName)
			subject := c.String(subjectFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			apiEmail := model.APIEmail{
				Subject:    utility.ToStringPtr(subject),
				Recipients: recipients,
				Body:       utility.ToStringPtr(body),
			}

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			if err := client.SendNotification(ctx, "email", apiEmail); err != nil {
				return errors.Wrap(err, "sending email notification")
			}

			return nil
		},
	}
}
