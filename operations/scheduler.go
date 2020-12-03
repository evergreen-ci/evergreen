package operations

import (
	"context"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func Scheduler() cli.Command {
	return cli.Command{
		Name:  "scheduler",
		Usage: "scheduler debugging utilities",
		Subcommands: []cli.Command{
			compareTasks(),
		},
		Flags: addPathFlag(),
	}
}

func compareTasks() cli.Command {
	return cli.Command{
		Name:    "compare-tasks",
		Usage:   "compares 2 tasks, showing which one would be sorted higher and why",
		Aliases: []string{"c"},
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
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

			order, logic, err := client.CompareTasks(ctx, args)
			if err != nil {
				return err
			}
			grip.Notice("The order of tasks is:")
			for _, t := range order {
				grip.Info(t)
			}
			grip.Notice("Below is the list of explicit comparisons done. The format is 'task1 task2 reason'")
			for t1, v := range logic {
				for t2, reason := range v {
					grip.Infof("%s %s %s", t1, t2, reason)
				}
			}
			return nil
		},
	}
}
