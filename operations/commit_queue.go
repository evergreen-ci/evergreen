package operations

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/client"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	itemFlagName        = "item"
	pauseFlagName       = "pause"
	resumeFlagName      = "resume"
	commitsFlagName     = "commits"
	existingPatchFlag   = "existing-patch"
	backportProjectFlag = "backport-project"
	commitShaFlag       = "commit-sha"
	commitMessageFlag   = "commit-message"
	githubAuthorFlag    = "author"
	patchAuthorFlag     = "author"

	noCommits             = "No Commits Added"
	commitQueuePatchLabel = "Commit Queue Merge:"
	commitFmtString       = "'%s' into '%s/%s:%s'"
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
			enqueuePatch(),
			backport(),
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
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			return listCommitQueue(ctx, client, ac, projectID, conf.UIServerHost)
		},
	}
}

func deleteItem() cli.Command {
	return cli.Command{
		Name:  "delete",
		Usage: "delete a patch from a project's commit queue",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  joinFlagNames(itemFlagName, "i"),
				Usage: "specify the patch ID or PR number to delete",
			},
		},
		Before: mergeBeforeFuncs(
			requireStringFlag(itemFlagName),
			setPlainLogger,
		),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			item := c.String(itemFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			showCQMessageForPatch(ctx, client, item)
			return deleteCommitQueueItem(ctx, client, item)
		},
	}
}

func mergeCommand() cli.Command {
	return cli.Command{
		Name:  "merge",
		Usage: "test and merge a feature branch",
		Flags: mergeFlagSlices(addProjectFlag(), addLargeFlag(), addRefFlag(), addCommitsFlag(), addSkipConfirmFlag(
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
			cli.StringFlag{
				Name: githubAuthorFlag,
				Usage: "optionally define the patch author by providing a GitHub username (this will only work if the " +
					"submitter has permission and the provided user has provided the username in their Evergreen settings)",
			},
		)),
		Before: mergeBeforeFuncs(
			autoUpdateCLI,
			setPlainLogger,
			mutuallyExclusiveArgs(false, refFlagName, commitsFlagName),
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ref := c.String(refFlagName)
			commits := c.String(commitsFlagName)
			if commits != "" {
				ref = ""
			}

			params := mergeParams{
				project:      c.String(projectFlagName),
				ref:          ref,
				commits:      commits,
				id:           c.String(resumeFlagName),
				pause:        c.Bool(pauseFlagName),
				skipConfirm:  c.Bool(skipConfirmFlagName),
				large:        c.Bool(largeFlagName),
				force:        c.Bool(forceFlagName),
				githubAuthor: c.String(githubAuthorFlag),
			}
			if params.force && !params.skipConfirm && !confirm(
				"Forcing item to front of queue will be reported. Continue?", false) {
				return errors.New("Merge aborted.")
			}
			conf, err := NewClientSettings(c.Parent().Parent().String(confFlagName))
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			return params.mergeBranch(ctx, conf, client, ac)
		},
	}
}

func setModuleCommand() cli.Command {
	return cli.Command{
		Name:  "set-module",
		Usage: "update or add module to an existing merge patch",
		Flags: mergeFlagSlices(addLargeFlag(), addPatchIDFlag(), addModuleFlag(), addSkipConfirmFlag(), addRefFlag(), addCommitsFlag()),
		Before: mergeBeforeFuncs(
			autoUpdateCLI,
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
				skipConfirm: c.Bool(skipConfirmFlagName),
			}

			conf, err := NewClientSettings(c.Parent().Parent().String(confFlagName))
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			ac, rc, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}
			ctx := context.Background()
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()
			showCQMessageForPatch(ctx, client, params.patchID)

			return errors.WithStack(params.addModule(ac, rc))
		},
	}
}

