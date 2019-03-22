package operations

import (
	"context"

	"github.com/evergreen-ci/evergreen/rest/client"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	itemFlagName       = "item"
	refFlagName        = "ref"
	pauseFlagName      = "pause"
	identifierFlagName = "identifier"
)

func CommitQueue() cli.Command {
	return cli.Command{
		Name:  "commit-queue",
		Usage: "interact with the commit queue",
		Subcommands: []cli.Command{
			listQueue(),
			deleteItem(),
			mergeCommand(),
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
			confPath := c.Parent().Parent().String(confFlagName)
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
			confPath := c.Parent().Parent().String(confFlagName)
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

func mergeCommand() cli.Command {
	return cli.Command{
		Name:  "merge",
		Usage: "test and merge a feature branch",
		Flags: addProjectFlag(
			cli.StringFlag{
				Name:  joinFlagNames(refFlagName, "r"),
				Usage: "merge branch `REF`",
				Value: "HEAD",
			},
			cli.StringFlag{
				Name:  identifierFlagName,
				Usage: "finalize a preexisting item with `ID`",
			},
			cli.BoolFlag{
				Name:  pauseFlagName,
				Usage: "wait to enqueue an item until finalized",
			},
		),
		Before: mergeBeforeFuncs(
			setPlainLogger,
		),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			projectID := c.String(projectFlagName)
			ref := c.String(refFlagName)
			id := c.String(identifierFlagName)
			pause := c.Bool(pauseFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			client := conf.GetRestCommunicator(ctx)
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			params := mergeParams{
				projectID: projectID,
				ref:       ref,
				id:        id,
				pause:     pause,
			}
			return params.mergeBranch(ctx, client, ac)
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

type mergeParams struct {
	projectID string
	ref       string
	id        string
	pause     bool
}

func (p *mergeParams) mergeBranch(ctx context.Context, client client.Communicator, ac *legacyClient) error {
	if p.id == "" {
		if p.projectID == "" {
			return errors.New("no project ID provided")
		}

		projectRef, err := ac.GetProjectRef(p.projectID)
		if err != nil {
			return err
		}

		diffData, err := getFeaturePatchInfo(projectRef.Branch, p.ref)
		if err != nil {
			return errors.Wrap(err, "can't generate patches")
		}

		if len(diffData.patches) == 0 {
			if !confirm("Patch submission is empty. Continue?(y/n)", true) {
				return nil
			}
		}

		grip.Info(diffData.stat)
		grip.Info(diffData.log)
		if !confirm("This is a summary of the merge patch to be submitted. Continue? (y/n):", true) {
			return nil
		}

		p.id, err = uploadPatches(ctx, client, p.projectID, diffData.patches)
		if err != nil {
			return err
		}
	}

	if !p.pause {
		if err := enqueueItem(ctx, client, p.id); err != nil {
			return err
		}
	}

	return nil
}

type diffData struct {
	stat    string
	log     string
	patches string
}

func getFeaturePatchInfo(projectBranch, ref string) (diffData, error) {
	upstream := projectBranch + "@{upstream}"
	revisionRange := upstream + ".." + ref

	stat, err := gitCmd("diff", "--no-ext-diff", "--stat", revisionRange)
	if err != nil {
		return diffData{}, errors.Wrap(err, "can't get stat")
	}

	log, err := gitCmd("log", "--oneline", revisionRange)
	if err != nil {
		return diffData{}, errors.Wrap(err, "can't get log")
	}

	patches, err := gitCmd("format-patch", "--no-ext-diff", "--stdout", revisionRange)
	if err != nil {
		return diffData{}, errors.Wrap(err, "can't generate patch")
	}

	return diffData{stat, log, patches}, nil
}

func uploadPatches(ctx context.Context, client client.Communicator, projectID, patches string) (string, error) {
	id, err := client.UploadPatches(ctx, projectID, patches)
	if err != nil {
		return "", err
	}

	grip.Infof("Item ID is '%s'", id)
	return id, nil
}

func enqueueItem(ctx context.Context, client client.Communicator, id string) error {
	position, err := client.EnqueueItem(ctx, id)
	if err != nil {
		return err
	}

	grip.Infof("Queue position is '%d'", position)
	return nil
}
