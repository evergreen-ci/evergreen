package units

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/google/go-github/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

const (
	commitQueueJobName = "commit-queue"
)

func init() {
	registry.AddJobType(commitQueueJobName, func() amboy.Job { return makeCommitQueueJob() })
}

type commitQueueJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	QueueID  string `bson:"queue_id" json:"queue_id" yaml:"queue_id"`
	env      evergreen.Environment
}

func makeCommitQueueJob() *commitQueueJob {
	job := &commitQueueJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    commitQueueJobName,
				Version: 0,
			},
		},
	}
	job.SetDependency(dependency.NewAlways())

	return job
}

func NewCommitQueueJob(env evergreen.Environment, queueID string, id string) amboy.Job {
	job := makeCommitQueueJob()
	job.QueueID = queueID
	job.env = env
	job.SetID(fmt.Sprintf("%s:%s_%s", commitQueueJobName, queueID, id))

	return job
}

func (j *commitQueueJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	// reconstitute the environment because it's not stored in the database
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	// stop if degraded
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(errors.New("can't get degraded mode flags"))
		return
	}
	if flags.CommitQueueDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     commitQueueJobName,
			"message": "commit queue processing is disabled",
		})
		return
	}

	// stop if project is disabled
	projectRef, err := model.FindOneProjectRef(j.QueueID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "can't find project for queue id %s", j.QueueID))
		return
	}
	if !projectRef.CommitQueue.Enabled {
		grip.Info(message.Fields{
			"source":  "commit queue",
			"job_id":  j.ID(),
			"message": "project has commit queue disabled",
		})
		return
	}

	// pull the next item off the queue
	cq, err := commitqueue.FindOneId(j.QueueID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "can't find commit queue for id %s", j.QueueID))
		return
	}
	nextItem := cq.Next()
	if nextItem == nil {
		return
	}
	if cq.Processing {
		grip.Info(message.Fields{
			"source":             "commit queue",
			"job_id":             j.ID(),
			"item_id":            nextItem.Issue,
			"project_id":         cq.ProjectID,
			"processing_seconds": time.Since(cq.ProcessingUpdatedTime).Seconds(),
		})
		return
	}
	j.AddError(errors.Wrap(cq.SetProcessing(true), "can't set processing to true"))

	// log time waiting in queue
	timeWaiting := time.Now().Sub(nextItem.EnqueueTime)
	grip.Info(message.Fields{
		"source":       "commit queue",
		"job_id":       j.ID(),
		"item_id":      nextItem.Issue,
		"project_id":   cq.ProjectID,
		"time_waiting": timeWaiting.Seconds(),
		"queue_length": len(cq.Queue),
		"message":      "dequeued commit queue item",
	})

	conf := j.env.Settings()
	githubToken, err := conf.GetGithubOauthToken()
	if err != nil {
		j.AddError(errors.Wrap(err, "can't get github token"))
		j.AddError(errors.Wrap(cq.SetProcessing(false), "can't set processing to false"))
		return
	}

	// create a version with the item and subscribe to its completion
	if projectRef.CommitQueue.PatchType == commitqueue.PRPatchType {
		j.processGitHubPRItem(ctx, cq, nextItem, projectRef, githubToken)
	}
	if projectRef.CommitQueue.PatchType == commitqueue.CLIPatchType {
		j.processCLIPatchItem(ctx, cq, nextItem, projectRef, githubToken)
	}

	grip.Info(message.Fields{
		"source":  "commit queue",
		"job_id":  j.ID(),
		"item":    nextItem,
		"message": "finished processing item",
	})
}

