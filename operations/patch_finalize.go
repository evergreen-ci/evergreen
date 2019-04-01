package operations

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func PatchFinalize() cli.Command {
	return cli.Command{
		Name:   "finalize-patch",
		Usage:  "finalize an existing patch",
		Flags:  addPatchIDFlag(),
		Before: requirePatchIDFlag,
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			patchID := c.String(patchIDFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			client := conf.GetRestCommunicator(ctx)
			defer client.Close()

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			if err = ac.FinalizePatch(patchID); err != nil {
				return err
			}

			fmt.Println("Patch finalized.")
			return nil
		},
	}
}
