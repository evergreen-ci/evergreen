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
		return nil, errors.Wrap(err, "can't get evergreen configuration")
	}
	ghToken, err := conf.GetGithubOauthToken()
	if err != nil {
		return nil, errors.Wrap(err, "can't get Github OAuth token from configuration")
	}

	ctxWithCancel, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pr, err := thirdparty.GetGithubPullRequest(ctxWithCancel, ghToken, owner, repo, PRNum)
	if err != nil {
		return nil, errors.Wrap(err, "call to Github API failed")
	}

	return pr, nil
}

func (pc *DBCommitQueueConnector) AddPatchForPr(ctx context.Context, projectRef model.ProjectRef, prNum int, modules []restModel.APIModule, messageOverride string) (string, error) {
	settings, err := evergreen.GetConfig()
	if err != nil {
		return "", errors.Wrap(err, "unable to get evergreen settings")
	}
	githubToken, err := settings.GetGithubOauthToken()
	if err != nil {
		return "", errors.Wrap(err, "unable to get github token")
	}
	pr, err := thirdparty.GetPullRequest(ctx, prNum, githubToken, projectRef.Owner, projectRef.Repo)
	if err != nil {
		return "", err
	}

	title := fmt.Sprintf("%s (#%d)", pr.GetTitle(), prNum)
	patchDoc, err := patch.MakeNewMergePatch(pr, projectRef.Id, evergreen.CommitQueueAlias, title, messageOverride)
	if err != nil {
		return "", errors.Wrap(err, "unable to make commit queue patch")
	}

	p, patchSummaries, projectConfig, err := getPatchInfo(ctx, githubToken, patchDoc)
	if err != nil {
		return "", err
	}

	errs := validator.CheckProjectErrors(projectConfig, false)
	errs = append(errs, validator.CheckProjectSettings(projectConfig, &projectRef)...)
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
			"message":    "error updating pull request with merge error",
			"project":    projectRef.Identifier,
			"pr":         prNum,
			"merge_errs": catcher.Resolve(),
		}))

		return "", errors.Wrap(catcher.Resolve(), "errors found validating project configuration file")
	}

	if err = writePatchInfo(patchDoc, patchSummaries, p); err != nil {
		return "", err
	}

	serviceModules := []commitqueue.Module{}
	for _, module := range modules {
		serviceModules = append(serviceModules, *restModel.APIModuleToService(module))
	}
	modulePRs, modulePatches, err := model.GetModulesFromPR(ctx, githubToken, prNum, serviceModules, projectConfig)
	if err != nil {
		return "", err
	}
	patchDoc.Patches = append(patchDoc.Patches, modulePatches...)

	// populate tasks/variants matching the commitqueue alias
	projectConfig.BuildProjectTVPairs(patchDoc, patchDoc.Alias)

	if err = units.AddMergeTaskAndVariant(patchDoc, projectConfig, &projectRef, commitqueue.SourcePullRequest); err != nil {
		return "", err
	}

	if err = patchDoc.Insert(); err != nil {
		return "", errors.Wrap(err, "unable to add patch")
	}

	catcher = grip.NewBasicCatcher()
	for _, modulePR := range modulePRs {
		catcher.Add(thirdparty.SendCommitQueueGithubStatus(modulePR, message.GithubStatePending, "added to queue", patchDoc.Id.Hex()))
	}

	return patchDoc.Id.Hex(), catcher.Resolve()
}

func getPatchInfo(ctx context.Context, githubToken string, patchDoc *patch.Patch) (string, []thirdparty.Summary, *model.Project, error) {
	patchContent, summaries, err := thirdparty.GetGithubPullRequestDiff(ctx, githubToken, patchDoc.GithubPatchData)
	if err != nil {
		return "", nil, nil, errors.Wrap(err, "can't get diff")
	}

	// fetch the latest config file
	config, patchConfig, err := model.GetPatchedProject(ctx, patchDoc, githubToken)
	if err != nil {
		return "", nil, nil, errors.Wrap(err, "can't get remote config file")
	}

	patchDoc.PatchedParserProject = patchConfig.PatchedParserProject
	return patchContent, summaries, config, nil
}

func writePatchInfo(patchDoc *patch.Patch, patchSummaries []thirdparty.Summary, patchContent string) error {
	patchFileID := fmt.Sprintf("%s_%s", patchDoc.Id.Hex(), patchDoc.Githash)
	if err := db.WriteGridFile(patch.GridFSPrefix, patchFileID, strings.NewReader(patchContent)); err != nil {
		return errors.Wrap(err, "failed to write patch file to db")
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
		return 0, errors.Wrapf(err, "can't query for queue id '%s'", projectID)
	}
	if q == nil {
		return 0, errors.Errorf("no commit queue found for '%s'", projectID)
	}

	itemInterface, err := item.ToService()
	if err != nil {
		return 0, errors.Wrap(err, "item cannot be converted to DB model")
	}

	itemService := itemInterface.(commitqueue.CommitQueueItem)
	if enqueueNext {
		var position int
		position, err = q.EnqueueAtFront(itemService)
		if err != nil {
			return 0, errors.Wrapf(err, "can't force enqueue item to queue '%s'", projectID)
		}
		return position, nil
	}

	position, err := q.Enqueue(itemService)
	if err != nil {
		return 0, errors.Wrapf(err, "can't enqueue item to queue '%s'", projectID)
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
		return nil, errors.Wrap(err, "can't get commit queue from database")
	}
	if cqService == nil {
		return nil, errors.Errorf("no commit queue found for '%s'", id)
	}

	apiCommitQueue := &restModel.APICommitQueue{}
	if err = apiCommitQueue.BuildFromService(*cqService); err != nil {
		return nil, errors.Wrap(err, "can't read commit queue into API model")
	}

	return apiCommitQueue, nil
}

