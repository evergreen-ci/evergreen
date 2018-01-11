package units

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/mongodb/amboy"
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
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment

	Owner string `bson:"owner" json:"owner" yaml:"owner"`
	Repo  string `bson:"repo" json:"repo" yaml:"repo"`
}

func makeRepotrackerJob() *repotrackerJob {
	return &repotrackerJob{
		env: evergreen.GetEnvironment(),
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    githubStatusUpdateJobName,
				Version: 0,
				Format:  amboy.BSON,
			},
		},
	}
}

// NewGithubStatusUpdateJobForBuild creates a job to update github's API from a Build.
// Status will be reported as 'evergreen-[build variant name]'
func NewRepotrackerJob(msgID, owner, repo string) amboy.Job {
	job := makeRepotrackerJob()
	job.Owner = owner
	job.Repo = repo

	job.SetID(fmt.Sprintf("%s:%s/%s-%s", repotrackerJobName, owner, repo, msgID))
	return job
}

func (j *repotrackerJob) Run() {
	defer j.MarkComplete()

	adminSettings, err := admin.GetSettings()
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving admin settings"))
		return
	}
	if adminSettings.ServiceFlags.RepotrackerPushEventDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     repotrackerJobName,
			"message": "github push events triggering repotracker is disabled",
		})
		j.AddError(errors.New("github push events triggering repotracker is disabled"))
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

	ref, err := fetchProjectRefByRepo(j.Owner, j.Repo)
	if err != nil {
		j.AddError(err)
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
			"job":    repotrackerJobName,
			"job_id": j.ID(),
			"repo":   fmt.Sprintf("%s/%s", j.Owner, j.Repo),
		}))
		j.AddError(err)
	}
}

func fetchProjectRefByRepo(owner, repo string) (*model.ProjectRef, error) {
	ref, err := model.FindOneProjectRefByRepo(owner, repo)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return nil, errors.New("can't find project ref for project")
	}

	return ref, nil
}
