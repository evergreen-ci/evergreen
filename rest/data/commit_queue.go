package data

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type DBCommitQueueConnector struct{}

func (pc *DBCommitQueueConnector) AddPatchForPR(ctx context.Context, projectRef model.ProjectRef, prNum int, modules []restModel.APIModule, messageOverride string) (*patch.Patch, error) {
	settings, err := evergreen.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "getting admin settings")
	}
	githubToken, err := settings.GetGithubOauthToken()
	if err != nil {
		return nil, errors.Wrap(err, "getting GitHub OAuth token from admin settings")
	}
	pr, err := thirdparty.GetMergeablePullRequest(ctx, prNum, githubToken, projectRef.Owner, projectRef.Repo)
	if err != nil {
		return nil, err
	}

	title := fmt.Sprintf("%s (#%d)", pr.GetTitle(), prNum)
	patchDoc, err := patch.MakeNewMergePatch(pr, projectRef.Id, evergreen.CommitQueueAlias, title, messageOverride)
	if err != nil {
		return nil, errors.Wrap(err, "making commit queue patch")
	}

	p, patchSummaries, proj, pp, err := getPatchInfo(ctx, settings, githubToken, patchDoc)
	if err != nil {
		return nil, err
	}

	errs := validator.CheckProjectErrors(proj, false)
	isConfigDefined := len(patchDoc.PatchedProjectConfig) > 0
	errs = append(errs, validator.CheckProjectSettings(proj, &projectRef, isConfigDefined)...)
	errs = append(errs, validator.CheckPatchedProjectConfigErrors(patchDoc.PatchedProjectConfig)...)
	catcher := grip.NewBasicCatcher()
	for _, validationErr := range errs.AtLevel(validator.Error) {
		catcher.Add(validationErr)
	}
	if catcher.HasErrors() {
		update := units.NewGithubStatusUpdateJobForProcessingError(
			commitqueue.GithubContext,
			pr.Base.User.GetLogin(),
			pr.Base.Repo.GetName(),
			pr.Head.GetRef(),
			units.InvalidConfig,
		)
		update.Run(ctx)
		grip.Error(message.WrapError(update.Error(), message.Fields{
			"message":    "error updating pull request with merge",
			"project":    projectRef.Identifier,
			"pr":         prNum,
			"merge_errs": catcher.Resolve(),
		}))

		return nil, errors.Wrap(catcher.Resolve(), "invalid project configuration file")
	}

	if err = writePatchInfo(patchDoc, patchSummaries, p); err != nil {
		return nil, err
	}

	serviceModules := []commitqueue.Module{}
	for _, module := range modules {
		serviceModules = append(serviceModules, *restModel.APIModuleToService(module))
	}
	modulePRs, modulePatches, err := model.GetModulesFromPR(ctx, githubToken, serviceModules, proj)
	if err != nil {
		return nil, err
	}
	patchDoc.Patches = append(patchDoc.Patches, modulePatches...)

	// populate tasks/variants matching the commitqueue alias
	proj.BuildProjectTVPairs(patchDoc, patchDoc.Alias)

	pp, err = units.AddMergeTaskAndVariant(ctx, patchDoc, proj, &projectRef, commitqueue.SourcePullRequest)
	if err != nil {
		return nil, err
	}

	env := evergreen.GetEnvironment()
	pp.Init(patchDoc.Id.Hex(), patchDoc.CreateTime)
	ppStorageMethod, err := model.ParserProjectUpsertOneWithS3Fallback(ctx, env.Settings(), evergreen.ProjectStorageMethodDB, pp)
	if err != nil {
		return nil, errors.Wrapf(err, "upsert parser project '%s' for patch '%s'", pp.Id, patchDoc.Id.Hex())
	}
	patchDoc.ProjectStorageMethod = ppStorageMethod

	if err = patchDoc.Insert(); err != nil {
		return nil, errors.Wrap(err, "inserting patch")
	}

	catcher = grip.NewBasicCatcher()
	for _, modulePR := range modulePRs {
		catcher.Add(thirdparty.SendCommitQueueGithubStatus(env, modulePR, message.GithubStatePending, "added to queue", patchDoc.Id.Hex()))
	}

	return patchDoc, catcher.Resolve()
}

