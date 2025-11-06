package operations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func PromptAuthentication() cli.Command {
	return cli.Command{
		Name:  "auth",
		Usage: "forces the authentication flow to obtain a new OAuth token",
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			confPath := c.Parent().String(confFlagName)
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			comm, err := conf.setupRestCommunicator(ctx, false)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer comm.Close()

			if err = conf.SetOAuthToken(ctx, comm); err != nil {
				return errors.Wrap(err, "setting OAuth token")
			}

			return nil
		},
	}
}
