package command

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/github"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

const (
	// valid Github merge methods
	githubMergeMethodMerge  = "merge"
	githubMergeMethodSquash = "squash"
	githubMergeMethodRebase = "rebase"

	GithubContext = "evergreen/commitqueue"
)

type gitMergePr struct {
	URL   string `mapstructure:"url"`
	Token string `mapstructure:"token"`

	statusSender send.Sender
	base
}

func gitMergePrFactory() Command   { return &gitMergePr{} }
func (c *gitMergePr) Name() string { return "git.merge_pr" }

func (c *gitMergePr) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return err
	}

	return nil
}

func (c *gitMergePr) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	var err error
	defer func() {
		pErr := recovery.HandlePanicWithError(recover(), nil, "unexpected error in git.merge_pr")
		status := evergreen.MergeTestSucceeded
		if err != nil || pErr != nil {
			status = evergreen.MergeTestFailed
		}
		logger.Task().Error(err)
		logger.Task().Critical(pErr)
		td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
		logger.Task().Error(comm.ConcludeMerge(ctx, conf.Task.Version, status, td))
	}()
	if err = util.ExpandValues(c, conf.Expansions); err != nil {
		return errors.Wrap(err, "can't apply expansions")
	}

	var patchDoc *patch.Patch
	patchDoc, err = comm.GetTaskPatch(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, "")
	if err != nil {
		return errors.Wrap(err, "unable to get patch")
	}

	token := c.Token
	if token == "" {
		token = conf.Expansions.Get(evergreen.GlobalGitHubTokenExpansion)
	}
	httpClient := utility.GetOAuth2HTTPClient(token)
	defer utility.PutHTTPClient(httpClient)
	githubClient := github.NewClient(httpClient)

	c.statusSender, err = send.NewGithubStatusLogger("evergreen", &send.GithubOptions{
		Token: token,
	}, "")
	if err != nil {
		return errors.Wrap(err, "failed to setup github status logger")
	}

	status := evergreen.PatchFailed
	if patchDoc.MergeStatus == evergreen.PatchSucceeded {
		status = evergreen.PatchSucceeded
	}
	c.sendPatchResult(patchDoc.GithubPatchData, conf, status)
	if status != evergreen.PatchSucceeded {
		logger.Task().Warning("at least 1 task failed, will not merge pull request")
		return nil
	}
	// only successful patches should get past here. Failed patches will just send the failed
	// status to github

	githubCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mergeMethod := conf.ProjectRef.CommitQueue.MergeMethod
	title := patchDoc.GithubPatchData.CommitTitle
	if mergeMethod == githubMergeMethodMerge {
		// if the merge method is to add a merge commit, send no title to the github API so that they use the default merge commit title
		title = ""
	}

	mergeOpts := &github.PullRequestOptions{
		MergeMethod: mergeMethod,
		CommitTitle: title,
		SHA:         patchDoc.GithubPatchData.HeadHash,
	}

	// do the merge
	var res *github.PullRequestMergeResult
	res, _, err = githubClient.PullRequests.Merge(githubCtx, conf.ProjectRef.Owner, conf.ProjectRef.Repo,
		patchDoc.GithubPatchData.PRNumber, patchDoc.GithubPatchData.CommitTitle, mergeOpts)
	if err != nil {
		c.sendMergeFailedStatus(fmt.Sprintf("Github Error: %s", err.Error()), patchDoc.GithubPatchData, conf)
		return errors.Wrap(err, "can't access GitHub merge API")
	}

	if !res.GetMerged() {
		c.sendMergeFailedStatus(res.GetMessage(), patchDoc.GithubPatchData, conf)
		return errors.Errorf("Github refused to merge PR '%s/%s:%d': '%s'", conf.ProjectRef.Owner, conf.ProjectRef.Repo, patchDoc.GithubPatchData.PRNumber, res.GetMessage())
	}

	return nil
}

func (c *gitMergePr) sendMergeFailedStatus(githubMessage string, pr thirdparty.GithubPatch, conf *internal.TaskConfig) {
	state := message.GithubStateFailure
	description := fmt.Sprintf("merge failed: %s", githubMessage)

	status := message.GithubStatus{
		Owner:       conf.ProjectRef.Owner,
		Repo:        conf.ProjectRef.Repo,
		Ref:         pr.HeadHash,
		Context:     GithubContext,
		State:       state,
		Description: description,
	}
	msg := message.NewGithubStatusMessageWithRepo(level.Notice, status)

	c.statusSender.Send(msg)
}

func (c *gitMergePr) sendPatchResult(pr thirdparty.GithubPatch, conf *internal.TaskConfig, patchStatus string) {
	state := message.GithubStateFailure
	description := "merge test failed"
	if patchStatus == evergreen.PatchSucceeded {
		state = message.GithubStateSuccess
		description = "merge test succeeded"
	}

	status := message.GithubStatus{
		Owner:       conf.ProjectRef.Owner,
		Repo:        conf.ProjectRef.Repo,
		Ref:         pr.HeadHash,
		Context:     GithubContext,
		State:       state,
		Description: description,
		URL:         c.URL,
	}
	msg := message.NewGithubStatusMessageWithRepo(level.Notice, status)

	c.statusSender.Send(msg)
}
