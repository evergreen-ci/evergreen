package operations

import (
	"errors"
	"os"
	"strings"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/urfave/cli"
)

var (
	requireClientConfig = func(c *cli.Context) error {
		if c.Parent().String(confFlagName) == "" {
			return errors.New("command line configuration path is not specified")
		}
		return nil
	}

	requireServiceConfig = func(c *cli.Context) error {
		if c.String(confFlagName) == "" {
			return errors.New("service configuration path is not specified")
		}
		return nil
	}

	setPlainLogger = func(c *cli.Conetext) error {
		grip.CatchWarning(grip.SetSender(send.MakePlainLogger()))
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
			path = c.Arg().Get(0)
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			return errors.Errorf("configuration file %s does not exist", path)
		}

		c.Set(pathFlagName, path)
		return nil
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
)

func requireStringFlag(name string) cli.BeforeFunc {
	return func(c *cli.Context) error {
		if c.String(name) == "" {
			return errors.Errorf("flag '--%s' was not specified", name)
		}
		return nil
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

func mergeBeforeFuncs(ops ...func(c *cli.Context) error) cli.BeforeFunc {
	return func(c *cli.Context) error {
		catcher := grip.NewBasicCatcher()

		for _, op := range ops {
			catcher.Add(op())
		}

		return catcher.Resolve()
	}
}
