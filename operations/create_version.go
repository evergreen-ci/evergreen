package operations

import (
	"context"
	"io/ioutil"
	"os"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func CreateVersion() cli.Command {
	return cli.Command{
		Name:   "create-version",
		Before: mergeBeforeFuncs(requirePathFlag, setPlainLogger),
		Usage:  "creates a set of runnable tasks from a config file",
		Flags: mergeFlagSlices(addPathFlag(), addProjectFlag(
			cli.StringFlag{
				Name:  joinFlagNames(messageFlagName, "m"),
				Usage: "description for this version",
			},
			cli.BoolFlag{
				Name:  joinFlagNames(activeFlagName, "a"),
				Usage: "true to schedule this version to run",
			})),
		Action: func(c *cli.Context) error {
			project := c.String(projectFlagName)
			if project == "" {
				return errors.New("must specify a project")
			}
			confPath := c.Parent().String(confFlagName)
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			ctx := context.Background()
			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			filePath := c.String(pathFlagName)
			f, err := os.Open(filePath)
			if err != nil {
				return errors.Wrapf(err, "error opening file %s", filePath)
			}
			config, err := ioutil.ReadAll(f)
			if err != nil {
				return errors.Wrapf(err, "error reading file %s", filePath)
			}
			v, err := client.CreateVersionFromConfig(ctx, project, c.String(messageFlagName), c.Bool(activeFlagName), config)
			if err != nil {
				return errors.Wrap(err, "error creating version")
			}
			if v == nil {
				return errors.New("no version created due to unknown error")
			}
			grip.Infof("version '%s' successfully created", v.Id)

			return nil
		},
	}
}
