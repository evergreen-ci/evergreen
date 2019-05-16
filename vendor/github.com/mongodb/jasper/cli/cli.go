package cli

import (
	"math"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	hostFlagName         = "host"
	portFlagName         = "port"
	serviceFlagName      = "service"
	certFilePathFlagName = "cert_path"

	serviceREST = "rest"
	serviceRPC  = "rpc"
)

// Jasper is the CLI interface to Jasper services.
func Jasper() cli.Command {
	return cli.Command{
		Name:  "jasper",
		Usage: "Jasper CLI to interact with Jasper services",
		Before: mergeBeforeFuncs(
			func(c *cli.Context) error {
				port := c.GlobalInt(portFlagName)
				if port < 0 || port > math.MaxUint16 {
					return errors.New("port must be within 0-65535 inclusive")
				}
				return nil
			},
			func(c *cli.Context) error {
				service := c.GlobalString(serviceFlagName)
				if service != serviceREST && service != serviceRPC {
					return errors.Errorf("service must be '%s' or '%s'", serviceREST, serviceRPC)
				}
				return nil
			},
		),
		Subcommands: []cli.Command{
			Manager(),
			Process(),
		},
	}
}
