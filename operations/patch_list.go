package operations

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func PatchList() cli.Command {
	const (
		numberFlagName      = "number"
		showSummaryFlagName = "show-summary"
	)

	return cli.Command{
		Name:  "list-patches",
		Usage: "show existing patches",
		Flags: mergeFlagSlices(addPatchIDFlag(
			cli.IntFlag{
				Name:  joinFlagNames(numberFlagName, "n"),
				Usage: "number of patches to show (0 for all patches)",
				Value: 5,
			},
			cli.BoolFlag{
				Name:  joinFlagNames(showSummaryFlagName, "s"),
				Usage: "show a summary of the diff for each patch",
			})),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			number := c.Int(numberFlagName)
			showSummary := c.Bool(showSummaryFlagName)
			patchID := c.String(patchIDFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			client := conf.GetRestCommunicator(ctx)
			defer client.Close()

			ac, rc, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			patches := []patch.Patch{}
			if patchID != "" {
				var res *patch.Patch
				if res, err = rc.GetPatch(patchID); err != nil {
					return err
				}
				patches = append(patches, *res)
			} else {
				patches, err = ac.GetPatches(number)
				if err != nil {
					return err
				}
			}

			for _, p := range patches {
				disp, err := getPatchDisplay(&p, showSummary, conf.UIServerHost)
				if err != nil {
					return err
				}
				fmt.Println(disp)
			}
			return nil
		},
	}
}
