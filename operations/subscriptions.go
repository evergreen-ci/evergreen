package operations

import (
	"context"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func Subscriptions() cli.Command {
	return cli.Command{
		Name:   "subscriptions",
		Usage:  "for managing subscriptions in Evergreen",
		Before: setPlainLogger,
		Subcommands: []cli.Command{
			subscriptionsList(),
		},
	}
}

func subscriptionsList() cli.Command {
	return cli.Command{
		Name:  "list",
		Usage: "list subscriptions belonging to a user",
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			confPath := c.Parent().Parent().String(ConfFlagName)
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}

			comm, err := conf.setupRestCommunicator(ctx, false)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer comm.Close()
			subs, err := comm.GetSubscriptions(ctx)
			if err != nil {
				return errors.Wrap(err, "fetching subscriptions")
			}

			if len(subs) == 0 {
				grip.Info("no subscriptions found")
			}

			for i := range subs {
				grip.Info(subs[i].String())
			}

			return nil
		},
	}
}
