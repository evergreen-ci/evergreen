package command

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/github"
	"github.com/mitchellh/mapstructure"
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
	if status != evergreen.PatchSucceeded {
		logger.Task().Warning("at least 1 task failed, will not merge pull request")
		return nil
	}
	// only successful patches should get past here. Failed patches will just send the failed
	// status to github

	githubCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mergeOpts := &github.PullRequestOptions{
		MergeMethod:        conf.ProjectRef.CommitQueue.MergeMethod,
		DontDefaultIfBlank: true, // note this means that we will never merge with the default message (concatenated commit messages)
	}

	// do the merge
	var res *github.PullRequestMergeResult
	res, _, err = githubClient.PullRequests.Merge(githubCtx, conf.ProjectRef.Owner, conf.ProjectRef.Repo,
		patchDoc.GithubPatchData.PRNumber, patchDoc.GithubPatchData.CommitTitle, mergeOpts)
	if err != nil {
		return errors.Wrap(err, "can't access GitHub merge API")
	}

	if !res.GetMerged() {
		return errors.Errorf("Github refused to merge PR '%s/%s:%d': '%s'", conf.ProjectRef.Owner, conf.ProjectRef.Repo, patchDoc.GithubPatchData.PRNumber, res.GetMessage())
	}

	return nil
}
