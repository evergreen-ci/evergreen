package operations

import "github.com/urfave/cli"

func Host() cli.Command {
	return cli.Command{
		Name:  "host",
		Usage: "manage Evergreen spawn and build hosts",
		Subcommands: []cli.Command{
			hostCreate(),
			hostModify(),
			hostConfigure(),
			hostStop(),
			hostStart(),
			hostAttach(),
			hostDetach(),
			hostList(),
			hostTerminate(),
			hostProvision(),
			hostSetup(),
			hostFetch(),
			hostSSH(),
			hostRunCommand(),
			hostRsync(),
			hostFindBy(),
		},
	}
}