func getPatchInfo(ctx context.Context, settings *evergreen.Settings, githubToken string, patchDoc *patch.Patch) (string, []thirdparty.Summary, *model.Project, *model.ParserProject, error) {
	patchContent, summaries, err := thirdparty.GetGithubPullRequestDiff(ctx, githubToken, patchDoc.GithubPatchData)
	if err != nil {
		return "", nil, nil, nil, errors.Wrap(err, "getting GitHub PR diff")
	}

	// fetch the latest config file
	config, patchConfig, err := model.GetPatchedProject(ctx, settings, patchDoc, githubToken)
	if err != nil {
		return "", nil, nil, nil, errors.Wrap(err, "getting remote config file")
	}

	patchDoc.PatchedProjectConfig = patchConfig.PatchedProjectConfig
	return patchContent, summaries, config, patchConfig.PatchedParserProject, nil
}

// writePatchInfo writes a PR patch's contents to gridFS and stores this info with the patch.
func writePatchInfo(patchDoc *patch.Patch, patchSummaries []thirdparty.Summary, patchContent string) error {
	patchFileID := fmt.Sprintf("%s_%s", patchDoc.Id.Hex(), patchDoc.Githash)
	if err := db.WriteGridFile(patch.GridFSPrefix, patchFileID, strings.NewReader(patchContent)); err != nil {
		return errors.Wrap(err, "writing patch file to DB")
	}

	// no name for the main patch
	patchDoc.Patches = append(patchDoc.Patches, patch.ModulePatch{
		Githash: patchDoc.Githash,
		PatchSet: patch.PatchSet{
			PatchFileId:    patchFileID,
			Summary:        patchSummaries,
			CommitMessages: []string{patchDoc.GithubPatchData.CommitTitle},
		},
	})

	return nil
}

// EnqueueItem will enqueue an item to a project's commit queue.
// If enqueueNext is true, move the commit queue item to be processed next.
func EnqueueItem(projectID string, item restModel.APICommitQueueItem, enqueueNext bool) (int, error) {
	q, err := commitqueue.FindOneId(projectID)
	if err != nil {
		return 0, errors.Wrapf(err, "finding commit queue for project '%s'", projectID)
	}
	if q == nil {
		return 0, errors.Errorf("commit queue not found for project '%s'", projectID)
	}

	itemService := item.ToService()
	if enqueueNext {
		var position int
		position, err = q.EnqueueAtFront(itemService)
		if err != nil {
			return 0, errors.Wrapf(err, "force enqueueing item into commit queue for project '%s'", projectID)
		}
		return position, nil
	}

	position, err := q.Enqueue(itemService)
	if err != nil {
		return 0, errors.Wrapf(err, "enqueueing item to commit queue for project '%s'", projectID)
	}

	return position, nil
}

func FindCommitQueueForProject(name string) (*restModel.APICommitQueue, error) {
	id, err := model.GetIdForProject(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	cqService, err := commitqueue.FindOneId(id)
	if err != nil {
		return nil, errors.Wrapf(err, "finding commit queue for project '%s'", id)
	}
	if cqService == nil {
		return nil, errors.Errorf("commit queue not found for project '%s'", id)
	}

	apiCommitQueue := &restModel.APICommitQueue{}
	apiCommitQueue.BuildFromService(*cqService)
	return apiCommitQueue, nil
}

// CommitQueueRemoveItem dequeues an item from the commit queue. If the item is
// already being tested in a batch, later items in the batch are restarted.
func CommitQueueRemoveItem(cqId, issue, user, reason string) (*restModel.APICommitQueueItem, error) {
	cq, err := commitqueue.FindOneId(cqId)
	if err != nil {
		return nil, errors.Wrapf(err, "getting commit queue '%s'", cqId)
	}
	if cq == nil {
		return nil, errors.Errorf("commit queue '%s' not found", cqId)
	}

	itemIdx := cq.FindItem(issue)
	if itemIdx == -1 {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("commit queue item '%s' not found", issue),
		}
	}
	item := cq.Queue[itemIdx]

	var removed *commitqueue.CommitQueueItem
	if item.Version != "" {
		// If the patch has been finalized, it may already be running in a
		// batch, so it has to restart later items that are running in its
		// batch.
		removed, err = model.DequeueAndRestartForVersion(cq, cq.ProjectID, item.Version, user, reason)
		if err != nil {
			return nil, errors.Wrap(err, "dequeueing and restarting finalized commit queue item")
		}
	} else {
		// If the patch hasn't been finalized yet, it can simply be removed from
		// the commit queue.
		removed, err = model.RemoveItemAndPreventMerge(cq, issue, user)
		if err != nil {
			return nil, errors.Wrap(err, "removing unfinalized commit queue item")
		}
		if removed == nil {
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    errors.Errorf("item '%s' not found in commit queue", issue).Error(),
			}
		}
	}

	apiRemovedItem := restModel.APICommitQueueItem{}
	apiRemovedItem.BuildFromService(*removed)
	return &apiRemovedItem, nil
}

