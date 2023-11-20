package operations

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func PatchSetModule() cli.Command {
	const largeFlagName = "large"

	return cli.Command{
		Name:    "patch-set-module",
		Aliases: []string{"set-module"},
		Usage:   "update or add module to an existing patch",
		Flags: mergeFlagSlices(addModuleFlag(), addSkipConfirmFlag(), addRefFlag(), addUncommittedChangesFlag(),
			addPatchFinalizeFlag(), addPreserveCommitsFlag(
				cli.BoolFlag{
					Name:  largeFlagName,
					Usage: "enable submitting larger patches (>16MB)",
				},
				cli.StringFlag{
					Name:  joinFlagNames(patchIDFlagName, "id", "i"),
					Usage: "specify the ID of a patch (defaults to user's latest submitted patch)",
				})),
		Before: mergeBeforeFuncs(
			autoUpdateCLI,
			setPlainLogger,
			requireModuleFlag,
			mutuallyExclusiveArgs(false, uncommittedChangesFlag, preserveCommitsFlag),
		),
		Action: func(c *cli.Context) error {
			params := &patchParams{
				Project:         c.String(moduleFlagName),
				SkipConfirm:     c.Bool(skipConfirmFlagName),
				Finalize:        c.Bool(patchFinalizeFlagName),
				Large:           c.Bool(largeFlagName),
				Ref:             c.String(refFlagName),
				Uncommitted:     c.Bool(uncommittedChangesFlag),
				PreserveCommits: c.Bool(preserveCommitsFlag),
			}
			confPath := c.Parent().String(confFlagName)
			moduleName := c.String(moduleFlagName)
			patchID := c.String(patchIDFlagName)
			args := c.Args()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			var existingPatch *patch.Patch
			if patchID == "" {
				patchList, err := ac.GetPatches(1)
				if err != nil {
					return errors.Wrapf(err, "getting latest patch from user")
				}
				if len(patchList) == 0 {
					return errors.New("no patches found from user")
				}
				existingPatch = &patchList[0]
				patchID = existingPatch.Id.Hex()
			} else {
				existingPatch, err = ac.GetPatch(patchID)
			}
			if err != nil {
				return errors.Wrapf(err, "getting patch '%s'", patchID)
			}
			if existingPatch.IsCommitQueuePatch() {
				return errors.New("Use `commit-queue set-module` instead of `set-module` for commit queue patches")
			}
			module, err := conf.getModule(patchID, moduleName)
			if err != nil {
				return err
			}
			if err := addModuleToPatch(params, args, conf, existingPatch, module, ""); err != nil {
				return err
			}
			if params.Finalize {
				if err = ac.FinalizePatch(patchID); err != nil {
					return errors.Wrapf(err, "finalizing patch '%s'", patchID)
				}
				grip.Info("Patch finalized.")
			}
			return nil
		},
	}
}

func addModuleToPatch(params *patchParams, args cli.Args, conf *ClientSettings,
	p *patch.Patch, module *model.Module, modulePath string) error {
	patchId := p.Id.Hex()

	preserveCommits := params.PreserveCommits || conf.PreserveCommits
	if !params.SkipConfirm {
		keepGoing, err := confirmUncommittedChanges(modulePath, preserveCommits, params.Uncommitted || conf.UncommittedChanges)
		if err != nil {
			return errors.Wrap(err, "confirming uncommitted changes")
		}
		if !keepGoing {
			return errors.New("patch aborted")
		}
	}

	ref := params.Ref
	if params.Uncommitted || conf.UncommittedChanges {
		ref = ""
	}

	// Diff against the module branch.
	diffData, err := loadGitData(modulePath, module.Branch, ref, "", preserveCommits, args...)
	if err != nil {
		return err
	}
	if err = validatePatchSize(diffData, params.Large); err != nil {
		return err
	}

	if !params.SkipConfirm {
		grip.Infof("Using branch '%s' for module '%s'.", module.Branch, module.Name)
		if diffData.patchSummary != "" {
			fmt.Println(diffData.patchSummary)
		}
		if len(diffData.fullPatch) > 0 {
			if !confirm("This is a summary of the module patch to be submitted. Include this module's changes?", true) {
				return nil
			}
		} else {
			if !confirm("Patch submission for the module is empty. Continue?", true) {
				return nil
			}
		}
	}
	moduleParams := UpdatePatchModuleParams{
		patchID: patchId,
		module:  module.Name,
		patch:   diffData.fullPatch,
		base:    diffData.base,
	}
	ac, _, err := conf.getLegacyClients()
	if err != nil {
		return errors.Wrap(err, "setting up legacy Evergreen client")
	}
	err = ac.UpdatePatchModule(moduleParams)
	if err != nil {
		return err
	}
	grip.Infof("Module '%s' updated, base commit is '%s' (and will override the manifest).", module.Name, diffData.base)
	return nil
}

func PatchRemoveModule() cli.Command {
	return cli.Command{
		Name:    "patch-remove-module",
		Aliases: []string{"rm-module", "patch-rm-module"},
		Usage:   "remove a module from an existing patch",
		Flags:   mergeFlagSlices(addPatchIDFlag(), addModuleFlag()),
		Before:  mergeBeforeFuncs(requirePatchIDFlag, requireModuleFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			patchID := c.String(patchIDFlagName)
			module := c.String(moduleFlagName)

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			err = ac.DeletePatchModule(patchID, module)
			if err != nil {
				return err
			}

			fmt.Println("Module removed.")
			return nil
		},
	}
}
