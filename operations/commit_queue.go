package operations

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
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
		Flags: mergeFlagSlices(addProjectFlag(), addLargeFlag(), addYesFlag(
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
			cli.StringFlag{
				Name:  joinFlagNames(patchDescriptionFlagName, "d"),
				Usage: "description for the patch",
			},
		)),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			params := mergeParams{
				projectID:   c.String(projectFlagName),
				ref:         c.String(refFlagName),
				id:          c.String(identifierFlagName),
				pause:       c.Bool(pauseFlagName),
				description: c.String(patchDescriptionFlagName),
				skipConfirm: c.Bool(yesFlagName),
				large:       c.Bool(largeFlagName),
			}

			conf, err := NewClientSettings(c.Parent().String(confFlagName))
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing legacy evergreen client")
			}

			client := conf.GetRestCommunicator(ctx)
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			return params.mergeBranch(ctx, conf, client, ac)
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
	projectID   string
	ref         string
	id          string
	pause       bool
	description string
	skipConfirm bool
	large       bool
}

func (p *mergeParams) mergeBranch(ctx context.Context, conf *ClientSettings, client client.Communicator, ac *legacyClient) error {
	if p.id == "" {
		if err := p.uploadMergePatch(conf, ac); err != nil {
			return err
		}
	}

	if !p.pause {
		position, err := client.EnqueueItem(ctx, p.projectID, p.id)
		if err != nil {
			return err
		}
		grip.Infof("Queue position is %d", position)
	}

	return nil
}

func (p *mergeParams) uploadMergePatch(conf *ClientSettings, ac *legacyClient) error {
	patchParams := &patchParams{
		Project:     p.projectID,
		SkipConfirm: p.skipConfirm,
		Description: p.description,
		Large:       p.large,
		Alias:       evergreen.CommitQueueAlias,
	}

	if err := patchParams.loadProject(conf); err != nil {
		return errors.Wrap(err, "invalid project ID")
	}

	ref, err := ac.GetProjectRef(patchParams.Project)
	if err != nil {
		if apiErr, ok := err.(APIError); ok && apiErr.code == http.StatusNotFound {
			err = errors.WithStack(err)
		}
		return errors.Wrap(err, "can't get project ref")
	}

	diffData, err := getFeaturePatchInfo(ref.Branch, p.ref)
	if err != nil {
		return errors.Wrap(err, "can't generate patches")
	}

	if p.description == "" {
		p.description, err = gitCmd("rev-parse", "--abbrev-ref", p.ref)
		if err != nil {
			return errors.Wrapf(err, "can't get branch name for ref %s", p.ref)
		}
	}

	patch, err := patchParams.createPatch(ac, conf, diffData)
	if err != nil {
		return err
	}

	p.id = patch.Id.Hex()

	return nil
}

func getFeaturePatchInfo(projectBranch, ref string) (*localDiff, error) {
	upstream := projectBranch + "@{upstream}"
	revisionRange := upstream + ".." + ref

	stat, err := gitCmd("diff", "--no-ext-diff", "--stat", revisionRange)
	if err != nil {
		return nil, errors.Wrap(err, "can't get stat")
	}

	log, err := gitCmd("log", "--oneline", revisionRange)
	if err != nil {
		return nil, errors.Wrap(err, "can't get log")
	}

	patches, err := gitCmd("format-patch", "--no-ext-diff", "--stdout", revisionRange)
	if err != nil {
		return nil, errors.Wrap(err, "can't generate patch")
	}

	mergeBase, err := gitMergeBase(upstream, ref)
	if err != nil {
		return nil, errors.Errorf("Error getting merge base: %v", err)
	}

	return &localDiff{
		fullPatch:    patches,
		patchSummary: stat,
		log:          log,
		base:         mergeBase,
	}, nil
}
