package units

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/utility"
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
	yaml "gopkg.in/yaml.v3"
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
	job.SetEnqueueAllScopes(true)
	job.SetScopes([]string{fmt.Sprintf("%s.%s", commitQueueJobName, queueID)})

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
	projectRef, err := model.FindMergedProjectRef(j.QueueID, "", false)
	if err != nil {
		j.AddError(errors.Wrapf(err, "can't find project for queue id %s", j.QueueID))
		return
	}
	if projectRef == nil {
		j.AddError(errors.Errorf("no project found for queue id %s", j.QueueID))
		return
	}
	if !projectRef.CommitQueue.IsEnabled() {
		grip.Info(message.Fields{
			"source":  "commit queue",
			"job_id":  j.ID(),
			"message": "project has commit queue disabled",
		})
		return
	}

	cq, err := commitqueue.FindOneId(j.QueueID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "can't find commit queue for id %s", j.QueueID))
		return
	}
	if cq == nil {
		j.AddError(errors.Errorf("no commit queue found for id %s", j.QueueID))
		return
	}

	front, hasItem := cq.Next()
	grip.InfoWhen(hasItem, message.Fields{
		"source":       "commit queue",
		"job_id":       j.ID(),
		"item_id":      front.Issue,
		"project_id":   cq.ProjectID,
		"waiting_secs": time.Since(front.EnqueueTime).Seconds(),
	})

	conf, err := evergreen.GetConfig()
	if err != nil {
		j.AddError(errors.Wrap(err, "can't get settings"))
		return
	}
	githubToken, err := conf.GetGithubOauthToken()
	if err != nil {
		j.AddError(errors.Wrap(err, "can't get github token"))
		return
	}
	j.TryUnstick(ctx, cq, projectRef, githubToken)

	if cq.Processing() {
		return
	}

	batchSize := conf.CommitQueue.BatchSize
	if batchSize < 1 {
		batchSize = 1
	}
	nextItems := cq.NextUnprocessed(batchSize)
	if len(nextItems) == 0 {
		return
	}

	for _, nextItem := range nextItems {
		// log time waiting in queue
		grip.Info(message.Fields{
			"source":       "commit queue",
			"job_id":       j.ID(),
			"item_id":      nextItem.Issue,
			"project_id":   cq.ProjectID,
			"time_waiting": time.Now().Sub(nextItem.EnqueueTime).Seconds(),
			"time_elapsed": time.Now().Sub(nextItem.ProcessingStartTime).Seconds(),
			"queue_length": len(cq.Queue),
			"message":      "started testing commit queue item",
		})

		if nextItem.Version != "" {
			grip.Error(message.Fields{
				"message": "tried to process an item twice",
				"queue":   cq.ProjectID,
				"item":    nextItem.Version,
			})
			j.AddError(j.addMergeTaskDependencies(*cq))
			return
		}

		// create a version with the item and subscribe to its completion
		if nextItem.Source == commitqueue.SourcePullRequest {
			j.processGitHubPRItem(ctx, cq, nextItem, projectRef, githubToken)
		} else if nextItem.Source == commitqueue.SourceDiff {
			j.processCLIPatchItem(ctx, cq, nextItem, projectRef, githubToken)
		} else {
			grip.Error(message.Fields{
				"message": "commit queue entry has unknown source",
				"entry":   nextItem,
				"project": projectRef.Identifier,
				"job_id":  j.ID(),
			})
		}

		grip.Info(message.Fields{
			"source":  "commit queue",
			"job_id":  j.ID(),
			"item":    nextItem,
			"message": "finished processing item",
		})
	}
	j.AddError(j.addMergeTaskDependencies(*cq))
}

func (j *commitQueueJob) addMergeTaskDependencies(cq commitqueue.CommitQueue) error {
	var prevMergeTask string
	for i, currentItem := range cq.Queue {
		if currentItem.Version == "" {
			return nil
		}
		mergeTask, err := task.FindMergeTaskForVersion(currentItem.Version)
		if err != nil {
			return errors.Wrap(err, "unable to find merge task")
		}
		if mergeTask == nil {
			return errors.Errorf("no merge task found for version '%s'", currentItem.Version)
		}
		dependency := task.Dependency{
			TaskId: prevMergeTask,
			Status: task.AllStatuses,
		}
		prevMergeTask = mergeTask.Id
		if i == 0 {
			continue
		}
		err = mergeTask.AddDependency(dependency)
		if err != nil {
			return errors.Wrap(err, "unable to add dependency")
		}
		err = mergeTask.UpdateDependsOn(dependency.Status, []string{dependency.TaskId})
		if err != nil {
			return errors.Wrap(err, "unable to update task dependencies")
		}
		err = model.RecomputeNumDependents(*mergeTask)
		if err != nil {
			return errors.Wrap(err, "unable to recompute number of dependencies")
		}
	}

	return nil
}

