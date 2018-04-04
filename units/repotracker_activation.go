package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
)

const versionActivationCatchupJobName = "version-activation-catchup"

func init() {
	registry.AddJobType(versionActivationCatchupJobName, func() amboy.Job {
		return makeVersionActivationCatchupJob()
	})

}

type versionActivationCatchup struct {
	Project  string `bson:"project" json:"project" yaml:"project"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
	env      evergreen.Environment
}

func makeVersionActivationCatchupJob() *versionActivationCatchup {
	j := &versionActivationCatchup{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    versionActivationCatchupJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j

}

func NewVersionActiationJob(project string, id string) amboy.Job {
	j := makeVersionActivationCatchupJob()
	j.Project = project

	j.SetID(fmt.Sprintf("%s.%s.%s", versionActivationCatchupJobName, project, id))
	j.SetPriority(-1)
	return j
}

func (j *versionActivationCatchup) Run(_ context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	conf := j.env.Settings()

	ref, err := model.FindOneProjectRef(j.Project)
	if err != nil {
		j.AddError(errors.WithStack(err))
		return
	}

	j.AddError(errors.Wrapf(repotracker.ActivateBuildsForProject(conf, *ref),
		"problem activating builds for project %s", j.Project))
}
