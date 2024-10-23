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
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
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
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting service flags"))
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
		j.AddError(errors.Wrapf(err, "finding project for commit queue '%s'", j.QueueID))
		return
	}
	if projectRef == nil {
		j.AddError(errors.Errorf("project not found for commit queue '%s'", j.QueueID))
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
		j.AddError(errors.Wrapf(err, "finding commit queue '%s'", j.QueueID))
		return
	}
	if cq == nil {
		j.AddError(errors.Errorf("commit queue '%s' not found", j.QueueID))
		return
	}

	front, hasItem := cq.Next()
	grip.InfoWhen(hasItem, message.Fields{
		"source":                  "commit queue",
		"job_id":                  j.ID(),
		"item":                    front.Issue,
		"project_id":              cq.ProjectID,
		"waiting_secs":            time.Since(front.EnqueueTime).Seconds(),
		"queue_length_at_enqueue": front.QueueLengthAtEnqueue,
		"message":                 "found item at the front of commit queue",
	})

	conf, err := evergreen.GetConfig(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting admin settings"))
		return
	}

	j.TryUnstick(ctx, cq, projectRef)

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
	beginBatchProcessingTime := time.Now()
	grip.Info(message.Fields{
		"source":       "commit queue",
		"job_id":       j.ID(),
		"items":        len(nextItems),
		"project_id":   cq.ProjectID,
		"queue_length": len(cq.Queue),
		"batch_size":   batchSize,
		"message":      "starting processing batch of commit queue items",
	})
	for _, nextItem := range nextItems {
		// log time waiting in queue
		grip.Info(message.Fields{
			"source":       "commit queue",
			"job_id":       j.ID(),
			"item":         nextItem,
			"project_id":   cq.ProjectID,
			"time_waiting": time.Since(nextItem.EnqueueTime).Seconds(),
			"queue_length": len(cq.Queue),
			"message":      "starting processing of commit queue item",
		})

		if nextItem.Version != "" {
			grip.Error(message.Fields{
				"message": "tried to process an item twice",
				"queue":   cq.ProjectID,
				"item":    nextItem.Version,
			})
			j.AddError(j.addMergeTaskDependencies(ctx, *cq))
			return
		}
		// create a version with the item and subscribe to its completion
		if nextItem.Source == commitqueue.SourcePullRequest {
			j.processGitHubPRItem(ctx, cq, &nextItem, projectRef)
		} else if nextItem.Source == commitqueue.SourceDiff {
			j.processCLIPatchItem(ctx, cq, &nextItem, projectRef)
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
			"message": "finished processing commit queue item",
		})
	}
	grip.Info(message.Fields{
		"source":               "commit queue",
		"job_id":               j.ID(),
		"items":                len(nextItems),
		"message":              "finished processing batch of commit queue items",
		"processing_time_secs": time.Since(beginBatchProcessingTime).Seconds(),
	})
	j.AddError(j.addMergeTaskDependencies(ctx, *cq))
}

func (j *commitQueueJob) addMergeTaskDependencies(ctx context.Context, cq commitqueue.CommitQueue) error {
	var prevMergeTask string
	for i, currentItem := range cq.Queue {
		if currentItem.Version == "" {
			return nil
		}
		mergeTask, err := task.FindMergeTaskForVersion(currentItem.Version)
		if err != nil {
			return errors.Wrapf(err, "finding merge task from version '%s'", currentItem.Version)
		}
		if mergeTask == nil {
			return errors.Errorf("merge task not found for version '%s'", currentItem.Version)
		}
		dependency := task.Dependency{
			TaskId: prevMergeTask,
			Status: task.AllStatuses,
		}
		prevMergeTask = mergeTask.Id
		if i == 0 {
			continue
		}
		err = mergeTask.AddDependency(ctx, dependency)
		if err != nil {
			return errors.Wrapf(err, "adding dependency of merge task '%s' on previous merge task '%s'", mergeTask.Id, dependency.TaskId)
		}
		err = mergeTask.UpdateDependsOn(dependency.Status, []string{dependency.TaskId})
		if err != nil {
			return errors.Wrapf(err, "updating tasks depending on merge task '%s' to also depend on previous merge task '%s'", mergeTask.Id, dependency.TaskId)
		}
		err = model.RecomputeNumDependents(ctx, *mergeTask)
		if err != nil {
			return errors.Wrapf(err, "recomputing number of dependencies for merge task '%s'", mergeTask.Id)
		}
	}

	return nil
}

