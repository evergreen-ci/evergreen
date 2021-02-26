package data

import (
	"context"
	"fmt"
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
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

type DBCommitQueueConnector struct{}

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

	patchDoc, err := patch.MakeNewMergePatch(pr, projectRef.Id, evergreen.CommitQueueAlias, messageOverride)
	if err != nil {
		return "", errors.Wrap(err, "unable to make commit queue patch")
	}

	p, patchSummaries, projectConfig, err := getPatchInfo(ctx, githubToken, patchDoc)
	if err != nil {
		return "", err
	}

	errs := validator.CheckProjectSyntax(projectConfig)
	errs = append(errs, validator.CheckProjectSettings(projectConfig, &projectRef)...)
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

	modulePRs, modulePatches, err := getModules(ctx, githubToken, prNum, modules, projectConfig)
	if err != nil {
		return "", err
	}
	patchDoc.Patches = append(patchDoc.Patches, modulePatches...)

	// populate tasks/variants matching the commitqueue alias
	projectConfig.BuildProjectTVPairs(patchDoc, patchDoc.Alias)

	if err = addMergeTaskAndVariant(patchDoc, projectConfig, &projectRef, commitqueue.SourcePullRequest); err != nil {
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
	config, projectYaml, err := model.GetPatchedProject(ctx, patchDoc, githubToken)
	if err != nil {
		return "", nil, nil, errors.Wrap(err, "can't get remote config file")
	}

	patchDoc.PatchedConfig = projectYaml
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

func getModules(ctx context.Context, githubToken string, prNum int, modules []restModel.APIModule, projectConfig *model.Project) ([]*github.PullRequest, []patch.ModulePatch, error) {
	var modulePRs []*github.PullRequest
	var modulePatches []patch.ModulePatch
	for _, mod := range modules {
		module, err := projectConfig.GetModuleByName(utility.FromStringPtr(mod.Module))
		if err != nil {
			return nil, nil, errors.Wrapf(err, "can't get module for module name '%s'", *mod.Module)
		}
		owner, repo, err := thirdparty.ParseGitUrl(module.Repo)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "module '%s' misconfigured (malformed URL)", *mod.Module)
		}

		pr, err := thirdparty.GetPullRequest(ctx, prNum, githubToken, owner, repo)
		if err != nil {
			return nil, nil, errors.Wrap(err, "PR not valid for merge")
		}
		modulePRs = append(modulePRs, pr)
		githash := pr.GetMergeCommitSHA()

		modulePatches = append(modulePatches, patch.ModulePatch{
			ModuleName: utility.FromStringPtr(mod.Module),
			Githash:    githash,
			PatchSet: patch.PatchSet{
				Patch: utility.FromStringPtr(mod.Issue),
			},
		})
	}

	return modulePRs, modulePatches, nil
}