type UserRepoInfo struct {
	Username string
	Owner    string
	Repo     string
}

func (pc *DBCommitQueueConnector) IsAuthorizedToPatchAndMerge(ctx context.Context, settings *evergreen.Settings, args UserRepoInfo) (bool, error) {
	// In the org
	token, err := settings.GetGithubOauthToken()
	if err != nil {
		return false, errors.Wrap(err, "getting GitHub OAuth token from admin settings")
	}

	requiredOrganization := settings.GithubPRCreatorOrg
	if requiredOrganization == "" {
		return false, errors.New("no GitHub PR creator organization configured")
	}

	ctxWithCancel, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	inOrg, err := thirdparty.GithubUserInOrganization(ctxWithCancel, token, requiredOrganization, args.Username)
	if err != nil {
		return false, errors.Wrap(err, "checking if user is in required GitHub organization")
	}
	if !inOrg {
		return false, nil
	}

	// Has repository merge permission
	// See: https://help.github.com/articles/repository-permission-levels-for-an-organization/
	ctxWithCancel, cancel = context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	permission, err := thirdparty.GitHubUserPermissionLevel(ctxWithCancel, token, args.Owner, args.Repo, args.Username)
	if err != nil {
		return false, errors.Wrap(err, "getting GitHub user permissions")
	}
	mergePermissions := []string{"admin", "write"}
	hasPermission := utility.StringSliceContains(mergePermissions, permission)

	return hasPermission, nil
}

// EnqueuePRToCommitQueue enqueues an item to the commit queue to test and merge a PR.
func EnqueuePRToCommitQueue(ctx context.Context, env evergreen.Environment, sc Connector, info commitqueue.EnqueuePRInfo) (*restModel.APIPatch, error) {
	settings := env.Settings()
	userRepo := UserRepoInfo{
		Username: info.Username,
		Owner:    info.Owner,
		Repo:     info.Repo,
	}
	authorized, err := sc.IsAuthorizedToPatchAndMerge(ctx, settings, userRepo)
	if err != nil {
		return nil, errors.Wrap(err, "getting user info from GitHub API")
	}
	if !authorized {
		return nil, errors.Errorf("user '%s' is not authorized to merge", userRepo.Username)
	}

	pr, err := getPRAndCheckMergeable(ctx, env, sc, info)
	if err != nil {
		return nil, err
	}

	cqInfo := restModel.ParseGitHubComment(info.CommitMessage)
	baseBranch := *pr.Base.Ref
	projectRef, err := model.FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(userRepo.Owner, userRepo.Repo, baseBranch)
	if err != nil {
		return nil, errors.Wrapf(err, "getting project for '%s:%s' tracking branch '%s'", userRepo.Owner, userRepo.Repo, baseBranch)
	}
	if projectRef == nil {
		return nil, errors.Errorf("no project with commit queue enabled for '%s:%s' tracking branch '%s'", userRepo.Owner, userRepo.Repo, baseBranch)
	}

	patchDoc, errMsg, err := tryEnqueueItemForPR(ctx, sc, projectRef, info.PR, cqInfo)
	if err != nil {
		sendErr := thirdparty.SendCommitQueueGithubStatus(env, pr, message.GithubStateFailure, errMsg, "")
		grip.Error(message.WrapError(sendErr, message.Fields{
			"message": "error sending patch creation failure to github",
			"owner":   userRepo.Owner,
			"repo":    userRepo.Repo,
			"pr":      info.PR,
		}))
		return nil, errors.Wrap(err, "enqueueing item to commit queue for PR")
	}

	if pr == nil || pr.Head == nil || pr.Head.SHA == nil {
		return nil, errors.New("PR contains no head branch SHA")
	}
	pushJob := units.NewGithubStatusUpdateJobForPushToCommitQueue(userRepo.Owner, userRepo.Repo, *pr.Head.SHA, info.PR, patchDoc.Id.Hex())
	q := env.LocalQueue()
	if err = q.Put(ctx, pushJob); err != nil {
		return nil, errors.Wrapf(err, "queueing notification for commit queue push for item '%d'", info.PR)
	}
	apiPatch := &restModel.APIPatch{}
	if err = apiPatch.BuildFromService(*patchDoc, nil); err != nil {
		return nil, errors.Wrap(err, "converting patch to API model")
	}
	return apiPatch, nil
}

