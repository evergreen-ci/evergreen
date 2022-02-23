package operations

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/urfave/cli"
)

func Version() cli.Command {
	return cli.Command{
		Name:    "version",
		Aliases: []string{"v"},
		Usage:   "prints the revision of the current binary",
		Before:  autoUpdateCLI,
		Action: func(c *cli.Context) error {
			fmt.Println(evergreen.ClientVersion)
			return nil
		},
	}
}
