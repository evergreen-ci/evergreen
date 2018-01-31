package units

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
)

const stepbackActivationCatchupJobName = "stepback-activation-catchup"

func init() {
	registry.AddJobType(stepbackActivationCatchupJobName, func() amboy.Job {
		return makeStepbackActivationCatchupJob()
	})

}

type stepbackActivationCatchup struct {
	Project  string `bson:"project" json:"project" yaml:"project"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	env evergreen.Environment
}

func makeStepbackActivationCatchupJob() *stepbackActivationCatchup {
	return &stepbackActivationCatchup{
		env: evergreen.GetEnvironment(),
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    stepbackActivationCatchupJobName,
				Version: 0,
				Format:  amboy.BSON,
			},
		},
	}
}

func NewStepbackActiationJob(env evergreen.Environment, project string, id string) amboy.Job {
	j := makeStepbackActivationCatchupJob()
	j.Project = project
	j.env = env
	job.SetID("%s.%s.%s", stepbackActivationCatchupJobName, project, id)
	return job
}

func (j *stepbackActivationCatchup) Run() {
	defer j.MarkComplete()

	conf := j.env.Settings()

	ref, err := model.FindOneProjectRef(j.Project)
	if err != nil {
		j.AddError(errors.WithStack(err))
		return
	}

	j.AddError(errors.Wrap(repotracker.ActivateBuildsForProject(conf, ref),
		"problem activating builds for project %s", j.Project))
}