// getPRAndCheckBase gets the Github PR and verifies that base and base ref is set
func getPRAndCheckBase(ctx context.Context, sc Connector, info commitqueue.EnqueuePRInfo) (*github.PullRequest, error) {
	pr, err := sc.GetGitHubPR(ctx, info.Owner, info.Repo, info.PR)
	if err != nil {
		return nil, errors.Wrap(err, "getting PR from GitHub API")
	}
	if pr == nil || pr.Base == nil || pr.Base.Ref == nil {
		return nil, errors.New("PR contains no base branch label")
	}
	return pr, nil
}

// getPRAndCheckMergeable gets the Github PR, verifies base, and verifies that the PR is mergeable.
// Attempts to refresh the status if the PR is marked as blocked but the patch is successful.
func getPRAndCheckMergeable(ctx context.Context, env evergreen.Environment, sc Connector, info commitqueue.EnqueuePRInfo) (*github.PullRequest, error) {
	pr, err := getPRAndCheckBase(ctx, sc, info)
	if err != nil {
		return nil, err
	}
	mergeableState := pr.GetMergeableState()

	grip.Debug(message.Fields{
		"message":        "checking PR mergeable status",
		"ticket":         "EVG-19680",
		"owner":          info.Owner,
		"repo":           info.Repo,
		"pr":             info.PR,
		"mergeble_state": mergeableState,
	})
	// If the PR is blocked, refresh status, and re-check PR.
	// We do this even if the patch isn't finished since the PR checks may not rely on all variants.
	if mergeableState == thirdparty.GithubPRBlocked {
		p, err := patch.FindLatestGithubPRPatch(info.Owner, info.Repo, info.PR)
		if err != nil {
			// Not required that such a patch exists, since the commit queue can be enabled without PR checks, so we log at debugging.
			grip.Debug(message.WrapError(err, message.Fields{
				"message": "couldn't find latest PR patch when enqueuing PR",
				"ticket":  "EVG-19098",
				"owner":   info.Owner,
				"repo":    info.Repo,
				"pr":      info.PR,
			}))
		} else if p != nil && p.Version != "" {
			grip.Debug(message.Fields{
				"message": "calling refresh from commit queue",
				"ticket":  "EVG-19827",
				"owner":   info.Owner,
				"repo":    info.Repo,
				"pr":      info.PR,
			})
			refreshJob := units.NewGithubStatusRefreshJob(p)
			refreshJob.Run(ctx)
			pr, err = getPRAndCheckBase(ctx, sc, info)
			if err != nil {
				return nil, err
			}
		}
	}

	if !thirdparty.IsUnblockedGithubStatus(mergeableState) {
		errMsg := fmt.Sprintf("PR is '%s'; branch protection settings are likely not met", mergeableState)
		grip.Debug(message.Fields{
			"message":  errMsg,
			"state":    pr.GetMergeableState(),
			"owner":    info.Owner,
			"repo":     info.Repo,
			"pr_title": pr.GetTitle(),
			"pr_num":   pr.GetNumber(),
		})
		sendErr := thirdparty.SendCommitQueueGithubStatus(env, pr, message.GithubStateFailure, errMsg, "")

		grip.Error(message.WrapError(sendErr, message.Fields{
			"message": "error sending patch creation failure to github",
			"owner":   info.Owner,
			"repo":    info.Repo,
			"pr":      pr.GetNumber(),
		}))
		return nil, errors.New(errMsg)
	}

	return pr, nil
}

