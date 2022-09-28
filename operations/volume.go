package operations

import "github.com/urfave/cli"

func Volume() cli.Command {
	return cli.Command{
		Name:  "volume",
		Usage: "manage Evergreen EBS volumes",
		Subcommands: []cli.Command{
			hostCreateVolume(),
			hostDeleteVolume(),
			hostListVolume(),
			hostModifyVolume(),
		},
	}
}
