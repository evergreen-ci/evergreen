package operations

import "github.com/urfave/cli"

func Host() cli.Command {
	return cli.Command{
		Name:  "host",
		Usage: "manage evergreen spawn and build hosts",
		Subcommands: []cli.Command{
			hostCreate(),
			hostModify(),
			hostStop(),
			hostStart(),
			hostList(),
			hostTerminate(),
			hostSetup(),
			hostTeardown(),
		},
	}
}