func CommitQueueRemoveItem(identifier, issue, user string) (*restModel.APICommitQueueItem, error) {
	id, err := model.GetIdForProject(identifier)
	if err != nil {
		return nil, errors.Wrapf(err, "can't find projectRef for '%s'", identifier)
	}
	cq, err := commitqueue.FindOneId(id)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get commit queue for project '%s'", identifier)
	}
	if cq == nil {
		return nil, errors.Errorf("no commit queue found for '%s'", identifier)
	}
	version, err := model.GetVersionForCommitQueueItem(cq, issue)
	if err != nil {
		return nil, errors.Wrapf(err, "error verifying if version exists for issue '%s'", issue)
	}
	removed, err := cq.RemoveItemAndPreventMerge(issue, version != nil, user)
	if err != nil {
		return nil, errors.Wrap(err, "unable to remove item")
	}
	if removed == nil {
		return nil, errors.Errorf("item %s not found in queue", issue)
	}
	apiRemovedItem := restModel.APICommitQueueItem{}
	if err = apiRemovedItem.BuildFromService(*removed); err != nil {
		return nil, err
	}
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
		return false, errors.Wrap(err, "can't get Github OAuth token from configuration")
	}

	requiredOrganization := settings.GithubPRCreatorOrg
	if requiredOrganization == "" {
		return false, errors.New("no GitHub PR creator organization configured")
	}

	ctxWithCancel, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	inOrg, err := thirdparty.GithubUserInOrganization(ctxWithCancel, token, requiredOrganization, args.Username)
	if err != nil {
		return false, errors.Wrap(err, "call to Github API failed")
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
		return false, errors.Wrap(err, "call to Github API failed")
	}
	mergePermissions := []string{"admin", "write"}
	hasPermission := utility.StringSliceContains(mergePermissions, permission)

	return hasPermission, nil
}

func CreatePatchForMerge(ctx context.Context, existingPatchID, commitMessage string) (*restModel.APIPatch, error) {
	existingPatch, err := patch.FindOneId(existingPatchID)
	if err != nil {
		return nil, errors.Wrap(err, "can't get patch")
	}
	if existingPatch == nil {
		return nil, errors.Errorf("no patch found for id '%s'", existingPatchID)
	}

	newPatch, err := model.MakeMergePatchFromExisting(ctx, existingPatch, commitMessage)
	if err != nil {
		return nil, errors.Wrap(err, "can't create new patch")
	}

	apiPatch := &restModel.APIPatch{}
	if err = apiPatch.BuildFromService(*newPatch); err != nil {
		return nil, errors.Wrap(err, "problem building API patch")
	}
	return apiPatch, nil
}

func ConcludeMerge(patchID, status string) error {
	event.LogCommitQueueConcludeTest(patchID, status)
	p, err := patch.FindOneId(patchID)
	if err != nil {
		return errors.Wrap(err, "error finding patch")
	}
	if p == nil {
		return errors.Errorf("patch '%s' not found", patchID)
	}
	cq, err := commitqueue.FindOneId(p.Project)
	if err != nil {
		return errors.Wrapf(err, "can't find commit queue for '%s'", p.Project)
	}
	if cq == nil {
		return errors.Errorf("no commit queue found for '%s'", p.Project)
	}
	item := ""
	for _, entry := range cq.Queue {
		if entry.Version == patchID {
			item = entry.Issue
			break
		}
	}
	if item == "" {
		return errors.Errorf("no entry found for patch '%s'", patchID)
	}
	found, err := cq.Remove(item)
	if err != nil {
		return errors.Wrapf(err, "can't dequeue '%s' from commit queue", item)
	}
	if found == nil {
		return errors.Errorf("item '%s' did not exist on the queue", item)
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
			Message:    errors.Wrap(err, "unable to find patch").Error(),
		}
	}
	if p == nil {
		return nil, errors.Errorf("patch '%s' not found", patchId)
	}
	cq, err := commitqueue.FindOneId(p.Project)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "unable to find commit queue").Error(),
		}
	}
	if cq == nil {
		return nil, errors.Errorf("no commit queue for project '%s' found", p.Project)
	}
	additionalPatches := []string{}
	for _, item := range cq.Queue {
		if item.Version == patchId {
			return additionalPatches, nil
		} else if item.Version != "" {
			additionalPatches = append(additionalPatches, item.Version)
		}
	}
	return nil, errors.Errorf("patch '%s' not found in queue", patchId)
}
