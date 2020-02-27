package operations

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/rest/client"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	itemFlagName    = "item"
	pauseFlagName   = "pause"
	resumeFlagName  = "resume"
	commitsFlagName = "commits"
)

func CommitQueue() cli.Command {
	return cli.Command{
		Name:  "commit-queue",
		Usage: "interact with the commit queue",
		Subcommands: []cli.Command{
			listQueue(),
			deleteItem(),
			mergeCommand(),
			setModuleCommand(),
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
			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing legacy evergreen client")
			}

			return listCommitQueue(ctx, client, ac, projectID, conf.UIServerHost)
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
			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			return deleteCommitQueueItem(ctx, client, projectID, item)
		},
	}
}

func mergeCommand() cli.Command {
	return cli.Command{
		Name:  "merge",
		Usage: "test and merge a feature branch",
		Flags: mergeFlagSlices(addProjectFlag(), addLargeFlag(), addRefFlag(), addCommitsFlag(), addYesFlag(
			cli.StringFlag{
				Name:  joinFlagNames(resumeFlagName, "r", patchFinalizeFlagName, "f"),
				Usage: "resume testing a preexisting item with `ID`",
			},
			cli.BoolFlag{
				Name:  pauseFlagName,
				Usage: "wait to enqueue an item until finalized",
			},
			cli.BoolFlag{
				Name:  forceFlagName,
				Usage: "force item to front of queue",
			},
		)),
		Before: mergeBeforeFuncs(
			setPlainLogger,
			mutuallyExclusiveArgs(false, refFlagName, commitsFlagName),
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			params := mergeParams{
				projectID:   c.String(projectFlagName),
				ref:         c.String(refFlagName),
				commits:     c.String(commitsFlagName),
				id:          c.String(resumeFlagName),
				pause:       c.Bool(pauseFlagName),
				skipConfirm: c.Bool(yesFlagName),
				large:       c.Bool(largeFlagName),
				force:       c.Bool(forceFlagName),
			}
			if params.force && !confirm("Forcing item to front of queue will be reported. Continue? (y/n)", false) {
				return errors.New("Merge aborted.")
			}
			conf, err := NewClientSettings(c.Parent().Parent().String(confFlagName))
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing legacy evergreen client")
			}

			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			return params.mergeBranch(ctx, conf, client, ac)
		},
	}
}

func setModuleCommand() cli.Command {
	return cli.Command{
		Name:  "set-module",
		Usage: "update or add module to an existing merge patch",
		Flags: mergeFlagSlices(addLargeFlag(), addPatchIDFlag(), addModuleFlag(), addYesFlag(), addRefFlag(), addCommitsFlag()),
		Before: mergeBeforeFuncs(
			requirePatchIDFlag,
			requireModuleFlag,
			setPlainLogger,
			mutuallyExclusiveArgs(false, refFlagName, commitsFlagName),
		),
		Action: func(c *cli.Context) error {
			params := moduleParams{
				patchID:     c.String(patchIDFlagName),
				module:      c.String(moduleFlagName),
				ref:         c.String(refFlagName),
				commits:     c.String(commitsFlagName),
				large:       c.Bool(largeFlagName),
				skipConfirm: c.Bool(yesFlagName),
			}

			conf, err := NewClientSettings(c.Parent().Parent().String(confFlagName))
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}
			ac, rc, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			return errors.WithStack(params.addModule(ac, rc))
		},
	}
}

func listCommitQueue(ctx context.Context, client client.Communicator, ac *legacyClient, projectID string, uiServerHost string) error {
	cq, err := client.GetCommitQueue(ctx, projectID)
	if err != nil {
		return err
	}
	projectRef, err := ac.GetProjectRef(projectID)
	if err != nil {
		return errors.Wrapf(err, "can't find project for queue id '%s'", projectID)
	}
	grip.Infof("Project: %s\n", projectID)
	grip.Infof("Type of queue: %s\n", projectRef.CommitQueue.PatchType)

	if projectRef.CommitQueue.PatchType == commitqueue.PRPatchType {
		grip.Infof("Owner: %s\n", projectRef.Owner)
		grip.Infof("Repo: %s\n", projectRef.Repo)
	}

	grip.Infof("Queue Length: %d\n", len(cq.Queue))
	for i, item := range cq.Queue {
		grip.Infof("%d:", i)
		if projectRef.CommitQueue.PatchType == commitqueue.PRPatchType {
			listPRCommitQueueItem(item, projectRef, uiServerHost)
		}
		if projectRef.CommitQueue.PatchType == commitqueue.CLIPatchType {
			listCLICommitQueueItem(item, ac, uiServerHost)
		}
		listModules(item)
	}

	return nil
}

func listPRCommitQueueItem(item restModel.APICommitQueueItem, projectRef *model.ProjectRef, uiServerHost string) {
	issue := restModel.FromStringPtr(item.Issue)
	prDisplay := `
           PR # : %s
            URL : %s
`
	url := fmt.Sprintf("https://github.com/%s/%s/pull/%s", projectRef.Owner, projectRef.Repo, issue)
	grip.Infof(prDisplay, issue, url)

	prDisplayVersion := "          Build : %s/version/%s"
	if restModel.FromStringPtr(item.Version) != "" {
		grip.Infof(prDisplayVersion, uiServerHost, restModel.FromStringPtr(item.Version))
	}

	grip.Info("\n")
}

