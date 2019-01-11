package units

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
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

func NewCommitQueueJob(queueID string, id string) amboy.Job {
	job := makeCommitQueueJob()
	job.QueueID = queueID
	job.SetID(fmt.Sprintf("%s:%s_%s", commitQueueJobName, queueID, id))

	return job
}

func (j *commitQueueJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	projectRef, err := model.FindOneProjectRef(j.QueueID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "can't find project for queue id %s", j.QueueID))
		return
	}

	if !projectRef.CommitQEnabled {
		return
	}

	cq, err := commitqueue.FindOneId(j.QueueID)
	nextItem := cq.Next()
	nextItemInt, err := strconv.Atoi(nextItem)
	if err != nil {
		j.AddError(errors.Wrapf(err, "can't parse next item \"%s\" as int", nextItem))
		return
	}

	alreadyProcessed, err := versionExists(nextItemInt)
	if err != nil {
		j.AddError(errors.Wrap(err, "can't query for versions for this item"))
	}
	if alreadyProcessed {
		return
	}

	githubToken, err := getGitHubToken()
	if err != nil {
		j.AddError(errors.Wrap(err, "can't get github token"))
	}

	pr, err := thirdparty.GetGithubPullRequest(ctx, githubToken, projectRef.Owner, projectRef.Repo, nextItemInt)
	if err != nil {
		j.AddError(errors.Wrap(err, "can't get PR from GitHub"))
		return
	}

	if validatePR(pr) != nil {
		j.AddError(errors.Wrap(err, "invalid PR"))
		return
	}

	// GitHub hasn't yet tested if the PR is mergeable.
	// Check back later
	// See: https://developer.github.com/v3/pulls/#response-1
	if pr.Mergeable == nil {
		return
	}

	if !*pr.Mergeable {
		err = sendCommitQueueGithubStatus(pr, message.GithubStateFailure, "PR not mergeable", "")
		if err != nil {
			j.AddError(errors.Wrap(err, "can't send github status"))
		}
		return
	}

	v, err := makeVersion(ctx, githubToken, projectRef, pr)
	if err != nil {
		j.AddError(errors.Wrap(err, "can't make version"))
		return
	}

	err = sendCommitQueueGithubStatus(pr, message.GithubStatePending, "preparing to test merge", v.Id)
	if err != nil {
		j.AddError(errors.Wrap(err, "can't send github status"))
	}

	err = subscribeMerge(projectRef, pr, v.Id)
	if err != nil {
		j.AddError(errors.Wrap(err, "can't subscribe GitHub PR merge to version"))
	}
}

func versionExists(prNumber int) (bool, error) {
	patch, err := patch.FindOneByGithubPRNum(prNumber)
	if err != nil {
		return false, errors.Wrap(err, "can't query for patch matching PR Number")
	}
	return patch != nil, nil
}

func getGitHubToken() (string, error) {
	conf, err := evergreen.GetConfig()
	if err != nil {
		return "", errors.Wrap(err, "can't get Evergreen Config")
	}

	githubToken, err := conf.GetGithubOauthToken()
	if err != nil {
		return "", errors.Wrap(err, "can't get GitHub token from Evergreen Config")
	}

	return githubToken, nil
}