func enqueuePatch() cli.Command {
	return cli.Command{
		Name:  "enqueue-patch",
		Usage: "enqueue an existing patch on the commit queue",
		Flags: mergeFlagSlices(addSkipConfirmFlag(), addPatchIDFlag(
			cli.BoolFlag{
				Name:  forceFlagName,
				Usage: "force item to front of queue",
			},
			cli.StringFlag{
				Name:  commitMessageFlag,
				Usage: "commit message for the new commit (default is the existing patch description)",
			},
		)),
		Before: mergeBeforeFuncs(
			autoUpdateCLI,
			checkCommitMessageFlag,
			requirePatchIDFlag,
			setPlainLogger,
		),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().Parent().String(confFlagName)
			patchID := c.String(patchIDFlagName)
			commitMessage := c.String(commitMessageFlag)
			force := c.Bool(forceFlagName)
			skipConfirm := c.Bool(skipConfirmFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			// verify the patch can be enqueued
			existingPatch, err := ac.GetPatch(patchID)
			if err != nil {
				return errors.Wrapf(err, "getting patch '%s'", patchID)
			}
			if !existingPatch.HasValidGitInfo() {
				return errors.Errorf("patch '%s' is not eligible to be enqueued", patchID)
			}

			// confirm multiple commits
			multipleCommits := false
			for _, p := range existingPatch.Patches {
				if len(p.PatchSet.CommitMessages) > 1 {
					multipleCommits = true
				}
			}
			if multipleCommits && !skipConfirm &&
				!confirm("Original patch has multiple commits (these will be tested together but merged separately). Continue?", false) {
				return errors.New("enqueue aborted")
			}

			showCQMessageForPatch(ctx, client, patchID)

			if commitMessage == "" {
				commitMessage = existingPatch.Description
			}

			// create the new merge patch
			mergePatch, err := client.CreatePatchForMerge(ctx, patchID, commitMessage)
			if err != nil {
				return errors.Wrap(err, "creating commit queue patch")
			}
			uiV2, err := client.GetUiV2URL(ctx)
			if err != nil {
				return errors.Wrap(err, "getting UI v2 URL")
			}
			patchDisp, err := getAPICommitQueuePatchDisplay(mergePatch, false, uiV2)
			if err != nil {
				grip.Errorf("can't print patch display for new patch '%s'", mergePatch.Id)
			}
			grip.Info("Patch successfully created.")
			grip.Info(patchDisp)

			// enqueue the patch
			position, err := client.EnqueueItem(ctx, utility.FromStringPtr(mergePatch.Id), force)
			if err != nil {
				return errors.Wrap(err, "enqueueing new patch")
			}
			grip.Infof("Queue position is %d.", position)

			return nil
		},
	}
}

