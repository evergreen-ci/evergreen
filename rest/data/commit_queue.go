package data

import (
	"context"
	"fmt"
	"net/http"
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
	"github.com/google/go-github/v34/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type DBCommitQueueConnector struct{}

// GetGitHubPR takes the owner, repo, and PR number.
func (pc *DBCommitQueueConnector) GetGitHubPR(ctx context.Context, owner, repo string, PRNum int) (*github.PullRequest, error) {
	conf, err := evergreen.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "getting admin settings")
	}
	ghToken, err := conf.GetGithubOauthToken()
	if err != nil {
		return nil, errors.Wrap(err, "getting GitHub OAuth token from admin settings")
	}

	ctxWithCancel, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pr, err := thirdparty.GetGithubPullRequest(ctxWithCancel, ghToken, owner, repo, PRNum)
	if err != nil {
		return nil, errors.Wrap(err, "getting GitHub PR from GitHub API")
	}

	return pr, nil
}

func (pc *DBCommitQueueConnector) AddPatchForPr(ctx context.Context, projectRef model.ProjectRef, prNum int, modules []restModel.APIModule, messageOverride string) (string, error) {
	settings, err := evergreen.GetConfig()
	if err != nil {
		return "", errors.Wrap(err, "getting admin settings")
	}
	githubToken, err := settings.GetGithubOauthToken()
	if err != nil {
		return "", errors.Wrap(err, "getting GitHub OAuth token from admin settings")
	}
	pr, err := thirdparty.GetMergeablePullRequest(ctx, prNum, githubToken, projectRef.Owner, projectRef.Repo)
	if err != nil {
		return "", err
	}

	title := fmt.Sprintf("%s (#%d)", pr.GetTitle(), prNum)
	patchDoc, err := patch.MakeNewMergePatch(pr, projectRef.Id, evergreen.CommitQueueAlias, title, messageOverride)
	if err != nil {
		return "", errors.Wrap(err, "making commit queue patch")
	}

	p, patchSummaries, proj, err := getPatchInfo(ctx, githubToken, patchDoc)
	if err != nil {
		return "", err
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

		return "", errors.Wrap(catcher.Resolve(), "invalid project configuration file")
	}

	if err = writePatchInfo(patchDoc, patchSummaries, p); err != nil {
		return "", err
	}

	serviceModules := []commitqueue.Module{}
	for _, module := range modules {
		serviceModules = append(serviceModules, *restModel.APIModuleToService(module))
	}
	modulePRs, modulePatches, err := model.GetModulesFromPR(ctx, githubToken, serviceModules, proj)
	if err != nil {
		return "", err
	}
	patchDoc.Patches = append(patchDoc.Patches, modulePatches...)

	// populate tasks/variants matching the commitqueue alias
	proj.BuildProjectTVPairs(patchDoc, patchDoc.Alias)

	if err = units.AddMergeTaskAndVariant(patchDoc, proj, &projectRef, commitqueue.SourcePullRequest); err != nil {
		return "", err
	}

	if err = patchDoc.Insert(); err != nil {
		return "", errors.Wrap(err, "inserting patch")
	}

	catcher = grip.NewBasicCatcher()
	for _, modulePR := range modulePRs {
		catcher.Add(thirdparty.SendCommitQueueGithubStatus(evergreen.GetEnvironment(), modulePR, message.GithubStatePending, "added to queue", patchDoc.Id.Hex()))
	}

	return patchDoc.Id.Hex(), catcher.Resolve()
}

func getPatchInfo(ctx context.Context, githubToken string, patchDoc *patch.Patch) (string, []thirdparty.Summary, *model.Project, error) {
	patchContent, summaries, err := thirdparty.GetGithubPullRequestDiff(ctx, githubToken, patchDoc.GithubPatchData)
	if err != nil {
		return "", nil, nil, errors.Wrap(err, "getting GitHub PR diff")
	}

	// fetch the latest config file
	config, patchConfig, err := model.GetPatchedProject(ctx, patchDoc, githubToken)
	if err != nil {
		return "", nil, nil, errors.Wrap(err, "getting remote config file")
	}

	patchDoc.PatchedParserProject = patchConfig.PatchedParserProject
	patchDoc.PatchedProjectConfig = patchConfig.PatchedProjectConfig
	return patchContent, summaries, config, nil
}

func writePatchInfo(patchDoc *patch.Patch, patchSummaries []thirdparty.Summary, patchContent string) error {
	patchFileID := fmt.Sprintf("%s_%s", patchDoc.Id.Hex(), patchDoc.Githash)
	if err := db.WriteGridFile(patch.GridFSPrefix, patchFileID, strings.NewReader(patchContent)); err != nil {
		return errors.Wrap(err, "writing patch file to DB")
	}

	// no name for the main patch
	patchDoc.Patches = append(patchDoc.Patches, patch.ModulePatch{
		Githash: patchDoc.Githash,
		PatchSet: patch.PatchSet{
			PatchFileId: patchFileID,
			Summary:     patchSummaries,
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

func CommitQueueRemoveItem(identifier, issue, user string) (*restModel.APICommitQueueItem, error) {
	id, err := model.GetIdForProject(identifier)
	if err != nil {
		return nil, errors.Wrapf(err, "finding project ref '%s'", identifier)
	}
	cq, err := commitqueue.FindOneId(id)
	if err != nil {
		return nil, errors.Wrapf(err, "getting commit queue for project '%s'", identifier)
	}
	if cq == nil {
		return nil, errors.Errorf("commit queue not found for project '%s'", identifier)
	}
	version, err := model.GetVersionForCommitQueueItem(cq, issue)
	if err != nil {
		return nil, errors.Wrapf(err, "verifying if version exists for issue '%s'", issue)
	}
	removed, err := cq.RemoveItemAndPreventMerge(issue, version != nil, user)
	if err != nil {
		return nil, errors.Wrapf(err, "removing item and preventing merge for commit queue item '%s'", issue)
	}
	if removed == nil {
		return nil, errors.Errorf("item '%s' not found in commit queue", issue)
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

func CreatePatchForMerge(ctx context.Context, existingPatchID, commitMessage string) (*restModel.APIPatch, error) {
	existingPatch, err := patch.FindOneId(existingPatchID)
	if err != nil {
		return nil, errors.Wrap(err, "finding patch")
	}
	if existingPatch == nil {
		return nil, errors.Errorf("patch '%s' not found", existingPatchID)
	}

	newPatch, err := model.MakeMergePatchFromExisting(ctx, existingPatch, commitMessage)
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
	event.LogCommitQueueConcludeTest(patchID, status)
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
	item := ""
	for _, entry := range cq.Queue {
		if entry.Version == patchID {
			item = entry.Issue
			break
		}
	}
	if item == "" {
		return errors.Errorf("commit queue item for patch '%s' not found", patchID)
	}
	found, err := cq.Remove(item)
	if err != nil {
		return errors.Wrapf(err, "dequeueing item '%s' from commit queue", item)
	}
	if found == nil {
		return errors.Errorf("item '%s' not found in commit queue", item)
	}
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
