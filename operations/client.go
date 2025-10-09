package operations

import (
	"context"
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
			getOAuthToken(),
		},
	}
}

const (
	optOut = `********************************************************************************

Evergreen is in the process of migrating to OAuth for authentication. Your browser should open to complete authentication.

If you do not wish to have your browser opened automatically, you can "oauth.do_not_use_browser: true" inside your client configuration file, often located at ~/.evergreen.yml.

If you would like to temporarily opt out of using OAuth, you can set "do_not_run_kanopy_oidc: true" in your client configuration file. Opting out is only available temporarily until deprecation, please see DEVPROD-4160.

********************************************************************************`
)

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

func getOAuthToken() cli.Command {
	return cli.Command{
		Name:  "get-oauth-token",
		Usage: "gets a valid OAuth token to authenticate with Evergreen's REST API",
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

			fmt.Println(conf.OAuth.AccessToken)

			return nil
		},
	}
}
