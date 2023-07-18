package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
)

const (
	lastContainerFinishTimeJobName = "last-container-finish-time"
)

func init() {
	registry.AddJobType(lastContainerFinishTimeJobName, func() amboy.Job {
		return makeLastContainerFinishTimeJob()
	})

}

type lastContainerFinishTimeJob struct {
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
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
	return j
}

func NewLastContainerFinishTimeJob(id string) amboy.Job {
	j := makeLastContainerFinishTimeJob()

	j.SetID(fmt.Sprintf("%s.%s", lastContainerFinishTimeJobName, id))
	return j
}

func (j *lastContainerFinishTimeJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	// get pairs of host ID and finish time for each host with containers
	times, err := host.AggregateLastContainerFinishTimes(ctx)
	j.AddError(errors.Wrap(err, "getting container parents and their last container finish times"))

	// update last container finish time for each host with containers
	for _, time := range times {
		h, err := host.FindOneByIdOrTag(ctx, time.Id)
		if err != nil {
			j.AddError(errors.Wrapf(err, "finding host '%s'", time.Id))
			continue
		} else if h == nil {
			continue
		}
		j.AddError(errors.Wrapf(h.UpdateLastContainerFinishTime(ctx, time.FinishTime), "updating last container finish time for container parent '%s'", h.Id))
	}
}
