package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/google/go-github/v70/github"
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

	j.logInternalAppRateLimit(ctx)
	j.logProjectAppRateLimit(ctx)
}

// logInternalAppRateLimit logs the GitHub rate limit info for the Evergreen
// internal app.
func (j *githubAPILimitJob) logInternalAppRateLimit(ctx context.Context) {
	limit, err := thirdparty.CheckGithubAPILimit(ctx, "", "", nil)
	if err != nil {
		j.AddError(errors.Wrap(err, "checking Evergreen internal app GitHub API rate limit"))
		return
	}

	rateLimitInfo := getRateLimitInfo(limit)
	grip.Info(message.Fields{
		"message":           "GitHub API rate limit",
		"remaining":         rateLimitInfo.remaining,
		"limit":             rateLimitInfo.limit,
		"reset":             rateLimitInfo.resetAt,
		"minutes_remaining": rateLimitInfo.minsRemainingToReset,
		"percentage":        rateLimitInfo.remainingPercentage,
	})

}

func (j *githubAPILimitJob) logProjectAppRateLimit(ctx context.Context) {
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
			j.AddError(errors.Wrapf(err, "checking GitHub API rate limit for app ID %d",
				projectAppAuth.appAuth.AppID))
			continue
		}

		rateLimitInfo := getRateLimitInfo(limit)
		// kim: NOTE: consider exposing as Honeycomb trace as well because users
		// might use it.
		grip.Info(message.Fields{
			"message":           "project GitHub app API rate limit",
			"app_id":            projectAppAuth.appAuth.AppID,
			"project_ids":       projectAppAuth.projectIDs,
			"remaining":         rateLimitInfo.remaining,
			"limit":             rateLimitInfo.limit,
			"reset":             rateLimitInfo.resetAt,
			"minutes_remaining": rateLimitInfo.minsRemainingToReset,
			"percentage":        rateLimitInfo.remainingPercentage,
		})
	}
}

// ghRateLimitInfo contains GitHub API rate limit information.
type ghRateLimitInfo struct {
	// remaining is the total number of remaining requests.
	remaining int
	// limit is the total number of requests allowed.
	limit int
	// remainingPercentage is the percentage of remaining requests out of the
	// limit.
	remainingPercentage float32
	// resetAt is the time when the rate limit resets.
	resetAt time.Time
	// minsRemainingToReset is the number of minutes until the rate limit
	// resets.
	minsRemainingToReset float64
}

func getRateLimitInfo(limit *github.RateLimits) ghRateLimitInfo {
	remaining := limit.Core.Remaining
	maxLimit := limit.Core.Limit
	resetTime := limit.Core.Reset.Time
	return ghRateLimitInfo{
		remaining:            remaining,
		limit:                maxLimit,
		remainingPercentage:  float32(remaining) / float32(maxLimit),
		resetAt:              resetTime,
		minsRemainingToReset: time.Until(resetTime).Minutes(),
	}
}
