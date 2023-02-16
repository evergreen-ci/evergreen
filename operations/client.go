package operations

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func Client() cli.Command {
	return cli.Command{
		Name:  "client",
		Usage: "convenience methods for scripts which use client settings",
		Subcommands: []cli.Command{
			getUser(),
			getAPIKey(),
			getAPIUrl(),
			getUIUrl(),
		},
	}
}

func getUser() cli.Command {
	return cli.Command{
		Name:    "get-user",
		Aliases: []string{"user"},
		Usage:   "get username from client settings",
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			fmt.Println(conf.User)
			return nil
		},
	}
}

func getAPIKey() cli.Command {
	return cli.Command{
		Name:    "get-api-key",
		Aliases: []string{"key"},
		Usage:   "get API key from client settings",
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			fmt.Println(conf.APIKey)
			return nil
		},
	}
}

func getAPIUrl() cli.Command {
	return cli.Command{
		Name:    "get-api-url",
		Aliases: []string{"api"},
		Usage:   "get API URL from client settings",
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			fmt.Println(conf.APIServerHost)
			return nil
		},
	}
}

func getUIUrl() cli.Command {
	return cli.Command{
		Name:    "get-ui-url",
		Aliases: []string{"ui"},
		Usage:   "get UI URL from client settings",
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			fmt.Println(conf.UIServerHost)
			return nil
		},
	}
}
