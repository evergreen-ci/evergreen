package operations

import (
	"os"
	"strings"

	"github.com/evergreen-ci/evergreen/util"
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
				return errors.New("must specify the path to an evergreen configuration")
			}
			path = c.Args().Get(0)
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			return errors.Errorf("configuration file %s does not exist", path)
		}

		return c.Set(pathFlagName, path)
	}

	requireHostFlag = func(c *cli.Context) error {
		host := c.String(hostFlagName)
		if host == "" {
			if c.NArg() != 1 {
				return errors.New("must specify a host id")
			}
			host = c.Args().Get(0)
		}

		return c.Set(hostFlagName, host)
	}

	requirePatchIDFlag = func(c *cli.Context) error {
		patch := c.String(patchIDFlagName)
		if patch == "" {
			return errors.New("must specify a patch id")
		}
		return nil
	}

	requireModuleFlag = func(c *cli.Context) error {
		if c.String(moduleFlagName) == "" {
			return errors.New("must specify a module")
		}
		return nil
	}

	addPositionalMigrationIds = func(c *cli.Context) error {
		if c.NArg() == 0 {
			return nil
		}

		catcher := grip.NewSimpleCatcher()
		for _, arg := range c.Args() {
			catcher.Add(c.Set(anserMigrationIDFlagName, arg))
		}

		return catcher.Resolve()
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

// nolint: unused, deadcode
func requireStringSliceValueChoices(name string, options []string) cli.BeforeFunc {
	return func(c *cli.Context) error {
		catcher := grip.NewBasicCatcher()
		for _, val := range c.StringSlice(name) {
			if !util.StringSliceContains(options, val) {
				catcher.Add(errors.Errorf("flag '--%s' value of '%s' is not an acceptable value %s",
					name, val, options))
			}
		}

		return catcher.Resolve()
	}
}

func requireStringValueChoices(name string, options []string) cli.BeforeFunc {
	return func(c *cli.Context) error {
		val := c.String(name)
		if !util.StringSliceContains(options, val) {
			return errors.Errorf("flag '--%s' value of '%s' is not an acceptable value %s",
				name, val, options)
		}

		return nil
	}
}

// nolint: unused, deadcode
func requireStringLengthIfSpecified(name string, length int) cli.BeforeFunc {
	return func(c *cli.Context) error {
		val := c.String(name)

		if val == "" {
			return nil
		}

		if len(val) != length {
			return errors.Errorf("option '--%s' has length %d, not %d, which is required",
				name, len(val), length)
		}

		return nil
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
