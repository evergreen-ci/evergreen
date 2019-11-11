package repotracker

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	// the repotracker polls version control (github) for new commits
	RunnerName = "repotracker"

	// githubAPILimitCeiling is arbitrary but corresponds to when we start logging errors in
	// thirdparty/github.go/getGithubRateLimit
	githubAPILimitCeiling = 20
)

func getTracker(conf *evergreen.Settings, project model.ProjectRef) (*RepoTracker, error) {
	token, err := conf.GetGithubOauthToken()
	if err != nil {
		grip.Warning(message.Fields{
			"runner":  RunnerName,
			"message": "Github credentials not specified in Evergreen credentials file",
		})
		return nil, errors.WithStack(err)
	}

	tracker := &RepoTracker{
		Settings:   conf,
		ProjectRef: &project,
		RepoPoller: NewGithubRepositoryPoller(&project, token),
	}

	return tracker, nil
}

func CollectRevisionsForProject(ctx context.Context, conf *evergreen.Settings, project model.ProjectRef) error {
	if !project.Enabled || project.RepotrackerDisabled {
		return errors.Errorf("project disabled: %s", project.Identifier)
	}

	tracker, err := getTracker(conf, project)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"project": project.Identifier,
			"message": "problem fetching repotracker",
			"runner":  RunnerName,
		}))
		return errors.Wrap(err, "problem fetching repotracker")
	}

	if err = tracker.FetchRevisions(ctx); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"project": project.Identifier,
			"message": "problem fetching revisions",
			"runner":  RunnerName,
		}))

		return errors.Wrap(err, "repotracker encountered error")
	}

	return nil
}

func ActivateBuildsForProject(project model.ProjectRef) error {
	if !project.Enabled {
		return errors.Errorf("project disabled: %s", project.Identifier)
	}

	if err := model.DoProjectActivation(project.Identifier); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "problem activating recent commit for project",
			"runner":  RunnerName,
			"mode":    "catch up",
			"project": project.Identifier,
		}))

		return errors.WithStack(err)
	}

	return nil
}

// CheckGithubAPIResources returns true when the github API is ready,
// accessible and with sufficient quota to satisfy our needs
func CheckGithubAPIResources(ctx context.Context, githubToken string) bool {

	remaining, err := thirdparty.CheckGithubAPILimit(ctx, githubToken)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"runner":  RunnerName,
			"message": "problem checking github api limit",
		}))
		return false
	}
	if remaining < githubAPILimitCeiling {
		grip.Error(message.Fields{
			"runner":   RunnerName,
			"message":  "too few github API requests remaining",
			"requests": remaining,
			"ceiling":  githubAPILimitCeiling,
		})

		return false
	}

	return true
}
