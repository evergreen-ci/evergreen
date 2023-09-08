package operations

import (
	"context"
	"os"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/patch"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
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
	repeatPatchIdFlag          = "repeat-patch"
	includeModulesFlag         = "include-modules"
	autoDescriptionFlag        = "auto-description"
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
		addReuseFlags(),
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
				Name:  joinFlagNames(autoDescriptionFlag, "ad"),
				Usage: "use last commit message as the patch description",
			},
			cli.BoolFlag{
				Name:  patchVerboseFlagName,
				Usage: "show patch summary",
			},
			cli.StringSliceFlag{
				Name:  patchTriggerAliasFlag,
				Usage: "patch trigger alias (set by project admin) specifying tasks from other projects",
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
			mutuallyExclusiveArgs(false, patchDescriptionFlagName, autoDescriptionFlag),
			mutuallyExclusiveArgs(false, preserveCommitsFlag, uncommittedChangesFlag),
			mutuallyExclusiveArgs(false, repeatDefinitionFlag, repeatPatchIdFlag,
				repeatFailedDefinitionFlag),
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
		Flags: getPatchFlags(
			cli.BoolFlag{
				Name:  includeModulesFlag,
				Usage: "if this boolean is set, Evergreen will include module diffs using changes from defined module paths",
			},
		),
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
				AutoDescription:   c.Bool(autoDescriptionFlag),
				Finalize:          c.Bool(patchFinalizeFlagName),
				Browse:            c.Bool(patchBrowseFlagName),
				ShowSummary:       c.Bool(patchVerboseFlagName),
				Large:             c.Bool(largeFlagName),
				Alias:             c.String(patchAliasFlagName),
				Ref:               c.String(refFlagName),
				Uncommitted:       c.Bool(uncommittedChangesFlag),
				PreserveCommits:   c.Bool(preserveCommitsFlag),
				TriggerAliases:    utility.SplitCommas(c.StringSlice(patchTriggerAliasFlag)),
			}

			var err error
			params.addReuseFlags(c)
			includeModules := c.Bool(includeModulesFlag)
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
				keepGoing, err = confirmUncommittedChanges("", params.PreserveCommits, params.Uncommitted || conf.UncommittedChanges)
				if err != nil {
					return errors.Wrap(err, "confirming uncommitted changes")
				}
				if keepGoing && utility.StringSliceContains(params.Variants, "all") && utility.StringSliceContains(params.Tasks, "all") {
					keepGoing = confirm(`For some projects, scheduling all tasks/variants may result in a very large patch build. Continue?`, true)
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

			ac, rc, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			ref, err := params.validatePatchCommand(ctx, conf, ac, comm)
			if err != nil {
				return err
			}
			params.Description = params.getDescription()

			isReusing := params.RepeatDefinition || params.RepeatFailed
			hasTasksOrVariants := len(params.Tasks) > 0 || len(params.Variants) > 0
			hasRegexTasksOrVariants := len(params.RegexTasks) > 0 || len(params.RegexVariants) > 0

			if isReusing && (hasTasksOrVariants || hasRegexTasksOrVariants || len(params.Alias) > 0) {
				return errors.Errorf("can't define tasks, variants, regex tasks, regex variants or aliases when reusing previous patch's tasks and variants")
			}

			diffData, err := loadGitData("", ref.Branch, params.Ref, "", params.PreserveCommits, args...)
			if err != nil {
				return err
			}

			if err = params.validateSubmission(diffData); err != nil {
				return err
			}
			var originalFinalize bool
			// If including modules, don't finalize the patch until we've checked all modules for changes.
			if includeModules {
				originalFinalize = params.Finalize
				params.Finalize = false
			}
			newPatch, err := params.createPatch(ac, diffData)
			if err != nil {
				return err
			}
			patchId := newPatch.Id.Hex()
			if includeModules {
				proj, err := rc.GetPatchedConfig(patchId)
				if err != nil {
					return err
				}

				for _, module := range proj.Modules {
					modulePath, err := params.getModulePath(conf, module.Name)
					if err != nil {
						grip.Error(err)
						continue
					}
					if err = addModuleToPatch(params, args, conf, newPatch, &module, modulePath); err != nil {
						grip.Errorf("Error adding module '%s' to patch: %s", module.Name, err)
					}
				}
			}

			if originalFinalize {
				if err = ac.FinalizePatch(patchId); err != nil {
					return errors.Wrapf(err, "finalizing patch '%s'", patchId)
				}
			}

			if err = params.displayPatch(newPatch, conf.UIServerHost, false); err != nil {
				grip.Error(err)
			}
			params.setDefaultProject(conf)
			return nil
		},
	}
}

