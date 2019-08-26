package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	// ClientCommand represents the Jasper client interface as a CLI command.
	ClientCommand = "client"

	serviceFlagName = "service"

	// clientConnectionTimeout is the max time an operation can run before being
	// cancelled.
	clientConnectionTimeout = 30 * time.Second
)

// Client encapsulates the client-side interface to a Jasper service.
// Operations read from standard input (if necessary) and write the result to
// standard output.
func Client() cli.Command {
	return cli.Command{
		Name:  ClientCommand,
		Usage: "tools for making requests to Jasper services, oriented for machine use",
		Subcommands: []cli.Command{
			Manager(),
			Process(),
			Remote(),
		},
	}
}

// clientFlags returns flags used by all client commands.
func clientFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  hostFlagName,
			Usage: "the host running the Jasper service",
			Value: defaultLocalHostName,
		},
		cli.IntFlag{
			Name:  portFlagName,
			Usage: fmt.Sprintf("the port running the Jasper service (if service is '%s', default port is %d; if service is '%s', default port is %d)", RESTService, defaultRESTPort, RPCService, defaultRPCPort),
		},
		cli.StringFlag{
			Name:  joinFlagNames(serviceFlagName, "s"),
			Usage: fmt.Sprintf("the type of Jasper service ('%s' or '%s')", RESTService, RPCService),
		},
		cli.StringFlag{
			Name:  credsFilePathFlagName,
			Usage: "the path to the file containing the credentials",
		},
	}
}

// clientBefore returns the cli.BeforeFunc used by all client commands.
func clientBefore() func(*cli.Context) error {
	return mergeBeforeFuncs(
		func(c *cli.Context) error {
			service := c.String(serviceFlagName)
			if service != RESTService && service != RPCService {
				return errors.Errorf("service must be '%s' or '%s'", RESTService, RPCService)
			}
			return nil
		},
		func(c *cli.Context) error {
			if c.Int(portFlagName) != 0 {
				return nil
			}
			switch c.String(serviceFlagName) {
			case RESTService:
				if err := c.Set(portFlagName, strconv.Itoa(defaultRESTPort)); err != nil {
					return err
				}
			case RPCService:
				if err := c.Set(portFlagName, strconv.Itoa(defaultRPCPort)); err != nil {
					return err
				}
			}
			return validatePort(portFlagName)(c)
		},
	)
}
