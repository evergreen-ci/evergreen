package command

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/v52/github"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

type gitMergePR struct {
	Token string `mapstructure:"token"`

	statusSender send.Sender
	base
}

const (
	mergePRAttempts      = 3
	mergePRRetryMinDelay = 10 * time.Second
	mergePRRetryMaxDelay = 30 * time.Second
)

func gitMergePRFactory() Command   { return &gitMergePR{} }
func (c *gitMergePR) Name() string { return "git.merge_pr" }

func (c *gitMergePR) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	return nil
}

func (c *gitMergePR) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	var err error
	defer func() {
		pErr := recovery.HandlePanicWithError(recover(), nil, fmt.Sprintf("unexpected error in '%s'", c.Name()))
		status := evergreen.MergeTestSucceeded
		if err != nil || pErr != nil {
			status = evergreen.MergeTestFailed
		}
		logger.Task().Error(err)
		logger.Task().Critical(pErr)
		td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
		logger.Task().Error(comm.ConcludeMerge(ctx, conf.Task.Version, status, td))
	}()
	if err = util.ExpandValues(c, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	var patchDoc *patch.Patch
	patchDoc, err = comm.GetTaskPatch(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, "")
	if err != nil {
		return errors.Wrap(err, "getting patch")
	}

	token := c.Token

	appToken := conf.Expansions.Get(evergreen.GithubAppToken)

	c.statusSender, err = send.NewGithubStatusLogger("evergreen", &send.GithubOptions{
		Token:       token,
		MinDelay:    evergreen.GithubRetryMinDelay,
		MaxAttempts: evergreen.GitHubRetryAttempts,
	}, "")
	if err != nil {
		return errors.Wrap(err, "setting up GitHub status logger")
	}

	if patchDoc.MergeStatus != evergreen.VersionSucceeded {
		logger.Task().Warning("At least 1 task failed, will not merge pull request.")
		return nil
	}
	// only successful patches should get past here. Failed patches will just send the failed
	// status to GitHub.

	mergeOpts := &github.PullRequestOptions{
		MergeMethod:        conf.ProjectRef.CommitQueue.MergeMethod,
		CommitTitle:        patchDoc.GithubPatchData.CommitTitle,
		DontDefaultIfBlank: true, // note this means that we will never merge with the default message (concatenated commit messages)
	}

	// Do the merge and assign to error so the defer statement handles this correctly.
	// Add retry logic in case multiple PRs are merged in quick succession, since
	// it takes GitHub some time to put the PR back in a mergeable state.
	err = utility.Retry(ctx, func() (bool, error) {
		err = thirdparty.MergePullRequest(ctx, appToken, conf.ProjectRef.Owner, conf.ProjectRef.Repo,
			patchDoc.GithubPatchData.CommitMessage, patchDoc.GithubPatchData.PRNumber, mergeOpts)
		if err != nil {
			return true, errors.Wrap(err, "getting pull request data from GitHub")
		}

		return false, nil
	}, utility.RetryOptions{
		MaxAttempts: mergePRAttempts,
		MinDelay:    mergePRRetryMinDelay,
		MaxDelay:    mergePRRetryMaxDelay,
	})

	return err
}
