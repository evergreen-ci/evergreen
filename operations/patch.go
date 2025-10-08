package operations

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/client"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	patchDescriptionFlagName             = "description"
	patchVerboseFlagName                 = "verbose"
	patchTriggerAliasFlag                = "trigger-alias"
	repeatDefinitionFlag                 = "repeat"
	repeatFailedDefinitionFlag           = "repeat-failed"
	repeatPatchIdFlag                    = "repeat-patch"
	includeModulesFlag                   = "include-modules"
	autoDescriptionFlag                  = "auto-description"
	testSelectionIncludeVariantsFlagName = "test-selection-include-variants"
	testSelectionIncludeTasksFlagName    = "test-selection-include-tasks"
	testSelectionExcludeVariantsFlagName = "test-selection-exclude-variants"
	testSelectionExcludeTasksFlagName    = "test-selection-exclude-tasks"
)

func getPatchFlags(flags ...cli.Flag) []cli.Flag {
	return mergeFlagSlices(
		addProjectFlag(flags...),
		addPatchFinalizeFlag(),
		addVariantsFlag(),
		addParameterFlag(),
		addPatchBrowseFlag(),
		addLargeFlag(),
		addSkipConfirmFlag(),
		addRefFlag(),
		addUncommittedChangesFlag(),
		addReuseFlags(
			cli.BoolFlag{
				Name:  joinFlagNames(jsonFlagName, "j"),
				Usage: "outputs the patch as a JSON object; suppresses warnings and confirmations",
			},
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
			cli.StringSliceFlag{
				Name:  joinFlagNames(testSelectionIncludeVariantsFlagName, "tsv"),
				Usage: "regex variant names that should have test selection enabled",
			},
			cli.StringSliceFlag{
				Name:  joinFlagNames(testSelectionIncludeTasksFlagName, "tst"),
				Usage: "regex task names that should have test selection enabled",
			},
			cli.StringSliceFlag{
				Name:  testSelectionExcludeVariantsFlagName,
				Usage: "regex variant names that should have test selection disabled (overrides included variants)",
			},
			cli.StringSliceFlag{
				Name:  testSelectionExcludeTasksFlagName,
				Usage: "regex task names that should have test selection disabled (overrides included tasks)",
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
			mutuallyExclusiveArgs(false, repeatDefinitionFlag, repeatPatchIdFlag),
			mutuallyExclusiveArgs(false, repeatPatchIdFlag, includeModulesFlag),
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
			outputJSON := c.Bool(jsonFlagName)
			if outputJSON {
				// If outputting the patch data as JSON, suppress any non-error
				// logs since the logs won't be in JSON format. Errors should
				// still appear so users can diagnose issues.
				l := grip.GetSender().Level()
				l.Threshold = level.Error
				grip.Error(errors.Wrap(grip.SetLevel(l), "increasing log level to suppress non-errors for JSON output"))
			}
			args := c.Args()
			params := &patchParams{
				Project:                            c.String(projectFlagName),
				Path:                               c.String(pathFlagName),
				Variants:                           utility.SplitCommas(c.StringSlice(variantsFlagName)),
				Tasks:                              utility.SplitCommas(c.StringSlice(tasksFlagName)),
				RegexVariants:                      utility.SplitCommas(c.StringSlice(regexVariantsFlagName)),
				RegexTasks:                         utility.SplitCommas(c.StringSlice(regexTasksFlagName)),
				RegexTestSelectionVariants:         utility.SplitCommas(c.StringSlice(testSelectionIncludeVariantsFlagName)),
				RegexTestSelectionExcludedVariants: utility.SplitCommas(c.StringSlice(testSelectionExcludeVariantsFlagName)),
				RegexTestSelectionTasks:            utility.SplitCommas(c.StringSlice(testSelectionIncludeTasksFlagName)),
				RegexTestSelectionExcludedTasks:    utility.SplitCommas(c.StringSlice(testSelectionExcludeTasksFlagName)),
				SkipConfirm:                        c.Bool(skipConfirmFlagName) || outputJSON,
				Description:                        c.String(patchDescriptionFlagName),
				AutoDescription:                    c.Bool(autoDescriptionFlag),
				Browse:                             c.Bool(patchBrowseFlagName),
				ShowSummary:                        c.Bool(patchVerboseFlagName),
				Large:                              c.Bool(largeFlagName),
				Alias:                              c.String(patchAliasFlagName),
				Ref:                                c.String(refFlagName),
				Uncommitted:                        c.Bool(uncommittedChangesFlag),
				PreserveCommits:                    c.Bool(preserveCommitsFlag),
				TriggerAliases:                     utility.SplitCommas(c.StringSlice(patchTriggerAliasFlag)),
				RepeatPatchId:                      c.String(repeatPatchIdFlag),
				RepeatDefinition:                   c.Bool(repeatDefinitionFlag) || c.String(repeatPatchIdFlag) != "",
				RepeatFailed:                       c.Bool(repeatFailedDefinitionFlag),
				IncludeModules:                     c.Bool(includeModulesFlag),
			}

			var err error
			shouldFinalize := c.Bool(patchFinalizeFlagName)
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

			comm, err := conf.setupRestCommunicator(ctx, !outputJSON)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer comm.Close()

			ac, rc, err := conf.getLegacyClients(comm)
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			ref, err := params.validatePatchCommand(ctx, conf, ac, comm)
			if err != nil {
				return err
			}
			params.Description = params.getDescription()

			hasTasks := len(params.Tasks) > 0 || len(params.RegexTasks) > 0
			hasVariants := len(params.Variants) > 0 || len(params.RegexVariants) > 0
			if hasTasks && !hasVariants {
				grip.Warningf("warning - you specified tasks without specifying variants")
			}
			if hasVariants && !hasTasks {
				grip.Warningf("warning - you specified variants without specifying tasks")
			}

			isReusing := params.RepeatDefinition || params.RepeatFailed
			hasTasksOrVariants := len(params.Tasks) > 0 || len(params.Variants) > 0
			hasRegexTasksOrVariants := len(params.RegexTasks) > 0 || len(params.RegexVariants) > 0

			if isReusing && (hasTasksOrVariants || hasRegexTasksOrVariants || len(params.Alias) > 0) {
				return errors.Errorf("can't define tasks, variants, regex tasks, regex variants or aliases when reusing previous patch's tasks and variants")
			}

			remote, err := gitGetRemote("", ref.Owner, ref.Repo)
			if err != nil {
				return errors.Errorf("you do not have a remote tracking your Evergreen project. The project to track is https://github.com/%s/%s", ref.Owner, ref.Repo)
			}

			diffData, err := loadGitData("", remote, ref.Branch, params.Ref, "", params.PreserveCommits, args...)
			if err != nil {
				return err
			}

			if err = params.validateSubmission(diffData); err != nil {
				return err
			}
			// Initialize module path cache in case these have already been set by the user. We use a cache here
			// to avoid asking the user repeatedly for paths, in the case that they aren't writing them back to their configuration file.
			modulePathCache := conf.getModulePathsForProject(params.Project)
			if params.IncludeModules {
				localModuleIncludes, err := getLocalModuleIncludes(params, conf, params.Path, ref.RemotePath, modulePathCache)
				if err != nil {
					return err
				}
				params.LocalModuleIncludes = localModuleIncludes
			}

			newPatch, err := params.createPatch(ac, diffData)
			if err != nil {
				return errors.Wrapf(err, "creating cli patch")
			}
			patchId := newPatch.Id.Hex()
			if params.IncludeModules {
				proj, err := rc.GetPatchedConfig(patchId)
				if err != nil {
					return err
				}
				if proj == nil {
					return errors.Errorf("project config for '%s' not found", patchId)
				}

				for _, module := range proj.Modules {
					modulePath, err := params.getModulePath(conf, module.Name, modulePathCache)
					if err != nil {
						grip.Error(err)
						continue
					}
					if err = addModuleToPatch(comm, params, args, conf, newPatch, &module, modulePath); err != nil {
						grip.Errorf("Error adding module '%s' to patch: %s", module.Name, err)
					}
				}
			}

			if shouldFinalize {
				shouldContinue, err := checkForLargeNumFinalizedTasks(ctx, comm, rc, params, patchId)
				if err != nil {
					return err
				}
				if shouldContinue {
					if err = ac.FinalizePatch(patchId); err != nil {
						return errors.Wrapf(err, "finalizing patch '%s'", patchId)
					}
					newPatch.Activated = true
				}
			}

			outputParams := outputPatchParams{
				patches:    []patch.Patch{*newPatch},
				uiHost:     conf.UIServerHost,
				outputJSON: outputJSON,
			}
			if err = params.displayPatch(ctx, ac, outputParams); err != nil {
				grip.Error(err)
			}
			params.setDefaultProject(conf)
			return nil
		},
	}
}

// checkForLargeNumFinalizedTasks retrieves an un-finalized patch document, counts the number of tasks it contains,
// and prompts the user with a confirmation popup if the number of tasks is greater than the largeNumFinalizedTasksThreshold.
// It returns true if the finalization process should go through, and false otherwise.
func checkForLargeNumFinalizedTasks(ctx context.Context, comm client.Communicator, rc *legacyClient, params *patchParams, patchId string) (bool, error) {
	if params.SkipConfirm {
		return true, nil
	}
	existingPatch, err := rc.GetPatch(patchId)
	if err != nil {
		return false, errors.Wrapf(err, "getting patch '%s'", patchId)
	}
	if existingPatch == nil {
		return false, errors.Wrapf(err, "patch '%s' not found", patchId)
	}
	proj, err := rc.GetPatchedConfig(patchId)
	if err != nil {
		return false, errors.Wrapf(err, "getting patched config for patch '%s'", patchId)
	}
	if proj == nil {
		return false, errors.Errorf("project config for '%s' not found", patchId)
	}
	generatorTasks := proj.TasksThatCallCommand(evergreen.GenerateTasksCommandName)
	var tvPairs []model.TVPair
	numTasksToFinalize := 0
	for _, vt := range existingPatch.VariantsTasks {
		for _, t := range vt.Tasks {
			if _, ok := generatorTasks[t]; ok {
				tvPairs = append(tvPairs, model.TVPair{
					TaskName: t,
					Variant:  vt.Variant,
				})
			}
		}
		numTasksToFinalize += len(vt.Tasks)
	}
	numEstimatedGeneratedTasks, err := comm.GetEstimatedGeneratedTasks(ctx, patchId, tvPairs)
	if err != nil {
		return false, errors.Wrapf(err, "getting estimated generated tasks for patch '%s'", patchId)
	}
	numTasksToFinalize += numEstimatedGeneratedTasks
	if numTasksToFinalize > largeNumFinalizedTasksThreshold {
		if !confirm(fmt.Sprintf("This is a large patch, expected to schedule %d tasks (%d via task generation). Finalize anyway?", numTasksToFinalize, numEstimatedGeneratedTasks), true) {
			return false, nil
		}
	}
	return true, nil
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
		allowEmptyFlagName  = "allow-empty"
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
			cli.BoolFlag{
				Name:  allowEmptyFlagName,
				Usage: "create an empty patch with no diff",
			},
			cli.StringFlag{
				Name: patchAuthorFlag,
				Usage: "optionally define the patch author by providing an Evergreen username; " +
					"if not found or provided, will default to the submitter",
			},
		),
		Before: mergeBeforeFuncs(
			autoUpdateCLI,
			setPlainLogger,
			mutuallyExclusiveArgs(false, patchDescriptionFlagName, autoDescriptionFlag),
			mutuallyExclusiveArgs(false, diffPathFlagName, diffPatchIdFlagName),
			mutuallyExclusiveArgs(false, allowEmptyFlagName, diffPatchIdFlagName),
			mutuallyExclusiveArgs(false, allowEmptyFlagName, diffPathFlagName),
			mutuallyExclusiveArgs(false, baseFlagName, diffPatchIdFlagName),
		),
		Action: func(c *cli.Context) error {
			diffPatchId := c.String(diffPatchIdFlagName)
			diffFilePath := c.String(diffPathFlagName)
			allowEmpty := c.Bool(allowEmptyFlagName)
			if diffPatchId == "" && diffFilePath != "" {
				if _, err := os.Stat(diffFilePath); os.IsNotExist(err) {
					return errors.Errorf("file '%s' does not exist", diffFilePath)
				}
			}
			confPath := c.Parent().String(confFlagName)
			outputJSON := c.Bool(jsonFlagName)
			if outputJSON {
				// If outputting the patch data as JSON, suppress any non-error
				// logs since the logs won't be in JSON format. Errors should
				// still appear so users can diagnose issues.
				l := grip.GetSender().Level()
				l.Threshold = level.Error
				grip.Error(errors.Wrap(grip.SetLevel(l), "increasing log level to suppress non-errors for JSON output"))
			}
			params := &patchParams{
				Project:          c.String(projectFlagName),
				Variants:         utility.SplitCommas(c.StringSlice(variantsFlagName)),
				Tasks:            utility.SplitCommas(c.StringSlice(tasksFlagName)),
				Alias:            c.String(patchAliasFlagName),
				SkipConfirm:      c.Bool(skipConfirmFlagName) || outputJSON,
				Description:      c.String(patchDescriptionFlagName),
				AutoDescription:  c.Bool(autoDescriptionFlag),
				ShowSummary:      c.Bool(patchVerboseFlagName),
				Large:            c.Bool(largeFlagName),
				PatchAuthor:      c.String(patchAuthorFlag),
				RepeatPatchId:    c.String(repeatPatchIdFlag),
				RepeatDefinition: c.Bool(repeatDefinitionFlag) || c.String(repeatPatchIdFlag) != "",
				RepeatFailed:     c.Bool(repeatFailedDefinitionFlag),
			}
			var err error
			diffPath := c.String(diffPathFlagName)
			base := c.String(baseFlagName)
			shouldFinalize := c.Bool(patchFinalizeFlagName)
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

			comm, err := conf.setupRestCommunicator(ctx, !outputJSON)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer comm.Close()

			ac, rc, err := conf.getLegacyClients(comm)

			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			if _, err = params.validatePatchCommand(ctx, conf, ac, comm); err != nil {
				return err
			}
			params.Description = params.getDescription()
			var diffData localDiff
			var rp *restmodel.APIRawPatch
			if allowEmpty {
				diffData.base = base
			} else if diffPatchId == "" {
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

			if shouldFinalize {
				patchId := newPatch.Id.Hex()
				shouldContinue, err := checkForLargeNumFinalizedTasks(ctx, comm, rc, params, patchId)
				if err != nil {
					return err
				}
				if shouldContinue {
					if err = ac.FinalizePatch(patchId); err != nil {
						return errors.Wrapf(err, "finalizing patch '%s'", patchId)
					}
					newPatch.Activated = true
				}
			}

			outputParams := outputPatchParams{
				patches:    []patch.Patch{*newPatch},
				uiHost:     conf.UIServerHost,
				outputJSON: outputJSON,
			}

			return params.displayPatch(ctx, ac, outputParams)
		},
	}
}