func (j *commitQueueJob) TryUnstick(ctx context.Context, cq *commitqueue.CommitQueue, projectRef *model.ProjectRef) {
	nextItem, valid := cq.Next()
	if !valid {
		return
	}

	if nextItem.Version == "" {
		return
	}

	// unstick the queue if the patch is done.
	if !patch.IsValidId(nextItem.Version) {
		err := errors.Errorf("patch at the top of the queue has an invalid patch ID '%s', likely because it was removed", nextItem.Version)
		j.dequeue(ctx, cq, nextItem, err.Error())
		j.logError(err, nextItem)
		return
	}

	patchDoc, err := patch.FindOne(patch.ByStringId(nextItem.Version).WithFields(patch.FinishTimeKey, patch.StatusKey))
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding patch '%s' for commit queue '%s'", nextItem.Version, j.QueueID))
		return
	}
	if patchDoc == nil {
		err := errors.New("patch at the top of the queue is nil, likely because it was removed")
		j.dequeue(ctx, cq, nextItem, err.Error())
		j.logError(err, nextItem)
		if nextItem.Source == commitqueue.SourcePullRequest {
			pr, _, err := checkPR(ctx, nextItem.Issue, projectRef.Owner, projectRef.Repo)
			if err != nil {
				j.AddError(err)
				return
			}
			j.AddError(thirdparty.SendCommitQueueGithubStatus(ctx, j.env, pr, message.GithubStateFailure, "commit queue entry was stuck with no patch", ""))
		}
		return
	}

	mergeTask, err := task.FindMergeTaskForVersion(nextItem.Version)
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding merge task for version '%s'", nextItem.Version))
	}
	if mergeTask != nil {
		// check that the merge task can run. Assume that if we're here the merge task
		// should in fact run (ie. has not been dequeued due to a task failure)
		if !mergeTask.Activated || mergeTask.Priority < 0 {
			grip.Error(message.Fields{
				"message":  "merge task is not dispatchable, dequeueing the version",
				"project":  mergeTask.Project,
				"task":     mergeTask.Id,
				"active":   mergeTask.Activated,
				"priority": mergeTask.Priority,
				"version":  nextItem.Version,
				"source":   "commit queue",
				"job_id":   j.ID(),
			})
			j.dequeue(ctx, cq, nextItem, "merge task was found deactivated or disabled")
			return
		}
		if mergeTask.Blocked() {
			// The head of the commit queue could be blocked temporarily if its
			// dependencies are in the process of restarting running tasks due
			// to a failure in a previous commit queue item and the asynchronous
			// nature of aborting/restarting tasks. Once they're all done
			// resetting, the merge task should be unblocked.
			stillResetting, err := mergeTask.FindAbortingAndResettingDependencies()
			grip.Error(message.WrapError(err, message.Fields{
				"message": "cannot check number of dependencies for blocked merge task that are still waiting to abort and reset",
				"project": mergeTask.Project,
				"task":    mergeTask.Id,
				"version": nextItem.Version,
				"source":  "commit queue",
				"job_id":  j.ID(),
			}))

			var taskIDsResetting []string
			for _, t := range stillResetting {
				taskIDsResetting = append(taskIDsResetting, t.Id)
			}
			grip.InfoWhen(len(taskIDsResetting) > 0, message.Fields{
				"message":                "cannot determine if merge task is stuck or not because some tasks are still waiting to abort and reset",
				"dependencies_resetting": taskIDsResetting,
				"project":                mergeTask.Project,
				"task":                   mergeTask.Id,
				"version":                nextItem.Version,
				"source":                 "commit queue",
				"job_id":                 j.ID(),
			})

			if err == nil && len(stillResetting) == 0 {
				grip.Error(message.Fields{
					"message": "merge task is not dispatchable because it is blocked, dequeueing the version",
					"project": mergeTask.Project,
					"task":    mergeTask.Id,
					"version": mergeTask.Version,
					"source":  "commit queue",
					"job_id":  j.ID(),
				})
				j.dequeue(ctx, cq, nextItem, "merge task was found blocked")
				return
			}
		}
		if mergeTask.IsFinished() {
			grip.Info(message.Fields{
				"message":               "patch is already done, dequeueing",
				"source":                "commit queue",
				"job_id":                j.ID(),
				"item":                  nextItem,
				"project_id":            cq.ProjectID,
				"time_since_enqueue":    time.Since(nextItem.EnqueueTime).Seconds(),
				"time_since_patch_done": time.Since(patchDoc.FinishTime).Seconds(),
			})
			j.dequeue(ctx, cq, nextItem, "patch is already finished but still in the commit queue")
		}
	}
}