func (j *commitQueueJob) processGitHubPRItem(ctx context.Context, cq *commitqueue.CommitQueue, nextItem *commitqueue.CommitQueueItem, projectRef *model.ProjectRef, githubToken string) {
	pr, dequeue, err := checkPR(ctx, githubToken, nextItem.Issue, projectRef.Owner, projectRef.Repo)
	if err != nil {
		j.logError(err, "PR not valid for merge", nextItem)
		if dequeue {
			if pr != nil {
				j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "PR not valid for merge", ""))
			}
			j.dequeue(cq, nextItem)
		} else {
			j.logError(cq.SetProcessing(false), "can't set processing to false", nextItem)
		}
		return
	}

	patchDoc, err := patch.MakeMergePatch(pr, projectRef.Identifier, evergreen.CommitQueueAlias)
	if err != nil {
		j.logError(err, "can't make patch", nextItem)
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't make patch", ""))
		j.dequeue(cq, nextItem)
		return
	}

	patch, patchSummaries, projectConfig, err := getPatchInfo(ctx, githubToken, patchDoc)
	if err != nil {
		j.logError(err, "can't get patch info", nextItem)
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't make patch", ""))
		j.dequeue(cq, nextItem)
		return
	}

	errs := validator.CheckProjectSyntax(projectConfig)
	catcher := grip.NewBasicCatcher()
	for _, validationError := range errs {
		if validationError.Level == validator.Error {
			catcher.Add(validationError)
		}
	}
	if catcher.HasErrors() {
		update := NewGithubStatusUpdateJobForProcessingError(
			commitqueue.Context,
			pr.Base.User.GetLogin(),
			pr.Base.Repo.GetName(),
			pr.Head.GetRef(),
			InvalidConfig,
		)
		update.Run(ctx)
		j.AddError(update.Error())
		j.logError(catcher.Resolve(), "invalid config file", nextItem)
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't make patch", ""))
		j.dequeue(cq, nextItem)
		return
	}

	if err = writePatchInfo(patchDoc, patchSummaries, patch); err != nil {
		j.logError(err, "can't make patch", nextItem)
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't make patch", ""))
		j.dequeue(cq, nextItem)
		return
	}

	modulePRs, modulePatches, dequeue, err := getModules(ctx, githubToken, nextItem, projectConfig)
	if err != nil {
		j.logError(err, "can't get modules", nextItem)
		if dequeue {
			j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't get modules", ""))
			j.dequeue(cq, nextItem)
		} else {
			j.logError(cq.SetProcessing(false), "can't set processing to false", nextItem)
		}
		return
	}
	patchDoc.Patches = append(patchDoc.Patches, modulePatches...)

	// populate tasks/variants matching the commitqueue alias
	projectConfig.BuildProjectTVPairs(patchDoc, patchDoc.Alias)

	if err = patchDoc.Insert(); err != nil {
		j.logError(err, "can't insert patch", nextItem)
		j.dequeue(cq, nextItem)
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't make patch", ""))
	}
	nextItem.Version = patchDoc.Id.Hex()
	if err = cq.UpdateVersion(*nextItem); err != nil {
		j.logError(err, "problem saving version", nextItem)
	}
	v, err := model.FinalizePatch(ctx, patchDoc, evergreen.MergeTestRequester, githubToken)
	if err != nil {
		j.logError(err, "can't finalize patch", nextItem)
		j.dequeue(cq, nextItem)
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't finalize patch", ""))
	}

	err = subscribeGitHubPRs(pr, modulePRs, projectRef, v.Id)
	if err != nil {
		j.logError(err, "can't subscribe for PR merge", nextItem)
		j.dequeue(cq, nextItem)
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't sign up merge", v.Id))
	}

	j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStatePending, "preparing to test merge", v.Id))
	for _, modulePR := range modulePRs {
		j.AddError(sendCommitQueueGithubStatus(j.env, modulePR, message.GithubStatePending, "preparing to test merge", v.Id))
	}

	event.LogCommitQueueStartTestEvent(v.Id)
}

