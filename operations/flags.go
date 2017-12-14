package operations

import (
	"errors"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/urfave/cli"
)

const (
	confFlagName       = "conf"
	adminFlagsFlagName = "flags"
)

func serviceConfigFlags(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:    confFlagName,
		Aliases: []string{"c", "config"},
		Usage:   "path to the service configuration file",
		Default: evergreen.DefaultServiceConfigurationFileName,
	})
}

func clientConfigFlags(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:    confFlagName,
		Aliases: []string{"c", "config"},
		Usage:   "path to the service configuration file, defaults to ~/.evergreen.yml",
	})

}

func adminFlagFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringSliceFlag{
		Name:  flagFlagName,
		Usage: "specify a flag to disable; may specify more than once",
	})
}

func requireConfig(ops ...func(c *cli.Context) error) cli.BeforeFunc {
	return mergeBeforeFuncs(append(ops, func(c *cli.Context) error {
		if c.String("config") == "" {
			catcher.Add(errors.New("command line configuration path is not specified"))
		}
	}))
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
