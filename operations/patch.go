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
	patchDescriptionFlagName   = "description"
	patchVerboseFlagName       = "verbose"
	patchTriggerAliasFlag      = "trigger-alias"
	repeatDefinitionFlag       = "repeat"
	repeatFailedDefinitionFlag = "repeat-failed"
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
		addSkipConfirmFlag(),
		addRefFlag(),
		addUncommittedChangesFlag(),
		addPreserveCommitsFlag(
			cli.StringSliceFlag{
				Name:  joinFlagNames(tasksFlagName, "t"),
				Usage: "task names (\"all\" for all tasks)",
			},
			cli.StringFlag{
				Name:  joinFlagNames(patchAliasFlagName, "a"),
				Usage: "patch alias (set by project admin) or local alias (set individually in evergreen.yml)",
			},
			cli.StringFlag{
				Name:  joinFlagNames(patchDescriptionFlagName, "d"),
				Usage: "description for the patch",
			},
			cli.BoolFlag{
				Name:  patchVerboseFlagName,
				Usage: "show patch summary",
			},
			cli.StringSliceFlag{
				Name:  patchTriggerAliasFlag,
				Usage: "patch trigger alias (set by project admin) specifying tasks from other projects",
			},
			cli.BoolFlag{
				Name:  joinFlagNames(repeatDefinitionFlag, "reuse"),
				Usage: "use all of the same tasks/variants defined for the last patch you scheduled for this project",
			},
			cli.BoolFlag{
				Name:  joinFlagNames(repeatFailedDefinitionFlag, "rf"),
				Usage: "use only the failed tasks/variants defined for the last patch you scheduled for this project",
			},
			cli.StringFlag{
				Name:  pathFlagName,
				Usage: "path to an Evergreen project configuration file",
			},
			cli.StringSliceFlag{
				Name:  joinFlagNames(regexVariantsFlagName, "rv"),
				Usage: "regex variant names",
			},
			cli.StringSliceFlag{
				Name:  joinFlagNames(regexTasksFlagName, "rt"),
				Usage: "regex task names",
			},
		))
}

