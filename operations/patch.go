package operations

import (
	"context"
	"io/ioutil"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	patchDescriptionFlagName = "description"
	patchFinalizeFlagName    = "finalize"
	patchVerboseFlagName     = "verbose"
)

func getPatchFlags(flags ...cli.Flag) []cli.Flag {
	return mergeFlagSlices(addProjectFlag(flags...), addVariantsFlag(), addTasksFlag(), addLargeFlag(), addYesFlag(
		cli.StringFlag{
			Name:  patchDescriptionFlagName,
			Usage: "description for the patch",
		},
		cli.BoolFlag{
			Name:  patchFinalizeFlagName,
			Usage: "schedule tasks immediately",
		},
		cli.BoolFlag{
			Name:  patchVerboseFlagName,
			Usage: "show patch summary",
		}))
}

func Patch() cli.Command {
	return cli.Command{
		Name:    "patch",
		Aliases: []string{"create-patch", "submit-patch"},
		Usage:   "submit a new patch to evergreen",
		Flags:   getPatchFlags(),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			args := c.Args()
			params := &patchParams{
				Project:     c.String(projectFlagName),
				Variants:    c.StringSlice(variantsFlagName),
				Tasks:       c.StringSlice(tasksFlagName),
				SkipConfirm: c.Bool(yesFlagName),
				Description: c.String(patchDescriptionFlagName),
				Finalize:    c.Bool(patchFinalizeFlagName),
				ShowSummary: c.Bool(patchVerboseFlagName),
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

func PatchFile() cli.Command {
	const (
		baseFlagName     = "base"
		diffPathFlagName = "diff-file"
	)

	return cli.Command{
		Name:  "patch-file",
		Usage: "submit patch using a diff file",
		Flags: getPatchFlags(
			cli.StringFlag{
				Name:  joinFlagNames("base", "b"),
				Usage: "githash of base",
			},
			cli.StringFlag{
				Name:  diffPathFlagName,
				Usage: "path to a file for diff of the patch",
			},
		),
		Before: mergeBeforeFuncs(requireFileExists(diffPathFlagName)),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			params := &patchParams{
				Project:     c.String(projectFlagName),
				Variants:    c.StringSlice(variantsFlagName),
				Tasks:       c.StringSlice(tasksFlagName),
				SkipConfirm: c.Bool(yesFlagName),
				Description: c.String(patchDescriptionFlagName),
				Finalize:    c.Bool(patchFinalizeFlagName),
				ShowSummary: c.Bool(patchVerboseFlagName),
				Large:       c.Bool(largeFlagName),
			}
			diffPath := c.String(diffPathFlagName)
			base := c.String(baseFlagName)

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

			if _, err = params.validatePatchCommand(conf, ac); err != nil {
				return err
			}

			fullPatch, err := ioutil.ReadFile(diffPath)
			if err != nil {
				return errors.Wrap(err, "problem reading diff file")
			}

			diffData := &localDiff{string(fullPatch), "", "", base}

			return params.createPatch(ac, conf, diffData)
		},
	}
}
