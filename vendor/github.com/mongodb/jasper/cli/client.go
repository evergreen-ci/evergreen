package cli

import (
	"time"

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
