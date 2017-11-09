package cli

import (
	"context"
	"io/ioutil"
	"strings"

	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type PublicKeyCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	List       bool     `short:"l" long:"list" description:"list the public keys for your user that evergreen knows about"`
	Delete     bool     `short:"D" long:"delete" description:"delete a public key with given name from evergreen"`
	Add        bool     `short:"a" long:"add" description:"add a public key with given name and public key file"`

	keyName string
	keyFile string
}

func (c *PublicKeyCommand) Execute(args []string) error {
	if err := c.validateFlags(args); err != nil {
		return err
	}
	ctx := context.Background()
	client, settings, err := getAPIV2Client(ctx, c.GlobalOpts)
	if err != nil {
		return err
	}

	client.SetAPIUser(settings.User)
	client.SetAPIKey(settings.APIKey)

	if c.List {
		return c.listKeys(ctx, client)

	} else if c.Delete {
		return c.listDeleteKey(ctx, client)

	} else {
		return c.listAddKey(ctx, client)
	}
}

func (c *PublicKeyCommand) validateFlags(args []string) error {
	opts := []bool{c.List, c.Delete, c.Add}
	numOpts := 0
	for _, b := range opts {
		if b {
			numOpts++
		}
	}

	if numOpts != 1 {
		errStr := ""
		if numOpts == 0 {
			errStr = "No flag specified"

		} else if numOpts > 1 {
			errStr = "Conflicting Options"
		}
		return errors.Errorf("%s, specify exactly one of --add [KeyName] [KeyFile], --delete [KeyName], or --list", errStr)
	}

	if c.List {
		if len(args) != 0 {
			return errors.New("Unexpected extra arguments to list")
		}

	} else if c.Delete {
		if len(args) == 0 {
			return errors.New("Too few arguments to delete, need key name")

		} else if len(args) > 1 {
			return errors.New("Unexpected extra arguments to delete")
		}

		c.keyName = args[0]

	} else if c.Add {
		if len(args) < 2 {
			return errors.New("Too few arguments to add, need key name and public key file path")

		} else if len(args) > 2 {
			return errors.New("Unexpected extra arguments to add")
		}
		c.keyName = args[0]
		c.keyFile = args[1]
	}

	return nil
}

func (c *PublicKeyCommand) listKeys(ctx context.Context, client client.Communicator) error {
	keys, err := client.GetCurrentUsersKeys(ctx)
	if err != nil {
		return err
	}

	grip.Infoln("Public keys stored in Evergreen:")
	if len(keys) == 0 {
		grip.Infoln("No keys found")

	} else {
		for _, key := range keys {
			grip.Infof("Name: '%s', Key: '%s'\n", key.Name, key.Key)
		}
	}
	return nil
}

func (c *PublicKeyCommand) listDeleteKey(ctx context.Context, client client.Communicator) error {
	if c.keyName == "" {
		return errors.New("keys delete requires a key name")
	}

	if err := client.DeletePublicKey(ctx, c.keyName); err != nil {
		return err
	}

	grip.Infof("Successfully deleted key: '%s'\n", c.keyName)

	return nil
}

func (c *PublicKeyCommand) listAddKey(ctx context.Context, client client.Communicator) error {
	if c.keyName == "" || c.keyFile == "" {
		return errors.New("keys add requires key name and a path to a public key file")
	}

	keyFileContents, err := ioutil.ReadFile(c.keyFile)
	if err != nil {
		return errors.Wrap(err, "can't read public key file")
	}

	pubKey := string(keyFileContents)
	// verify that this isn't a private key
	if !strings.HasPrefix(strings.TrimSpace(pubKey), "ssh-") {
		return errors.Errorf("'%s' does not appear to be an ssh public key", c.keyFile)
	}

	if err := client.AddPublicKey(ctx, c.keyName, pubKey); err != nil {
		return err
	}

	grip.Infof("Successfully added key: '%s'\n", c.keyName)

	return nil
}
