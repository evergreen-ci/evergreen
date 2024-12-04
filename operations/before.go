package operations

import (
	"context"
	"os"
	"strings"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

var (
	requireClientConfig = func(c *cli.Context) error {
		if c.Parent().String(confFlagName) == "" {
			return errors.New("command line configuration path is not specified")
		}
		return nil
	}

	setPlainLogger = func(c *cli.Context) error {
		grip.Warning(grip.SetSender(send.MakePlainLogger()))
		return nil
	}

	requireVariantsFlag = func(c *cli.Context) error {
		variants := c.StringSlice(variantsFlagName)
		if len(variants) == 0 {
			return errors.New("must specify at least one variant")
		}
		return nil
	}

	requirePathFlag = func(c *cli.Context) error {
		path := c.String(pathFlagName)
		if path == "" {
			if c.NArg() != 1 {
				return errors.New("must specify the path to an Evergreen configuration")
			}
			path = c.Args().Get(0)
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			return errors.Errorf("configuration file '%s' does not exist", path)
		}

		return c.Set(pathFlagName, path)
	}

	requireHostFlag = func(c *cli.Context) error {
		host := c.String(hostFlagName)
		if host == "" {
			if c.NArg() != 1 {
				return errors.New("must specify a host ID")
			}
			host = c.Args().Get(0)
		}

		return c.Set(hostFlagName, host)
	}

	requireProjectFlag = func(c *cli.Context) error {
		if c.String(projectFlagName) == "" {
			return errors.New("must specify a project")
		}
		return nil
	}

	requirePatchIDFlag = func(c *cli.Context) error {
		patch := c.String(patchIDFlagName)
		if patch == "" {
			return errors.New("must specify a patch ID")
		}
		return nil
	}

	requireModuleFlag = func(c *cli.Context) error {
		if c.String(moduleFlagName) == "" {
			return errors.New("must specify a module")
		}
		return nil
	}

	// autoUpdateCLI is to be run before commonly used command line functions and will automatically update and install a newer
	// CLI version if one is found, and then run the command on that new CLI version.
	// Some functions that one would expect to return quickly have been omitted from having this as a 'before' function since downloading and installing
	// takes time that would be cumbersome to the user (e.g. list functions, delete functions).
	autoUpdateCLI = func(c *cli.Context) error {
		rootCtx := c
		var i int
		// Since this could be used at any sub-command context level, walk up
		// the parent contexts until the root context containing the config flag
		// is found.
		// Parent() should return either nil (no parent) or the exact
		// same context (the root context's parent is self-referential).
		for parentCtx := rootCtx.Parent(); parentCtx != nil && parentCtx != rootCtx; parentCtx = rootCtx.Parent() {
			if i > 10 {
				// This is just to avoid a potential infinite loop. It seems
				// really unlikely the CLI will have more than 10 nested
				// sub-commands.
				break
			}
			rootCtx = parentCtx
			i++
		}

		confPath := rootCtx.String(confFlagName)
		// we do not return an error in case of failure to find the config path flag because we do not want to block the underlying CLI operation.
		if confPath == "" {
			grip.Warning("Config path flag had no config set, skipping auto upgrade CLI check.")
			return nil
		}

		conf, err := NewClientSettings(confPath)
		if err != nil {
			grip.Errorf("Problem loading configuration: %s", err.Error())
		}
		if conf != nil && conf.AutoUpgradeCLI {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err = checkAndUpdateVersion(conf, ctx, true, false, true); err != nil {
				grip.Errorf("Automatic CLI update failed! Continuing with command execution. Error: %s", err.Error())
			}
		}
		return nil
	}
)

func requireFileExists(name string) cli.BeforeFunc {
	return func(c *cli.Context) error {
		path := c.String(name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return errors.Errorf("file '%s' does not exist", path)
		}

		return nil
	}
}

func requireStringFlag(name string) cli.BeforeFunc {
	return func(c *cli.Context) error {
		if c.String(name) == "" {
			return errors.Errorf("flag '--%s' was not specified", name)
		}
		return nil
	}
}

//nolint:unused
func requireStringSliceValueChoices(name string, options []string) cli.BeforeFunc {
	return func(c *cli.Context) error {
		catcher := grip.NewBasicCatcher()
		for _, val := range c.StringSlice(name) {
			if !utility.StringSliceContains(options, val) {
				catcher.Add(errors.Errorf("flag '--%s' value of '%s' is not an acceptable value %s",
					name, val, options))
			}
		}

		return catcher.Resolve()
	}
}

func requireIntValueBetween(name string, min, max int) cli.BeforeFunc {
	return func(c *cli.Context) error {
		val := c.Int(name)
		if val < min || val > max {
			return errors.Errorf("value of option '--%s' (%d) should be between %d and %d",
				name, val, min, max)
		}
		return nil
	}
}

func requireOnlyOneBool(flags ...string) cli.BeforeFunc {
	return func(c *cli.Context) error {
		count := 0
		for idx := range flags {
			if c.Bool(flags[idx]) {
				count++
			}
		}

		if count != 1 {
			return errors.Errorf("must specify one and only one of: --%s", strings.Join(flags, ", --"))
		}
		return nil
	}
}

func requireAtLeastOneBool(flags ...string) cli.BeforeFunc {
	return func(c *cli.Context) error {
		for idx := range flags {
			if c.Bool(flags[idx]) {
				return nil
			}
		}

		return errors.Errorf("must specify at least one of the following options: %s",
			strings.Join(flags, ", "))
	}
}

func requireAtLeastOneFlag(flags ...string) cli.BeforeFunc {
	return func(c *cli.Context) error {
		for idx := range flags {
			if c.IsSet(flags[idx]) {
				return nil
			}
		}

		return errors.Errorf("must specify at least one of the following options: %s",
			strings.Join(flags, ", "))
	}
}

func mergeBeforeFuncs(ops ...func(c *cli.Context) error) cli.BeforeFunc {
	return func(c *cli.Context) error {
		catcher := grip.NewBasicCatcher()

		for _, op := range ops {
			catcher.Add(op(c))
		}

		return catcher.Resolve()
	}
}

// mutuallyExclusiveArgs allows only one of the flags to be set
// if required is true, one of the flags must be set
func mutuallyExclusiveArgs(required bool, flags ...string) cli.BeforeFunc {
	return func(c *cli.Context) error {
		providedCount := 0
		for _, flag := range flags {
			if c.IsSet(flag) {
				providedCount++
			}
		}

		if providedCount > 1 {
			return errors.Errorf("only one of (%s) can be set", strings.Join(flags, " | "))
		}

		if required && providedCount == 0 {
			return errors.Errorf("one of (%s) must be set", strings.Join(flags, " | "))
		}

		return nil
	}
}

func requireWorkingDirFlag(dirFlagName string) cli.BeforeFunc {
	return func(c *cli.Context) error {
		wd := c.String(dirFlagName)
		if wd == "" {
			var err error
			wd, err = os.Getwd()
			if err != nil {
				return errors.Wrap(err, "finding working directory")
			}
			return c.Set(dirFlagName, wd)
		}
		return nil
	}
}
