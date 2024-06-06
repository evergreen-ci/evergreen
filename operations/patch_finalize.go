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
		Before: mergeBeforeFuncs(autoUpdateCLI, requirePatchIDFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			patchID := c.String(patchIDFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}

			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			existingPatch, err := ac.GetPatch(patchID)
			if err != nil {
				return errors.Wrapf(err, "getting patch '%s'", patchID)
			}
			if existingPatch == nil {
				return errors.Wrapf(err, "patch '%s' not found", patchID)
			}
			numTasksToFinalize := 0
			for _, vt := range existingPatch.VariantsTasks {
				numTasksToFinalize += len(vt.Tasks)
				for _, dt := range vt.DisplayTasks {
					numTasksToFinalize += len(dt.ExecTasks)
				}
			}
			if numTasksToFinalize > largeNumFinalizedTasksThreshold {
				if !confirm(fmt.Sprintf("This is a large patch build, expected to schedule %d tasks. Continue?", numTasksToFinalize), true) {
					return errors.New("patch aborted")
				}
			}

			if err = ac.FinalizePatch(patchID); err != nil {
				return err
			}

			fmt.Println("Patch finalized.")
			return nil
		},
	}
}
