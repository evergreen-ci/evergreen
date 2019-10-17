package operations

import (
	"context"
	"strings"

	"github.com/evergreen-ci/evergreen/rest/model"
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
		Usage: "send a slack message",
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
			confPath := c.Parent().Parent().String(confFlagName)
			target := c.String(targetFlagName)
			msg := c.String(msgFlagName)

			if err := validateTargetHasOctothorpeOrArobase(target); err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			apiSlack := model.APISlack{
				Target: model.ToAPIString(target),
				Msg:    model.ToAPIString(msg),
			}

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			if err := client.SendNotification(ctx, "slack", apiSlack); err != nil {
				return errors.Wrap(err, "problem contacting evergreen service")
			}

			return nil
		},
	}
}

func validateTargetHasOctothorpeOrArobase(target string) error {
	if !strings.HasPrefix(target, "#") && !strings.HasPrefix(target, "@") {
		return errors.New("target must begin with '#' or '@'")
	}
	return nil
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
				Usage: "message sender",
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
			confPath := c.Parent().Parent().String(confFlagName)
			from := c.String(fromFlagName)
			recipients := c.StringSlice(recipientsFlagName)
			body := c.String(bodyFlagName)
			subject := c.String(subjectFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			apiEmail := model.APIEmail{
				From:       model.ToAPIString(from),
				Subject:    model.ToAPIString(subject),
				Recipients: recipients,
				Body:       model.ToAPIString(body),
			}

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			if err := client.SendNotification(ctx, "email", apiEmail); err != nil {
				return errors.Wrap(err, "problem contacting evergreen service")
			}

			return nil
		},
	}
}
