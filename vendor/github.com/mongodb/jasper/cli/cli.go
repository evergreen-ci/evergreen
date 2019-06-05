package cli

import (
	"github.com/urfave/cli"
)

const (
	hostFlagName         = "host"
	portFlagName         = "port"
	serviceFlagName      = "service"
	certFilePathFlagName = "cert_path"

	defaultLocalHostName = "localhost"
)

// Jasper is the CLI interface to Jasper services.
func Jasper() cli.Command {
	return cli.Command{
		Name:  "jasper",
		Usage: "Jasper CLI to interact with Jasper services",
		Subcommands: []cli.Command{
			Client(),
			Service(),
		},
	}
}
