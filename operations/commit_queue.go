package operations

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/rest/client"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func CommitQueue() cli.Command {
	const (
		listFlagName   = "list"
		deleteFlagName = "delete"
		itemFlagName   = "item"
	)

	return cli.Command{
		Name:  "commit-queue",
		Usage: "interact with the commit queue",
		Flags: addProjectFlag(
			cli.BoolFlag{
				Name:  listFlagName,
				Usage: "list the contents of a project's commit queue",
			},
			cli.BoolFlag{
				Name:  deleteFlagName,
				Usage: "delete an item from a project's commit queue",
			},
			cli.StringFlag{
				Name:  joinFlagNames(itemFlagName, "i"),
				Usage: "delete `ITEM`",
			}),
		Before: mergeBeforeFuncs(
			requireOnlyOneBool(listFlagName, deleteFlagName),
			requireFlagsForBool(listFlagName, projectFlagName),
			requireFlagsForBool(deleteFlagName, projectFlagName, itemFlagName),
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

			switch {
			case c.Bool(listFlagName):
				return listCommitQueue(ctx, client, projectID)
			case c.Bool(deleteFlagName):
				return deleteCommitQueueItem(ctx, client, projectID, item)
			}
			return errors.Errorf("this code should not be reachable")
		},
	}
}

func listCommitQueue(ctx context.Context, client client.Communicator, projectID string) error {
	cq, err := client.GetCommitQueue(ctx, projectID)
	if err != nil {
		return errors.Wrapf(err, "can't get commit queue list for project '%s'", projectID)
	}

	fmt.Printf("Project: %s\n", restModel.FromAPIString(cq.ProjectID))
	fmt.Printf("Queue Length: %d\n", len(cq.Queue))
	for i, item := range cq.Queue {
		fmt.Printf("\t%d: %s\n", i+1, restModel.FromAPIString(item))
	}

	return nil
}

func deleteCommitQueueItem(ctx context.Context, client client.Communicator, projectID, item string) error {
	err := client.DeleteCommitQueueItem(ctx, projectID, item)
	if err != nil {
		return errors.Wrapf(err, "can't delete item '%s' from commit queue '%s'", item, projectID)
	}

	fmt.Printf("Item '%s' deleted\n", item)

	return nil
}