func listCLICommitQueueItem(item restModel.APICommitQueueItem, ac *legacyClient, uiServerHost string) {
	issue := restModel.FromStringPtr(item.Issue)
	p, err := ac.GetPatch(issue)
	if err != nil {
		grip.Error(message.WrapErrorf(err, "\terror getting patch for issue '%s'", issue))
		return
	}

	if p.Author != "" {
		grip.Infof("Author: %s", p.Author)
	}
	disp, err := getPatchDisplay(p, false, uiServerHost)
	if err != nil {
		grip.Error(message.WrapError(err, "\terror getting patch display"))
		return
	}
	grip.Info(disp)
}

func listModules(item restModel.APICommitQueueItem) {
	if len(item.Modules) > 0 {
		grip.Infof("\tModules :")

		for j, module := range item.Modules {
			grip.Infof("\t\t%d: %s (%s)\n", j+1, restModel.FromStringPtr(module.Module), restModel.FromStringPtr(module.Issue))
		}
		grip.Info("\n")
	}
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
	commits     string
	ref         string
	id          string
	pause       bool
	skipConfirm bool
	large       bool
	force       bool
}

func (p *mergeParams) mergeBranch(ctx context.Context, conf *ClientSettings, client client.Communicator, ac *legacyClient) error {
	if p.id == "" {
		if err := p.uploadMergePatch(conf, ac); err != nil {
			return err
		}
	}
	if p.pause {
		return nil
	}
	position, err := client.EnqueueItem(ctx, p.id, p.force)
	if err != nil {
		return err
	}
	grip.Infof("Queue position is %d", position)

	return nil
}

func (p *mergeParams) uploadMergePatch(conf *ClientSettings, ac *legacyClient) error {
	patchParams := &patchParams{
		Project:     p.projectID,
		SkipConfirm: p.skipConfirm,
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
	if !ref.CommitQueue.Enabled || ref.CommitQueue.PatchType != commitqueue.CLIPatchType {
		return errors.New("CLI commit queue not enabled for project")
	}

	commitCount, err := gitCommitCount(ref.Branch, p.ref, p.commits)
	if err != nil {
		return errors.Wrap(err, "can't get commit count")
	}
	if commitCount > 1 && !confirm("Commit queue patch has multiple commits. Continue? (y/n):", false) {
		return errors.New("patch aborted")
	}

	if err = isValidCommitsFormat(p.commits); err != nil {
		return err
	}

	diffData, err := loadGitData(ref.Branch, p.ref, p.commits, true)
	if err != nil {
		return errors.Wrap(err, "can't generate patches")
	}

	if commitCount > 0 {
		patchParams.Description, err = gitCommitMessages(ref.Branch, p.ref, p.commits)
		if err != nil {
			return errors.Wrap(err, "can't get commit messages")
		}
	}

	patch, err := patchParams.createPatch(ac, conf, diffData)
	if err != nil {
		return err
	}

	p.id = patch.Id.Hex()

	return nil
}

type moduleParams struct {
	patchID     string
	module      string
	ref         string
	commits     string
	large       bool
	skipConfirm bool
}

func (p *moduleParams) addModule(ac *legacyClient, rc *legacyClient) error {
	proj, err := rc.GetPatchedConfig(p.patchID)
	if err != nil {
		return err
	}
	module, err := proj.GetModuleByName(p.module)
	if err != nil {
		return errors.Wrapf(err, "could not find module '%s'", p.module)
	}

	commitCount, err := gitCommitCount(module.Branch, p.ref, p.commits)
	if err != nil {
		return errors.Wrap(err, "can't get commit count")
	}
	if commitCount == 0 {
		return errors.New("No commits for module")
	}
	if commitCount > 1 && !confirm("Commit queue module patch has multiple commits. Continue? (y/n):", false) {
		return errors.New("module patch aborted")
	}

	patch, err := rc.GetPatch(p.patchID)
	if err != nil {
		return errors.Wrapf(err, "can't get patch '%s'", p.patchID)
	}

	if err = isValidCommitsFormat(p.commits); err != nil {
		return err
	}

	message := ""
	if patch.Description == "" {
		message, err = gitCommitMessages(module.Branch, p.ref, p.commits)
		if err != nil {
			return errors.Wrap(err, "can't get module commit messages")
		}
	}

	diffData, err := loadGitData(module.Branch, p.ref, p.commits, true)
	if err != nil {
		return errors.Wrap(err, "can't get patch data")
	}
	if err = validatePatchSize(diffData, p.large); err != nil {
		return err
	}

	if !p.skipConfirm {
		grip.InfoWhen(diffData.patchSummary != "", diffData.patchSummary)
		if !confirm("This is a summary of the patch to be submitted. Continue? (y/n):", true) {
			return nil
		}
	}

	params := UpdatePatchModuleParams{
		patchID: p.patchID,
		module:  p.module,
		patch:   diffData.fullPatch,
		base:    diffData.base,
		message: message,
	}
	err = ac.UpdatePatchModule(params)
	if err != nil {
		return errors.WithStack(err)
	}
	grip.Info("Module updated")
	return nil
}
