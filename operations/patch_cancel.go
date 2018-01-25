package operations

import (
	"context"
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

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			_ = conf.GetRestCommunicator(ctx)

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			if err = ac.CancelPatch(patchID); err != nil {
				return err
			}

			fmt.Println("Patch canceled.")
			return nil
		},
	}
}
