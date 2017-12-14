package operations

import (
	"github.com/evergreen-ci/evergreen"
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