func backport() cli.Command {
	return cli.Command{
		Name: "backport",
		Usage: "Backport low-risk commits -- note the patch created from this will create a regular patch to validate " +
			"the backport, and if the patch succeeds then the changes are automatically added to the commit queue.",
		Flags: mergeFlagSlices(
			addPatchFinalizeFlag(),
			addPatchBrowseFlag(
				cli.StringSliceFlag{
					Name:  joinFlagNames(tasksFlagName, "t"),
					Usage: "tasks to validate the backport",
				},
				cli.StringSliceFlag{
					Name:  joinFlagNames(variantsFlagName, "v"),
					Usage: "variants to validate the backport",
				},
				cli.StringFlag{
					Name:  joinFlagNames(patchAliasFlagName, "a"),
					Usage: "patch alias to select tasks/variants to validate the backport",
				},
				cli.StringFlag{
					Name:  joinFlagNames(existingPatchFlag, "e"),
					Usage: "existing commit queue patch",
				},
				cli.StringFlag{
					Name:  joinFlagNames(commitShaFlag, "s"),
					Usage: "existing commit SHA to backport",
				},
				cli.StringFlag{
					Name:  joinFlagNames(backportProjectFlag, "b"),
					Usage: "project to backport onto",
				},
			)),
		Before: mergeBeforeFuncs(
			autoUpdateCLI,
			setPlainLogger,
			requireStringFlag(backportProjectFlag),
			mutuallyExclusiveArgs(true, existingPatchFlag, commitShaFlag),
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			confPath := c.Parent().Parent().String(confFlagName)
			patchParams := &patchParams{
				Tasks:    c.StringSlice(tasksFlagName),
				Variants: c.StringSlice(variantsFlagName),
				Alias:    c.String(patchAliasFlagName),
				Finalize: c.Bool(patchFinalizeFlagName),
				Project:  c.String(backportProjectFlag),
				Browse:   c.Bool(patchBrowseFlagName),
				BackportOf: patch.BackportInfo{
					PatchID: c.String(existingPatchFlag),
					SHA:     c.String(commitShaFlag),
				},
			}

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}
			showCQMessageForProject(ac, patchParams.Project)
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			if _, err = patchParams.validatePatchCommand(ctx, conf, ac, client); err != nil {
				return err
			}

			if len(patchParams.BackportOf.PatchID) > 0 {
				var existingPatch *patch.Patch
				existingPatch, err = ac.GetPatch(patchParams.BackportOf.PatchID)
				if err != nil {
					return errors.Wrapf(err, "getting existing patch '%s'", patchParams.BackportOf.PatchID)
				}
				if !existingPatch.IsCommitQueuePatch() {
					return errors.Errorf("patch '%s' is not a commit queue patch", patchParams.BackportOf.PatchID)
				}
			}

			uiV2, err := client.GetUiV2URL(ctx)
			if err != nil {
				return errors.Wrap(err, "getting UI v2 URL")
			}
			latestVersions, err := client.GetRecentVersionsForProject(ctx, patchParams.Project, evergreen.RepotrackerVersionRequester)
			if err != nil {
				return errors.Wrapf(err, "getting latest repotracker version for project '%s'", patchParams.Project)
			}
			if len(latestVersions) == 0 {
				return errors.Errorf("no repotracker versions exist in project '%s'", patchParams.Project)
			}
			gitMetadata, err := getGitConfigMetadata()
			if err != nil {
				return errors.Wrap(err, "getting git metadata")
			}
			var backportPatch *patch.Patch
			backportPatch, err = patchParams.createPatch(ac, &localDiff{base: utility.FromStringPtr(latestVersions[0].Revision), gitMetadata: gitMetadata})
			if err != nil {
				return errors.Wrap(err, "uploading backport patch")
			}

			if err = patchParams.displayPatch(backportPatch, uiV2, false); err != nil {
				return errors.Wrap(err, "getting result display")
			}

			return nil
		},
	}
}

func listCommitQueue(ctx context.Context, client client.Communicator, ac *legacyClient, projectID string, uiServerHost string) error {
	projectRef, err := ac.GetProjectRef(projectID)
	if err != nil {
		return errors.Wrapf(err, "finding project '%s' for commit queue", projectID)
	}
	cq, err := client.GetCommitQueue(ctx, projectRef.Id)
	if err != nil {
		return err
	}
	grip.Infof("Project: %s\n", projectID)
	if projectRef.CommitQueue.Message != "" {
		grip.Infof("Message: %s\n", projectRef.CommitQueue.Message)
	}

	grip.Infof("Queue Length: %d\n", len(cq.Queue))
	for i, item := range cq.Queue {
		grip.Infof("%d:", i)
		if utility.FromStringPtr(item.Source) == commitqueue.SourcePullRequest {
			listPRCommitQueueItem(item, projectRef, uiServerHost)
		} else if utility.FromStringPtr(item.Source) == commitqueue.SourceDiff {
			listCLICommitQueueItem(item, ac, uiServerHost)
		}
		listModules(item)
	}

	return nil
}

func listPRCommitQueueItem(item restModel.APICommitQueueItem, projectRef *model.ProjectRef, uiServerHost string) {
	issue := utility.FromStringPtr(item.Issue)
	prDisplay := `
            PR # : %s
            URL : %s
`
	url := fmt.Sprintf("https://github.com/%s/%s/pull/%s", projectRef.Owner, projectRef.Repo, issue)
	grip.Infof(prDisplay, issue, url)

	prDisplayVersion := "          Build : %s/version/%s"
	if utility.FromStringPtr(item.Version) != "" {
		grip.Infof(prDisplayVersion, uiServerHost, utility.FromStringPtr(item.Version))
	}

	grip.Info("\n")
}

