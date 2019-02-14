package units

import (
	"context"
	"fmt"
	"strconv"

	"github.com/evergreen-ci/evergreen"
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
	yaml "gopkg.in/yaml.v2"
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
	if nextItem == "" {
		return
	}

	nextItemInt, err := strconv.Atoi(nextItem)
	if err != nil {
		j.AddError(errors.Wrapf(err, "can't parse next item \"%s\" as int", nextItem))
		_, err2 := cq.Remove(nextItem)
		j.AddError(errors.Wrapf(err2, "error dequeuing item '%s'", nextItem))
		grip.Error(message.WrapError(err, message.Fields{
			"job":     commitQueueJobName,
			"source":  "commit queue",
			"project": j.QueueID,
			"item":    nextItem,
			"message": "next item can't be parsed as an int",
		}))
		return
	}

	conf := j.env.Settings()
	githubToken, err := conf.GetGithubOauthToken()
	if err != nil {
		j.AddError(errors.Wrap(err, "can't get github token"))
		return
	}

	pr, err := thirdparty.GetGithubPullRequest(ctx, githubToken, projectRef.Owner, projectRef.Repo, nextItemInt)
	if err != nil {
		j.AddError(errors.Wrap(err, "can't get PR from GitHub"))
		grip.Error(message.WrapError(err, message.Fields{
			"job":     commitQueueJobName,
			"source":  "commit queue",
			"project": j.QueueID,
			"item":    nextItem,
			"message": "can't get PR from GitHub",
		}))
		return
	}

	if err = validatePR(pr); err != nil {
		j.AddError(errors.Wrap(err, "invalid PR"))
		_, err2 := cq.Remove(nextItem)
		j.AddError(errors.Wrapf(err2, "error dequeuing item '%s'", nextItem))
		grip.Error(message.WrapError(err, message.Fields{
			"job":     commitQueueJobName,
			"source":  "commit queue",
			"project": j.QueueID,
			"item":    nextItem,
			"message": "invalid PR",
		})
		return
	}

	// GitHub hasn't yet tested if the PR is mergeable.
	// Check back later
	// See: https://developer.github.com/v3/pulls/#response-1
	if pr.Mergeable == nil {
		return
	}

	if !*pr.Mergeable {
		err = sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "PR not mergeable", "")
		if err != nil {
			j.AddError(errors.Wrap(err, "can't send github status"))
		}
		return
	}

	// check if a patch has already been created for this PR
	mergeCommitSHA := pr.GetMergeCommitSHA()
	existingPatch, err := patch.FindOneByGithubPRNumAndMergeCommitSHA(nextItemInt, mergeCommitSHA)
	if err != nil {
		j.AddError(errors.Wrap(err, "can't query for patch matching PR Number"))
		return
	}
	if existingPatch != nil {
		return
	}

	v, err := makeVersion(ctx, githubToken, projectRef.Identifier, pr)
	if err != nil {
		j.AddError(errors.Wrap(err, "can't make version"))
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't make version", ""))
		_, err2 := cq.Remove(nextItem)
		j.AddError(errors.Wrapf(err2, "error dequeuing item '%s'", nextItem))
		grip.Error(message.WrapError(err, message.Fields{
			"job":     commitQueueJobName,
			"source":  "commit queue",
			"project": j.QueueID,
			"item":    nextItem,
			"message": "can't make version",
		})
		return
	}

	err = sendCommitQueueGithubStatus(j.env, pr, message.GithubStatePending, "preparing to test merge", v.Id)
	if err != nil {
		j.AddError(errors.Wrap(err, "can't send github status"))
	}

	err = subscribeMerge(projectRef, pr, v.Id)
	if err != nil {
		j.AddError(errors.Wrap(err, "can't subscribe GitHub PR merge to version"))
		j.AddError(sendCommitQueueGithubStatus(j.env, pr, message.GithubStateFailure, "can't start merge", ""))
		grip.Error(message.WrapError(err, message.Fields{
			"job":     commitQueueJobName,
			"source":  "commit queue",
			"project": j.QueueID,
			"item":    nextItem,
			"message": "can't subscribe for merge sender",
		})
	}

	grip.Info(message.Fields{
		"source":  "commit queue",
		"job_id":  j.ID(),
		"item":    nextItem,
		"message": "finished processing item",
	})
}

func validatePR(pr *github.PullRequest) error {
	if pr == nil {
		return errors.New("No PR provided")
	}

	catcher := grip.NewSimpleCatcher()
	if pr.GetMergeCommitSHA() == "" {
		catcher.Add(errors.New("no merge commit SHA"))
	}
	if pr.GetUser() == nil || pr.GetUser().GetID() == 0 {
		catcher.Add(errors.New("no valid user"))
	}
	if pr.GetBase() == nil || pr.GetBase().GetSHA() == "" {
		catcher.Add(errors.New("no valid base SHA"))
	}
	if pr.GetBase() == nil || pr.GetBase().GetRepo() == nil || pr.GetBase().GetRepo().GetName() == "" {
		catcher.Add(errors.New("no valid base repo name"))
	}
	if pr.GetBase() == nil || pr.GetBase().GetRepo() == nil || pr.GetBase().GetRepo().GetOwner() == nil || pr.GetBase().GetRepo().GetOwner().GetLogin() == "" {
		catcher.Add(errors.New("no valid base repo owner login"))
	}
	if pr.GetHead() == nil || pr.GetHead().GetSHA() == "" {
		catcher.Add(errors.New("no valid head SHA"))
	}
	if pr.GetNumber() == 0 {
		catcher.Add(errors.New("no valid pr number"))
	}
	if pr.GetTitle() == "" {
		catcher.Add(errors.New("no valid title"))
	}

	return catcher.Resolve()
}

func makeVersion(ctx context.Context, githubToken string, projectID string, pr *github.PullRequest) (*model.Version, error) {
	patchDoc, err := patch.MakeMergePatch(pr, projectID, commitQueueAlias)
	if err != nil {
		return nil, errors.Wrap(err, "can't make patch")
	}

	config, err := validator.GetPatchedProject(ctx, patchDoc, githubToken)
	if err != nil {
		return nil, errors.Wrap(err, "can't get remote config file")
	}

	yamlBytes, err := yaml.Marshal(config)
	if err != nil {
		return nil, errors.Wrap(err, "can't marshal project config to yaml")
	}
	patchDoc.PatchedConfig = string(yamlBytes)

	config.BuildProjectTVPairs(patchDoc, patchDoc.Alias)

	if err = patchDoc.Insert(); err != nil {
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
		Context:     "evergreen/commitqueue",
		State:       state,
		Description: description,
		URL:         url,
	}

	c := message.NewGithubStatusMessageWithRepo(level.Notice, msg)
	sender.Send(c)

	return nil
}

func subscribeMerge(projectRef *model.ProjectRef, pr *github.PullRequest, patchID string) error {
	mergeSubscriber := event.NewGithubMergeSubscriber(event.GithubMergeSubscriber{
		ProjectID:     projectRef.Identifier,
		Owner:         projectRef.Owner,
		Repo:          projectRef.Repo,
		PRNumber:      *pr.Number,
		Ref:           *pr.Head.SHA,
		CommitMessage: "Merged by commit queue",
		MergeMethod:   projectRef.CommitQueue.MergeMethod,
		CommitTitle:   *pr.Title,
	})
	patchSub := event.NewPatchOutcomeSubscription(patchID, mergeSubscriber)
	if err := patchSub.Upsert(); err != nil {
		return errors.Wrapf(err, "failed to insert patch subscription for commit queue merge on PR %d", *pr.Number)
	}

	return nil
}