func (j *commitQueueJob) processCLIPatchItem(ctx context.Context, cq *commitqueue.CommitQueue, nextItem *commitqueue.CommitQueueItem, projectRef *model.ProjectRef, githubToken string) {
	patchDoc, err := patch.FindOne(patch.ById(patch.NewId(nextItem.Issue)))
	if err != nil {
		j.logError(err, "can't find patch", nextItem)
		j.dequeue(cq, nextItem)
		return
	}

	project, err := updatePatch(ctx, githubToken, projectRef, patchDoc)
	if err != nil {
		j.logError(err, "can't update patch", nextItem)
		j.dequeue(cq, nextItem)
		return
	}

	if err = addMergeTaskAndVariant(patchDoc, project); err != nil {
		j.logError(err, "can't set patch project config", nextItem)
		j.dequeue(cq, nextItem)
		return
	}

	project.BuildProjectTVPairs(patchDoc, patchDoc.Alias)
	if err = patchDoc.UpdateGithashProjectAndTasks(); err != nil {
		j.logError(err, "can't update patch in db", nextItem)
		j.dequeue(cq, nextItem)
		return
	}

	v, err := model.FinalizePatch(ctx, patchDoc, evergreen.MergeTestRequester, githubToken)
	if err != nil {
		j.logError(err, "can't finalize patch", nextItem)
		j.dequeue(cq, nextItem)
		return
	}

	subscriber := event.NewCommitQueueDequeueSubscriber()

	patchSub := event.NewPatchOutcomeSubscription(nextItem.Issue, subscriber)
	if err = patchSub.Upsert(); err != nil {
		j.logError(err, "failed to insert patch subscription", nextItem)
		j.dequeue(cq, nextItem)
	}

	if err = setDefaultNotification(patchDoc.Author); err != nil {
		j.logError(err, "failed to set default notification", nextItem)
	}
	event.LogCommitQueueStartTestEvent(v.Id)
}

func (j *commitQueueJob) logError(err error, msg string, item *commitqueue.CommitQueueItem) {
	if err == nil {
		return
	}
	j.AddError(errors.Wrap(err, msg))
	grip.Error(message.WrapError(err, message.Fields{
		"job_id":  j.ID(),
		"source":  "commit queue",
		"project": j.QueueID,
		"item":    item,
		"message": msg,
	}))
}

func (j *commitQueueJob) dequeue(cq *commitqueue.CommitQueue, item *commitqueue.CommitQueueItem) {
	_, err := cq.Remove(item.Issue)
	j.logError(err, fmt.Sprintf("error dequeuing item '%s'", item.Issue), item)
}

func checkPR(ctx context.Context, githubToken, issue, owner, repo string) (*github.PullRequest, bool, error) {
	issueInt, err := strconv.Atoi(issue)
	if err != nil {
		return nil, true, errors.Wrapf(err, "can't parse issue '%s' as int", issue)
	}

	pr, err := thirdparty.GetGithubPullRequest(ctx, githubToken, owner, repo, issueInt)
	if err != nil {
		return nil, false, errors.Wrap(err, "can't get PR from GitHub")
	}

	if err = thirdparty.ValidatePR(pr); err != nil {
		return nil, true, errors.Wrap(err, "GitHub returned an incomplete PR")
	}

	if pr.Mergeable == nil {
		if *pr.Merged {
			return pr, true, errors.New("PR is already merged")
		}
		// GitHub hasn't yet tested if the PR is mergeable.
		// Check back later
		// See: https://developer.github.com/v3/pulls/#response-1
		return pr, false, errors.New("GitHub hasn't yet generated a merge commit")
	}

	if !*pr.Mergeable {
		return pr, true, errors.New("PR is not mergeable")
	}

	return pr, false, nil
}

func getModules(ctx context.Context, githubToken string, nextItem *commitqueue.CommitQueueItem, projectConfig *model.Project) ([]*github.PullRequest, []patch.ModulePatch, bool, error) {
	var modulePRs []*github.PullRequest
	var modulePatches []patch.ModulePatch
	for _, mod := range nextItem.Modules {
		module, err := projectConfig.GetModuleByName(mod.Module)
		if err != nil {
			return nil, nil, true, errors.Wrapf(err, "can't get module for module name '%s'", mod.Module)
		}
		owner, repo, err := thirdparty.ParseGitUrl(module.Repo)
		if err != nil {
			return nil, nil, true, errors.Wrapf(err, "module '%s' misconfigured (malformed URL)", mod.Module)
		}

		pr, dequeue, err := checkPR(ctx, githubToken, mod.Issue, owner, repo)
		if err != nil {
			return nil, nil, dequeue, errors.Wrap(err, "PR not valid for merge")
		}
		modulePRs = append(modulePRs, pr)
		githash := pr.GetMergeCommitSHA()

		modulePatches = append(modulePatches, patch.ModulePatch{
			ModuleName: mod.Module,
			Githash:    githash,
			PatchSet: patch.PatchSet{
				Patch: mod.Issue,
			},
		})
	}

	return modulePRs, modulePatches, false, nil
}

