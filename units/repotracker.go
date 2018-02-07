package units

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	repotrackerJobName = "repotracker"
)

func init() {
	registry.AddJobType(repotrackerJobName, func() amboy.Job { return makeRepotrackerJob() })
}

type repotrackerJob struct {
	ProjectID string `bson:"project_id" json:"project_id" yaml:"project_id"`
	job.Base  `bson:"job_base" json:"job_base" yaml:"job_base"`
	env       evergreen.Environment
}

func makeRepotrackerJob() *repotrackerJob {
	j := &repotrackerJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    repotrackerJobName,
				Version: 0,
				Format:  amboy.BSON,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewRepotrackerJob creates a job to run repotracker against a repository.
// The code creating this job is responsible for verifying that the project
// should track push events
func NewRepotrackerJob(msgID, projectID string) amboy.Job {
	job := makeRepotrackerJob()
	job.ProjectID = projectID

	job.SetID(fmt.Sprintf("%s:%s:%s", repotrackerJobName, msgID, projectID))
	return job
}

func (j *repotrackerJob) Run() {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	adminSettings, err := evergreen.GetConfig()
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving admin settings"))
		return
	}
	if adminSettings.ServiceFlags.RepotrackerDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     repotrackerJobName,
			"id":      j.ID(),
			"message": "repotracker is disabled",
		})
		j.AddError(errors.New("repotracker is disabled"))
		return
	}

	settings := j.env.Settings()
	if settings == nil {
		j.AddError(errors.New("settings is empty"))
		return
	}
	token, err := settings.GetGithubOauthToken()
	if err != nil {
		j.AddError(errors.New("github token is missing"))
		return
	}

	ref, err := model.FindOneProjectRef(j.ProjectID)
	if err != nil {
		j.AddError(err)
		return
	}
	if ref == nil {
		j.AddError(errors.New("can't find project ref for project"))
		return
	}

	if !ref.TracksPushEvents {
		grip.Error(message.WrapError(err, message.Fields{
			"job":     repotrackerJobName,
			"job_id":  j.ID(),
			"project": ref.Identifier,
			"error":   "programmer error",
		}))
		j.AddError(errors.New("programmer error: repotrackerJobs should" +
			" not be created for projects that don't track push events"))
		return
	}

	if !repotracker.CheckGithubAPIResources(token) {
		j.AddError(errors.New("Github API is not ready"))
		return
	}
	err = repotracker.CollectRevisionsForProject(settings, *ref,
		settings.RepoTracker.MaxRepoRevisionsToSearch)

	if err != nil {
		grip.Info(message.WrapError(err, message.Fields{
			"job":     repotrackerJobName,
			"job_id":  j.ID(),
			"project": j.ProjectID,
		}))
		j.AddError(err)
	}
}