func listCLICommitQueueItem(item restModel.APICommitQueueItem, ac *legacyClient, uiServerHost string) {
	issue := utility.FromStringPtr(item.Issue)
	p, err := ac.GetPatch(issue)
	if err != nil {
		grip.Error(errors.Wrapf(err, "getting patch for issue '%s'", issue))
		return
	}

	if p.Author != "" {
		grip.Infof("Author: %s", p.Author)
	}
	disp, err := getPatchDisplay(p, false, uiServerHost, false)
	if err != nil {
		grip.Error(errors.Wrapf(err, "getting patch display summary for patch '%s'", p.Id.Hex()))
		return
	}
	grip.Info(disp)
}

func listModules(item restModel.APICommitQueueItem) {
	if len(item.Modules) > 0 {
		grip.Infof("\tModules :")

		for j, module := range item.Modules {
			grip.Infof("\t\t%d: %s (%s)\n", j+1, utility.FromStringPtr(module.Module), utility.FromStringPtr(module.Issue))
		}
		grip.Info("\n")
	}
}

func deleteCommitQueueItem(ctx context.Context, client client.Communicator, item string) error {
	err := client.DeleteCommitQueueItem(ctx, item)
	if err != nil {
		return err
	}

	grip.Infof("Item '%s' deleted\n", item)

	return nil
}

type mergeParams struct {
	project      string
	commits      string
	ref          string
	id           string
	pause        bool
	skipConfirm  bool
	large        bool
	force        bool
	githubAuthor string
}

func (p *mergeParams) mergeBranch(ctx context.Context, conf *ClientSettings, client client.Communicator, ac *legacyClient) error {
	if p.id == "" {
		showCQMessageForProject(ac, p.project)
		uiV2, err := client.GetUiV2URL(ctx)
		if err != nil {
			return errors.Wrap(err, "getting UI v2 URL")
		}
		if err := p.uploadMergePatch(conf, ac, uiV2); err != nil {
			return err
		}
	} else {
		showCQMessageForPatch(ctx, client, p.id)
	}
	if p.pause {
		return nil
	}
	position, err := client.EnqueueItem(ctx, p.id, p.force)
	if err != nil {
		return err
	}
	grip.Infof("Queue position is %d.", position)

	return nil
}

