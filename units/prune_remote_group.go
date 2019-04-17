package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
)

const pruneRemoteQueueGroupJobName = "prune-remote-queue-group"

func init() {
	registry.AddJobType(pruneRemoteQueueGroupJobName,
		func() amboy.Job { return makePruneRemoteQueueGroup() })
}

type pruneRemoteQueueGroup struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`

	env evergreen.Environment
}

func NewPruneRemoteQueueGroup(id string) amboy.Job {
	j := makePruneRemoteQueueGroup()
	j.SetID(fmt.Sprintf("%s-%s", pruneRemoteQueueGroupJobName, id))
	j.SetPriority(-1)

	return j
}

func makePruneRemoteQueueGroup() *pruneRemoteQueueGroup {
	j := &pruneRemoteQueueGroup{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    pruneRemoteQueueGroupJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func (j *pruneRemoteQueueGroup) Run(ctx context.Context) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	j.AddError(j.env.RemoteQueueGroup().Prune(ctx))
	j.AddError(errors.Wrap(ctx.Err(), "context expired before prune finished"))
	return
}