func (j *commitQueueJob) processGitHubPRItem(ctx context.Context, cq *commitqueue.CommitQueue, nextItem *commitqueue.CommitQueueItem, projectRef *model.ProjectRef) {
	pr, dequeue, err := checkPR(ctx, nextItem.Issue, projectRef.Owner, projectRef.Repo)
	if err != nil {
		msg := "GitHub PR is not valid for merge"
		err = errors.Wrap(err, msg)
		j.logError(err, *nextItem)
		if dequeue {
			if pr != nil {
				j.AddError(thirdparty.SendCommitQueueGithubStatus(ctx, j.env, pr, message.GithubStateFailure, msg, ""))
			}
			j.dequeue(ctx, cq, *nextItem, err.Error())
		}
		return
	}

	v, project, err := updateAndFinalizeCqPatch(ctx, j.env.Settings(), nextItem.PatchId, commitqueue.SourcePullRequest, projectRef)
	if err != nil {
		j.logError(err, *nextItem)
		j.dequeue(ctx, cq, *nextItem, err.Error())
		j.AddError(err)
		j.AddError(thirdparty.SendCommitQueueGithubStatus(ctx, j.env, pr, message.GithubStateFailure, err.Error(), ""))
		return
	}
	nextItem.Version = v.Id
	nextItem.ProcessingStartTime = time.Now()
	if err = cq.UpdateVersion(nextItem); err != nil {
		err = errors.Wrap(err, "updating commit queue item's version")
		j.logError(err, *nextItem)
		j.dequeue(ctx, cq, *nextItem, err.Error())
		j.AddError(thirdparty.SendCommitQueueGithubStatus(ctx, j.env, pr, message.GithubStateFailure, "can't update commit queue item", ""))
		return
	}

	j.AddError(thirdparty.SendCommitQueueGithubStatus(ctx, j.env, pr, message.GithubStatePending, "preparing to test merge", v.Id))
	modulePRs, _, err := model.GetModulesFromPR(ctx, nextItem.Modules, project)
	if err != nil {
		err = errors.Wrap(err, "getting PR modules")
		j.logError(err, *nextItem)
		j.AddError(thirdparty.SendCommitQueueGithubStatus(ctx, j.env, pr, message.GithubStateFailure, "can't get modules", ""))
		j.dequeue(ctx, cq, *nextItem, err.Error())
		return
	}
	for _, modulePR := range modulePRs {
		j.AddError(thirdparty.SendCommitQueueGithubStatus(ctx, j.env, modulePR, message.GithubStatePending, "preparing to test merge", v.Id))
	}

	event.LogCommitQueueStartTestEvent(v.Id)
}

func (j *commitQueueJob) processCLIPatchItem(ctx context.Context, cq *commitqueue.CommitQueue, nextItem *commitqueue.CommitQueueItem, projectRef *model.ProjectRef) {
	v, _, err := updateAndFinalizeCqPatch(ctx, j.env.Settings(), nextItem.Issue, commitqueue.SourceDiff, projectRef)
	if err != nil {
		j.logError(err, *nextItem)
		event.LogCommitQueueEnqueueFailed(nextItem.Issue, err)
		j.dequeue(ctx, cq, *nextItem, err.Error())
		return
	}

	nextItem.Version = v.Id
	nextItem.ProcessingStartTime = time.Now()
	if err = cq.UpdateVersion(nextItem); err != nil {
		err = errors.Wrap(err, "updating commit queue item's version")
		j.logError(err, *nextItem)
		j.dequeue(ctx, cq, *nextItem, err.Error())
		return
	}

	if err = setDefaultNotification(v.Author); err != nil {
		err = errors.Wrap(err, "setting up default commit queue notification")
		j.logError(err, *nextItem)
	}
	event.LogCommitQueueStartTestEvent(v.Id)
}

