package cli

import (
	"github.com/urfave/cli"
)

const (
	// JasperCommand represents the Jasper interface as a CLI command.
	JasperCommand = "jasper"

	hostFlagName          = "host"
	portFlagName          = "port"
	credsFilePathFlagName = "creds_path"

	defaultLocalHostName = "localhost"
)

// Jasper is the CLI interface to Jasper services.
func Jasper() cli.Command {
	return cli.Command{
		Name:  JasperCommand,
		Usage: "Jasper CLI to interact with Jasper services",
		Subcommands: []cli.Command{
			Client(),
			Service(),
		},
	}
}