func getPatchInfo(ctx context.Context, githubToken string, patchDoc *patch.Patch) (string, []patch.Summary, *model.Project, error) {
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

func writePatchInfo(patchDoc *patch.Patch, patchSummaries []patch.Summary, patchContent string) error {
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

func sendCommitQueueGithubStatus(env evergreen.Environment, pr *github.PullRequest, state message.GithubState, description, versionID string) error {
	sender, err := env.GetSender(evergreen.SenderGithubStatus)
	if err != nil {
		return errors.Wrap(err, "can't get GitHub status sender")
	}

	var url string
	if versionID != "" {
		uiConfig := evergreen.UIConfig{}
		if err := uiConfig.Get(env); err == nil {
			urlBase := uiConfig.Url
			url = fmt.Sprintf("%s/version/%s", urlBase, versionID)
		}
	}

	msg := message.GithubStatus{
		Owner:       *pr.Base.Repo.Owner.Login,
		Repo:        *pr.Base.Repo.Name,
		Ref:         *pr.Head.SHA,
		Context:     commitqueue.Context,
		State:       state,
		Description: description,
		URL:         url,
	}

	c := message.NewGithubStatusMessageWithRepo(level.Notice, msg)
	sender.Send(c)

	return nil
}

func subscribeGitHubPRs(pr *github.PullRequest, modulePRs []*github.PullRequest, projectRef *model.ProjectRef, patchID string) error {
	prs := make([]event.PRInfo, 0, len(modulePRs)+1)
	for _, modulePR := range modulePRs {
		prs = append(prs, event.PRInfo{
			Owner:       modulePR.Base.Repo.Owner.GetLogin(),
			Repo:        *modulePR.Base.Repo.Name,
			Ref:         *modulePR.Head.SHA,
			PRNum:       *modulePR.Number,
			CommitTitle: fmt.Sprintf("%s (#%d)", *modulePR.Title, *modulePR.Number),
		})
	}
	prs = append(prs, event.PRInfo{
		Owner:       projectRef.Owner,
		Repo:        projectRef.Repo,
		Ref:         *pr.Head.SHA,
		PRNum:       *pr.Number,
		CommitTitle: fmt.Sprintf("%s (#%d)", *pr.Title, *pr.Number),
	})

	mergeSubscriber := event.NewGithubMergeSubscriber(event.GithubMergeSubscriber{
		PRs:         prs,
		Item:        strconv.Itoa(*pr.Number),
		MergeMethod: projectRef.CommitQueue.MergeMethod,
	})
	patchSub := event.NewPatchOutcomeSubscription(patchID, mergeSubscriber)
	if err := patchSub.Upsert(); err != nil {
		return errors.Wrapf(err, "failed to insert patch subscription for commit queue merge on PR %d", *pr.Number)
	}

	return nil
}

func validateBranch(branch *github.Branch) error {
	if branch == nil {
		return errors.New("branch is nil")
	}
	if branch.Commit == nil {
		return errors.New("commit is nil")
	}
	if branch.Commit.SHA == nil {
		return errors.New("SHA is nil")
	}
	return nil
}

func addMergeTaskAndVariant(patchDoc *patch.Patch, project *model.Project) error {
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
				CommitQueueMerge: true,
			},
		},
		Modules: modules,
	}

	// Merge task depends on all commit queue tasks matching the alias
	// (protect against a user removing tasks from the patch)
	execPairs, _, err := project.BuildProjectTVPairsWithAlias(evergreen.CommitQueueAlias)
	if err != nil {
		return errors.Wrap(err, "can't get alias pairs")
	}
	dependencies := make([]model.TaskUnitDependency, 0, len(execPairs))
	for _, pair := range execPairs {
		dependencies = append(dependencies, model.TaskUnitDependency{
			Name:    pair.TaskName,
			Variant: pair.Variant,
		})
	}

	mergeTask := model.ProjectTask{
		Name: evergreen.MergeTaskName,
		Commands: []model.PluginCommandConf{
			{
				Command: "git.get_project",
				Type:    evergreen.CommandTypeSetup,
				Params: map[string]interface{}{
					"directory": "src",
				},
			},
			{
				Command: "git.push",
				Params: map[string]interface{}{
					"directory":       "src",
					"committer_name":  settings.CommitQueue.CommitterName,
					"committer_email": settings.CommitQueue.CommitterEmail,
				},
			},
		},
		DependsOn: dependencies,
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
	catcher := grip.NewBasicCatcher()
	for _, validationError := range validationErrors {
		if validationError.Level == validator.Error {
			catcher.Add(validationError)
		}
	}
	if catcher.HasErrors() {
		return errors.Errorf("project validation failed: %s", catcher.Resolve())
	}

	yamlBytes, err := yaml.Marshal(project)
	if err != nil {
		return errors.Wrap(err, "can't marshall remote config file")
	}

	patchDoc.PatchedConfig = string(yamlBytes)
	patchDoc.BuildVariants = append(patchDoc.BuildVariants, "commit-queue-merge")
	patchDoc.Tasks = append(patchDoc.Tasks, "merge-patch")

	return nil
}

