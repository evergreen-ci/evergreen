package cli

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/pkg/errors"
)

type PublicKeyCommand struct {
	GlobalOpts *Options       `no-flag:"true"`
	List       bool           `short:"l" long:"list" description:"list the public keys for your user that evergreen knows about"`
	Delete     bool           `short:"D" long:"delete" description:"delete a public key with given name from evergreen"`
	Add        bool           `short:"a" long:"add" description:"add a public key with given name and public key file"`
	Positional positionalArgs `positional-args:"yes"`
}

type positionalArgs struct {
	KeyName string
	KeyFile string
}

func (c *PublicKeyCommand) Execute(x []string) error {
	if err := c.validateFlags(); err != nil {
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

func (c *PublicKeyCommand) validateFlags() error {
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

	return nil
}

func (c *PublicKeyCommand) listKeys(ctx context.Context, client client.Communicator) error {
	keys, err := client.GetCurrentUsersKeys(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Public keys stored in Evergreen:")
	if len(keys) == 0 {
		fmt.Println("No keys found")

	} else {
		for _, key := range keys {
			fmt.Printf("Name: '%s', Key: '%s'\n", key.Name, key.Key)
		}
	}
	return nil
}

func (c *PublicKeyCommand) listDeleteKey(ctx context.Context, client client.Communicator) error {
	if c.Positional.KeyName == "" {
		return errors.New("keys delete requires a key name")
	}

	if err := client.DeletePublicKey(ctx, c.Positional.KeyName); err != nil {
		return err
	}

	fmt.Printf("Successfully deleted key: '%s'\n", c.Positional.KeyName)

	return nil
}

func (c *PublicKeyCommand) listAddKey(ctx context.Context, client client.Communicator) error {
	if c.Positional.KeyName == "" || c.Positional.KeyFile == "" {
		return errors.New("keys add requires key name and a path to a public key file")
	}

	keyFileContents, err := ioutil.ReadFile(c.Positional.KeyFile)
	if err != nil {
		return errors.Wrap(err, "can't read public key file")
	}

	pubKey := string(keyFileContents)
	// verify that this isn't a private key
	if !strings.HasPrefix(strings.TrimSpace(pubKey), "ssh-") {
		return errors.Errorf("'%s' does not appear to be an ssh public key", c.Positional.KeyFile)
	}

	if err := client.AddPublicKey(ctx, c.Positional.KeyName, pubKey); err != nil {
		return err
	}

	fmt.Printf("Successfully added key: '%s'\n", c.Positional.KeyName)

	return nil
}