func (j *commitQueueJob) TryUnstick(ctx context.Context, cq *commitqueue.CommitQueue, projectRef *model.ProjectRef, githubToken string) {
	nextItem, valid := cq.Next()
	if !valid {
		return
	}

	if nextItem.Version == "" {
		return
	}

	// unstick the queue if the patch is done.
	if !patch.IsValidId(nextItem.Version) {
		j.dequeue(cq, nextItem)
		j.logError(errors.Errorf("The Patch id '%s' is not an object id", nextItem.Issue), "The patch was removed from the queue.", nextItem)
		return
	}

	patchDoc, err := patch.FindOne(patch.ByStringId(nextItem.Version).WithFields(patch.FinishTimeKey, patch.StatusKey))
	if err != nil {
		j.AddError(errors.Wrapf(err, "error finding the patch for %s", j.QueueID))
		return
	}
	if patchDoc == nil {
		j.dequeue(cq, nextItem)
		j.logError(errors.New("The patch on top of the queue is nil"), "The patch was removed from the queue.", nextItem)
		if nextItem.Source == commitqueue.SourcePullRequest {
			pr, _, err := checkPR(ctx, githubToken, nextItem.Issue, projectRef.Owner, projectRef.Repo)
			if err != nil {
				j.AddError(err)
				return
			}
			j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "commit queue entry was stuck with no patch", ""))
		}
		return
	}

	mergeTask, err := task.FindMergeTaskForVersion(nextItem.Version)
	if err != nil {
		j.AddError(errors.Wrap(err, "unable to find merge task for version"))
	}
	if mergeTask != nil {
		// check that the merge task can run. Assume that if we're here the merge task
		// should in fact run (ie. has not been dequeued due to a task failure)
		blocked := mergeTask.Blocked()
		if !mergeTask.Activated || mergeTask.Priority < 0 || blocked {
			grip.Error(message.Fields{
				"message":  "merge task is not dispatchable",
				"project":  mergeTask.Project,
				"task":     mergeTask.Id,
				"active":   mergeTask.Activated,
				"priority": mergeTask.Priority,
				"blocked":  blocked,
				"source":   "commit queue",
				"job_id":   j.ID(),
			})
			j.dequeue(cq, nextItem)
			event.LogCommitQueueConcludeTest(nextItem.Version, evergreen.EnqueueFailed)
		}
	}

	// patch is done
	if !utility.IsZeroTime(patchDoc.FinishTime) {
		j.dequeue(cq, nextItem)
		status := evergreen.MergeTestSucceeded
		if patchDoc.Status == evergreen.PatchFailed {
			status = evergreen.MergeTestFailed
		}
		event.LogCommitQueueConcludeTest(nextItem.Version, status)
		grip.Info(message.Fields{
			"source":                "commit queue",
			"patch status":          status,
			"job_id":                j.ID(),
			"item_id":               nextItem.Issue,
			"project_id":            cq.ProjectID,
			"time_since_enqueue":    time.Since(nextItem.EnqueueTime).Seconds(),
			"time_since_patch_done": time.Since(patchDoc.FinishTime).Seconds(),
			"message":               "patch done and dequeued",
		})
	}

	return
}

func (j *commitQueueJob) processGitHubPRItem(ctx context.Context, cq *commitqueue.CommitQueue, nextItem commitqueue.CommitQueueItem, projectRef *model.ProjectRef, githubToken string) {
	pr, dequeue, err := checkPR(ctx, githubToken, nextItem.Issue, projectRef.Owner, projectRef.Repo)
	if err != nil {
		j.logError(err, "PR not valid for merge", nextItem)
		if dequeue {
			if pr != nil {
				j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "PR not valid for merge", ""))
			}
			j.dequeue(cq, nextItem)
		}
		return
	}

	patchDoc, err := patch.FindOneId(nextItem.PatchId)
	if err != nil {
		j.AddError(errors.Wrapf(err, "error finding patch '%s'", nextItem.Version))
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "no patch found", ""))
		j.dequeue(cq, nextItem)
		return
	}
	if patchDoc == nil {
		j.AddError(errors.Errorf("patch '%s' not found", nextItem.Version))
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "no patch found", ""))
		j.dequeue(cq, nextItem)
		return
	}
	projectConfig, _, err := model.GetPatchedProject(ctx, patchDoc, githubToken)
	if err != nil {
		j.logError(err, "problem getting patched project", nextItem)
		j.dequeue(cq, nextItem)
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't get project config", ""))
	}

	v, err := model.FinalizePatch(ctx, patchDoc, evergreen.MergeTestRequester, githubToken)
	if err != nil {
		j.logError(err, "can't finalize patch", nextItem)
		j.dequeue(cq, nextItem)
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't finalize patch", ""))
		return
	}
	nextItem.Version = v.Id
	if err = cq.UpdateVersion(nextItem); err != nil {
		j.logError(err, "problem saving version", nextItem)
		j.dequeue(cq, nextItem)
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't update commit queue item", ""))
		return
	}

	j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStatePending, "preparing to test merge", v.Id))
	modulePRs, _, err := model.GetModulesFromPR(ctx, githubToken, patchDoc.GithubPatchData.PRNumber, nextItem.Modules, projectConfig)
	if err != nil {
		j.logError(err, "can't get modules", nextItem)
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't get modules", ""))
		j.dequeue(cq, nextItem)
		return
	}
	for _, modulePR := range modulePRs {
		j.AddError(sendCommitQueueGithubStatus(j.env, modulePR, message.GithubStatePending, "preparing to test merge", patchDoc.Id.Hex()))
	}

	event.LogCommitQueueStartTestEvent(v.Id)
}