func (p *patchParams) addReuseFlags(c *cli.Context) {
	p.RepeatPatchId = c.String(repeatPatchIdFlag)
	p.RepeatDefinition = c.Bool(repeatDefinitionFlag) || p.RepeatPatchId != ""
	p.RepeatFailed = c.Bool(repeatFailedDefinitionFlag)
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
		baseFlagName        = "base"
		diffPathFlagName    = "diff-file"
		diffPatchIdFlagName = "diff-patchId"
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
			cli.StringFlag{
				Name:  diffPatchIdFlagName,
				Usage: "patch id to fetch the full diff (including modules) from",
			},
			cli.StringFlag{
				Name: patchAuthorFlag,
				Usage: "optionally define the patch author by providing an Evergreen username; " +
					"if not found or provided, will default to the submitter",
			},
		),
		Before: mergeBeforeFuncs(
			autoUpdateCLI,
			mutuallyExclusiveArgs(false, patchDescriptionFlagName, autoDescriptionFlag),
			mutuallyExclusiveArgs(false, diffPathFlagName, diffPatchIdFlagName),
			mutuallyExclusiveArgs(false, baseFlagName, diffPatchIdFlagName),
		),
		Action: func(c *cli.Context) error {
			diffPatchId := c.String(diffPatchIdFlagName)
			diffFilePath := c.String(diffPathFlagName)
			if diffPatchId == "" && diffFilePath != "" {
				if _, err := os.Stat(diffFilePath); os.IsNotExist(err) {
					return errors.Errorf("file '%s' does not exist", diffFilePath)
				}
			}
			confPath := c.Parent().String(confFlagName)
			params := &patchParams{
				Project:         c.String(projectFlagName),
				Variants:        utility.SplitCommas(c.StringSlice(variantsFlagName)),
				Tasks:           utility.SplitCommas(c.StringSlice(tasksFlagName)),
				Alias:           c.String(patchAliasFlagName),
				SkipConfirm:     c.Bool(skipConfirmFlagName),
				Description:     c.String(patchDescriptionFlagName),
				AutoDescription: c.Bool(autoDescriptionFlag),
				Finalize:        c.Bool(patchFinalizeFlagName),
				ShowSummary:     c.Bool(patchVerboseFlagName),
				Large:           c.Bool(largeFlagName),
				SyncTasks:       utility.SplitCommas(c.StringSlice(syncTasksFlagName)),
				PatchAuthor:     c.String(patchAuthorFlag),
			}
			var err error
			diffPath := c.String(diffPathFlagName)
			base := c.String(baseFlagName)
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

			var diffData localDiff
			var rp *restmodel.APIRawPatch
			if diffPatchId == "" {
				fullPatch, err := os.ReadFile(diffPath)
				if err != nil {
					return errors.Wrapf(err, "reading diff file '%s'", diffPath)
				}
				diffData.fullPatch = string(fullPatch)
				diffData.base = base
			} else {
				rp, err = comm.GetRawPatchWithModules(ctx, diffPatchId)
				if err != nil {
					return errors.Wrap(err, "getting raw patch with modules")
				}
				if rp == nil {
					return errors.Wrap(err, "patch not found")
				}
				diffData.fullPatch = rp.Patch.Diff
				diffData.base = rp.Patch.Githash
			}

			if err = params.validateSubmission(&diffData); err != nil {
				return err
			}
			newPatch, err := params.createPatch(ac, &diffData)
			if err != nil {
				return err
			}

			if rp != nil {
				for _, module := range rp.RawModules {
					moduleParams := UpdatePatchModuleParams{
						patchID: newPatch.Id.Hex(),
						module:  module.Name,
						patch:   module.Diff,
						base:    module.Githash,
					}
					if err = ac.UpdatePatchModule(moduleParams); err != nil {
						return err
					}
					grip.Infof("Module '%s' updated.", module.Name)

				}
			}

			return params.displayPatch(newPatch, conf.UIServerHost, false)
		},
	}
}
