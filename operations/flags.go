package operations

import (
	"errors"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/urfave/cli"
)

const (
	confFlagName = "conf"
)

func configFlags(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.Flag{
		cli.StringFlag{
			Name:    confFlagName,
			Usage:   "path to the service configuration file",
			Default: evergreen.DefaultServiceConfigurationFileName,
		},
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

func loadSettings(path string) *model.CLISettings