func (j *commitQueueJob) processCLIPatchItem(ctx context.Context, cq *commitqueue.CommitQueue, nextItem commitqueue.CommitQueueItem, projectRef *model.ProjectRef, githubToken string) {
	patchDoc, err := patch.FindOneId(nextItem.Issue)
	if err != nil {
		j.logError(err, "can't find patch", nextItem)
		event.LogCommitQueueEnqueueFailed(nextItem.Issue, err)
		j.dequeue(cq, nextItem)
		return
	}
	if patchDoc == nil {
		j.logError(err, "patch not found", nextItem)
		event.LogCommitQueueEnqueueFailed(nextItem.Issue, err)
		j.dequeue(cq, nextItem)
		return
	}

	project, err := updatePatch(ctx, githubToken, projectRef, patchDoc)
	if err != nil {
		j.logError(err, "can't update patch", nextItem)
		event.LogCommitQueueEnqueueFailed(nextItem.Issue, err)
		j.dequeue(cq, nextItem)
		return
	}

	if err = AddMergeTaskAndVariant(patchDoc, project, projectRef, commitqueue.SourceDiff); err != nil {
		j.logError(err, "can't set patch project config", nextItem)
		event.LogCommitQueueEnqueueFailed(nextItem.Issue, err)
		j.dequeue(cq, nextItem)
		return
	}

	if err = patchDoc.UpdateGithashProjectAndTasks(); err != nil {
		j.logError(err, "can't update patch in db", nextItem)
		event.LogCommitQueueEnqueueFailed(nextItem.Issue, err)
		j.dequeue(cq, nextItem)
		return
	}

	v, err := model.FinalizePatch(ctx, patchDoc, evergreen.MergeTestRequester, githubToken)
	if err != nil {
		j.logError(err, "can't finalize patch", nextItem)
		event.LogCommitQueueEnqueueFailed(nextItem.Issue, err)
		j.dequeue(cq, nextItem)
		return
	}
	nextItem.Version = v.Id
	if err = cq.UpdateVersion(nextItem); err != nil {
		j.logError(err, "problem saving version", nextItem)
		j.dequeue(cq, nextItem)
		return
	}

	if err = setDefaultNotification(patchDoc.Author); err != nil {
		j.logError(err, "failed to set default notification", nextItem)
	}
	event.LogCommitQueueStartTestEvent(v.Id)
}

func (j *commitQueueJob) logError(err error, msg string, item commitqueue.CommitQueueItem) {
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

func (j *commitQueueJob) dequeue(cq *commitqueue.CommitQueue, item commitqueue.CommitQueueItem) {
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

	return pr, false, nil
}

func sendCommitQueueGithubStatus(env evergreen.Environment, pr *github.PullRequest, state message.GithubState, description, versionID string) error {
	if pr == nil {
		return nil
	}
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
		Context:     commitqueue.GithubContext,
		State:       state,
		Description: description,
		URL:         url,
	}

	c := message.NewGithubStatusMessageWithRepo(level.Notice, msg)
	sender.Send(c)

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

func AddMergeTaskAndVariant(patchDoc *patch.Patch, project *model.Project, projectRef *model.ProjectRef, source string) error {
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
	dependencies := []model.TaskUnitDependency{}
	for _, vt := range patchDoc.VariantsTasks {
		for _, t := range vt.Tasks {
			dependencies = append(dependencies, model.TaskUnitDependency{
				Name:    t,
				Variant: vt.Variant,
				Status:  "",
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

	validationErrors := validator.CheckProjectSyntax(project, true)
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

		return u.UpdateSettings(u.Settings)
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

	// rebuild patch build variants and tasks
	patchDoc.BuildVariants = []string{}
	patchDoc.VariantsTasks = []patch.VariantTasks{}
	patchDoc.Tasks = []string{}
	project.BuildProjectTVPairs(patchDoc, patchDoc.Alias)

	return project, nil
}
