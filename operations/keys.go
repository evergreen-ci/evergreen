package operations

import (
	"context"
	"io/ioutil"
	"strings"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
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
			requireClientConfig,
			requireFileExists(keyFileFlagName),
			func(c *cli.Context) error {
				numArgs := c.NArg()

				if numArgs < 2 {
					return errors.New("Too few arguments to add, need key name and public key file path")
				} else if numArgs > 2 {
					return errors.New("too many arguments to add a key")
				}
				args := c.Args()

				c.Set(keyNameFlagName, args[0])
				c.Set(keyFileFlagName, args[1])

				return nil
			}),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			keyName := c.String(keyNameFlagName)
			keyFile := c.String(keyFileFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			client := conf.GetRestCommunicator(ctx)
			defer client.Close()

			keyFileContents, err := ioutil.ReadFile(keyFile)
			if err != nil {
				return errors.Wrap(err, "can't read public key file")
			}

			pubKey := string(keyFileContents)
			// verify that this isn't a private key
			if !strings.HasPrefix(strings.TrimSpace(pubKey), "ssh-") {
				return errors.Errorf("'%s' does not appear to be an ssh public key", keyFile)
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
				return nil
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
