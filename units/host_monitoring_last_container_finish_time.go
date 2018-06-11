package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
)

const (
	lastContainerFinishTimeJobName = "last-container-finish-time"
)

func init() {
	registry.AddJobType(lastContainerFinishTimeJobName, func() amboy.Job {
		return makeIdleHostJob()
	})

}

type lastContainerFinishTimeJob struct {
	job.Base   `bson:"metadata" json:"metadata" yaml:"metadata"`
	Terminated bool `bson:"terminated" json:"terminated" yaml:"terminated"`

	env      evergreen.Environment
	settings *evergreen.Settings
}

func makeLastContainerFinishTimeJob() *lastContainerFinishTimeJob {
	j := &lastContainerFinishTimeJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    lastContainerFinishTimeJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())

	return j
}

func NewLastContainerFinishTimeJob(env evergreen.Environment, id string) amboy.Job {
	j := makeLastContainerFinishTimeJob()

	j.SetID(fmt.Sprintf("%s.%s", lastContainerFinishTimeJobName, id))
	return j
}

func (j *lastContainerFinishTimeJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.settings == nil {
		j.settings = j.env.Settings()
	}

	// get pairs of host ID and finish time for each host with containers
	times, err := host.AggregateLastContainerFinishTimes()
	j.AddError(err)

	// update last container finish time for each host with containers
	for _, time := range times {
		h, err := host.FindOneId(time.Id)
		j.AddError(err)
		err = h.UpdateLastContainerFinishTime(time.FinishTime)
		j.AddError(err)
	}
}
