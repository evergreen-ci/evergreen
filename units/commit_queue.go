package units

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
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
)

const (
	commitQueueJobName = "commit-queue"
	commitQueueAlias   = "__commit_queue"
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

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

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

	cq, err := commitqueue.FindOneId(j.QueueID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "can't find commit queue for id %s", j.QueueID))
		return
	}
	nextItem := cq.Next()
	if nextItem == nil {
		return
	}
	j.AddError(errors.Wrap(cq.SetProcessing(true), "can't set processing to true"))

	conf := j.env.Settings()
	githubToken, err := conf.GetGithubOauthToken()
	if err != nil {
		j.AddError(errors.Wrap(err, "can't get github token"))
		j.AddError(errors.Wrap(cq.SetProcessing(false), "can't set processing to false"))
		return
	}

	pr, dequeue, err := checkPR(ctx, githubToken, nextItem.Issue, projectRef.Owner, projectRef.Repo)
	if err != nil {
		j.logError(err, "PR not valid for merge", nextItem)
		if dequeue {
			if pr != nil {
				j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "PR not valid for merge", ""))
			}
			j.dequeue(cq, nextItem)
		} else {
			j.AddError(errors.Wrap(cq.SetProcessing(false), "can't set processing to false"))
		}
		return
	}

	patchDoc, err := patch.MakeMergePatch(pr, projectRef.Identifier, commitQueueAlias)
	if err != nil {
		j.logError(err, "can't make patch", nextItem)
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't make patch", ""))
		j.dequeue(cq, nextItem)
		return
	}

	patch, patchSummaries, projectConfig, invalid, err := getPatchInfo(ctx, githubToken, patchDoc)
	if err != nil {
		j.logError(err, "can't get patch info", nextItem)
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't make patch", ""))
		j.dequeue(cq, nextItem)
		if invalid {
			update := NewGithubStatusUpdateJobForBadConfig(projectRef.Identifier, j.ID())
			update.Run(ctx)
			j.AddError(update.Error())
		}
		return
	}

	if err = writePatchInfo(patchDoc, projectConfig, patchSummaries, patch); err != nil {
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
			j.AddError(errors.Wrap(cq.SetProcessing(false), "can't set processing to false"))
		}
		return
	}
	patchDoc.Patches = append(patchDoc.Patches, modulePatches...)

	v, err := makeVersion(ctx, githubToken, projectConfig, patchDoc)
	if err != nil {
		j.logError(err, "can't make version", nextItem)
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't make version", ""))
		j.dequeue(cq, nextItem)
		return
	}

	j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStatePending, "preparing to test merge", v.Id))
	for _, modulePR := range modulePRs {
		j.AddError(sendCommitQueueGithubStatus(j.env, modulePR, message.GithubStatePending, "preparing to test merge", v.Id))
	}

	err = subscribeMerge(projectRef.Identifier, projectRef.Owner, projectRef.Repo, projectRef.CommitQueue.MergeMethod, v.Id, pr)
	if err != nil {
		j.logError(err, "can't subscribe GitHub PR merge to version", nextItem)
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't start merge", ""))
		j.dequeue(cq, nextItem)
	}

	for _, modulePR := range modulePRs {
		err = subscribeMerge("", modulePR.Base.Repo.Owner.GetLogin(), *modulePR.Base.Repo.Name, projectRef.CommitQueue.MergeMethod, v.Id, modulePR)
		if err != nil {
			j.logError(err, "can't subscribe GitHub PR merge to version for module", nextItem)
			j.AddError(sendCommitQueueGithubStatus(j.env, modulePR, message.GithubStateFailure, "can't start merge", ""))
		}
	}

	grip.Info(message.Fields{
		"source":  "commit queue",
		"job_id":  j.ID(),
		"item":    nextItem,
		"message": "finished processing item",
	})
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

func getPatchInfo(ctx context.Context, githubToken string, patchDoc *patch.Patch) (string, []patch.Summary, *model.Project, bool, error) {
	patchContent, summaries, err := thirdparty.GetGithubPullRequestDiff(ctx, githubToken, patchDoc.GithubPatchData)
	if err != nil {
		return "", nil, nil, false, errors.Wrap(err, "can't get diff")
	}

	// fetch the latest config file
	yamlBytes, err := validator.GetPatchedProject(ctx, patchDoc, githubToken)
	if err != nil {
		return "", nil, nil, false, errors.Wrap(err, "can't get remote config file")
	}

	patchDoc.PatchedConfig = string(yamlBytes)
	config, invalid, err := validator.ValidateProjectPatch(yamlBytes, patchDoc.Project)
	if err != nil {
		return "", nil, nil, invalid, errors.Wrap(err, "invalid config patch")
	}

	return patchContent, summaries, config, false, nil
}

func writePatchInfo(patchDoc *patch.Patch, config *model.Project, patchSummaries []patch.Summary, patchContent string) error {
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

func makeVersion(ctx context.Context, githubToken string, project *model.Project, patchDoc *patch.Patch) (*model.Version, error) {
	project.BuildProjectTVPairs(patchDoc, patchDoc.Alias)

	if err := patchDoc.Insert(); err != nil {
		return nil, errors.Wrap(err, "can't insert patch")
	}

	v, err := model.FinalizePatch(ctx, patchDoc, evergreen.MergeTestRequester, githubToken)
	if err != nil {
		return nil, errors.Wrap(err, "can't finalize patch")
	}

	return v, nil
}

func sendCommitQueueGithubStatus(env evergreen.Environment, pr *github.PullRequest, state message.GithubState, description, versionID string) error {
	sender, err := env.GetSender(evergreen.SenderGithubStatus)
	if err != nil {
		return errors.Wrap(err, "can't get GitHub status sender")
	}

	var url string
	if versionID != "" {
		uiConfig := evergreen.UIConfig{}
		if err := uiConfig.Get(); err == nil {
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

func subscribeMerge(projectID, owner, repo, mergeMethod, patchID string, pr *github.PullRequest) error {
	mergeSubscriber := event.NewGithubMergeSubscriber(event.GithubMergeSubscriber{
		ProjectID:     projectID,
		Owner:         owner,
		Repo:          repo,
		PRNumber:      *pr.Number,
		Ref:           *pr.Head.SHA,
		CommitMessage: "Merged by commit queue",
		MergeMethod:   mergeMethod,
		CommitTitle:   *pr.Title,
	})
	patchSub := event.NewPatchOutcomeSubscription(patchID, mergeSubscriber)
	if err := patchSub.Upsert(); err != nil {
		return errors.Wrapf(err, "failed to insert patch subscription for commit queue merge on PR %d", *pr.Number)
	}

	return nil
}
