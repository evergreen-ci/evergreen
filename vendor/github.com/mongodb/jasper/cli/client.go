package cli

import (
	"time"

	"github.com/urfave/cli"
)

// clientConnectionTimeout is the max time an operation can run before being
// cancelled.
const clientConnectionTimeout = 30 * time.Second

// Client encapsulates the client-side interface to a Jasper service. Operations
// read from standard input (if necessary) and write the result to standard
// output.
func Client() cli.Command {
	return cli.Command{
		Name:  "client",
		Usage: "tools for making requests to Jasper services",
		Subcommands: []cli.Command{
			Manager(),
			Process(),
		},
	}
}
