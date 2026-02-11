package operations

import (
	"context"
	"os"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func Keys() cli.Command {
	return cli.Command{
		Name:    "keys",
		Aliases: []string{"key", "pubkey"},
		Usage:   "manage your public keys with the Evergreen service",
		Subcommands: []cli.Command{
			keysAdd(),
			keysList(),
			keysDelete(),
		},
	}
}

func keysAdd() cli.Command {
	const (
		keyNameFlagName = "name"
		keyFileFlagName = "file"
	)

	return cli.Command{
		Name:  "add",
		Usage: "add a public key",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  keyNameFlagName,
				Usage: "specify the name of a key to add",
			},
			cli.StringFlag{
				Name:  keyFileFlagName,
				Usage: "specify the path of a file, which must exist",
			},
		},
		Before: mergeBeforeFuncs(
			setPlainLogger,
			requireFileExists(keyFileFlagName),
			func(c *cli.Context) error {
				keyName := c.String(keyNameFlagName)
				if keyName == "" {
					return errors.New("key name cannot be empty")
				}
				return nil
			}),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(ConfFlagName)
			keyName := c.String(keyNameFlagName)
			keyFile := c.String(keyFileFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}

			client, err := conf.SetupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			keyFileContents, err := os.ReadFile(keyFile)
			if err != nil {
				return errors.Wrapf(err, "reading public key file '%s'", keyFile)
			}

			pubKey := string(keyFileContents)
			// verify that this isn't a private key
			if err := evergreen.ValidateSSHKey(pubKey); err != nil {
				return errors.Errorf("'%s' does not appear to be a valid ssh public key", keyFile)
			}

			if err := client.AddPublicKey(ctx, keyName, pubKey); err != nil {
				return err
			}

			return nil
		},
	}
}

func keysList() cli.Command {
	return cli.Command{
		Name:   "list",
		Usage:  "list all public keys for the current user",
		Before: setPlainLogger,
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(ConfFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}

			client, err := conf.SetupRestCommunicator(ctx, false)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			keys, err := client.GetCurrentUsersKeys(ctx)
			if err != nil {
				return errors.Wrap(err, "fetching keys")
			}

			if len(keys) == 0 {
				grip.Info("No keys found")
			} else {
				grip.Info("Public keys stored in Evergreen:")
				for _, key := range keys {
					grip.Infof("Name: '%s', Key: '%s'\n", utility.FromStringPtr(key.Name), utility.FromStringPtr(key.Key))
				}
			}

			return nil
		},
	}
}
func keysDelete() cli.Command {
	return cli.Command{
		Name:  "delete",
		Usage: "delete a public key",
		Before: mergeBeforeFuncs(
			setPlainLogger,
			func(c *cli.Context) error {
				if c.NArg() != 1 {
					return errors.New("must specify only one key to delete at a time")
				}

				if c.Args().Get(0) == "" {
					return errors.New("keys delete requires a key name")
				}
				return nil
			}),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(ConfFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}

			client, err := conf.SetupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			keyName := c.Args().Get(0)

			if err := client.DeletePublicKey(ctx, keyName); err != nil {
				return errors.Wrap(err, "deleting public key")
			}

			grip.Infof("Successfully deleted key: '%s'\n", keyName)

			return nil
		},
	}
}
