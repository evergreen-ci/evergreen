package repotracker

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// use sentinel error values to provide compatibility with the original system
var (
	errProjectDisabled  = errors.New("project disabled")
	errEncounteredError = errors.New("repotracker encountered error")
)

func CollectRevisionsForProject(conf *evergreen.Settings, project model.ProjectRef, num int) error {
	if !project.Enabled {
		return errors.Wrap(errProjectDisabled, project.String())
	}
	token, err := conf.GetGithubOauthToken()
	if err != nil {
		return err
	}

	tracker := &RepoTracker{
		Settings:   conf,
		ProjectRef: &project,
		RepoPoller: NewGithubRepositoryPoller(&project, token),
	}

	if err := tracker.FetchRevisions(num); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"project": project.Identifier,
			"message": "problem fetching revisions",
			"runner":  RunnerName,
		}))

		return errors.Wrap(errEncounteredError, err.Error())
	}

	return nil
}

func CheckGithubAPIResources(githubToken string) bool {
	status, err := thirdparty.GetGithubAPIStatus()
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"runner":  RunnerName,
			"message": "problem contacting github",
		}))
		return false
	}

	if status != thirdparty.GithubAPIStatusGood {
		if status == thirdparty.GithubAPIStatusMajor {
			grip.Warning(message.Fields{
				"status":  status,
				"runner":  RunnerName,
				"message": "skipping repotracker because of GithubAPI status",
			})

			return false
		}

		grip.Notice(message.Fields{
			"message": "github api status degraded",
			"status":  status,
			"runner":  RunnerName,
		})
	}

	remaining, err := thirdparty.CheckGithubAPILimit(githubToken)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"runner":  RunnerName,
			"message": "problem checking github api limit",
		}))
		return false
	}
	if remaining < githubAPILimitCeiling {
		grip.Warning(message.Fields{
			"runner":   RunnerName,
			"message":  "too few github API requests remaining",
			"requests": remaining,
			"ceiling":  githubAPILimitCeiling,
		})

		return false
	}

	return true
}