func updateAndFinalizeCqPatch(ctx context.Context, settings *evergreen.Settings, patchId, source string, projectRef *model.ProjectRef) (*model.Version, *model.Project, error) {
	patchDoc, err := patch.FindOneId(patchId)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "finding patch '%s'", patchId)
	}
	if patchDoc == nil {
		return nil, nil, errors.Errorf("patch '%s' not found", patchId)
	}

	project, pp, err := updatePatch(ctx, settings, projectRef, patchDoc)
	if err != nil {
		return nil, nil, errors.Wrap(err, "updating patch")
	}

	pp, err = AddMergeTaskAndVariant(ctx, patchDoc, project, projectRef, source)
	if err != nil {
		return nil, nil, errors.Wrap(err, "updating patch project config to include merge task")
	}

	if err = patchDoc.UpdateGithashProjectAndTasks(); err != nil {
		return nil, nil, errors.Wrap(err, "updating patch after including merge task")
	}

	// The parser project is typically created when the patch is created. This
	// is a special exception where it upserts the parser project right before
	// it's finalized, because the original patch might be very outdated
	// compared to the latest project configuration. For the commit queue, it's
	// best to test against the latest project configuration.
	pp.Init(patchDoc.Id.Hex(), patchDoc.CreateTime)
	ppStorageMethod, err := model.ParserProjectUpsertOneWithS3Fallback(ctx, settings, evergreen.ProjectStorageMethodDB, pp)
	if err != nil {
		return nil, nil, errors.Wrap(err, "upserting parser project for patch")
	}
	patchDoc.ProjectStorageMethod = ppStorageMethod

	v, err := model.FinalizePatch(ctx, patchDoc, evergreen.MergeTestRequester)
	if err != nil {
		return nil, nil, errors.Wrap(err, "finalizing patch")
	}

	return v, project, nil
}

func (j *commitQueueJob) logError(err error, item commitqueue.CommitQueueItem) {
	if err == nil {
		return
	}
	j.AddError(err)
	grip.Error(message.WrapError(err, message.Fields{
		"job_id":  j.ID(),
		"source":  "commit queue",
		"project": j.QueueID,
		"item":    item,
	}))
}

func (j *commitQueueJob) dequeue(ctx context.Context, cq *commitqueue.CommitQueue, item commitqueue.CommitQueueItem, reason string) {
	_, err := model.CommitQueueRemoveItem(ctx, cq, item, evergreen.User, reason)
	j.logError(errors.Wrap(err, "dequeueing commit queue item"), item)
}

