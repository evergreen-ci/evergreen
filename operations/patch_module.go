package operations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mongodb/grip"
	"github.com/urfave/cli"
)

func PatchSetModule() cli.Command {
	const largeFlagName = "large"

	return cli.Command{
		Name:    "patch-set-module",
		Aliases: []string{"set-module"},
		Usage:   "update or add module to an existing patch",
		Flags: mergeFlagSlices(addPathFlag(), addModuleFlag(), addYesFlag(
			cli.BoolFlag{
				Name:  largeFlagName,
				Usage: "enable submitting larger patches (>16MB)",
			})),
		Before: mergeBeforeFuncs(requirePatchIDFlag, requireModuleFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			module := c.String(moduleFlagName)
			patchID := c.String(patchIDFlagName)
			large := c.Bool(largeFlagName)
			skipConfirm := c.Bool(yesFlagName)
			project := s.String(projectFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			_ = conf.GetRestCommunicator(ctx)

			ac, rc, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			notifyUserUpdate(ac)

			proj, err := rc.GetPatchedConfig(smc.PatchId)
			if err != nil {
				return err
			}

			moduleBranch, err := getModuleBranch(module, proj)
			if err != nil {
				grip.Error(err)
				mods, merr := ac.GetPatchModules(patchID, proj.Identifier)
				if merr != nil {
					return errors.Wrap(merr, "errors fetching list of available modules")
				}

				if len(mods) != 0 {
					grip.Noticef("known modules includes:\n\t%s", strings.Join(mods, "\n\t"))
				}

				return errors.Errorf("could not set specified module: \"%s\"", module)
			}

			// diff against the module branch.
			diffData, err := loadGitData(moduleBranch, args...)
			if err != nil {
				return err
			}
			if err = validatePatchSize(diffData, large); err != nil {
				return err
			}

			if !skipConfirm {
				fmt.Printf("Using branch %v for module %v \n", moduleBranch, module)
				if diffData.patchSummary != "" {
					fmt.Println(diffData.patchSummary)
				}

				if !confirm("This is a summary of the patch to be submitted. Continue? (y/n):", true) {
					return nil
				}
			}

			err = ac.UpdatePatchModule(patchID, module, diffData.fullPatch, diffData.base)
			if err != nil {
				mods, err := ac.GetPatchModules(patchID, project)
				var msg string
				if err != nil {
					msg = fmt.Sprintf("could not find module named %s or retrieve list of modules",
						module)
				} else if len(mods) == 0 {
					msg = fmt.Sprintf("could not find modules for this project. %s is not a module. "+
						"see the evergreen configuration file for module configuration.",
						module)
				} else {
					msg = fmt.Sprintf("could not find module named '%s', select correct module from:\n\t%s",
						module, strings.Join(mods, "\n\t"))
				}
				grip.Error(msg)
				return err

			}
			fmt.Println("Module updated.")
			return nil
		},
	}
}

func PatchRemoveModule() cli.Command {
	return cli.Command{
		Name:    "patch-remove-module",
		Aliases: []string{"rm-module", "patch-rm-module"},
		Usage:   "remove a module from an existing patch",
		Flags:   mergeFlagSlices(addPathFlag(), addModuleFlag()),
		Before:  mergeBeforeFuncs(requirePatchIDFlag, requireModuleFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			patchID := c.String(patchIDFlagName)
			module := c.String(moduleFlagName)

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

			err = ac.DeletePatchModule(patchID, module)
			if err != nil {
				return err
			}

			fmt.Println("Module removed.")
			return nil
		},
	}
}
