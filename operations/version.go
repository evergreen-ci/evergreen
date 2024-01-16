package operations

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/urfave/cli"
)

const buildRevisionFlag = "build-revision"

func Version() cli.Command {
	return cli.Command{
		Name:    "version",
		Aliases: []string{"v"},
		Usage:   "prints the revision of the current binary",
		Flags: []cli.Flag{
			cli.BoolFlag{
				Name:  joinFlagNames(buildRevisionFlag, "b"),
				Usage: "print the build revision instead of the client version",
			},
		},
		Action: func(c *cli.Context) error {
			if c.Bool(buildRevisionFlag) {
				fmt.Println(evergreen.BuildRevision)
			} else {
				fmt.Println(evergreen.ClientVersion)
			}

			return nil
		},
	}
}