func checkPR(ctx context.Context, issue, owner, repo string) (*github.PullRequest, bool, error) {
	issueInt, err := strconv.Atoi(issue)
	if err != nil {
		return nil, true, errors.Wrapf(err, "parsing issue '%s' as int", issue)
	}

	pr, err := thirdparty.GetGithubPullRequest(ctx, owner, repo, issueInt)
	if err != nil {
		return nil, false, errors.Wrap(err, "getting PR from GitHub")
	}

	if err = thirdparty.ValidatePR(pr); err != nil {
		return nil, true, errors.Wrap(err, "GitHub returned an incomplete PR")
	}

	return pr, false, nil
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

// AddMergeTaskAndVariant adds the merge task, the merge task's dependencies,
// and the merge build variant to the project and returns the modified parser
// project.
func AddMergeTaskAndVariant(ctx context.Context, patchDoc *patch.Patch, project *model.Project, projectRef *model.ProjectRef, source string) (*model.ParserProject, error) {
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "retrieving admin settings")
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
				Variant:          evergreen.MergeTaskVariant,
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

	mergeTaskCmds, err := getMergeTaskCommands(settings, source)
	if err != nil {
		return nil, errors.Wrap(err, "getting merge task commands")
	}

	mergeTask := model.ProjectTask{
		Name:      evergreen.MergeTaskName,
		DependsOn: dependencies,
		Commands:  mergeTaskCmds,
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

	validationErrors := validator.CheckProjectErrors(ctx, project, true)
	validationErrors = append(validationErrors, validator.CheckProjectSettings(ctx, settings, project, projectRef, false)...)
	validationErrors = append(validationErrors, validator.CheckPatchedProjectConfigErrors(patchDoc.PatchedProjectConfig)...)
	catcher := grip.NewBasicCatcher()
	for _, validationErr := range validationErrors.AtLevel(validator.Error) {
		catcher.Add(validationErr)
	}
	if catcher.HasErrors() {
		return nil, errors.Wrap(catcher.Resolve(), "validating project")
	}
	yamlBytes, err := yaml.Marshal(project)
	if err != nil {
		return nil, errors.Wrap(err, "marshalling remote config file")
	}

	var updatedProject model.Project
	updatedPP, err := model.LoadProjectInto(ctx, yamlBytes, nil, patchDoc.Project, &updatedProject)
	if err != nil {
		return nil, errors.Wrap(err, "converting updated project YAML to parser project")
	}

	patchDoc.BuildVariants = append(patchDoc.BuildVariants, evergreen.MergeTaskVariant)
	patchDoc.Tasks = append(patchDoc.Tasks, evergreen.MergeTaskName)
	patchDoc.VariantsTasks = append(patchDoc.VariantsTasks, patch.VariantTasks{
		Variant: evergreen.MergeTaskVariant,
		Tasks:   []string{evergreen.MergeTaskName},
	})

	return updatedPP, nil
}

func getMergeTaskCommands(settings *evergreen.Settings, source string) ([]model.PluginCommandConf, error) {
	switch source {
	case commitqueue.SourceDiff:
		return []model.PluginCommandConf{
			{
				Command: "git.get_project",
				Type:    evergreen.CommandTypeSetup,
				Params: map[string]interface{}{
					"directory":       "src",
					"committer_name":  settings.CommitQueue.CommitterName,
					"committer_email": settings.CommitQueue.CommitterEmail,
				},
			},
			{
				Command: "git.push",
				Params: map[string]interface{}{
					"directory": "src",
				},
			},
		}, nil
	case commitqueue.SourcePullRequest:
		return []model.PluginCommandConf{
			{
				Command: "git.merge_pr",
			},
		}, nil
	default:
		return nil, errors.Errorf("unknown commit queue source '%s'", source)
	}
}

func setDefaultNotification(username string) error {
	u, err := user.FindOneById(username)
	if err != nil {
		return errors.Wrapf(err, "finding user '%s'", username)
	}
	if u == nil {
		return errors.Errorf("user '%s' not found", username)
	}

	// The user has never saved their notification settings
	if u.Settings.Notifications.CommitQueue == "" {
		u.Settings.Notifications.CommitQueue = user.PreferenceEmail
		commitQueueSubscriber := event.NewEmailSubscriber(u.Email())
		commitQueueSubscription, err := event.CreateOrUpdateGeneralSubscription(event.GeneralSubscriptionCommitQueue,
			"", commitQueueSubscriber, u.Id)
		if err != nil {
			return errors.Wrap(err, "creating default email subscription")
		}
		u.Settings.Notifications.CommitQueueID = commitQueueSubscription.ID

		return u.UpdateSettings(u.Settings)
	}

	return nil
}

func updatePatch(ctx context.Context, settings *evergreen.Settings, projectRef *model.ProjectRef, patchDoc *patch.Patch) (*model.Project, *model.ParserProject, error) {
	branch, err := thirdparty.GetBranchEvent(ctx, projectRef.Owner, projectRef.Repo, projectRef.Branch)
	if err != nil {
		return nil, nil, errors.Wrap(err, "getting branch")
	}
	if err = validateBranch(branch); err != nil {
		return nil, nil, errors.Wrap(err, "GitHub returned an invalid branch")
	}

	sha := *branch.Commit.SHA
	patchDoc.Githash = sha

	// Ensure that the project remote configuration loads directly from GitHub
	// rather than loading from the cached information from the patch document.
	patchDoc.ProjectStorageMethod = ""
	patchDoc.PatchedProjectConfig = ""
	project, patchConfig, err := model.GetPatchedProject(ctx, settings, patchDoc)
	if err != nil {
		return nil, nil, errors.Wrap(err, "getting updated project config")
	}
	patchDoc.PatchedProjectConfig = patchConfig.PatchedProjectConfig

	// Update module githashes
	for i, mod := range patchDoc.Patches {
		if mod.ModuleName == "" {
			patchDoc.Patches[i].Githash = sha
			continue
		}

		module, err := project.GetModuleByName(mod.ModuleName)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "getting module '%s'", mod.ModuleName)
		}

		owner, repo, err := module.GetOwnerAndRepo()
		if err != nil {
			return nil, nil, errors.Wrapf(err, "getting module owner and repo '%s'", module.Name)
		}

		sha, err = getBranchCommitHash(ctx, owner, repo, module.Branch)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "getting commit hash for branch '%s'", module.Branch)
		}
		patchDoc.Patches[i].Githash = sha
	}

	// rebuild patch build variants and tasks
	patchDoc.BuildVariants = []string{}
	patchDoc.VariantsTasks = []patch.VariantTasks{}
	patchDoc.Tasks = []string{}
	project.BuildProjectTVPairs(patchDoc, patchDoc.Alias)

	return project, patchConfig.PatchedParserProject, nil
}

// getBranchCommitHash retrieves the most recent commit hash for branch for the given repo, module, and branch name.
func getBranchCommitHash(ctx context.Context, owner, repo, moduleBranch string) (string, error) {
	branch, err := thirdparty.GetBranchEvent(ctx, owner, repo, moduleBranch)
	if err != nil {
		return "", errors.Wrap(err, "getting branch")
	}
	if err = validateBranch(branch); err != nil {
		return "", errors.Wrap(err, "GitHub returned invalid branch")
	}
	return utility.FromStringPtr(branch.Commit.SHA), nil
}
