package data

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
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
	settings, err := evergreen.GetConfig(ctx)
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

	p, patchSummaries, proj, _, err := getPatchInfo(ctx, settings, githubToken, patchDoc)
	if err != nil {
		return nil, err
	}

	errs := validator.CheckProjectErrors(ctx, proj, false)
	isConfigDefined := len(patchDoc.PatchedProjectConfig) > 0
	errs = append(errs, validator.CheckProjectSettings(ctx, settings, proj, &projectRef, isConfigDefined)...)
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

	if err = patchDoc.Insert(); err != nil {
		return nil, errors.Wrap(err, "inserting patch")
	}

	catcher = grip.NewBasicCatcher()
	env := evergreen.GetEnvironment()
	for _, modulePR := range modulePRs {
		catcher.Add(thirdparty.SendCommitQueueGithubStatus(ctx, env, modulePR, message.GithubStatePending, "added to queue", patchDoc.Id.Hex()))
	}

	return patchDoc, catcher.Resolve()
}

func getPatchInfo(ctx context.Context, settings *evergreen.Settings, githubToken string, patchDoc *patch.Patch) (string, []thirdparty.Summary, *model.Project, *model.ParserProject, error) {
	patchContent, summaries, err := thirdparty.GetGithubPullRequestDiff(ctx, githubToken, patchDoc.GithubPatchData)
	if err != nil && !strings.Contains(err.Error(), thirdparty.PRDiffTooLargeErrorMessage) {
		return "", nil, nil, nil, errors.Wrap(err, "getting GitHub PR diff")
	}

	// Fetch the latest config file.
	// Set a higher timeout for this operation.
	fetchCtx, ctxCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer ctxCancel()
	config, patchConfig, err := model.GetPatchedProject(fetchCtx, settings, patchDoc, githubToken)
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

// FindAndRemoveCommitQueueItem dequeues an item from the commit queue and returns the
// removed item. If the item is already being tested in a batch, later items in
// the batch are restarted.
func FindAndRemoveCommitQueueItem(ctx context.Context, cqId, issue, user, reason string) (*restModel.APICommitQueueItem, error) {
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

	removed, err := model.CommitQueueRemoveItem(ctx, cq, item, user, reason)
	if err != nil {
		return nil, err
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

// NewUserRepoInfo creates UserRepoInfo from PR information.
func NewUserRepoInfo(info commitqueue.EnqueuePRInfo) UserRepoInfo {
	return UserRepoInfo{
		Username: info.Username,
		Owner:    info.Owner,
		Repo:     info.Repo,
	}
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
	hasPermission, err := thirdparty.GitHubUserHasWritePermission(ctxWithCancel, token, args.Owner, args.Repo, args.Username)
	if err != nil {
		return false, errors.Wrap(err, "getting GitHub user permissions")
	}

	return hasPermission, nil
}

// EnqueuePRToCommitQueue enqueues an item to the commit queue to test and merge a PR.
func EnqueuePRToCommitQueue(ctx context.Context, env evergreen.Environment, sc Connector, info commitqueue.EnqueuePRInfo) (*restModel.APIPatch, error) {
	patchDoc, pr, err := getAndEnqueueCommitQueueItemForPR(ctx, env, sc, info)
	if err != nil {
		catcher := grip.NewBasicCatcher()
		if pr != nil && errors.Cause(err) != errNoCommitQueueForBranch {
			// Send back information for any error except one where the error is
			// that the commit queue is not enabled. This is because some
			// projects have their own workflow that use the `evergreen merge`
			// PR comment to trigger their own custom commit queue logic, and
			// want this handler to no-op.
			catcher.Wrap(sendGitHubCommitQueueError(ctx, env, sc, pr, NewUserRepoInfo(info), err), "propagating GitHub errors back to PR")
		}
		return nil, err
	}

	if pr == nil || pr.Head == nil || pr.Head.SHA == nil {
		return nil, errors.New("PR contains no head branch SHA")
	}
	pushJob := units.NewGithubStatusUpdateJobForPushToCommitQueue(info.Owner, info.Repo, *pr.Head.SHA, info.PR, patchDoc.Id.Hex())
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

// errNoCommitQueueForBranch indicates that there is no commit queue for the
// particular repo and branch.
var errNoCommitQueueForBranch = errors.New("no project with commit queue enabled for tracking branch")

// getAndEnqueueCommitQueueItemForPR gets information about a PR, validates that it's allowed to
// submit to the commit queue, and enqueues it. If it succeeds, it will return
// the created patch and the PR info. It may still return the PR info even if
// it fails to create the patch.
func getAndEnqueueCommitQueueItemForPR(ctx context.Context, env evergreen.Environment, sc Connector, info commitqueue.EnqueuePRInfo) (*patch.Patch, *github.PullRequest, error) {
	pr, err := getPRAndCheckBase(ctx, sc, info)
	if err != nil {
		return nil, nil, err
	}

	cqInfo := restModel.ParseGitHubComment(info.CommitMessage)
	baseBranch := *pr.Base.Ref
	projectRef, err := model.FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(info.Owner, info.Repo, baseBranch)
	if err != nil {
		return nil, pr, errors.Wrapf(err, "getting project for '%s:%s' tracking branch '%s'", info.Owner, info.Repo, baseBranch)
	}
	if projectRef == nil {
		return nil, pr, errors.Wrapf(errNoCommitQueueForBranch, "repo '%s:%s', branch '%s'", info.Owner, info.Repo, baseBranch)
	}

	if projectRef.CommitQueue.MergeQueue == model.MergeQueueGitHub {
		return nil, pr, errors.Wrapf(errors.New("This project is using GitHub merge queue. Click the merge button instead."), "repo '%s:%s', branch '%s'", info.Owner, info.Repo, baseBranch)
	}
	if projectRef.CommitQueue.CLIOnly {
		return nil, pr, errors.Errorf("This project can only use the CLI commit queue. Please use the command `evergreen commit-queue merge -p %s`", projectRef.Identifier)
	}

	authorized, err := sc.IsAuthorizedToPatchAndMerge(ctx, env.Settings(), NewUserRepoInfo(info))
	if err != nil {
		return nil, pr, errors.Wrap(err, "getting user info from GitHub API")
	}
	if !authorized {
		return nil, pr, errors.Errorf("user '%s' is not authorized to merge", info.Username)
	}

	pr, err = checkPRIsMergeable(ctx, sc, pr, info)
	if err != nil {
		return nil, pr, err
	}

	patchDoc, err := tryEnqueueItemForPR(ctx, sc, projectRef, info.PR, cqInfo)
	if err != nil {
		return nil, pr, errors.Wrap(err, "enqueueing item to commit queue for PR")
	}
	return patchDoc, pr, nil
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

// checkPRIsMergeable verifies that the PR is mergeable. It may refresh the PR
// state if it's out of date; otherwise, if it's up-to-date, it will return the
// PR info as-is.
func checkPRIsMergeable(ctx context.Context, sc Connector, pr *github.PullRequest, info commitqueue.EnqueuePRInfo) (*github.PullRequest, error) {
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
			// Merging might be blocked due to branch protection requiring
			// the PR patch to succeed. However, Evergreen does not guarantee
			// that the GitHub status is up-to-date with the actual status in
			// Evergreen. Refreshing the GitHub status here re-syncs the GitHub
			// status in case a stale Evergreen patch status might be blocking
			// the merge.
			refreshJob := units.NewGithubStatusRefreshJob(p)
			refreshJob.Run(ctx)
			refreshedPR, err := getPRAndCheckBase(ctx, sc, info)
			if err != nil {
				return pr, err
			}
			pr = refreshedPR
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

		return pr, errors.New(errMsg)
	}

	return pr, nil
}

// tryEnqueueItemForPR attempts to enqueue an item for a PR after checking it is
// in a valid state to enqueue. It returns the enqueued patch, and in the
// failure case it will return an error and a short error message to be sent to
// GitHub.
func tryEnqueueItemForPR(ctx context.Context, sc Connector, projectRef *model.ProjectRef, prNum int, cqInfo restModel.GithubCommentCqData) (*patch.Patch, error) {
	patchDoc, err := sc.AddPatchForPR(ctx, *projectRef, prNum, cqInfo.Modules, cqInfo.MessageOverride)
	if err != nil {
		return nil, errors.Wrap(err, "creating patch for PR")
	}

	item := restModel.APICommitQueueItem{
		Issue:           utility.ToStringPtr(strconv.Itoa(prNum)),
		MessageOverride: &cqInfo.MessageOverride,
		Modules:         cqInfo.Modules,
		Source:          utility.ToStringPtr(commitqueue.SourcePullRequest),
		PatchId:         utility.ToStringPtr(patchDoc.Id.Hex()),
	}
	if _, err = EnqueueItem(projectRef.Id, item, false); err != nil {
		return nil, errors.Wrap(err, "enqueueing commit queue item")
	}
	return patchDoc, nil
}

// sendGitHubCommitQueueError updates the GitHub status and posts a comment
// after an error has occurred related to the commit queue.
func sendGitHubCommitQueueError(ctx context.Context, env evergreen.Environment, sc Connector, pr *github.PullRequest, userRepo UserRepoInfo, err error) error {
	if err == nil {
		return nil
	}

	catcher := grip.NewBasicCatcher()
	catcher.Wrap(thirdparty.SendCommitQueueGithubStatus(ctx, env, pr, message.GithubStateFailure, err.Error(), ""), "sending GitHub status update")

	comment := fmt.Sprintf("Evergreen could not enqueue your PR in the commit queue. The error:\n%s", err)
	catcher.Wrap(sc.AddCommentToPR(ctx, userRepo.Owner, userRepo.Repo, pr.GetNumber(), comment), "writing error comment back to PR")

	return catcher.Resolve()
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

func ConcludeMerge(ctx context.Context, patchID, status string) error {
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
	err = model.SendCommitQueueResult(ctx, p, githubStatus, description)
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

// CheckCanRemoveCommitQueueItem checks if a patch can be removed from the commit queue by the given user.
func CheckCanRemoveCommitQueueItem(ctx context.Context, sc Connector, usr *user.DBUser, project *model.ProjectRef, itemId string) error {
	if !project.CommitQueue.IsEnabled() {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Errorf("commit queue is not enabled for project '%s'", project.Id).Error(),
		}
	}

	if canAlwaysSubmitPatchesForProject(usr, project.Id) {
		return nil
	}

	if bson.IsObjectIdHex(itemId) {
		patch, err := FindPatchById(itemId)
		if err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    errors.Wrapf(err, "finding patch '%s'", itemId).Error(),
			}
		}

		// TODO: Remove user lookup and OnlyAPI conditional statement after EVG-20118 is complete.
		// Since we don't require users to save their GitHub information before submitting patches,
		// we must allow them to remove any patches created by service users (OnlyAPI = true).
		patchAuthor := utility.FromStringPtr(patch.Author)
		patchUsr, err := user.FindOneById(patchAuthor)
		if err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    errors.Wrapf(err, "finding user '%s'", patchAuthor).Error(),
			}
		}
		if patchUsr == nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("user '%s' not found", patchAuthor),
			}
		}
		if patchUsr.OnlyAPI {
			return nil
		}

		if usr.Id != patchAuthor {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "not authorized to perform action on behalf of author",
			}
		}
	} else if itemInt, err := strconv.Atoi(itemId); err == nil {
		pr, err := sc.GetGitHubPR(ctx, project.Owner, project.Repo, itemInt)
		if err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    errors.Wrapf(err, "unable to get pull request info, PR number (%d) may be invalid", itemInt).Error(),
			}
		}

		var githubUID int
		if pr != nil && pr.User != nil && pr.User.ID != nil {
			githubUID = int(*pr.User.ID)
		}
		if githubUID == 0 || usr.Settings.GithubUser.UID != githubUID {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "not authorized to perform action on behalf of GitHub user",
			}
		}
	} else {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "commit queue item is not a valid identifier",
		}
	}

	return nil
}

// canAlwaysSubmitPatchesForProject returns true if the user is a superuser or project admin,
// or is authorized specifically to patch on behalf of other users.
func canAlwaysSubmitPatchesForProject(user *user.DBUser, projectId string) bool {
	if projectId == "" {
		grip.Error(message.Fields{
			"message": "projectID is empty",
			"op":      "middleware",
			"stack":   string(debug.Stack()),
		})
		return false
	}
	isAdmin := user.HasPermission(gimlet.PermissionOpts{
		Resource:      projectId,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionProjectSettings,
		RequiredLevel: evergreen.ProjectSettingsEdit.Value,
	})
	if isAdmin {
		return true
	}
	return user.HasPermission(gimlet.PermissionOpts{
		Resource:      projectId,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionPatches,
		RequiredLevel: evergreen.PatchSubmitAdmin.Value,
	})
}