func (p *mergeParams) uploadMergePatch(conf *ClientSettings, ac *legacyClient, uiV2Url string) error {
	patchParams := &patchParams{
		Project:      p.project,
		SkipConfirm:  p.skipConfirm,
		Large:        p.large,
		Alias:        evergreen.CommitQueueAlias,
		GithubAuthor: p.githubAuthor,
	}

	if err := patchParams.loadProject(conf); err != nil {
		return errors.Wrap(err, "invalid project ID")
	}

	ref, err := ac.GetProjectRef(patchParams.Project)
	if err != nil {
		if apiErr, ok := err.(APIError); ok && apiErr.code == http.StatusNotFound {
			err = errors.WithStack(err)
		}
		return errors.Wrap(err, "getting project ref")
	}
	if !ref.CommitQueue.IsEnabled() {
		return errors.New("commit queue not enabled for project")
	}
	if ref.CommitQueue.MergeQueue == model.MergeQueueGitHub {
		return errors.New("Commit queue merge method is set to GitHub, not Evergreen, in the project settings. You must merge from a GitHub PR.")
	}

	commitCount, err := gitCommitCount(ref.Branch, p.ref, p.commits)
	if err != nil {
		return errors.Wrap(err, "getting commit count")
	}
	if commitCount > 1 && !p.skipConfirm &&
		!confirm("Commit queue patch has multiple commits (these will be tested together but merged separately). Continue?", false) {
		return errors.New("patch aborted")
	}

	if err = isValidCommitsFormat(p.commits); err != nil {
		return err
	}

	diffData, err := loadGitData("", ref.Branch, p.ref, p.commits, true)
	if err != nil {
		return errors.Wrap(err, "generating patches")
	}

	commits := noCommits
	if commitCount > 0 {
		var commitMessages string
		commitMessages, err = gitCommitMessages(ref.Branch, p.ref, p.commits)
		if err != nil {
			return errors.Wrap(err, "getting commit messages")
		}
		commits = fmt.Sprintf(commitFmtString, commitMessages, ref.Owner, ref.Repo, ref.Branch)
	}
	patchParams.Description = fmt.Sprintf("%s %s", commitQueuePatchLabel, commits)

	if err = patchParams.validateSubmission(diffData); err != nil {
		return err
	}
	patch, err := patchParams.createPatch(ac, diffData)
	if err != nil {
		return err
	}
	if err = patchParams.displayPatch(patch, uiV2Url, true); err != nil {
		grip.Error("Patch information cannot be displayed.")
	}

	p.id = patch.Id.Hex()
	patchParams.setDefaultProject(conf)

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
	if err := isValidCommitsFormat(p.commits); err != nil {
		return err
	}

	proj, err := rc.GetPatchedConfig(p.patchID)
	if err != nil {
		return err
	}
	module, err := proj.GetModuleByName(p.module)
	if err != nil {
		return errors.Wrapf(err, "finding module '%s'", p.module)
	}

	commitCount, err := gitCommitCount(module.Branch, p.ref, p.commits)
	if err != nil {
		return errors.Wrap(err, "getting commit count")
	}
	if commitCount == 0 {
		return errors.New("no commits for module")
	}
	if commitCount > 1 && !p.skipConfirm &&
		!confirm("Commit queue module patch has multiple commits (these will be tested together but merged separately). Continue?", false) {
		return errors.New("module patch aborted")
	}

	owner, repo, err := module.GetOwnerAndRepo()
	if err != nil {
		return errors.Wrapf(err, "error getting owner and repo for '%s'", module.Name)
	}

	patch, err := rc.GetPatch(p.patchID)
	if err != nil {
		return errors.Wrapf(err, "get patch '%s'", p.patchID)
	}

	commitMessages, err := gitCommitMessages(module.Branch, p.ref, p.commits)
	if err != nil {
		return errors.Wrap(err, "getting module commit messages")
	}
	commits := fmt.Sprintf(commitFmtString, commitMessages, owner, repo, module.Branch)
	message := fmt.Sprintf("%s || %s", patch.Description, commits)
	// replace the description if the original patch was empty
	if strings.HasSuffix(patch.Description, noCommits) {
		message = fmt.Sprintf("%s %s", commitQueuePatchLabel, commits)
	}

	diffData, err := loadGitData("", module.Branch, p.ref, p.commits, true)
	if err != nil {
		return errors.Wrap(err, "getting patch data")
	}
	if err = validatePatchSize(diffData, p.large); err != nil {
		return err
	}

	if !p.skipConfirm {
		grip.InfoWhen(diffData.patchSummary != "", diffData.patchSummary)
		grip.InfoWhen(diffData.log != "", diffData.log)
		if !confirm("This is a summary of the patch to be submitted. Continue?", true) {
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
	grip.Info("Module updated.")
	return nil
}

func showCQMessageForProject(ac *legacyClient, projectID string) {
	projectRef, _ := ac.GetProjectRef(projectID)
	if projectRef != nil && projectRef.CommitQueue.Message != "" {
		grip.Info(projectRef.CommitQueue.Message)
	}
}

func showCQMessageForPatch(ctx context.Context, comm client.Communicator, patchID string) {
	message, _ := comm.GetMessageForPatch(ctx, patchID)
	if message != "" {
		grip.Info(message)
	}
}

func getAPICommitQueuePatchDisplay(apiPatch *restModel.APIPatch, summarize bool, uiHost string) (string, error) {
	servicePatch, err := apiPatch.ToService()
	if err != nil {
		return "", errors.Wrap(err, "converting patch to service model")
	}

	return getPatchDisplay(&servicePatch, summarize, uiHost, true)
}
