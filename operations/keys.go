package operations

import (
	"context"
	"errors"

	"github.com/mongodb/grip"
	"github.com/urfave/cli"
)

func Keys() cli.Command {
	return cli.Command{
		Name:    "keys",
		Aliases: []string{"key", "pubkey"},
		Usage:   "manage your public keys with the evergreen service",
		Subcommands: []cli.Command{
			keysAdd(),
			keysList(),
			keysDelete(),
		},
	}
}

func keysAdd() cli.Command {
	return cli.Command{
		Name:   "add",
		Usage:  "add a public key",
		Action: func(c *cli.Context) error {},
	}
}

func keysList() cli.Command {
	return cli.Command{
		Name:   "list",
		Usage:  "list all public keys for the current user",
		Before: mergeBeforeFuncs(setPlainLogger, requireClientConfig),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			client := conf.GetRestCommunicator(ctx)
			defer client.Close()

			keys, err := client.GetCurrentUsersKeys(ctx)
			if err != nil {
				return errors.Wrap(err, "problem fetching keys")
			}

			if len(keys) == 0 {
				grip.Info("No keys found")
			} else {
				grip.Info("Public keys stored in Evergreen:")
				for _, key := range keys {
					grip.Infof("Name: '%s', Key: '%s'\n", key.Name, key.Key)
				}
			}

			return nil
		},
	}
}
func keysDelete() cli.Command {
	return cli.Command{
		Name: "delete",
		Before: mergeBeforeFuncs(
			requireClientConfig,
			setPlainLogger,
			func(c *cli.Context) error {
				if c.NArg() != 1 {
					return errors.New("must specify only one key to delete at a time")
				}

				if c.Args().Get(0) == "" {
					return errors.New("keys delete requires a key name")
				}
			}),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			client := conf.GetRestCommunicator(ctx)
			defer client.Close()

			keyName := c.Args().Get(0)

			if err := client.DeletePublicKey(ctx, keyName); err != nil {
				return errors.Wrap(err, "problem deleting public key")
			}

			grip.Infof("Successfully deleted key: '%s'\n", keyName)

			return nil
		},
	}
}
