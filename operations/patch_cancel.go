package operations

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func PatchCancel() cli.Command {
	return cli.Command{
		Name:   "cancel-patch",
		Usage:  "cancel an existing patch",
		Flags:  addPatchIDFlag(),
		Before: requirePatchIDFlag,
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			patchID := c.String(patchIDFlagName)

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			if err = ac.CancelPatch(patchID); err != nil {
				return err
			}

			fmt.Println("Patch canceled.")
			return nil
		},
	}
}