// getLocalModuleIncludes reads and saves files module includes from the local project config.
func getLocalModuleIncludes(params *patchParams, conf *ClientSettings, path, remotePath string, modulePathCache map[string]string) ([]patch.LocalModuleInclude, error) {
	var yml []byte
	var err error
	if path != "" {
		yml, err = os.ReadFile(path)
	} else {
		yml, err = os.ReadFile(remotePath)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "reading local project config '%s'", remotePath)
	}
	p := model.ParserProject{}
	if err := util.UnmarshalYAMLWithFallback(yml, &p); err != nil {
		yamlErr := thirdparty.YAMLFormatError{Message: err.Error()}
		return nil, errors.Wrap(yamlErr, "unmarshalling parser project from local project config")
	}

	moduleIncludes := []patch.LocalModuleInclude{}
	for _, include := range p.Include {
		if include.Module == "" {
			continue
		}
		modulePath, err := params.getModulePath(conf, include.Module, modulePathCache)
		if err != nil {
			grip.Error(errors.Wrapf(err, "getting module path for '%s'", include.Module))
			continue
		}

		filePath := fmt.Sprintf("%s/%s", modulePath, include.FileName)
		fileContents, err := os.ReadFile(filePath)
		if err != nil {
			return nil, errors.Wrapf(err, "reading local module include file '%s'", filePath)
		}
		patchedInclude := patch.LocalModuleInclude{
			Module:      include.Module,
			FileName:    include.FileName,
			FileContent: fileContents,
		}
		moduleIncludes = append(moduleIncludes, patchedInclude)
	}
	return moduleIncludes, nil
}
