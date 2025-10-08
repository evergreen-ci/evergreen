package operations

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"golang.org/x/oauth2"
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
	optOut = "Evergreen CLI will attempt to retrieve or generate an OAuth token. To opt out of this, set 'do_not_use_oauth' to true in your config file. Opting out is only available temporarily until deprecation, please see DEVPROD-4160."
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

// configurationTokenLoader stores the OAuth tokens in the ClientSettings struct.
// It writes the updated tokens back to the config file when SaveToken is called.
type configurationTokenLoader struct {
	conf *ClientSettings
}

// The string parameters are suggested config paths to handle multiple
// configurations, but they are ignored because we only need one config file.
func (c *configurationTokenLoader) LoadToken(_ string) (*oauth2.Token, error) {
	if c == nil || c.conf == nil {
		return nil, os.ErrNotExist
	}
	return &oauth2.Token{
		AccessToken:  c.conf.OAuth.AccessToken,
		RefreshToken: c.conf.OAuth.RefreshToken,
		Expiry:       c.conf.OAuth.Expiry,
		ExpiresIn:    c.conf.OAuth.Expiry.Unix(),
	}, nil
}

func (c *configurationTokenLoader) SaveToken(_ string, token *oauth2.Token) error {
	if c == nil || c.conf == nil || token == nil {
		return os.ErrNotExist
	}
	c.conf.OAuth.AccessToken = token.AccessToken
	c.conf.OAuth.RefreshToken = token.RefreshToken
	c.conf.OAuth.Expiry = token.Expiry
	return c.conf.Write("")
}

func (c *configurationTokenLoader) DeleteToken(_ string) error {
	if c == nil || c.conf == nil {
		return errors.New("no configuration to save token to")
	}
	c.conf.OAuth.AccessToken = ""
	c.conf.OAuth.RefreshToken = ""
	c.conf.OAuth.Expiry = time.Time{}
	return c.conf.Write("")
}
