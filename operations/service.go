package operations

import "github.com/urfave/cli"

func Service() cli.Command {
	return cli.Command{
		Name:  "service",
		Usage: "run evergreen services",
		Subcommands: []cli.Command{
			startRunnerService(),
			startWebService(),
			handcrankRunner(),
		},
	}
}