// tryEnqueueItemForPR attempts to enqueue an item for a PR after checking it is in a valid state to enqueue. It returns the enqueued patch,
// and in the failure case it will return an error and a short error message to be sent to GitHub.
func tryEnqueueItemForPR(ctx context.Context, sc Connector, projectRef *model.ProjectRef, prNum int, cqInfo restModel.GithubCommentCqData) (*patch.Patch, string, error) {
	patchDoc, err := sc.AddPatchForPR(ctx, *projectRef, prNum, cqInfo.Modules, cqInfo.MessageOverride)
	if err != nil {
		return nil, "failed to create patch", errors.Wrap(err, "adding patch for PR")
	}

	item := restModel.APICommitQueueItem{
		Issue:           utility.ToStringPtr(strconv.Itoa(prNum)),
		MessageOverride: &cqInfo.MessageOverride,
		Modules:         cqInfo.Modules,
		Source:          utility.ToStringPtr(commitqueue.SourcePullRequest),
		PatchId:         utility.ToStringPtr(patchDoc.Id.Hex()),
	}
	if _, err = EnqueueItem(projectRef.Id, item, false); err != nil {
		return nil, "failed to enqueue commit item", errors.Wrap(err, "enqueueing commit queue item")
	}
	return patchDoc, "", nil
}

// CreatePatchForMerge creates a merge patch from an existing patch and enqueues
// it in the commit queue.
func CreatePatchForMerge(ctx context.Context, settings *evergreen.Settings, existingPatchID, commitMessage string) (*restModel.APIPatch, error) {
	existingPatch, err := patch.FindOneId(existingPatchID)
	if err != nil {
		return nil, errors.Wrap(err, "finding patch")
	}
	if existingPatch == nil {
		return nil, errors.Errorf("patch '%s' not found", existingPatchID)
	}

	newPatch, err := model.MakeMergePatchFromExisting(ctx, settings, existingPatch, commitMessage)
	if err != nil {
		return nil, errors.Wrapf(err, "creating new patch from existing patch '%s'", existingPatchID)
	}

	apiPatch := &restModel.APIPatch{}
	if err = apiPatch.BuildFromService(*newPatch, nil); err != nil {
		return nil, errors.Wrap(err, "converting patch to API model")
	}
	return apiPatch, nil
}

func ConcludeMerge(patchID, status string) error {
	p, err := patch.FindOneId(patchID)
	if err != nil {
		return errors.Wrap(err, "finding patch")
	}
	if p == nil {
		return errors.Errorf("patch '%s' not found", patchID)
	}
	cq, err := commitqueue.FindOneId(p.Project)
	if err != nil {
		return errors.Wrapf(err, "finding commit queue for project '%s'", p.Project)
	}
	if cq == nil {
		return errors.Errorf("commit queue for project '%s' not found", p.Project)
	}
	if _, err = cq.Remove(patchID); err != nil {
		return errors.Wrapf(err, "dequeueing item '%s' from commit queue", patchID)
	}

	event.LogCommitQueueConcludeTest(patchID, status)

	githubStatus := message.GithubStateFailure
	description := "merge test failed"
	if status == evergreen.MergeTestSucceeded {
		githubStatus = message.GithubStateSuccess
		description = "merge test succeeded"
	}
	err = model.SendCommitQueueResult(p, githubStatus, description)
	grip.Error(message.WrapError(err, message.Fields{
		"message": "unable to send github status",
		"patch":   patchID,
	}))

	return nil
}

func GetAdditionalPatches(patchId string) ([]string, error) {
	p, err := patch.FindOneId(patchId)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "finding patch").Error(),
		}
	}
	if p == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    errors.Errorf("patch '%s' not found", patchId).Error(),
		}
	}
	cq, err := commitqueue.FindOneId(p.Project)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "finding commit queue for project '%s'", p.Project).Error(),
		}
	}
	if cq == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    errors.Errorf("commit queue for project '%s' not found", p.Project).Error(),
		}
	}
	additionalPatches := []string{}
	for _, item := range cq.Queue {
		if item.Version == patchId {
			return additionalPatches, nil
		} else if item.Version != "" {
			additionalPatches = append(additionalPatches, item.Version)
		}
	}
	return nil, gimlet.ErrorResponse{
		StatusCode: http.StatusNotFound,
		Message:    errors.Errorf("patch '%s' not found in commit queue", patchId).Error(),
	}
}