func addMergeTaskAndVariant(patchDoc *patch.Patch, project *model.Project, projectRef *model.ProjectRef, source string) error {
	settings, err := evergreen.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error retrieving Evergreen config")
	}

	modules := make([]string, 0, len(patchDoc.Patches))
	for _, module := range patchDoc.Patches {
		if module.ModuleName != "" {
			modules = append(modules, module.ModuleName)
		}
	}

	mergeBuildVariant := model.BuildVariant{
		Name:        evergreen.MergeTaskVariant,
		DisplayName: "Commit Queue Merge",
		RunOn:       []string{settings.CommitQueue.MergeTaskDistro},
		Tasks: []model.BuildVariantTaskUnit{
			{
				Name:             evergreen.MergeTaskGroup,
				IsGroup:          true,
				CommitQueueMerge: true,
			},
		},
		Modules: modules,
	}

	// Merge task depends on all the tasks already in the patch
	status := ""
	if source == commitqueue.SourcePullRequest {
		// for pull requests we need to run the merge task at the end even if something
		// fails, so that it can tell github something failed
		status = model.AllStatuses
	}
	dependencies := []model.TaskUnitDependency{}
	for _, vt := range patchDoc.VariantsTasks {
		for _, t := range vt.Tasks {
			dependencies = append(dependencies, model.TaskUnitDependency{
				Name:    t,
				Variant: vt.Variant,
				Status:  status,
			})
		}
	}

	mergeTask := model.ProjectTask{
		Name: evergreen.MergeTaskName,
		Commands: []model.PluginCommandConf{
			{
				Command: "git.get_project",
				Type:    evergreen.CommandTypeSetup,
				Params: map[string]interface{}{
					"directory":       "src",
					"committer_name":  settings.CommitQueue.CommitterName,
					"committer_email": settings.CommitQueue.CommitterEmail,
				},
			},
		},
		DependsOn: dependencies,
	}

	if source == commitqueue.SourceDiff {
		mergeTask.Commands = append(mergeTask.Commands,
			model.PluginCommandConf{
				Command: "git.push",
				Params: map[string]interface{}{
					"directory": "src",
				},
			})
	} else if source == commitqueue.SourcePullRequest {
		mergeTask.Commands = append(mergeTask.Commands,
			model.PluginCommandConf{
				Command: "git.merge_pr",
				Params: map[string]interface{}{
					"url": fmt.Sprintf("%s/version/%s", settings.Ui.Url, patchDoc.Id.Hex()),
				},
			})
	} else {
		return errors.Errorf("unknown commit queue source: %s", source)
	}

	// Define as part of a task group with no pre to skip
	// running a project's pre before the merge task
	mergeTaskGroup := model.TaskGroup{
		Name:     evergreen.MergeTaskGroup,
		Tasks:    []string{evergreen.MergeTaskName},
		MaxHosts: 1,
	}

	project.BuildVariants = append(project.BuildVariants, mergeBuildVariant)
	project.Tasks = append(project.Tasks, mergeTask)
	project.TaskGroups = append(project.TaskGroups, mergeTaskGroup)

	validationErrors := validator.CheckProjectSyntax(project)
	validationErrors = append(validationErrors, validator.CheckProjectSettings(project, projectRef)...)
	catcher := grip.NewBasicCatcher()
	for _, validationErr := range validationErrors.AtLevel(validator.Error) {
		catcher.Add(validationErr)
	}
	if catcher.HasErrors() {
		return errors.Errorf("project validation failed: %s", catcher.Resolve())
	}

	yamlBytes, err := yaml.Marshal(project)
	if err != nil {
		return errors.Wrap(err, "can't marshall remote config file")
	}

	patchDoc.PatchedConfig = string(yamlBytes)
	patchDoc.BuildVariants = append(patchDoc.BuildVariants, evergreen.MergeTaskVariant)
	patchDoc.Tasks = append(patchDoc.Tasks, evergreen.MergeTaskName)
	patchDoc.VariantsTasks = append(patchDoc.VariantsTasks, patch.VariantTasks{
		Variant: evergreen.MergeTaskVariant,
		Tasks:   []string{evergreen.MergeTaskName},
	})

	return nil
}

