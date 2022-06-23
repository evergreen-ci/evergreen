package command

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/thirdparty"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/v34/github"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
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
		CommitTitle:        patchDoc.GithubPatchData.CommitTitle,
		DontDefaultIfBlank: true, // note this means that we will never merge with the default message (concatenated commit messages)
	}

	// do the merge
	return thirdparty.MergePullRequest(githubCtx, token, conf.ProjectRef.Owner, conf.ProjectRef.Repo,
		patchDoc.GithubPatchData.CommitMessage, patchDoc.GithubPatchData.PRNumber, mergeOpts)
}