func setDefaultNotification(username string) error {
	u, err := user.FindOneById(username)
	if err != nil {
		return errors.Wrap(err, "can't get user")
	}
	if u == nil {
		return errors.Errorf("no matching user for %s", username)
	}

	// The user has never saved their notification settings
	if u.Settings.Notifications.CommitQueue == "" {
		u.Settings.Notifications.CommitQueue = user.PreferenceEmail
		commitQueueSubscriber := event.NewEmailSubscriber(u.Email())
		commitQueueSubscription, err := event.CreateOrUpdateImplicitSubscription(event.ImplicitSubscriptionCommitQueue,
			"", commitQueueSubscriber, u.Id)
		if err != nil {
			return errors.Wrap(err, "can't create default email subscription")
		}
		u.Settings.Notifications.CommitQueueID = commitQueueSubscription.ID

		return model.SaveUserSettings(u.Id, u.Settings)
	}

	return nil
}

func updatePatch(ctx context.Context, githubToken string, projectRef *model.ProjectRef, patchDoc *patch.Patch) (*model.Project, error) {
	branch, err := thirdparty.GetBranchEvent(ctx, githubToken, projectRef.Owner, projectRef.Repo, projectRef.Branch)
	if err != nil {
		return nil, errors.Wrap(err, "can't get branch")
	}
	if err = validateBranch(branch); err != nil {
		return nil, errors.Wrap(err, "GitHub returned invalid branch")
	}

	sha := *branch.Commit.SHA
	patchDoc.Githash = sha

	// Refresh the cached project config
	patchDoc.PatchedConfig = ""
	project, projectYaml, err := model.GetPatchedProject(ctx, patchDoc, githubToken)
	if err != nil {
		return nil, errors.Wrap(err, "can't get updated project config")
	}
	patchDoc.PatchedConfig = projectYaml

	// Update module githashes
	for i, mod := range patchDoc.Patches {
		if mod.ModuleName == "" {
			patchDoc.Patches[i].Githash = sha
			continue
		}

		module, err := project.GetModuleByName(mod.ModuleName)
		if err != nil {
			return nil, errors.Wrapf(err, "can't get module for module name '%s'", mod.ModuleName)
		}
		owner, repo, err := thirdparty.ParseGitUrl(module.Repo)
		if err != nil {
			return nil, errors.Wrapf(err, "module '%s' misconfigured (malformed URL)", mod.ModuleName)
		}

		branch, err = thirdparty.GetBranchEvent(ctx, githubToken, owner, repo, module.Branch)
		if err != nil {
			return nil, errors.Wrap(err, "can't get branch")
		}
		if err = validateBranch(branch); err != nil {
			return nil, errors.Wrap(err, "GitHub returned invalid branch")
		}

		patchDoc.Patches[i].Githash = *branch.Commit.SHA
	}

	// reset patch build variants and tasks
	patchDoc.BuildVariants = []string{}
	patchDoc.VariantsTasks = []patch.VariantTasks{}
	patchDoc.Tasks = []string{}

	return project, nil
}