func (pc *DBCommitQueueConnector) EnqueueItem(projectID string, item restModel.APICommitQueueItem, enqueueNext bool) (int, error) {
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

func (pc *DBCommitQueueConnector) FindCommitQueueForProject(name string) (*restModel.APICommitQueue, error) {
	id, err := model.FindIdForProject(name)
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

func (pc *DBCommitQueueConnector) CommitQueueRemoveItem(id, issue, user string) (*restModel.APICommitQueueItem, error) {
	projectRef, err := model.FindOneProjectRef(id)
	if err != nil {
		return nil, errors.Wrapf(err, "can't find projectRef for '%s'", id)
	}
	if projectRef == nil {
		return nil, errors.Errorf("can't find project ref for '%s'", id)
	}
	cq, err := commitqueue.FindOneId(projectRef.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get commit queue for id '%s'", id)
	}
	if cq == nil {
		return nil, errors.Errorf("no commit queue found for '%s'", id)
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

func (pc *DBCommitQueueConnector) IsItemOnCommitQueue(id, item string) (bool, error) {
	cq, err := commitqueue.FindOneId(id)
	if err != nil {
		return false, errors.Wrapf(err, "can't get commit queue for id '%s'", id)
	}
	if cq == nil {
		return false, errors.Errorf("no commit queue found for '%s'", id)
	}

	pos := cq.FindItem(item)
	if pos >= 0 {
		return true, nil
	}
	return false, nil
}

func (pc *DBCommitQueueConnector) CommitQueueClearAll() (int, error) {
	return commitqueue.ClearAllCommitQueues()
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

	return inOrg && hasPermission, nil
}

func (pc *DBCommitQueueConnector) CreatePatchForMerge(ctx context.Context, existingPatchID, commitMessage string) (*restModel.APIPatch, error) {
	existingPatch, err := patch.FindOneId(existingPatchID)
	if err != nil {
		return nil, errors.Wrap(err, "can't get patch")
	}
	if existingPatch == nil {
		return nil, errors.Errorf("no patch found for id '%s'", existingPatchID)
	}

	newPatch, err := model.MakeMergePatchFromExisting(existingPatch, commitMessage)
	if err != nil {
		return nil, errors.Wrap(err, "can't create new patch")
	}

	apiPatch := &restModel.APIPatch{}
	if err = apiPatch.BuildFromService(*newPatch); err != nil {
		return nil, errors.Wrap(err, "problem building API patch")
	}
	return apiPatch, nil
}

func (pc *DBCommitQueueConnector) GetMessageForPatch(patchID string) (string, error) {
	requestedPatch, err := patch.FindOneId(patchID)
	if err != nil {
		return "", errors.Wrap(err, "error finding patch")
	}
	if requestedPatch == nil {
		return "", errors.New("no patch found")
	}
	project, err := model.FindOneProjectRef(requestedPatch.Project)
	if err != nil {
		return "", errors.Wrap(err, "unable to find project for patch")
	}
	if project == nil {
		return "", errors.New("patch has nonexistent project")
	}

	return project.CommitQueue.Message, nil
}

func (pc *DBCommitQueueConnector) ConcludeMerge(patchID, status string) error {
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
	return nil
}

func (pc *DBCommitQueueConnector) GetAdditionalPatches(patchId string) ([]string, error) {
	p, err := patch.FindOneId(patchId)
	if err != nil {
		return nil, errors.Wrap(err, "unable to find patch")
	}
	if p == nil {
		return nil, errors.Errorf("patch '%s' not found", patchId)
	}
	cq, err := commitqueue.FindOneId(p.Project)
	if err != nil {
		return nil, errors.Wrap(err, "unable to find commit queue")
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

type MockCommitQueueConnector struct {
	Queue map[string][]restModel.APICommitQueueItem
}

func (pc *MockCommitQueueConnector) GetGitHubPR(ctx context.Context, owner, repo string, PRNum int) (*github.PullRequest, error) {
	return &github.PullRequest{
		User: &github.User{
			ID:    github.Int64(1234),
			Login: github.String("github.user"),
		},
		Base: &github.PullRequestBranch{
			Ref: github.String("main"),
		},
		Head: &github.PullRequestBranch{
			SHA: github.String("abcdef1234"),
		},
	}, nil
}

func (pc *MockCommitQueueConnector) AddPatchForPr(ctx context.Context, projectRef model.ProjectRef, prNum int, modules []restModel.APIModule, messageOverride string) (string, error) {
	return "", nil
}

func (pc *MockCommitQueueConnector) EnqueueItem(projectID string, item restModel.APICommitQueueItem, enqueueNext bool) (int, error) {
	if pc.Queue == nil {
		pc.Queue = make(map[string][]restModel.APICommitQueueItem)
	}
	if enqueueNext && len(pc.Queue[projectID]) > 0 {
		q := pc.Queue[projectID]
		pc.Queue[projectID] = append([]restModel.APICommitQueueItem{q[0], item}, q[1:]...)
		return 1, nil
	}
	pc.Queue[projectID] = append(pc.Queue[projectID], item)
	return len(pc.Queue[projectID]) - 1, nil
}

func (pc *MockCommitQueueConnector) FindCommitQueueForProject(id string) (*restModel.APICommitQueue, error) {
	if _, ok := pc.Queue[id]; !ok {
		return nil, nil
	}

	return &restModel.APICommitQueue{ProjectID: utility.ToStringPtr(id), Queue: pc.Queue[id]}, nil
}

func (pc *MockCommitQueueConnector) CommitQueueRemoveItem(id, itemId, user string) (*restModel.APICommitQueueItem, error) {
	if _, ok := pc.Queue[id]; !ok {
		return nil, nil
	}

	for i := range pc.Queue[id] {
		if utility.FromStringPtr(pc.Queue[id][i].Issue) == itemId {
			item := pc.Queue[id][i]
			pc.Queue[id] = append(pc.Queue[id][:i], pc.Queue[id][i+1:]...)
			return &item, nil
		}
	}

	return nil, nil
}

func (pc *MockCommitQueueConnector) IsItemOnCommitQueue(id, item string) (bool, error) {
	queue, ok := pc.Queue[id]
	if !ok {
		return false, errors.Errorf("can't get commit queue for id '%s'", id)
	}
	for _, queueItem := range queue {
		if utility.FromStringPtr(queueItem.Issue) == item {
			return true, nil
		}
	}
	return false, nil
}

func (pc *MockCommitQueueConnector) CommitQueueClearAll() (int, error) {
	var count int
	for k, v := range pc.Queue {
		if len(v) > 0 {
			count++
		}
		pc.Queue[k] = []restModel.APICommitQueueItem{}
	}

	return count, nil
}

func (pc *MockCommitQueueConnector) IsAuthorizedToPatchAndMerge(context.Context, *evergreen.Settings, UserRepoInfo) (bool, error) {
	return true, nil
}

func (pc *MockCommitQueueConnector) CreatePatchForMerge(ctx context.Context, existingPatchID, commitMessage string) (*restModel.APIPatch, error) {
	return nil, nil
}
func (pc *MockCommitQueueConnector) GetMessageForPatch(patchID string) (string, error) {
	return "", nil
}

func (pc *MockCommitQueueConnector) ConcludeMerge(patchID, status string) error {
	return nil
}
func (pc *MockCommitQueueConnector) GetAdditionalPatches(patchId string) ([]string, error) {
	return nil, nil
}
