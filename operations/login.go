package operations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func Login() cli.Command {
	return cli.Command{
		Name:  "login",
		Usage: "authenticate the CLI with evergreen",
		Action: func(c *cli.Context) error {
			if _, err := login(c); err != nil {
				return errors.Wrap(err, "logging in")
			}

			return nil
		},
	}
}

// login has a user to authenticate using oauth and saves the token.
func login(c *cli.Context) (*ClientSettings, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	confPath := c.Parent().String(ConfFlagName)
	conf, err := NewClientSettings(confPath)
	if err != nil {
		return nil, errors.Wrap(err, "loading configuration")
	}

	if err = conf.SetOAuthToken(ctx); err != nil {
		return nil, errors.Wrap(err, "setting OAuth token")
	}

	return conf, nil
}
