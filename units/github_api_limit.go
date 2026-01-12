package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const githubAPILimitJobName = "github-api-limit"

func init() {
	registry.AddJobType(githubAPILimitJobName, func() amboy.Job {
		return makeGithubAPILimitJob()
	})
}

type githubAPILimitJob struct {
	job.Base `bson:"base" json:"base" yaml:"base"`
}

func makeGithubAPILimitJob() *githubAPILimitJob {
	j := &githubAPILimitJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    githubAPILimitJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewGithubAPILimitJob creates a job to log GitHub API rate limit information.
func NewGithubAPILimitJob(ts string) amboy.Job {
	j := makeGithubAPILimitJob()
	j.SetID(fmt.Sprintf("%s.%s", githubAPILimitJobName, ts))
	j.SetScopes([]string{githubAPILimitJobName})
	j.SetEnqueueAllScopes(true)
	return j
}

func (j *githubAPILimitJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	limit, err := thirdparty.CheckGithubAPILimit(ctx, "", "", nil)
	if err != nil {
		j.AddError(errors.Wrap(err, "checking GitHub API rate limit"))
		return
	}

	grip.Info(message.Fields{
		"message":           "GitHub API rate limit",
		"remaining":         limit.Core.Remaining,
		"limit":             limit.Core.Limit,
		"reset":             limit.Core.Reset.Time,
		"minutes_remaining": time.Until(limit.Core.Reset.Time).Minutes(),
		"percentage":        float32(limit.Core.Remaining) / float32(limit.Core.Limit),
	})

	// Keep track of GitHub apps and the project IDs that use them. A single app
	// could be reused by multiple projects.
	type projectAndAppAuth struct {
		appAuth     *githubapp.GithubAppAuth
		projectRefs []model.ProjectRef
		projectIDs  []string
	}
	ghAppsByAppID := map[int64]projectAndAppAuth{}

	// kim: NOTE: project/repo ref with API enabled -> GH app -> log rate limit.
	// kim: TODO: test logging in staging
	pRefs, err := model.FindProjectRefsUsingGitHubAppForAPI(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "finding project refs using GitHub app for API"))
		return
	}

	for _, pRef := range pRefs {
		ghAppAuth, err := pRef.GetGitHubAppAuth(ctx)
		if err != nil {
			j.AddError(errors.Wrapf(err, "getting GitHub app auth for project '%s'", pRef.Id))
			continue
		}
		if ghAppAuth == nil {
			continue
		}

		projectAppAuth, ok := ghAppsByAppID[ghAppAuth.AppID]
		if ok {
			projectAppAuth.projectRefs = append(projectAppAuth.projectRefs, pRef)
			projectAppAuth.projectIDs = append(projectAppAuth.projectIDs, pRef.Id)
		} else {
			projectAppAuth = projectAndAppAuth{
				appAuth:     ghAppAuth,
				projectRefs: []model.ProjectRef{pRef},
				projectIDs:  []string{pRef.Id},
			}
		}
		ghAppsByAppID[ghAppAuth.AppID] = projectAppAuth
	}

	for _, projectAppAuth := range ghAppsByAppID {
		// To check the rate limit for the app, this has to provide an
		// owner/repo that the app is installed on. Arbitrarily use the first
		// project ref's owner/repo under the assumption that every Evergreen
		// project that configures the app also has it installed on the
		// project's repo.
		pRef := projectAppAuth.projectRefs[0]
		limit, err := thirdparty.CheckGithubAPILimit(ctx, pRef.Owner, pRef.Repo, projectAppAuth.appAuth)
		if err != nil {
			j.AddError(errors.Wrapf(err, "checking GitHub API rate limit for app ID %d (used by projects '%s')",
				projectAppAuth.appAuth.AppID, projectAppAuth.projectIDs))
			continue
		}

		// kim: NOTE: consider exposing as Honeycomb trace as well because users
		// might use it.
		grip.Info(message.Fields{
			"message":           "project GitHub app API rate limit",
			"app_id":            projectAppAuth.appAuth.AppID,
			"project_ids":       projectAppAuth.projectIDs,
			"remaining":         limit.Core.Remaining,
			"limit":             limit.Core.Limit,
			"reset":             limit.Core.Reset.Time,
			"minutes_remaining": time.Until(limit.Core.Reset.Time).Minutes(),
			"percentage":        float32(limit.Core.Remaining) / float32(limit.Core.Limit),
		})
	}
}
