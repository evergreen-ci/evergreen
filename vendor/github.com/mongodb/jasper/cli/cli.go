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

// Jasper is the CLI interface to Jasper services. This is used by
// jasper.Manager implementations that manage processes remotely via
// commands via command executions. The interface is designed for
// machine interaction.
func Jasper() cli.Command {
	return cli.Command{
		Name:  JasperCommand,
		Usage: "Jasper CLI to interact with Jasper services",
		Subcommands: []cli.Command{
			Client(),
			Service(),
			Run(),
			List(),
			Clear(),
			Kill(),
			KillAll(),
			Download(),
		},
	}
}