func Patch() cli.Command {
	return cli.Command{
		Name: "patch",
		Before: mergeBeforeFuncs(
			autoUpdateCLI,
			setPlainLogger,
			mutuallyExclusiveArgs(false, preserveCommitsFlag, uncommittedChangesFlag),
			mutuallyExclusiveArgs(false, repeatDefinitionFlag, repeatFailedDefinitionFlag),
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
		Usage:   "submit a new patch to Evergreen",
		Flags:   getPatchFlags(),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			args := c.Args()
			params := &patchParams{
				Project:           c.String(projectFlagName),
				Path:              c.String(pathFlagName),
				Variants:          utility.SplitCommas(c.StringSlice(variantsFlagName)),
				Tasks:             utility.SplitCommas(c.StringSlice(tasksFlagName)),
				RegexVariants:     utility.SplitCommas(c.StringSlice(regexVariantsFlagName)),
				RegexTasks:        utility.SplitCommas(c.StringSlice(regexTasksFlagName)),
				SyncBuildVariants: utility.SplitCommas(c.StringSlice(syncBuildVariantsFlagName)),
				SyncTasks:         utility.SplitCommas(c.StringSlice(syncTasksFlagName)),
				SyncStatuses:      utility.SplitCommas(c.StringSlice(syncStatusesFlagName)),
				SyncTimeout:       c.Duration(syncTimeoutFlagName),
				SkipConfirm:       c.Bool(skipConfirmFlagName),
				Description:       c.String(patchDescriptionFlagName),
				Finalize:          c.Bool(patchFinalizeFlagName),
				Browse:            c.Bool(patchBrowseFlagName),
				ShowSummary:       c.Bool(patchVerboseFlagName),
				Large:             c.Bool(largeFlagName),
				Alias:             c.String(patchAliasFlagName),
				Ref:               c.String(refFlagName),
				Uncommitted:       c.Bool(uncommittedChangesFlag),
				PreserveCommits:   c.Bool(preserveCommitsFlag),
				TriggerAliases:    utility.SplitCommas(c.StringSlice(patchTriggerAliasFlag)),
				RepeatDefinition:  c.Bool(repeatDefinitionFlag),
				RepeatFailed:      c.Bool(repeatFailedDefinitionFlag),
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
				return errors.Wrap(err, "loading configuration")
			}

			params.PreserveCommits = params.PreserveCommits || conf.PreserveCommits
			if !params.SkipConfirm {
				var keepGoing bool
				keepGoing, err = confirmUncommittedChanges(params.PreserveCommits, params.Uncommitted || conf.UncommittedChanges)
				if err != nil {
					return errors.Wrap(err, "confirming uncommitted changes")
				}
				if keepGoing && utility.StringSliceContains(params.Variants, "all") && utility.StringSliceContains(params.Tasks, "all") {
					keepGoing = confirm(`For some projects, scheduling all tasks/variants may result in a very large patch build. Continue? (Y/n)`, true)
				}
				if !keepGoing {
					return errors.New("patch aborted")
				}
			}

			comm, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer comm.Close()

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			ref, err := params.validatePatchCommand(ctx, conf, ac, comm)
			if err != nil {
				return err
			}
			params.Description = params.getDescription()

			if err = params.setLocalAliases(conf); err != nil {
				return errors.Wrap(err, "setting local aliases")
			}

			if (params.RepeatDefinition || params.RepeatFailed) && (len(params.Tasks) > 0 || len(params.Variants) > 0) {
				return errors.Errorf("can't define tasks/variants when reusing previous patch's tasks and variants")
			}

			diffData, err := loadGitData(ref.Branch, params.Ref, "", params.PreserveCommits, args...)
			if err != nil {
				return err
			}

			if err = params.validateSubmission(diffData); err != nil {
				return err
			}
			newPatch, err := params.createPatch(ac, diffData)
			if err != nil {
				return err
			}
			if err = params.displayPatch(newPatch, conf.UIServerHost, false); err != nil {
				grip.Error(err)
			}
			params.setDefaultProject(conf)
			return nil
		},
	}
}

func getParametersFromInput(params []string) ([]patch.Parameter, error) {
	res := []patch.Parameter{}
	catcher := grip.NewBasicCatcher()
	for _, param := range params {
		pair := strings.Split(param, "=")
		if len(pair) < 2 {
			catcher.Errorf("could not parse parameter '%s' in key=value format", param)
		}
		key := pair[0]
		val := strings.Join(pair[1:], "=")
		res = append(res, patch.Parameter{Key: key, Value: val})
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
		Before: mergeBeforeFuncs(autoUpdateCLI, requireFileExists(diffPathFlagName)),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			params := &patchParams{
				Project:     c.String(projectFlagName),
				Variants:    utility.SplitCommas(c.StringSlice(variantsFlagName)),
				Tasks:       utility.SplitCommas(c.StringSlice(tasksFlagName)),
				Alias:       c.String(patchAliasFlagName),
				SkipConfirm: c.Bool(skipConfirmFlagName),
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
				return errors.Wrap(err, "loading configuration")
			}

			comm, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer comm.Close()

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			if _, err = params.validatePatchCommand(ctx, conf, ac, comm); err != nil {
				return err
			}
			params.Description = params.getDescription()

			fullPatch, err := ioutil.ReadFile(diffPath)
			if err != nil {
				return errors.Wrapf(err, "reading diff file '%s'", diffPath)
			}

			diffData := &localDiff{
				fullPatch: string(fullPatch),
				base:      base,
			}

			if err = params.validateSubmission(diffData); err != nil {
				return err
			}
			newPatch, err := params.createPatch(ac, diffData)
			if err != nil {
				return err
			}
			return params.displayPatch(newPatch, conf.UIServerHost, false)
		},
	}
}