func validatePR(pr *github.PullRequest) error {
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

func makeVersion(ctx context.Context, githubToken string, projectRef *model.ProjectRef, pr *github.PullRequest) (*model.Version, error) {
	config, err := getProjectConfigFromFileInCommit(ctx, githubToken, projectRef, *pr.MergeCommitSHA)

	patchDoc, err := makeMergePatch(pr, projectRef.Identifier, config)
	if err != nil {
		return nil, errors.Wrap(err, "can't make patch")
	}

	if err = patchDoc.Insert(); err != nil {
		return nil, errors.Wrap(err, "can't insert patch")
	}

	v, err := model.FinalizePatch(ctx, patchDoc, evergreen.MergeTestRequester, githubToken)
	if err != nil {
		return nil, errors.Wrap(err, "can't finalize patch")
	}

	return v, nil
}

func getProjectConfigFromFileInCommit(ctx context.Context, githubToken string, projectRef *model.ProjectRef, sha string) (string, error) {
	cancelCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	owner := projectRef.Owner
	repo := projectRef.Repo
	file := projectRef.CommitQConfigFile
	remoteConfigFile, err := thirdparty.GetGithubFile(cancelCtx, githubToken, owner, repo, file, sha)
	if err != nil {
		return "", errors.Wrapf(err, "error fetching file %s from commit %s", file, sha)
	}
	configFileContents, err := base64.StdEncoding.DecodeString(*remoteConfigFile.Content)
	if err != nil {
		return "", errors.Wrapf(err, "unable to decode file %s", file)
	}

	return string(configFileContents), nil
}

func makeMergePatch(pr *github.PullRequest, projectID string, configString string) (*patch.Patch, error) {
	u, err := getPatchUser(pr.GetUser().GetID())
	if err != nil {
		return nil, errors.Wrap(err, "can't get user for patch")
	}
	prAuthor := u.Id
	patchNumber, err := u.IncPatchNumber()
	if err != nil {
		return nil, errors.Wrap(err, "error computing patch num")
	}
	id := bson.NewObjectId()

	patchDoc := &patch.Patch{
		Id:            id,
		Project:       projectID,
		Author:        prAuthor,
		Githash:       *pr.Base.SHA,
		Description:   fmt.Sprintf("Commit Queue merge test PR #%d", *pr.Number),
		CreateTime:    time.Now(),
		Status:        evergreen.PatchCreated,
		PatchedConfig: configString,
		PatchNumber:   patchNumber,
		GithubPatchData: patch.GithubPatch{
			PRNumber:       *pr.Number,
			MergeCommitSHA: *pr.MergeCommitSHA,
		},
	}

	return patchDoc, nil
}

func getPatchUser(userUID int) (*user.DBUser, error) {
	u, err := user.FindByGithubUID(userUID)
	if err != nil {
		return nil, errors.Wrap(err, "can't look for user")
	}
	if u == nil {
		// set to a default user
		u, err = user.FindOne(user.ById(evergreen.GithubPatchUser))
		if err != nil {
			return nil, errors.Wrap(err, "can't get user for pull request")
		}
		// default user doesn't exist yet
		if u == nil {
			u = &user.DBUser{
				Id:       evergreen.GithubPatchUser,
				DispName: "Github Pull Requests",
				APIKey:   util.RandomString(),
			}
			if err = u.Insert(); err != nil {
				return nil, errors.Wrap(err, "failed to create github patch user")
			}
		}
	}

	return u, nil
}

func sendCommitQueueGithubStatus(pr *github.PullRequest, state message.GithubState, description, versionID string) error {
	env := evergreen.GetEnvironment()
	sender, err := env.GetSender(evergreen.SenderGithubStatus)
	if err != nil {
		return errors.Wrap(err, "can't get GitHub status sender")
	}

	url := ""
	if versionID != "" {
		url, _ = makeVersionURL(versionID)
	}

	msg := message.GithubStatus{
		Owner:       *pr.Base.Repo.Owner.Login,
		Repo:        *pr.Base.Repo.Name,
		Ref:         *pr.Head.SHA,
		Context:     "evergreen/commitqueue",
		State:       state,
		Description: "preparing to test merge",
		URL:         url,
	}

	c := message.NewGithubStatusMessageWithRepo(level.Notice, msg)
	sender.Send(c)

	return nil
}

func makeVersionURL(versionID string) (string, error) {
	uiConfig := evergreen.UIConfig{}
	if err := uiConfig.Get(); err != nil {
		return "", errors.Wrap(err, "can't get UI config")
	}
	urlBase := uiConfig.Url
	return fmt.Sprintf("%s/version/%s", urlBase, versionID), nil
}

func subscribeMerge(projectRef *model.ProjectRef, pr *github.PullRequest, patchID string) error {
	mergeSubscriber := event.NewGithubMergeSubscriber(event.GithubMergeSubscriber{
		ProjectID:     projectRef.Identifier,
		Owner:         projectRef.Owner,
		Repo:          projectRef.Repo,
		PRNumber:      *pr.Number,
		Ref:           *pr.Head.SHA,
		CommitMessage: "Merged by commit queue",
		MergeMethod:   projectRef.CommitQMergeMethod,
		CommitTitle:   *pr.Title,
	})
	patchSub := event.NewPatchOutcomeSubscription(patchID, mergeSubscriber)
	if err := patchSub.Upsert(); err != nil {
		return errors.Wrapf(err, "failed to insert patch subscription for commit queue merge on PR %d", *pr.Number)
	}

	return nil
}
