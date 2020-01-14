package operations

import (
	"context"
	"io/ioutil"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	patchDescriptionFlagName = "description"
	patchFinalizeFlagName    = "finalize"
	patchVerboseFlagName     = "verbose"
	patchAliasFlagName       = "alias"
	patchBrowseFlagName      = "browse"
)

func getPatchFlags(flags ...cli.Flag) []cli.Flag {
	return mergeFlagSlices(addProjectFlag(flags...), addVariantsFlag(), addTasksFlag(), addLargeFlag(), addYesFlag(), addRefFlag(), addUncommittedChangesFlag(
		cli.StringFlag{
			Name:  joinFlagNames(patchDescriptionFlagName, "d"),
			Usage: "description for the patch",
		},
		cli.StringFlag{
			Name:  joinFlagNames(patchAliasFlagName, "a"),
			Usage: "patch alias (set by project admin)",
		},
		cli.BoolFlag{
			Name:  joinFlagNames(patchFinalizeFlagName, "f"),
			Usage: "schedule tasks immediately",
		},
		cli.BoolFlag{
			Name:  joinFlagNames(patchBrowseFlagName),
			Usage: "open patch url in browser",
		},
		cli.BoolFlag{
			Name:  patchVerboseFlagName,
			Usage: "show patch summary",
		}))
}

func Patch() cli.Command {
	return cli.Command{
		Name:    "patch",
		Before:  setPlainLogger,
		Aliases: []string{"create-patch", "submit-patch"},
		Usage:   "submit a new patch to evergreen",
		Flags:   getPatchFlags(),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			args := c.Args()
			params := &patchParams{
				Alias:       c.String(patchAliasFlagName),
				Browse:      c.Bool(patchBrowseFlagName),
				Description: c.String(patchDescriptionFlagName),
				Finalize:    c.Bool(patchFinalizeFlagName),
				Large:       c.Bool(largeFlagName),
				Project:     c.String(projectFlagName),
				Ref:         c.String(refFlagName),
				ShowSummary: c.Bool(patchVerboseFlagName),
				SkipConfirm: c.Bool(yesFlagName),
				Tasks:       util.SplitCommas(c.StringSlice(tasksFlagName)),
				Uncommitted: c.Bool(uncommittedChangesFlag),
				Variants:    util.SplitCommas(c.StringSlice(variantsFlagName)),
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			uncommittedChanges, err := gitUncommittedChanges()
			if err != nil {
				return errors.Wrap(err, "can't test for uncommitted changes")
			}

			if (!params.Uncommitted && !conf.UncommittedChanges) && uncommittedChanges {
				grip.Infof("Uncommitted changes are omitted from patches by default.\nUse the '--%s, -u' flag or set 'patch_uncommitted_changes: true' in your ~/.evergreen.yml file to include uncommitted changes.", uncommittedChangesFlag)
			}

			comm := conf.setupRestCommunicator(ctx)
			defer comm.Close()

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			ref, err := params.validatePatchCommand(ctx, conf, ac, comm)
			if err != nil {
				return err
			}

			diffData, err := loadGitData(ref.Branch, params.Ref, args...)
			if err != nil {
				return err
			}

			_, err = params.createPatch(ac, conf, diffData)
			return err
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
				Name:  diffPathFlagName,
				Usage: "path to a file for diff of the patch",
			},
		),
		Before: mergeBeforeFuncs(requireFileExists(diffPathFlagName)),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			params := &patchParams{
				Alias:       c.String(patchAliasFlagName),
				Description: c.String(patchDescriptionFlagName),
				Finalize:    c.Bool(patchFinalizeFlagName),
				Large:       c.Bool(largeFlagName),
				Project:     c.String(projectFlagName),
				Ref:         c.String(refFlagName),
				ShowSummary: c.Bool(patchVerboseFlagName),
				SkipConfirm: c.Bool(yesFlagName),
				Tasks:       util.SplitCommas(c.StringSlice(tasksFlagName)),
				Variants:    util.SplitCommas(c.StringSlice(variantsFlagName)),
			}
			diffPath := c.String(diffPathFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			comm := conf.setupRestCommunicator(ctx)
			defer comm.Close()

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			if ref, err = params.validatePatchCommand(ctx, conf, ac, comm); err != nil {
				return err
			}
			var featureBranch string
			if ref == "" {
				featureBranch = "HEAD"
			} else {
				featureBranch = ref
			}
			mergeBase, err := gitMergeBase(ref.branch+"@{upstream}", featureBranch)

			fullPatch, err := ioutil.ReadFile(diffPath)
			if err != nil {
				return errors.Wrap(err, "problem reading diff file")
			}

			diffData := &localDiff{string(fullPatch), "", "", base}

			_, err = params.createPatch(ac, conf, diffData)
			return err
		},
	}
}
