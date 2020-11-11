package operations

import (
	"context"
	"io/ioutil"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	patchDescriptionFlagName = "description"
	patchVerboseFlagName     = "verbose"
)

func getPatchFlags(flags ...cli.Flag) []cli.Flag {
	return mergeFlagSlices(
		addProjectFlag(flags...),
		addPatchFinalizeFlag(),
		addVariantsFlag(),
		addParameterFlag(),
		addPatchBrowseFlag(),
		addSyncBuildVariantsFlag(),
		addSyncTasksFlag(),
		addSyncStatusesFlag(),
		addSyncTimeoutFlag(),
		addLargeFlag(),
		addYesFlag(),
		addRefFlag(),
		addUncommittedChangesFlag(),
		addPreserveCommitsFlag(
			cli.StringSliceFlag{
				Name:  joinFlagNames(tasksFlagName, "t"),
				Usage: "task names",
			},
			cli.StringFlag{
				Name:  joinFlagNames(patchAliasFlagName, "a"),
				Usage: "patch alias (set by project admin)",
			},
			cli.StringFlag{
				Name:  joinFlagNames(patchDescriptionFlagName, "d"),
				Usage: "description for the patch",
			},
			cli.BoolFlag{
				Name:  patchVerboseFlagName,
				Usage: "show patch summary",
			},
		))
}

func Patch() cli.Command {
	return cli.Command{
		Name: "patch",
		Before: mergeBeforeFuncs(
			setPlainLogger,
			mutuallyExclusiveArgs(false, preserveCommitsFlag, uncommittedChangesFlag),
			func(c *cli.Context) error {
				catcher := grip.NewBasicCatcher()
				for _, status := range utility.SplitCommas(c.StringSlice(syncStatusesFlagName)) {
					if !utility.StringSliceContains(evergreen.SyncStatuses, status) {
						catcher.Errorf("invalid sync status '%s'", status)
					}
				}
				return catcher.Resolve()
			},
		),
		Aliases: []string{"create-patch", "submit-patch"},
		Usage:   "submit a new patch to evergreen",
		Flags:   getPatchFlags(),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			args := c.Args()
			params := &patchParams{
				Project:           c.String(projectFlagName),
				Variants:          utility.SplitCommas(c.StringSlice(variantsFlagName)),
				Tasks:             utility.SplitCommas(c.StringSlice(tasksFlagName)),
				SyncBuildVariants: utility.SplitCommas(c.StringSlice(syncBuildVariantsFlagName)),
				SyncTasks:         utility.SplitCommas(c.StringSlice(syncTasksFlagName)),
				SyncStatuses:      utility.SplitCommas(c.StringSlice(syncStatusesFlagName)),
				SyncTimeout:       c.Duration(syncTimeoutFlagName),
				SkipConfirm:       c.Bool(yesFlagName),
				Description:       c.String(patchDescriptionFlagName),
				Finalize:          c.Bool(patchFinalizeFlagName),
				Browse:            c.Bool(patchBrowseFlagName),
				ShowSummary:       c.Bool(patchVerboseFlagName),
				Large:             c.Bool(largeFlagName),
				Alias:             c.String(patchAliasFlagName),
				Ref:               c.String(refFlagName),
				Uncommitted:       c.Bool(uncommittedChangesFlag),
				PreserveCommits:   c.Bool(preserveCommitsFlag),
			}

			var err error
			paramsPairs := c.StringSlice(parameterFlagName)
			params.Parameters, err = getParametersFromInput(paramsPairs)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			params.PreserveCommits = params.PreserveCommits || conf.PreserveCommits
			if !params.SkipConfirm {
				var keepGoing bool
				keepGoing, err = confirmUncommittedChanges(params.PreserveCommits, params.Uncommitted || conf.UncommittedChanges)
				if err != nil {
					return errors.Wrap(err, "can't test for uncommitted changes")
				}
				if !keepGoing {
					return errors.New("patch aborted")
				}
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
			params.Description = params.getDescription()

			diffData, err := loadGitData(ref.Branch, params.Ref, "", params.PreserveCommits, args...)
			if err != nil {
				return err
			}
			if !params.PreserveCommits {
				diffData.fullPatch, err = diffToMbox(diffData, params.Description)
				if err != nil {
					return err
				}
			}

			if err = params.validateSubmission(diffData); err != nil {
				return err
			}
			newPatch, err := params.createPatch(ac, diffData)
			if err != nil {
				return err
			}
			return params.displayPatch(conf, newPatch)
		},
	}
}

func getParametersFromInput(params []string) ([]patch.Parameter, error) {
	res := []patch.Parameter{}
	catcher := grip.NewBasicCatcher()
	for _, param := range params {
		pair := strings.Split(param, "=")
		if len(pair) != 2 {
			catcher.Add(errors.Errorf("problem parsing parameter '%s'", param))
			continue
		}

		res = append(res, patch.Parameter{Key: pair[0], Value: pair[1]})
	}
	return res, catcher.Resolve()
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
				Variants:    utility.SplitCommas(c.StringSlice(variantsFlagName)),
				Tasks:       utility.SplitCommas(c.StringSlice(tasksFlagName)),
				Alias:       c.String(patchAliasFlagName),
				SkipConfirm: c.Bool(yesFlagName),
				Description: c.String(patchDescriptionFlagName),
				Finalize:    c.Bool(patchFinalizeFlagName),
				ShowSummary: c.Bool(patchVerboseFlagName),
				Large:       c.Bool(largeFlagName),
				SyncTasks:   utility.SplitCommas(c.StringSlice(syncTasksFlagName)),
			}
			diffPath := c.String(diffPathFlagName)
			base := c.String(baseFlagName)

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

			if _, err = params.validatePatchCommand(ctx, conf, ac, comm); err != nil {
				return err
			}
			params.Description = params.getDescription()

			fullPatch, err := ioutil.ReadFile(diffPath)
			if err != nil {
				return errors.Wrap(err, "problem reading diff file")
			}

			diffData := &localDiff{string(fullPatch), "", "", base}

			if err = params.validateSubmission(diffData); err != nil {
				return err
			}
			newPatch, err := params.createPatch(ac, diffData)
			if err != nil {
				return err
			}
			return params.displayPatch(conf, newPatch)
		},
	}
}
