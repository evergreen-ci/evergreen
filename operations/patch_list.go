package operations

import (
	"context"
	"errors"
	"fmt"

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
		Flags: addPatchIDFlag(addVariantsFlag(
			cli.IntFlag{
				Name:    numberFlagName,
				Aliases: []string{"n"},
				Usage:   "number of patches to show (0 for all patches)",
				Value:   5,
			},
			cli.BoolFlag{
				Name:    showSummaryFlagName,
				Usage:   "show a summary of the diff for each patch",
				Aliases: []string{"s"},
			})),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			number := c.Int(numberFlagName)
			showSummary := c.Bool(showSummaryFlagName)

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

			notifyUserUpdate(ac)

			patches, err := ac.GetPatches(number)
			if err != nil {
				return err
			}
			for _, p := range patches {
				disp, err := getPatchDisplay(&p, showSummary, conf.UIServerHost)
				if err != nil {
					return err
				}
				fmt.Println(disp)
			}

		},
	}
}
