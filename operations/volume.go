package operations

import "github.com/urfave/cli"

func Volume() cli.Command {
	return cli.Command{
		Name:  "volume",
		Usage: "manage evergreen EBS volumes",
		Subcommands: []cli.Command{
			hostCreateVolume(),
			hostDeleteVolume(),
		},
	}
}
