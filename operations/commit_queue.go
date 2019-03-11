package operations

import (
	"context"

	"github.com/evergreen-ci/evergreen/rest/client"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const itemFlagName = "item"

func CommitQueue() cli.Command {
	return cli.Command{
		Name:  "commit-queue",
		Usage: "interact with the commit queue",
		Subcommands: []cli.Command{
			listQueue(),
			deleteItem(),
		},
	}
}

func listQueue() cli.Command {
	return cli.Command{
		Name:  "list",
		Usage: "list the contents of a project's commit queue",
		Flags: addProjectFlag(),
		Before: mergeBeforeFuncs(
			requireStringFlag(projectFlagName),
			setPlainLogger,
		),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			projectID := c.String(projectFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.GetRestCommunicator(ctx)
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			return listCommitQueue(ctx, client, projectID)
		},
	}
}

func deleteItem() cli.Command {
	return cli.Command{
		Name:  "delete",
		Usage: "delete an item from a project's commit queue",
		Flags: addProjectFlag(cli.StringFlag{
			Name:  joinFlagNames(itemFlagName, "i"),
			Usage: "delete `ITEM`",
		}),
		Before: mergeBeforeFuncs(
			requireStringFlag(projectFlagName),
			requireStringFlag(itemFlagName),
			setPlainLogger,
		),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			projectID := c.String(projectFlagName)
			item := c.String(itemFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			client := conf.GetRestCommunicator(ctx)
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			return deleteCommitQueueItem(ctx, client, projectID, item)
		},
	}
}

func listCommitQueue(ctx context.Context, client client.Communicator, projectID string) error {
	cq, err := client.GetCommitQueue(ctx, projectID)
	if err != nil {
		return err
	}

	grip.Infof("Project: %s\n", restModel.FromAPIString(cq.ProjectID))
	grip.Infof("Queue Length: %d\n", len(cq.Queue))
	for i, item := range cq.Queue {
		grip.Infof("\t%d: %s\n", i+1, restModel.FromAPIString(item.Issue))
		if len(item.Modules) > 0 {
			grip.Info("\tModules:\n")
			for j, module := range item.Modules {
				grip.Infof("\t\t%d: %s (%s)\n", j+1, restModel.FromAPIString(module.Module), restModel.FromAPIString(module.Issue))
			}
		}
	}

	return nil
}

func deleteCommitQueueItem(ctx context.Context, client client.Communicator, projectID, item string) error {
	err := client.DeleteCommitQueueItem(ctx, projectID, item)
	if err != nil {
		return err
	}

	grip.Infof("Item '%s' deleted\n", item)

	return nil
}
