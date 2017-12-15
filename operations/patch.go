package operations

import (
	"context"
	"errors"

	"github.com/urfave/cli"
)

func Patch() cli.Command {
	const (
		descriptionFlagName = "description"
		finalizeFlagName    = "finalize"
		verboseFlagName     = "verbose"
	)

	return cli.Command{
		Name:    "patch",
		Aliases: []string{"create-patch", "submit-patch"},
		Usage:   "submit a new patch to evergreen",
		Flags: mergeFlagSlices(addProjectFlag(), addVariantsFlag(), addTasksFlag(), addLargeFlag(), addYesFlag(
			cli.StringFlag{
				Name:  descriptionFlagName,
				Usage: "description for the patch",
			},
			cli.BoolFlag{
				Name:  finalizeFlagName,
				Usage: "schedule tasks immediately",
			},
			cli.BoolFlag{
				Name:  verboseFlagName,
				Usage: "show patch summary",
			})),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			params := &patchParams{
				Project:     c.String(projectFlagName),
				Variants:    c.StringSlice(variantsFlagName),
				Tasks:       c.StringSlice(tasksFlagName),
				SkipConfirm: c.Bool(yesFlagName),
				Description: c.String(descriptionFlagName),
				Finalize:    c.Bool(finalizeFlagName),
				ShowSummary: c.Bool(verboseFlagName),
				Large:       c.Bool(largeFlagName),
			}

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

			ref, err := params.validatePatchCommand(conf, ac)
			if err != nil {
				return err
			}

			diffData, err := loadGitData(ref.Branch, args...)
			if err != nil {
				return err
			}

			return params.createPatch(ac, conf, diffData)
		},
	}
}
