package operations

import (
	"context"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	legacyFlagName = "legacy"
)

func Scheduler() cli.Command {
	return cli.Command{
		Name:  "scheduler",
		Usage: "scheduler debugging utilities",
		Subcommands: []cli.Command{
			compareTasks(),
		},
		Flags: mergeFlagSlices(addPathFlag(
			cli.BoolFlag{
				Name:  joinFlagNames(legacyFlagName, "l"),
				Usage: "use the legacy planner if true (default is tunable)",
			})),
	}
}

func compareTasks() cli.Command {
	return cli.Command{
		Name:    "compare-tasks",
		Usage:   "compares 2 tasks, showing which one would be sorted higher and why",
		Aliases: []string{"c"},
		Before:  setPlainLogger,
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			useLegacy := c.Bool(legacyFlagName)
			args := c.Args()
			if len(args) < 2 {
				return errors.New("must provide at least 2 task IDs as arguments")
			}

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			ctx := context.Background()
			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			order, logic, err := client.CompareTasks(ctx, args, useLegacy)
			if err != nil {
				return err
			}
			grip.Notice("The order of tasks is:")
			for _, t := range order {
				grip.Info(t)
			}
			if useLegacy {
				grip.Notice("Below is the list of explicit comparisons done. The format is 'task1 task2 reason'")
				for t1, v := range logic {
					for t2, reason := range v {
						grip.Infof("%s %s %s", t1, t2, reason)
					}
				}
			} else {
				grip.Notice("Explicit comparisons and reasoning for tunable planner coming soon.")
			}

			return nil
		},
	}
}
